package typeless

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"

	stdjson "encoding/json"

	"golang.org/x/sync/errgroup"
)

const (
	AutoImportPlatformCodex  = "codex"
	AutoImportPlatformClaude = "claude"
	AutoImportPlatformCustom = "custom"

	AutoImportStatusPending = "pending"
	AutoImportStatusSyncing = "syncing"
	AutoImportStatusSynced  = "synced"
	AutoImportStatusFailed  = "failed"
)

const (
	autoImportMaxFiles          = 240
	autoImportMaxLinesPerFile   = 8000
	autoImportMaxMessageRunes   = 12000
	autoImportMaxExamplesPerHit = 3
	autoImportMaxLineBytes      = 32 * 1024 * 1024
)

type AutoImportSource struct {
	Platform string `json:"platform"`
	Enabled  bool   `json:"enabled"`
	Workdir  string `json:"workdir"`
}

type AutoImportCandidate struct {
	Term           string   `json:"term"`
	NormalizedTerm string   `json:"normalized_term"`
	Platform       string   `json:"platform"`
	Hits           int      `json:"hits"`
	Examples       []string `json:"examples"`
}

type AutoImportScanResult struct {
	ScannedFiles       int                   `json:"scanned_files"`
	ParsedMessages     int                   `json:"parsed_messages"`
	RawCandidates      int                   `json:"raw_candidates"`
	FilteredCandidates int                   `json:"filtered_candidates"`
	Items              []AutoImportCandidate `json:"items"`
}

type PendingDictionaryWord struct {
	Term      string `json:"term"`
	Platform  string `json:"platform"`
	Example   string `json:"example"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Error     string `json:"error"`
}

type autoImportStore struct {
	Words []PendingDictionaryWord `json:"words"`
}

type autoImportMessage struct {
	Platform string
	Text     string
}

type autoImportScanSummary struct {
	files    int
	messages int
}

type codexHistoryLine struct {
	Text string `json:"text"`
}

type claudeHistoryLine struct {
	Display string `json:"display"`
}

type autoImportItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexSessionPayload struct {
	Role    string             `json:"role"`
	Type    string             `json:"type"`
	Message string             `json:"message"`
	Content stdjson.RawMessage `json:"content"`
}

type codexSessionLine struct {
	Type    string              `json:"type"`
	Payload codexSessionPayload `json:"payload"`
}

type claudeProjectMessage struct {
	Role    string             `json:"role"`
	Content stdjson.RawMessage `json:"content"`
}

type claudeProjectLine struct {
	Type    string               `json:"type"`
	Message claudeProjectMessage `json:"message"`
}

func DefaultAutoImportStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "TypeLens", "auto-import-pending.json"), nil
}

func DefaultAutoImportSources() ([]AutoImportSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return []AutoImportSource{
		{
			Platform: AutoImportPlatformCodex,
			Enabled:  true,
			Workdir:  filepath.Join(home, ".codex"),
		},
		{
			Platform: AutoImportPlatformClaude,
			Enabled:  true,
			Workdir:  filepath.Join(home, ".claude"),
		},
	}, nil
}

func ScanAutoImportCandidates(
	ctx context.Context,
	sources []AutoImportSource,
	existingTerms map[string]struct{},
	pendingTerms map[string]struct{},
) (AutoImportScanResult, error) {
	messages := make([]autoImportMessage, 0, 512)
	summary := autoImportScanSummary{}
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		platformMessages, platformSummary, err := scanPlatformMessages(ctx, source)
		if err != nil {
			return AutoImportScanResult{}, err
		}
		messages = append(messages, platformMessages...)
		summary.files += platformSummary.files
		summary.messages += platformSummary.messages
	}

	rawCandidates := extractAutoImportCandidates(messages)
	result := AutoImportScanResult{
		ScannedFiles:   summary.files,
		ParsedMessages: summary.messages,
		RawCandidates:  len(rawCandidates),
	}

	filtered := make([]AutoImportCandidate, 0, len(rawCandidates))
	for _, candidate := range rawCandidates {
		if _, ok := existingTerms[candidate.NormalizedTerm]; ok {
			continue
		}
		if _, ok := pendingTerms[candidate.NormalizedTerm]; ok {
			continue
		}
		filtered = append(filtered, candidate)
	}
	result.FilteredCandidates = len(filtered)
	result.Items = filtered
	return result, nil
}

func scanPlatformMessages(ctx context.Context, source AutoImportSource) ([]autoImportMessage, autoImportScanSummary, error) {
	platform := normalizeAutoImportPlatform(source.Platform)
	if platform == "" {
		return nil, autoImportScanSummary{}, fmt.Errorf("未知平台 %q", source.Platform)
	}
	workdir := strings.TrimSpace(source.Workdir)
	if workdir == "" {
		return nil, autoImportScanSummary{}, fmt.Errorf("%s 工作目录不能为空", platform)
	}

	paths, err := discoverAutoImportFiles(platform, workdir)
	if err != nil {
		return nil, autoImportScanSummary{}, err
	}
	if len(paths) == 0 {
		return nil, autoImportScanSummary{}, nil
	}

	results := make([][]autoImportMessage, len(paths))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(autoImportWorkerLimit(len(paths)))
	for index, path := range paths {
		index := index
		path := path
		group.Go(func() error {
			if err := groupCtx.Err(); err != nil {
				return err
			}
			fileMessages, err := parseAutoImportFile(platform, path)
			if err != nil {
				return fmt.Errorf("解析 %s 失败: %w", path, err)
			}
			results[index] = fileMessages
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, autoImportScanSummary{}, err
	}

	messages := make([]autoImportMessage, 0, 256)
	for _, fileMessages := range results {
		messages = append(messages, fileMessages...)
	}

	return messages, autoImportScanSummary{
		files:    len(paths),
		messages: len(messages),
	}, nil
}

func discoverAutoImportFiles(platform, workdir string) ([]string, error) {
	info, err := os.Stat(workdir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("工作目录不是目录: %s", workdir)
	}

	paths := make([]string, 0, 64)
	addPath := func(path string) {
		if len(paths) >= autoImportMaxFiles {
			return
		}
		paths = append(paths, path)
	}

	switch platform {
	case AutoImportPlatformCodex:
		history := filepath.Join(workdir, "history.jsonl")
		if fileExists(history) {
			addPath(history)
		}
		sessionsRoot := filepath.Join(workdir, "sessions")
		_ = filepath.WalkDir(sessionsRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil || len(paths) >= autoImportMaxFiles {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(path), ".jsonl") {
				addPath(path)
			}
			return nil
		})
	case AutoImportPlatformClaude:
		history := filepath.Join(workdir, "history.jsonl")
		if fileExists(history) {
			addPath(history)
		}
		projectsRoot := filepath.Join(workdir, "projects")
		_ = filepath.WalkDir(projectsRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil || len(paths) >= autoImportMaxFiles {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if name == "subagents" || name == "plugins" || name == "cache" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(strings.ToLower(path), ".jsonl") {
				addPath(path)
			}
			return nil
		})
	case AutoImportPlatformCustom:
		history := filepath.Join(workdir, "history.jsonl")
		if fileExists(history) {
			addPath(history)
		}
		_ = filepath.WalkDir(workdir, func(path string, d os.DirEntry, err error) error {
			if err != nil || len(paths) >= autoImportMaxFiles {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if name == "node_modules" || name == ".git" || name == "subagents" || name == "plugins" || name == "cache" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(strings.ToLower(path), ".jsonl") {
				addPath(path)
			}
			return nil
		})
	default:
		return nil, fmt.Errorf("未知平台 %q", platform)
	}

	slices.Sort(paths)
	return paths, nil
}

func parseAutoImportFile(platform, path string) ([]autoImportMessage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 256*1024)

	messages := make([]autoImportMessage, 0, 32)
	textBuffer := make([]string, 0, 4)
	for lineNo := 0; ; lineNo++ {
		if lineNo >= autoImportMaxLinesPerFile {
			break
		}
		lineBytes, err := readJSONLLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		lineBytes = bytes.TrimSpace(lineBytes)
		if len(lineBytes) == 0 {
			continue
		}
		textBuffer = textBuffer[:0]
		texts, err := parseAutoImportLine(textBuffer, platform, path, lineBytes)
		if err != nil {
			continue
		}
		for _, text := range texts {
			text = normalizeAutoImportMessage(text)
			if text == "" {
				continue
			}
			messages = append(messages, autoImportMessage{
				Platform: platform,
				Text:     text,
			})
		}
	}
	return messages, nil
}

func parseAutoImportLine(dst []string, platform, path string, line []byte) ([]string, error) {
	switch platform {
	case AutoImportPlatformCodex:
		if strings.HasSuffix(path, "history.jsonl") {
			return parseCodexHistoryLine(dst, line)
		}
		return parseCodexSessionLine(dst, line)
	case AutoImportPlatformClaude:
		if strings.HasSuffix(path, "history.jsonl") {
			return parseClaudeHistoryLine(dst, line)
		}
		return parseClaudeProjectLine(dst, line)
	case AutoImportPlatformCustom:
		texts, err := parseCodexHistoryLine(dst, line)
		if err == nil && len(texts) > 0 {
			return texts, nil
		}
		texts, err = parseClaudeHistoryLine(dst, line)
		if err == nil && len(texts) > 0 {
			return texts, nil
		}
		texts, err = parseCodexSessionLine(dst, line)
		if err == nil && len(texts) > 0 {
			return texts, nil
		}
		texts, err = parseClaudeProjectLine(dst, line)
		if err == nil && len(texts) > 0 {
			return texts, nil
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("未知平台 %q", platform)
	}
}

func parseCodexHistoryLine(dst []string, line []byte) ([]string, error) {
	var payload codexHistoryLine
	if err := stdjson.Unmarshal(line, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Text) == "" {
		return nil, nil
	}
	return append(dst, payload.Text), nil
}

func parseClaudeHistoryLine(dst []string, line []byte) ([]string, error) {
	var payload claudeHistoryLine
	if err := stdjson.Unmarshal(line, &payload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Display) == "" {
		return nil, nil
	}
	return append(dst, payload.Display), nil
}

func parseCodexSessionLine(dst []string, line []byte) ([]string, error) {
	var payload codexSessionLine
	if err := stdjson.Unmarshal(line, &payload); err != nil {
		return nil, err
	}
	switch payload.Type {
	case "response_item":
		if payload.Payload.Role == "user" {
			return collectTextValues(dst, payload.Payload.Content)
		}
	case "event_msg":
		if payload.Payload.Type == "user_message" && strings.TrimSpace(payload.Payload.Message) != "" {
			return append(dst, payload.Payload.Message), nil
		}
	}
	return nil, nil
}

func parseClaudeProjectLine(dst []string, line []byte) ([]string, error) {
	var payload claudeProjectLine
	if err := stdjson.Unmarshal(line, &payload); err != nil {
		return nil, err
	}
	if payload.Type != "user" {
		return nil, nil
	}
	if payload.Message.Role != "user" {
		return nil, nil
	}
	return collectTextValues(dst, payload.Message.Content)
}

func collectTextValues(dst []string, raw stdjson.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil
	}

	switch raw[0] {
	case '"':
		var text string
		if err := stdjson.Unmarshal(raw, &text); err != nil {
			return nil, nil
		}
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		return append(dst, text), nil
	case '{':
		var item autoImportItem
		if err := stdjson.Unmarshal(raw, &item); err != nil {
			return nil, nil
		}
		if item.Type != "" && item.Type != "input_text" && item.Type != "text" {
			return nil, nil
		}
		if strings.TrimSpace(item.Text) == "" {
			return nil, nil
		}
		return append(dst, item.Text), nil
	case '[':
		decoder := stdjson.NewDecoder(bytes.NewReader(raw))
		token, err := decoder.Token()
		if err != nil {
			return nil, nil
		}
		delimiter, ok := token.(stdjson.Delim)
		if !ok || delimiter != '[' {
			return nil, nil
		}
		var item autoImportItem
		for decoder.More() {
			item = autoImportItem{}
			if err := decoder.Decode(&item); err != nil {
				return nil, nil
			}
			if item.Type != "" && item.Type != "input_text" && item.Type != "text" {
				continue
			}
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			dst = append(dst, item.Text)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, nil
		}
		return dst, nil
	default:
		return nil, nil
	}
}

func normalizeAutoImportMessage(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > autoImportMaxMessageRunes {
		text = string(runes[:autoImportMaxMessageRunes])
	}
	return text
}

var (
	englishTokenPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9._/-]*`)
	urlPattern          = regexp.MustCompile(`(?i)^(https?://|www\.)`)
	pathPattern         = regexp.MustCompile(`^([~./]|[A-Za-z]:\\)`)
)

var englishStopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "that": {}, "this": {}, "from": {}, "into": {},
	"then": {}, "when": {}, "what": {}, "where": {}, "which": {}, "while": {}, "have": {}, "has": {},
	"will": {}, "would": {}, "should": {}, "could": {}, "about": {}, "there": {}, "their": {}, "your": {},
	"please": {}, "help": {}, "need": {}, "make": {}, "just": {}, "also": {}, "only": {}, "start": {},
	"done": {}, "after": {}, "before": {}, "again": {}, "show": {}, "write": {}, "file": {}, "files": {},
	"json": {}, "jsonl": {}, "text": {}, "user": {}, "assistant": {}, "input": {}, "output": {},
}

func extractAutoImportCandidates(messages []autoImportMessage) []AutoImportCandidate {
	type candidateStat struct {
		term       string
		platform   string
		hits       int
		examples   []string
		exampleSet map[string]struct{}
	}
	stats := make(map[string]*candidateStat)

	for _, message := range messages {
		seenInMessage := make(map[string]struct{})
		for _, token := range extractTokensFromMessage(message.Text) {
			key := normalizeDictionaryTermKey(token)
			if _, ok := seenInMessage[key]; ok {
				continue
			}
			seenInMessage[key] = struct{}{}

			stat, ok := stats[key]
			if !ok {
				stat = &candidateStat{
					term:       token,
					platform:   message.Platform,
					exampleSet: make(map[string]struct{}, autoImportMaxExamplesPerHit),
				}
				stats[key] = stat
			}
			stat.hits++
			if preferAutoImportTerm(token, stat.term) {
				stat.term = token
			}
			if shouldReplaceAutoImportPlatform(message.Platform, stat.platform, stat.hits) {
				stat.platform = message.Platform
			}
			example := OneLine(message.Text, 96)
			if len(stat.examples) < autoImportMaxExamplesPerHit {
				if _, ok := stat.exampleSet[example]; !ok {
					stat.examples = append(stat.examples, example)
					stat.exampleSet[example] = struct{}{}
				}
			}
		}
	}

	candidates := make([]AutoImportCandidate, 0, len(stats))
	for _, stat := range stats {
		candidates = append(candidates, AutoImportCandidate{
			Term:           stat.term,
			NormalizedTerm: normalizeDictionaryTermKey(stat.term),
			Platform:       stat.platform,
			Hits:           stat.hits,
			Examples:       stat.examples,
		})
	}

	slices.SortFunc(candidates, func(left, right AutoImportCandidate) int {
		switch {
		case left.Hits != right.Hits:
			return right.Hits - left.Hits
		case left.Platform != right.Platform:
			return strings.Compare(left.Platform, right.Platform)
		default:
			return strings.Compare(left.NormalizedTerm, right.NormalizedTerm)
		}
	})
	return candidates
}

func extractTokensFromMessage(text string) []string {
	tokens := make([]string, 0, 24)
	seen := make(map[string]struct{}, 32)
	appendToken := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		normalized := normalizeDictionaryTermKey(token)
		if !isUsefulCandidateToken(token, normalized) {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		tokens = append(tokens, token)
	}

	for _, match := range englishTokenPattern.FindAllString(text, -1) {
		appendToken(match)
		for _, sub := range splitCamelCase(match) {
			appendToken(sub)
		}
		for _, sub := range splitCompositeToken(match) {
			appendToken(sub)
		}
	}
	for _, match := range extractChineseCandidates(text) {
		appendToken(match)
	}
	return tokens
}

func splitCompositeToken(token string) []string {
	parts := strings.FieldsFunc(token, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/'
	})
	var expanded []string
	for _, part := range parts {
		if part == "" || part == token {
			continue
		}
		expanded = append(expanded, part)
		expanded = append(expanded, splitCamelCase(part)...)
	}
	return expanded
}

func preferAutoImportTerm(next, current string) bool {
	switch {
	case current == "":
		return true
	case strings.ContainsAny(next, "_-./") && !strings.ContainsAny(current, "_-./"):
		return true
	case hasUpperLetter(next) && !hasUpperLetter(current):
		return true
	case len([]rune(next)) > len([]rune(current)):
		return true
	default:
		return false
	}
}

func shouldReplaceAutoImportPlatform(next, current string, totalHits int) bool {
	if current == "" {
		return true
	}
	if current == next {
		return false
	}
	return totalHits == 1
}

func hasUpperLetter(value string) bool {
	for _, r := range value {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func splitCamelCase(token string) []string {
	runes := []rune(token)
	if len(runes) < 2 {
		return nil
	}
	parts := make([]string, 0, 4)
	start := 0
	for index := 1; index < len(runes); index++ {
		prev := runes[index-1]
		current := runes[index]
		nextLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
		if unicode.IsLower(prev) && unicode.IsUpper(current) || unicode.IsUpper(prev) && unicode.IsUpper(current) && nextLower {
			parts = append(parts, string(runes[start:index]))
			start = index
		}
	}
	parts = append(parts, string(runes[start:]))
	if len(parts) <= 1 {
		return nil
	}
	return parts
}

func extractChineseCandidates(text string) []string {
	runes := []rune(text)
	results := make([]string, 0, 8)
	for index := 0; index < len(runes); {
		if !isChineseRune(runes[index]) {
			index++
			continue
		}
		start := index
		for index < len(runes) && isChineseRune(runes[index]) {
			index++
		}
		segment := runes[start:index]
		if len(segment) < 2 {
			continue
		}
		if len(segment) <= 8 {
			results = append(results, string(segment))
			continue
		}
		for size := 2; size <= 4; size++ {
			for offset := 0; offset+size <= len(segment); offset++ {
				results = append(results, string(segment[offset:offset+size]))
			}
		}
	}
	return results
}

func isChineseRune(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

func isUsefulCandidateToken(original, normalized string) bool {
	if normalized == "" {
		return false
	}
	if _, ok := englishStopWords[normalized]; ok {
		return false
	}
	if urlPattern.MatchString(normalized) || pathPattern.MatchString(original) {
		return false
	}
	if strings.Contains(original, "--") {
		return false
	}
	if len([]rune(original)) > 48 {
		return false
	}

	allDigits := true
	asciiLetterCount := 0
	hasUpper := false
	hasSeparator := false
	hasChinese := false
	for _, r := range original {
		switch {
		case unicode.IsDigit(r):
		case unicode.IsLetter(r):
			allDigits = false
			if r <= unicode.MaxASCII {
				asciiLetterCount++
				if unicode.IsUpper(r) {
					hasUpper = true
				}
			} else if isChineseRune(r) {
				hasChinese = true
			}
		default:
			allDigits = false
			if r == '_' || r == '-' || r == '.' || r == '/' {
				hasSeparator = true
			}
		}
	}
	if allDigits {
		return false
	}
	if hasChinese {
		return len([]rune(original)) >= 2
	}
	if asciiLetterCount < 2 && !hasUpper && !hasSeparator {
		return false
	}
	return true
}

func LoadPendingDictionaryWords(path string) ([]PendingDictionaryWord, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		defaultPath, err := DefaultAutoImportStatePath()
		if err != nil {
			return nil, err
		}
		path = defaultPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var store autoImportStore
	if err := stdjson.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return store.Words, nil
}

func SavePendingDictionaryWords(path string, words []PendingDictionaryWord) error {
	path = strings.TrimSpace(path)
	if path == "" {
		defaultPath, err := DefaultAutoImportStatePath()
		if err != nil {
			return err
		}
		path = defaultPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := stdjson.MarshalIndent(autoImportStore{Words: words}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func PendingDictionaryTermSet(words []PendingDictionaryWord) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		key := normalizeDictionaryTermKey(word.Term)
		if key == "" || word.Status == AutoImportStatusSynced {
			continue
		}
		set[key] = struct{}{}
	}
	return set
}

func DictionaryTermSet(words []DictionaryWord) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		key := normalizeDictionaryTermKey(word.Term)
		if key == "" {
			continue
		}
		set[key] = struct{}{}
	}
	return set
}

func MergePendingCandidates(existing []PendingDictionaryWord, candidates []AutoImportCandidate) ([]PendingDictionaryWord, int) {
	words := slices.Clone(existing)
	seen := make(map[string]struct{}, len(existing))
	for _, word := range existing {
		key := normalizeDictionaryTermKey(word.Term)
		if key != "" {
			seen[key] = struct{}{}
		}
	}

	now := time.Now().Format(time.RFC3339)
	added := 0
	for _, candidate := range candidates {
		key := normalizeDictionaryTermKey(candidate.Term)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		example := ""
		if len(candidate.Examples) > 0 {
			example = candidate.Examples[0]
		}
		words = append(words, PendingDictionaryWord{
			Term:      candidate.Term,
			Platform:  candidate.Platform,
			Example:   example,
			Status:    AutoImportStatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		})
		added++
	}
	return words, added
}

func UpdatePendingDictionaryWordStatus(words []PendingDictionaryWord, term, status, errorText string) []PendingDictionaryWord {
	now := time.Now().Format(time.RFC3339)
	next := slices.Clone(words)
	for index, word := range next {
		if normalizeDictionaryTermKey(word.Term) != normalizeDictionaryTermKey(term) {
			continue
		}
		word.Status = status
		word.Error = strings.TrimSpace(errorText)
		word.UpdatedAt = now
		next[index] = word
		break
	}
	return next
}

func FilterVisiblePendingWords(words []PendingDictionaryWord) []PendingDictionaryWord {
	filtered := make([]PendingDictionaryWord, 0, len(words))
	for _, word := range words {
		if word.Status == AutoImportStatusSynced {
			continue
		}
		filtered = append(filtered, word)
	}
	return filtered
}

func emitAutoImportLog(writer io.Writer, format string, args ...any) {
	if writer == nil {
		return
	}
	fmt.Fprintf(writer, format+"\n", args...)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func normalizeAutoImportPlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case AutoImportPlatformCodex:
		return AutoImportPlatformCodex
	case AutoImportPlatformClaude:
		return AutoImportPlatformClaude
	case AutoImportPlatformCustom:
		return AutoImportPlatformCustom
	default:
		return ""
	}
}

func autoImportWorkerLimit(fileCount int) int {
	if fileCount <= 1 {
		return 1
	}
	limit := runtime.GOMAXPROCS(0) * 2
	if limit < 4 {
		limit = 4
	}
	if limit > 16 {
		limit = 16
	}
	if fileCount < limit {
		return fileCount
	}
	return limit
}

func readJSONLLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		line = append(line, fragment...)
		if len(line) > autoImportMaxLineBytes {
			return nil, fmt.Errorf("jsonl line too large: %d bytes", len(line))
		}
		if err == nil {
			return bytesTrimLineEnding(line), nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(line) == 0 {
				return nil, io.EOF
			}
			return bytesTrimLineEnding(line), nil
		}
		return nil, err
	}
}

func bytesTrimLineEnding(line []byte) []byte {
	return bytes.TrimRight(line, "\r\n")
}
