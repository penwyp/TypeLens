package typeless

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	stdjson "encoding/json"

	"github.com/yanyiwu/gojieba"
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
	autoImportMaxFiles           = 240
	autoImportMaxLinesPerFile    = 8000
	autoImportMaxMessageRunes    = 12000
	autoImportMaxExamplesPerHit  = 3
	autoImportMaxLineBytes       = 32 * 1024 * 1024
	autoImportMaxFinalCandidates = 300
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

type autoImportProgressSnapshot struct {
	sourceIndex   int
	sourceCount   int
	sourceWorkdir string
	scannedFiles  int64
	totalFiles    int
	totalMessages int64
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
	return scanAutoImportCandidates(ctx, sources, existingTerms, pendingTerms, nil)
}

func ScanAutoImportCandidatesWithProgress(
	ctx context.Context,
	sources []AutoImportSource,
	existingTerms map[string]struct{},
	pendingTerms map[string]struct{},
	progressWriter io.Writer,
) (AutoImportScanResult, error) {
	return scanAutoImportCandidates(ctx, sources, existingTerms, pendingTerms, progressWriter)
}

func FilterAutoImportCandidates(
	candidates []AutoImportCandidate,
	existingTerms map[string]struct{},
	pendingTerms map[string]struct{},
) []AutoImportCandidate {
	filtered := make([]AutoImportCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := existingTerms[candidate.NormalizedTerm]; ok {
			continue
		}
		if _, ok := pendingTerms[candidate.NormalizedTerm]; ok {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return limitAutoImportCandidates(filtered)
}

func scanAutoImportCandidates(
	ctx context.Context,
	sources []AutoImportSource,
	existingTerms map[string]struct{},
	pendingTerms map[string]struct{},
	progressWriter io.Writer,
) (AutoImportScanResult, error) {
	type discoveredSource struct {
		platform string
		workdir  string
		paths    []string
	}

	discovered := make([]discoveredSource, 0, len(sources))
	totalFiles := 0
	enabledSourceCount := 0
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		enabledSourceCount++
	}

	receivedSourceIndex := 0
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		platform := normalizeAutoImportPlatform(source.Platform)
		if platform == "" {
			return AutoImportScanResult{}, fmt.Errorf("未知平台 %q", source.Platform)
		}
		workdir := strings.TrimSpace(source.Workdir)
		if workdir == "" {
			return AutoImportScanResult{}, fmt.Errorf("%s 工作目录不能为空", platform)
		}
		receivedSourceIndex++
		emitAutoImportLog(progressWriter, "已接收到目录 %d/%d：%s", receivedSourceIndex, enabledSourceCount, workdir)
		paths, err := discoverAutoImportFiles(platform, workdir)
		if err != nil {
			return AutoImportScanResult{}, err
		}
		discovered = append(discovered, discoveredSource{
			platform: platform,
			workdir:  workdir,
			paths:    paths,
		})
		totalFiles += len(paths)
	}

	emitAutoImportLog(progressWriter, "目录准备完成，共 %d 个目录，预计扫描文件 %d 个。", len(discovered), totalFiles)

	messages := make([]autoImportMessage, 0, 512)
	summary := autoImportScanSummary{}
	var scannedFiles atomic.Int64
	var totalMessages atomic.Int64
	for sourceIndex, source := range discovered {
		emitAutoImportLog(progressWriter, "正在扫描目录 %d/%d：%s", sourceIndex+1, len(discovered), source.workdir)
		platformMessages, platformSummary, err := scanPlatformMessages(
			ctx,
			source.platform,
			source.paths,
			sourceIndex+1,
			len(discovered),
			source.workdir,
			totalFiles,
			&scannedFiles,
			&totalMessages,
			progressWriter,
		)
		if err != nil {
			return AutoImportScanResult{}, err
		}
		messages = append(messages, platformMessages...)
		summary.files += platformSummary.files
		summary.messages += platformSummary.messages
	}
	emitAutoImportLog(progressWriter, "文件扫描完成，共 %d 个文件，累计文本 %d 条。", summary.files, summary.messages)
	emitAutoImportLog(progressWriter, "正在提取候选词。")

	rawCandidates := extractAutoImportCandidates(messages)
	result := AutoImportScanResult{
		ScannedFiles:   summary.files,
		ParsedMessages: summary.messages,
		RawCandidates:  len(rawCandidates),
	}
	emitAutoImportLog(progressWriter, "候选词提取完成，共 %d 个。", result.RawCandidates)
	emitAutoImportLog(progressWriter, "正在与现有词典做差集过滤并进行质量排序。")

	filtered := FilterAutoImportCandidates(rawCandidates, existingTerms, pendingTerms)
	result.FilteredCandidates = len(filtered)
	result.Items = filtered
	emitAutoImportLog(progressWriter, "差集过滤完成，最终候选词 %d 个。", result.FilteredCandidates)
	return result, nil
}

func scanPlatformMessages(
	ctx context.Context,
	platform string,
	paths []string,
	sourceIndex int,
	sourceCount int,
	sourceWorkdir string,
	totalFiles int,
	scannedFiles *atomic.Int64,
	totalMessages *atomic.Int64,
	progressWriter io.Writer,
) ([]autoImportMessage, autoImportScanSummary, error) {
	if len(paths) == 0 {
		emitAutoImportLog(progressWriter, "目录 %d/%d 没有可扫描文件：%s", sourceIndex, sourceCount, sourceWorkdir)
		return nil, autoImportScanSummary{}, nil
	}

	results := make([][]autoImportMessage, len(paths))
	done := make(chan struct{})
	defer close(done)
	go streamAutoImportProgress(done, scannedFiles, totalMessages, progressWriter, autoImportProgressSnapshot{
		sourceIndex:   sourceIndex,
		sourceCount:   sourceCount,
		sourceWorkdir: sourceWorkdir,
		totalFiles:    totalFiles,
	})

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
			scannedFiles.Add(1)
			totalMessages.Add(int64(len(fileMessages)))
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, autoImportScanSummary{}, err
	}
	emitAutoImportLog(
		progressWriter,
		"目录 %d/%d 扫描完成：%s，累计文件 %d/%d，累计文本 %d 条。",
		sourceIndex,
		sourceCount,
		sourceWorkdir,
		scannedFiles.Load(),
		totalFiles,
		totalMessages.Load(),
	)

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
			if text == "" || isNoisyAutoImportMessage(text) {
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

func streamAutoImportProgress(
	done <-chan struct{},
	scannedFiles *atomic.Int64,
	totalMessages *atomic.Int64,
	progressWriter io.Writer,
	snapshot autoImportProgressSnapshot,
) {
	if progressWriter == nil || snapshot.totalFiles <= 0 {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	lastScanned := int64(-1)
	lastMessages := int64(-1)
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			currentScanned := scannedFiles.Load()
			currentMessages := totalMessages.Load()
			if currentScanned == lastScanned && currentMessages == lastMessages {
				continue
			}
			lastScanned = currentScanned
			lastMessages = currentMessages
			emitAutoImportLog(
				progressWriter,
				"扫描进度：目录 %d/%d（%s），累计文件 %d/%d，累计文本 %d 条。",
				snapshot.sourceIndex,
				snapshot.sourceCount,
				snapshot.sourceWorkdir,
				currentScanned,
				snapshot.totalFiles,
				currentMessages,
			)
		}
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

func isNoisyAutoImportMessage(text string) bool {
	lower := strings.ToLower(text)
	for _, snippet := range autoImportNoisyMessageSnippets {
		if strings.Contains(lower, snippet) {
			return true
		}
	}
	if strings.Count(text, "<") >= 3 && strings.Count(text, ">") >= 3 {
		return true
	}
	return false
}

var (
	englishTokenPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9._/-]*`)
	urlPattern          = regexp.MustCompile(`(?i)^(https?://|www\.)`)
	pathPattern         = regexp.MustCompile(`^([~./]|[A-Za-z]:\\)`)
	autoImportJiebaOnce sync.Once
	autoImportJieba     *gojieba.Jieba
	englishDictOnce     sync.Once
	englishDictWords    map[string]struct{}
)

var englishStopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "that": {}, "this": {}, "from": {}, "into": {},
	"then": {}, "when": {}, "what": {}, "where": {}, "which": {}, "while": {}, "have": {}, "has": {},
	"will": {}, "would": {}, "should": {}, "could": {}, "about": {}, "there": {}, "their": {}, "your": {},
	"please": {}, "help": {}, "need": {}, "make": {}, "just": {}, "also": {}, "only": {}, "start": {},
	"done": {}, "after": {}, "before": {}, "again": {}, "show": {}, "write": {}, "file": {}, "files": {},
	"json": {}, "jsonl": {}, "text": {}, "user": {}, "assistant": {}, "input": {}, "output": {},
	"code": {}, "task": {}, "issue": {}, "bug": {}, "fix": {}, "test": {}, "tests": {}, "build": {},
	"update": {}, "remove": {}, "create": {}, "using": {}, "used": {}, "use": {}, "data": {}, "value": {},
	"values": {}, "result": {}, "results": {}, "error": {}, "errors": {}, "request": {}, "response": {},
	"client": {}, "server": {}, "local": {}, "remote": {}, "cache": {}, "state": {}, "import": {},
	"export": {}, "sync": {}, "history": {}, "message": {}, "messages": {}, "string": {}, "number": {},
	"boolean": {}, "object": {}, "array": {}, "list": {}, "items": {}, "item": {}, "content": {},
	"true": {}, "false": {}, "null": {}, "undefined": {}, "const": {}, "func": {}, "function": {},
	"class": {}, "method": {}, "variable": {}, "param": {}, "params": {}, "option": {}, "options": {},
}

var chineseNoiseFragments = []string{
	"请处理", "继续处理", "帮我", "扫描", "输出", "看一下", "做好", "这里也有", "结果输出", "预览结果输出",
}

var chineseNoiseRunes = map[rune]struct{}{
	'请': {}, '把': {}, '帮': {}, '我': {}, '你': {}, '他': {}, '她': {}, '它': {}, '们': {}, '的': {},
	'了': {}, '和': {}, '并': {}, '再': {}, '又': {}, '在': {}, '将': {}, '就': {}, '给': {}, '看': {},
	'一': {}, '下': {}, '吗': {}, '吧': {}, '啊': {},
}

type autoImportToken struct {
	Term           string
	NormalizedTerm string
	Fragment       bool
}

type autoImportDocument struct {
	Platform        string
	Text            string
	TokenFreq       map[string]int
	TokenTerms      map[string]string
	TokenStandalone map[string]bool
	Length          int
}

type autoImportTermDocStat struct {
	TF     int
	DocLen int
}

type autoImportTermStat struct {
	term       string
	normalized string
	platform   string
	tf         int
	hits       int
	standalone int
	fragments  int
	score      float64
	docs       []autoImportTermDocStat
	examples   []string
	exampleSet map[string]struct{}
	metrics    autoImportTermMetrics
}

type autoImportTermMetrics struct {
	length         int
	separatorCount int
	hasUpper       bool
	hasLower       bool
	hasDigit       bool
	hasSeparator   bool
	chinese        bool
	english        bool
	camelCase      bool
	plainEnglish   bool
	phrase         bool
	protected      bool
	reject         bool
}

type autoImportPhrasePattern struct {
	Pattern string
	Token   string
	Term    string
}

var autoImportCanonicalTerms = map[string]string{
	"ai": "AI", "api": "API", "bm25": "BM25", "rag": "RAG", "pitr": "PITR", "br": "BR",
	"tidb": "TiDB", "mysql": "MySQL", "redis": "Redis", "golang": "Golang", "go": "Go",
	"tiup": "TiUP", "e2e": "E2E", "http": "HTTP", "https": "HTTPS", "ssh": "SSH",
	"oauth": "OAuth", "github": "GitHub", "codex": "Codex", "claude": "Claude", "wails": "Wails",
	"react": "React", "typescript": "TypeScript",
}

var autoImportProtectedTerms = map[string]struct{}{
	"bm25": {}, "rag": {}, "pitr": {}, "br": {}, "tidb": {}, "tiup": {},
}

var autoImportDomainStopWords = map[string]struct{}{
	"怎么": {}, "如何": {}, "帮我": {}, "请问": {}, "一下": {}, "问题": {}, "方案": {}, "分析": {},
	"解释": {}, "处理": {}, "生成": {}, "使用": {}, "工具": {}, "实现": {}, "支持": {}, "看看": {},
	"自动": {}, "导入": {}, "预览": {}, "结果": {}, "输出": {}, "扫描": {}, "继续": {}, "这里": {},
	"这个": {}, "可以": {}, "如果": {}, "什么": {}, "文档": {}, "测试": {}, "确认": {}, "为什么": {},
	"直接": {}, "没有": {}, "任务": {}, "是否": {}, "代码": {}, "当前": {}, "已经": {}, "以及": {},
	"修改": {}, "开始": {}, "不是": {}, "然后": {}, "通过": {}, "需要": {}, "完成": {}, "执行": {},
	"前端": {}, "里面": {}, "但是": {}, "或者": {}, "命令": {}, "本地": {}, "状态": {}, "启动": {},
	"项目": {}, "进行": {}, "页面": {}, "增加": {}, "接口": {}, "文件": {}, "同时": {}, "脚本": {},
	"修复": {}, "流程": {}, "配置": {}, "失败": {}, "时候": {}, "有没有": {}, "信息": {}, "按照": {},
	"更新": {}, "还有": {}, "觉得": {}, "自己": {}, "这些": {}, "所有": {}, "两个": {},
	"you": {}, "your": {}, "them": {}, "can": {}, "not": {}, "open": {}, "read": {}, "review": {},
	"check": {}, "run": {}, "main": {}, "flow": {}, "step": {}, "current": {}, "context": {},
	"command": {}, "commands": {}, "instructions": {}, "instruction": {}, "environment": {}, "environment_context": {},
	"current_date": {}, "currentdate": {}, "path": {}, "paths": {}, "name": {}, "users": {}, "user": {},
	"email": {}, "mailbox": {}, "system": {}, "role": {}, "status": {}, "date": {}, "description": {},
	"analysis": {}, "workflow": {}, "scripts": {}, "script": {}, "host": {}, "repo": {}, "branch": {},
	"merge": {}, "install": {}, "failed": {}, "backup": {}, "gate": {}, "shell": {}, "root": {},
	"home": {}, "bin": {}, "pkg": {}, "div": {}, "docs": {}, "doc": {}, "load": {}, "visible": {},
	"exist": {}, "continue": {}, "com": {}, "penwyp": {}, "skill": {},
	"skills": {}, "skill.md": {}, "debug": {}, "prompt": {}, "mail": {}, "log": {}, "logs": {},
	"dashboard": {}, "timezone": {}, "worktree": {}, "tem": {},
	"ads": {}, "ace": {}, "obs": {}, "tolink": {}, "caveat": {}, "unless": {}, "instead": {}, "get": {},
	"reviews": {}, "file": {}, "files": {}, "add": {}, "any": {}, "how": {},
	"go": {}, "api": {}, "http": {}, "https": {}, "ssh": {}, "oauth": {}, "github": {}, "openai": {},
	"codex": {}, "claude": {}, "e2e": {}, "git": {},
	"tools": {}, "config": {}, "monitoring": {}, "generated": {}, "existing": {}, "backend": {},
	"frontend": {}, "coding": {}, "changes": {}, "available": {}, "specific": {}, "target": {},
	"creator": {}, "runtime": {}, "service": {}, "auth": {}, "entry": {}, "group": {}, "page": {},
	"language": {}, "field": {}, "needed": {}, "reference": {}, "references": {},
	"default": {}, "version": {}, "core": {}, "base": {}, "app": {}, "nodes": {}, "dir": {},
	"url": {}, "cmd": {}, "conf": {}, "yaml": {}, "best": {},
	"first": {}, "full": {}, "all": {}, "new": {}, "generated.": {},
	"data-type": {}, "fill_about_you": {}, "fetch_otp": {}, "birthday_hidden": {}, "day_visible": {},
	"month_visible": {}, "year_visible": {}, "about-you": {}, "local-command-stdout": {},
	"tasks": {}, "tokens": {}, "box": {}, "otp": {}, "liu": {}, "yifei": {},
}

var autoImportNoisyMessageSnippets = []string{
	"<environment_context>", "<local-command-caveat>", "<command-name>", "<command-message>",
	"<command-args>", "<turn_aborted>", "# agents.md instructions", "documentation & housekeeping",
	"would you like to run the following command?", "the user interrupted the previous turn on purpose",
}

var autoImportPhrasePatterns = []autoImportPhrasePattern{
	{Pattern: "claude code", Token: "claude_code", Term: "Claude_Code"},
	{Pattern: "chatgpt pro", Token: "chatgpt_pro", Term: "ChatGPT_Pro"},
	{Pattern: "tidb br", Token: "tidb_br", Term: "TiDB_BR"},
	{Pattern: "log backup", Token: "log_backup", Term: "log_backup"},
	{Pattern: "keyword extraction", Token: "keyword_extraction", Term: "keyword_extraction"},
}

func extractAutoImportCandidates(messages []autoImportMessage) []AutoImportCandidate {
	documents := buildAutoImportDocuments(messages)
	if len(documents) == 0 {
		return nil
	}
	stats, avgDocLen := collectAutoImportTermStats(documents)
	candidates := scoreAutoImportTermStats(stats, len(documents), avgDocLen)
	return limitAutoImportCandidates(candidates)
}

func buildAutoImportDocuments(messages []autoImportMessage) []autoImportDocument {
	documents := make([]autoImportDocument, 0, len(messages))
	for _, message := range messages {
		tokens := collectAutoImportTokens(message.Text)
		if len(tokens) == 0 {
			continue
		}
		document := autoImportDocument{
			Platform:        message.Platform,
			Text:            message.Text,
			TokenFreq:       make(map[string]int, len(tokens)),
			TokenTerms:      make(map[string]string, len(tokens)),
			TokenStandalone: make(map[string]bool, len(tokens)),
		}
		for _, token := range tokens {
			document.TokenFreq[token.NormalizedTerm]++
			document.Length++
			if !token.Fragment {
				document.TokenStandalone[token.NormalizedTerm] = true
			}
			if preferAutoImportTerm(token.Term, document.TokenTerms[token.NormalizedTerm]) {
				document.TokenTerms[token.NormalizedTerm] = token.Term
			}
		}
		documents = append(documents, document)
	}
	return documents
}

func collectAutoImportTermStats(documents []autoImportDocument) (map[string]*autoImportTermStat, float64) {
	stats := make(map[string]*autoImportTermStat)
	totalDocLen := 0
	for _, document := range documents {
		totalDocLen += document.Length
		example := OneLine(document.Text, 96)
		for normalized, tf := range document.TokenFreq {
			stat, ok := stats[normalized]
			if !ok {
				term := document.TokenTerms[normalized]
				stat = &autoImportTermStat{
					term:       term,
					normalized: normalized,
					platform:   document.Platform,
					exampleSet: make(map[string]struct{}, autoImportMaxExamplesPerHit),
					metrics:    classifyCandidateTerm(term),
				}
				stats[normalized] = stat
			}
			term := document.TokenTerms[normalized]
			if preferAutoImportTerm(term, stat.term) {
				stat.term = term
				stat.metrics = classifyCandidateTerm(term)
			}
			stat.tf += tf
			stat.hits++
			if document.TokenStandalone[normalized] {
				stat.standalone++
			} else {
				stat.fragments++
			}
			stat.docs = append(stat.docs, autoImportTermDocStat{
				TF:     tf,
				DocLen: maxInt(document.Length, 1),
			})
			if shouldReplaceAutoImportPlatform(document.Platform, stat.platform, stat.hits) {
				stat.platform = document.Platform
			}
			if len(stat.examples) < autoImportMaxExamplesPerHit {
				if _, ok := stat.exampleSet[example]; !ok {
					stat.examples = append(stat.examples, example)
					stat.exampleSet[example] = struct{}{}
				}
			}
		}
	}
	avgDocLen := 1.0
	if len(documents) > 0 && totalDocLen > 0 {
		avgDocLen = float64(totalDocLen) / float64(len(documents))
	}
	return stats, avgDocLen
}

func scoreAutoImportTermStats(stats map[string]*autoImportTermStat, totalDocs int, avgDocLen float64) []AutoImportCandidate {
	if len(stats) == 0 {
		return nil
	}
	type scoredCandidate struct {
		candidate AutoImportCandidate
		score     float64
	}
	scored := make([]scoredCandidate, 0, len(stats))
	for _, stat := range stats {
		score, ok := scoreAutoImportTermStat(stat, totalDocs, avgDocLen)
		if !ok {
			continue
		}
		stat.score = score
		scored = append(scored, scoredCandidate{
			score: score,
			candidate: AutoImportCandidate{
				Term:           stat.term,
				NormalizedTerm: stat.normalized,
				Platform:       stat.platform,
				Hits:           stat.hits,
				Examples:       stat.examples,
			},
		})
	}
	slices.SortFunc(scored, func(left, right scoredCandidate) int {
		switch {
		case left.score != right.score:
			if right.score > left.score {
				return 1
			}
			return -1
		case left.candidate.Hits != right.candidate.Hits:
			return right.candidate.Hits - left.candidate.Hits
		case left.candidate.Platform != right.candidate.Platform:
			return strings.Compare(left.candidate.Platform, right.candidate.Platform)
		default:
			return strings.Compare(left.candidate.NormalizedTerm, right.candidate.NormalizedTerm)
		}
	})
	candidates := make([]AutoImportCandidate, 0, len(scored))
	for _, item := range scored {
		candidates = append(candidates, item.candidate)
	}
	return candidates
}

func extractTokensFromMessage(text string) []string {
	seen := make(map[string]struct{}, 32)
	tokens := make([]string, 0, 24)
	for _, token := range collectAutoImportTokens(text) {
		if _, ok := seen[token.NormalizedTerm]; ok {
			continue
		}
		seen[token.NormalizedTerm] = struct{}{}
		tokens = append(tokens, token.Term)
	}
	return tokens
}

func collectAutoImportTokens(text string) []autoImportToken {
	tokens := make([]autoImportToken, 0, 32)
	appendToken := func(term string, fragment bool) {
		token, ok := normalizeAutoImportToken(term)
		if !ok {
			return
		}
		token.Fragment = fragment
		tokens = append(tokens, token)
	}

	for _, match := range englishTokenPattern.FindAllString(text, -1) {
		appendToken(match, false)
		for _, sub := range splitCamelCase(match) {
			appendToken(sub, true)
		}
		for _, sub := range splitCompositeToken(match) {
			appendToken(sub, true)
		}
	}
	for _, match := range extractChineseCandidates(text) {
		appendToken(match, false)
	}
	for _, phrase := range extractPhraseCandidates(text) {
		appendToken(phrase, false)
	}
	return tokens
}

func extractPhraseCandidates(text string) []string {
	normalizedText := normalizePhraseSourceText(text)
	if normalizedText == "" {
		return nil
	}
	results := make([]string, 0, 4)
	padded := " " + normalizedText + " "
	for _, pattern := range autoImportPhrasePatterns {
		needle := " " + pattern.Pattern + " "
		count := strings.Count(padded, needle)
		for i := 0; i < count; i++ {
			results = append(results, pattern.Term)
		}
	}
	return results
}

func normalizePhraseSourceText(text string) string {
	builder := strings.Builder{}
	builder.Grow(len(text))
	lastSpace := true
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				builder.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func normalizeAutoImportToken(term string) (autoImportToken, bool) {
	term = strings.TrimSpace(term)
	if term == "" {
		return autoImportToken{}, false
	}
	normalized := normalizeDictionaryTermKey(term)
	if canonical, ok := autoImportCanonicalTerms[normalized]; ok {
		term = canonical
	}
	if !isUsefulCandidateToken(term, normalized) {
		return autoImportToken{}, false
	}
	if isStopWordToken(term, normalized) {
		return autoImportToken{}, false
	}
	return autoImportToken{
		Term:           term,
		NormalizedTerm: normalized,
	}, true
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
	case isProtectedTerm(normalizeDictionaryTermKey(next)) && !isProtectedTerm(normalizeDictionaryTermKey(current)):
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
	segments := autoImportChineseTokenizer().Cut(text, true)
	results := make([]string, 0, len(segments))
	for _, segment := range segments {
		token := normalizeChineseCandidate(segment)
		if token == "" {
			continue
		}
		results = append(results, token)
	}
	return results
}

func isChineseRune(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

func autoImportChineseTokenizer() *gojieba.Jieba {
	autoImportJiebaOnce.Do(func() {
		autoImportJieba = gojieba.NewJieba()
	})
	return autoImportJieba
}

func normalizeChineseCandidate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.TrimFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if value == "" {
		return ""
	}
	hasChinese := false
	for _, r := range value {
		if isChineseRune(r) {
			hasChinese = true
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return ""
	}
	if !hasChinese {
		return ""
	}
	return value
}

func isUsefulCandidateToken(original, normalized string) bool {
	if normalized == "" {
		return false
	}
	if urlPattern.MatchString(normalized) || pathPattern.MatchString(original) {
		return false
	}
	if strings.Contains(original, "--") {
		return false
	}
	if strings.Contains(original, ".") && !strings.HasSuffix(strings.ToLower(original), ".md") {
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
		length := len([]rune(original))
		return length >= 2 && length <= 16
	}
	if asciiLetterCount < 2 && !hasUpper && !hasSeparator && !isProtectedTerm(normalized) {
		return false
	}
	if isPlainEnglishWord(normalized) && len([]rune(normalized)) <= 2 && !isProtectedTerm(normalized) {
		return false
	}
	return true
}

func isStopWordToken(term, normalized string) bool {
	if isProtectedTerm(normalized) {
		return false
	}
	if isTemplateLikeToken(term) {
		return true
	}
	if isDictionaryEnglishWord(term, normalized) {
		return true
	}
	if _, ok := englishStopWords[normalized]; ok {
		return true
	}
	if _, ok := autoImportDomainStopWords[normalized]; ok {
		return true
	}
	if isNoisyChineseCandidate(term) {
		return true
	}
	return false
}

func scoreAutoImportTermStat(stat *autoImportTermStat, totalDocs int, avgDocLen float64) (float64, bool) {
	metrics := stat.metrics
	if metrics.reject {
		return 0, false
	}
	if metrics.chinese {
		return 0, false
	}
	if stat.standalone == 0 && stat.fragments > 0 {
		return 0, false
	}
	if metrics.plainEnglish && stat.hits == 1 && stat.tf == 1 && !metrics.protected {
		return 0, false
	}

	df := minInt(stat.hits, maxInt(totalDocs, 1))
	idf := math.Log(1 + float64(totalDocs+1)/float64(df+1))
	tfidf := float64(stat.tf) * idf

	bm25 := 0.0
	const (
		k1 = 1.5
		b  = 0.75
	)
	if avgDocLen <= 0 {
		avgDocLen = 1
	}
	for _, doc := range stat.docs {
		tf := float64(doc.TF)
		docLen := float64(maxInt(doc.DocLen, 1))
		denominator := tf + k1*(1-b+b*docLen/avgDocLen)
		bm25 += idf * (tf * (k1 + 1) / denominator)
	}

	score := tfidf*0.65 + bm25*0.35
	score *= autoImportDomainBoost(stat)
	if score <= 0 {
		return 0, false
	}
	return score, true
}

func autoImportDomainBoost(stat *autoImportTermStat) float64 {
	metrics := stat.metrics
	boost := 1.0
	if metrics.protected {
		boost *= 1.35
	}
	if metrics.phrase {
		boost *= 1.3
	}
	if metrics.camelCase {
		boost *= 1.25
	} else if metrics.hasUpper {
		boost *= 1.1
	}
	if metrics.hasSeparator {
		boost *= 1.2
	}
	if metrics.hasDigit && (metrics.english || metrics.chinese) {
		boost *= 1.08
	}
	if metrics.chinese {
		switch {
		case metrics.length <= 4:
			boost *= 1.12
		case metrics.length >= 10:
			boost *= 0.82
		}
	}
	if metrics.plainEnglish {
		boost *= 0.62
	}
	if metrics.length >= 24 {
		boost *= 0.72
	}
	if stat.hits >= 3 {
		boost *= 1.08
	}
	return boost
}

func classifyCandidateTerm(term string) autoImportTermMetrics {
	runes := []rune(term)
	normalized := normalizeDictionaryTermKey(term)
	metrics := autoImportTermMetrics{
		length:    len(runes),
		phrase:    strings.ContainsAny(term, "_-./"),
		protected: isProtectedTerm(normalized),
	}
	for _, r := range runes {
		switch {
		case isChineseRune(r):
			metrics.chinese = true
		case unicode.IsUpper(r):
			metrics.hasUpper = true
			metrics.english = true
		case unicode.IsLower(r):
			metrics.hasLower = true
			metrics.english = true
		case unicode.IsDigit(r):
			metrics.hasDigit = true
		case r == '_' || r == '-' || r == '.' || r == '/':
			metrics.hasSeparator = true
			metrics.separatorCount++
		default:
			metrics.reject = true
			return metrics
		}
	}
	metrics.camelCase = metrics.hasUpper && metrics.hasLower && !metrics.hasSeparator
	metrics.plainEnglish = isPlainEnglishWord(term)
	if metrics.separatorCount >= 4 {
		metrics.reject = true
	}
	if metrics.length > 40 {
		metrics.reject = true
	}
	if metrics.plainEnglish && metrics.length > 24 {
		metrics.reject = true
	}
	if metrics.hasDigit && !metrics.english && !metrics.chinese && !metrics.hasSeparator {
		metrics.reject = true
	}
	return metrics
}

func isPlainEnglishWord(term string) bool {
	if term == "" {
		return false
	}
	for _, r := range term {
		if r > unicode.MaxASCII || !unicode.IsLower(r) {
			return false
		}
	}
	return true
}

func isProtectedTerm(normalized string) bool {
	_, ok := autoImportProtectedTerms[normalized]
	return ok
}

func isTemplateLikeToken(term string) bool {
	runes := []rune(term)
	if len(runes) < 3 {
		return false
	}
	hasLetter := false
	hasLower := false
	hasUpper := false
	for _, r := range runes {
		switch {
		case unicode.IsLower(r):
			hasLetter = true
			hasLower = true
		case unicode.IsUpper(r):
			hasLetter = true
			hasUpper = true
		case unicode.IsDigit(r):
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	if !hasLetter {
		return false
	}
	return hasUpper && !hasLower
}

func isDictionaryEnglishWord(term, normalized string) bool {
	if normalized == "" || isProtectedTerm(normalized) {
		return false
	}
	for _, r := range normalized {
		if r > unicode.MaxASCII || !unicode.IsLetter(r) {
			return false
		}
	}
	if strings.ContainsAny(term, "_-./") || hasUpperLetter(term) && strings.IndexFunc(term, unicode.IsUpper) > 0 {
		return false
	}
	if len(normalized) <= 2 {
		return false
	}
	words := loadEnglishDictionaryWords()
	if len(words) == 0 {
		return false
	}
	_, ok := words[normalized]
	return ok
}

func loadEnglishDictionaryWords() map[string]struct{} {
	englishDictOnce.Do(func() {
		englishDictWords = make(map[string]struct{}, 65536)
		for _, path := range []string{"/usr/share/dict/words", "/usr/share/dict/web2"} {
			file, err := os.Open(path)
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				word := strings.TrimSpace(scanner.Text())
				if word == "" {
					continue
				}
				lower := strings.ToLower(word)
				valid := true
				for _, r := range lower {
					if r > unicode.MaxASCII || !unicode.IsLetter(r) {
						valid = false
						break
					}
				}
				if valid && len(lower) >= 3 {
					englishDictWords[lower] = struct{}{}
				}
			}
			_ = file.Close()
			if len(englishDictWords) > 0 {
				return
			}
		}
	})
	return englishDictWords
}

func isNoisyChineseCandidate(term string) bool {
	normalized := normalizeDictionaryTermKey(term)
	if _, ok := autoImportDomainStopWords[normalized]; ok {
		return true
	}
	for _, fragment := range chineseNoiseFragments {
		if strings.Contains(term, fragment) {
			return true
		}
	}
	runes := []rune(term)
	if len(runes) == 0 {
		return true
	}
	stopCount := 0
	for _, r := range runes {
		if _, ok := chineseNoiseRunes[r]; ok {
			stopCount++
		}
	}
	if len(runes) <= 2 && stopCount > 0 {
		return true
	}
	if _, ok := chineseNoiseRunes[runes[0]]; ok {
		return true
	}
	if _, ok := chineseNoiseRunes[runes[len(runes)-1]]; ok {
		return true
	}
	return stopCount >= 2
}

func limitAutoImportCandidates(candidates []AutoImportCandidate) []AutoImportCandidate {
	if len(candidates) <= autoImportMaxFinalCandidates {
		return candidates
	}
	return candidates[:autoImportMaxFinalCandidates]
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
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
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
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
		switch word.Status {
		case AutoImportStatusPending, AutoImportStatusSyncing:
			filtered = append(filtered, word)
		}
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
