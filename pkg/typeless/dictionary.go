package typeless

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

const DefaultAPIHost = "https://api.typeless.com"

var DefaultDictionaryTerms = []string{
	"anthropic",
	"claude",
	"claude code",
	"codex",
}

type DictionaryWord struct {
	ID             string   `json:"user_dictionary_id"`
	Term           string   `json:"term"`
	Lang           string   `json:"lang"`
	Category       string   `json:"category"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	Auto           bool     `json:"auto"`
	Replace        bool     `json:"replace"`
	ReplaceTargets []string `json:"replace_targets"`
}

type DictionaryClient struct {
	apiHost      string
	userDataPath string
	timeout      time.Duration
	userID       string
}

type ImportResult struct {
	TotalInput int
	Unique     int
	Skipped    int
	Imported   int
	Terms      []string
}

type ImportOptions struct {
	DryRun         bool
	Concurrency    int
	ProgressWriter io.Writer
	ExistingTerms  map[string]struct{}
}

type ClearOptions struct {
	Concurrency    int
	ProgressWriter io.Writer
}

type ResetOptions struct {
	Concurrency    int
	ProgressWriter io.Writer
}

type ResetResult struct {
	TotalInput int
	Unique     int
	Kept       int
	Deleted    int
	Imported   int
}

type ResetPlan struct {
	Kept        int
	DeleteWords []DictionaryWord
	AddTerms    []string
}

type apiResponse[T any] struct {
	Status  string `json:"status"`
	Message string `json:"msg"`
	Data    T      `json:"data"`
}

type listDictionaryData struct {
	Words      []DictionaryWord `json:"words"`
	TotalCount int              `json:"total_count"`
}

func NewDictionaryClient(apiHost, userDataPath string, timeout time.Duration) *DictionaryClient {
	if apiHost == "" {
		apiHost = DefaultAPIHost
	}
	return &DictionaryClient{
		apiHost:      strings.TrimRight(apiHost, "/"),
		userDataPath: userDataPath,
		timeout:      timeout,
	}
}

func (c *DictionaryClient) ListAll(ctx context.Context) ([]DictionaryWord, error) {
	const pageSize = 150
	all := make([]DictionaryWord, 0)
	for offset := 0; ; offset += pageSize {
		words, total, err := c.list(ctx, offset, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, words...)
		if len(words) < pageSize || (total > 0 && len(all) >= total) {
			return all, nil
		}
	}
}

func (c *DictionaryClient) ImportTerms(ctx context.Context, terms []string, options ImportOptions) (ImportResult, error) {
	uniqueTerms := uniqueTrimmedTerms(terms)
	result := ImportResult{
		TotalInput: len(terms),
		Unique:     len(uniqueTerms),
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 1
	}

	existing := options.ExistingTerms
	if existing == nil {
		writeImportLog(options.ProgressWriter, "开始读取远端词典，准备导入 %d 行输入。", len(terms))
		existingWords, err := c.ListAll(ctx)
		if err != nil {
			return result, err
		}
		writeImportLog(options.ProgressWriter, "远端词典读取完成，共 %d 个已有词。", len(existingWords))
		existing = make(map[string]struct{}, len(existingWords))
		for _, word := range existingWords {
			key := normalizeDictionaryTermKey(word.Term)
			if key == "" {
				continue
			}
			existing[key] = struct{}{}
		}
	}
	writeImportLog(options.ProgressWriter, "开始本地去重与差集过滤。")

	for _, term := range uniqueTerms {
		if _, ok := existing[normalizeDictionaryTermKey(term)]; ok {
			result.Skipped++
			continue
		}
		result.Terms = append(result.Terms, term)
	}
	writeImportLog(
		options.ProgressWriter,
		"预处理完成：输入 %d 行，规范化去重后 %d 个，跳过已有 %d 个，待导入 %d 个。",
		result.TotalInput,
		result.Unique,
		result.Skipped,
		len(result.Terms),
	)
	if options.DryRun {
		result.Imported = len(result.Terms)
		return result, nil
	}

	writeImportLog(options.ProgressWriter, "开始并发导入，并发数 %d。", options.Concurrency)
	progress := newImportProgress(options.ProgressWriter, len(result.Terms))
	progress.start()
	defer progress.finish()

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(options.Concurrency)

	var imported atomic.Int64
	var skipped atomic.Int64
	for _, term := range result.Terms {
		term := term
		group.Go(func() error {
			if err := c.Add(groupCtx, term); err != nil {
				if isSkippableDictionaryAddError(err) {
					skipped.Add(1)
					writeImportLog(options.ProgressWriter, "跳过词 %q: %v", term, err)
					progress.increment()
					return nil
				}
				return fmt.Errorf("导入词 %q 失败: %w", term, err)
			}
			imported.Add(1)
			progress.increment()
			return nil
		})
	}

	result.Imported = int(imported.Load())
	result.Skipped += int(skipped.Load())
	if err := group.Wait(); err != nil {
		return result, err
	}
	return result, nil
}

func (c *DictionaryClient) Clear(ctx context.Context, options ClearOptions) (int, error) {
	if options.Concurrency <= 0 {
		options.Concurrency = 10
	}
	writeImportLog(options.ProgressWriter, "开始读取远端词典，准备清空。")
	words, err := c.ListAll(ctx)
	if err != nil {
		return 0, err
	}
	writeImportLog(options.ProgressWriter, "远端词典读取完成，共 %d 个已有词。", len(words))
	return c.deleteWords(ctx, words, options.Concurrency, options.ProgressWriter)
}

func (c *DictionaryClient) Reset(ctx context.Context, terms []string, options ResetOptions) (ResetResult, error) {
	uniqueTerms := uniqueTrimmedTerms(terms)
	result := ResetResult{
		TotalInput: len(terms),
		Unique:     len(uniqueTerms),
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 10
	}

	writeImportLog(options.ProgressWriter, "开始读取远端词典，准备执行差量重置。")
	existingWords, err := c.ListAll(ctx)
	if err != nil {
		return result, err
	}
	writeImportLog(options.ProgressWriter, "远端词典读取完成，共 %d 个已有词。", len(existingWords))

	plan := buildResetPlan(existingWords, uniqueTerms)
	result.Kept = plan.Kept

	writeImportLog(
		options.ProgressWriter,
		"差量分析完成：目标 %d 个，保留 %d 个，待删除 %d 个，待新增 %d 个。",
		result.Unique,
		result.Kept,
		len(plan.DeleteWords),
		len(plan.AddTerms),
	)

	deleted, err := c.deleteWords(ctx, plan.DeleteWords, options.Concurrency, options.ProgressWriter)
	if err != nil {
		result.Deleted = deleted
		return result, err
	}
	result.Deleted = deleted

	imported, err := c.addTerms(ctx, plan.AddTerms, options.Concurrency, options.ProgressWriter)
	result.Imported = imported
	if err != nil {
		return result, err
	}
	return result, nil
}

func (c *DictionaryClient) Add(ctx context.Context, term string) error {
	return c.runNodeRequest(ctx, nodeDictionaryRequest{
		Action: "add",
		Term:   term,
	})
}

func (c *DictionaryClient) Delete(ctx context.Context, id string) error {
	return c.runNodeRequest(ctx, nodeDictionaryRequest{
		Action: "delete",
		ID:     id,
	})
}

func (c *DictionaryClient) list(ctx context.Context, offset, size int) ([]DictionaryWord, int, error) {
	var response apiResponse[listDictionaryData]
	if err := c.runNodeRequest(ctx, nodeDictionaryRequest{
		Action: "list",
		Offset: offset,
		Size:   size,
	}, &response); err != nil {
		return nil, 0, err
	}
	if response.Status != "OK" {
		return nil, 0, fmt.Errorf("Typeless API 返回失败: %s", response.Message)
	}
	return response.Data.Words, response.Data.TotalCount, nil
}

func ReadTermsFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var terms []string
	for scanner.Scan() {
		terms = append(terms, scanner.Text())
	}
	return terms, scanner.Err()
}

func uniqueTrimmedTerms(terms []string) []string {
	seen := make(map[string]struct{}, len(terms))
	unique := make([]string, 0, len(terms))
	for _, term := range terms {
		trimmed := strings.TrimSpace(term)
		if trimmed == "" {
			continue
		}
		key := normalizeDictionaryTermKey(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, trimmed)
	}
	return unique
}

func normalizeDictionaryTermKey(term string) string {
	return strings.ToLower(strings.TrimSpace(term))
}

func isSkippableDictionaryAddError(err error) bool {
	statusCode, ok := httpStatusCodeFromError(err)
	return ok && statusCode >= 400 && statusCode < 500
}

func IsSkippableDictionaryAddError(err error) bool {
	return isSkippableDictionaryAddError(err)
}

func isSkippableDictionaryDeleteError(err error) bool {
	statusCode, ok := httpStatusCodeFromError(err)
	if !ok {
		return false
	}
	switch statusCode {
	case 400, 404, 409:
		return true
	default:
		return false
	}
}

func httpStatusCodeFromError(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	matches := httpStatusPattern.FindStringSubmatch(strings.ToLower(err.Error()))
	if len(matches) != 2 {
		return 0, false
	}
	var statusCode int
	if _, scanErr := fmt.Sscanf(matches[1], "%d", &statusCode); scanErr != nil {
		return 0, false
	}
	return statusCode, true
}

var httpStatusPattern = regexp.MustCompile(`http ([0-9]{3}):`)

func buildResetPlan(existingWords []DictionaryWord, uniqueTerms []string) ResetPlan {
	target := make(map[string]string, len(uniqueTerms))
	for _, term := range uniqueTerms {
		target[normalizeDictionaryTermKey(term)] = term
	}

	existing := make(map[string]DictionaryWord, len(existingWords))
	deleteWords := make([]DictionaryWord, 0)
	kept := 0
	for _, word := range existingWords {
		key := normalizeDictionaryTermKey(word.Term)
		if key == "" {
			continue
		}
		existing[key] = word
		if _, ok := target[key]; ok {
			kept++
			continue
		}
		if word.ID != "" {
			deleteWords = append(deleteWords, word)
		}
	}

	addTerms := make([]string, 0, len(uniqueTerms))
	for _, term := range uniqueTerms {
		if _, ok := existing[normalizeDictionaryTermKey(term)]; ok {
			continue
		}
		addTerms = append(addTerms, term)
	}

	return ResetPlan{
		Kept:        kept,
		DeleteWords: deleteWords,
		AddTerms:    addTerms,
	}
}

func (c *DictionaryClient) deleteWords(ctx context.Context, words []DictionaryWord, concurrency int, writer io.Writer) (int, error) {
	if len(words) == 0 {
		writeImportLog(writer, "无需删除词条。")
		return 0, nil
	}
	writeImportLog(writer, "开始并发删除多余词条，并发数 %d。", concurrency)
	progress := newImportProgress(writer, len(words))
	progress.start()
	defer progress.finish()

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(concurrency)
	var deleted atomic.Int64
	for _, word := range words {
		word := word
		group.Go(func() error {
			if err := c.Delete(groupCtx, word.ID); err != nil {
				if isSkippableDictionaryDeleteError(err) {
					writeImportLog(writer, "跳过删除词 %q: %v", word.Term, err)
					progress.increment()
					return nil
				}
				return fmt.Errorf("删除词 %q 失败: %w", word.Term, err)
			}
			deleted.Add(1)
			progress.increment()
			return nil
		})
	}
	err := group.Wait()
	return int(deleted.Load()), err
}

func (c *DictionaryClient) addTerms(ctx context.Context, terms []string, concurrency int, writer io.Writer) (int, error) {
	if len(terms) == 0 {
		writeImportLog(writer, "无需新增词条。")
		return 0, nil
	}
	writeImportLog(writer, "开始并发补充缺失词条，并发数 %d。", concurrency)
	progress := newImportProgress(writer, len(terms))
	progress.start()
	defer progress.finish()

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(concurrency)
	var imported atomic.Int64
	for _, term := range terms {
		term := term
		group.Go(func() error {
			if err := c.Add(groupCtx, term); err != nil {
				if isSkippableDictionaryAddError(err) {
					writeImportLog(writer, "跳过新增词 %q: %v", term, err)
					progress.increment()
					return nil
				}
				return fmt.Errorf("新增词 %q 失败: %w", term, err)
			}
			imported.Add(1)
			progress.increment()
			return nil
		})
	}
	err := group.Wait()
	return int(imported.Load()), err
}

func writeImportLog(writer io.Writer, format string, args ...any) {
	if writer == nil {
		return
	}
	fmt.Fprintf(writer, format+"\n", args...)
}

type importProgress struct {
	writer    io.Writer
	total     int
	done      chan struct{}
	startedAt time.Time
	completed atomic.Int64
}

func newImportProgress(writer io.Writer, total int) *importProgress {
	if writer == nil || total <= 0 {
		return nil
	}
	return &importProgress{
		writer:    writer,
		total:     total,
		done:      make(chan struct{}),
		startedAt: time.Now(),
	}
}

func (p *importProgress) start() {
	if p == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.print()
			case <-p.done:
				return
			}
		}
	}()
}

func (p *importProgress) increment() {
	if p == nil {
		return
	}
	p.completed.Add(1)
}

func (p *importProgress) finish() {
	if p == nil {
		return
	}
	p.print()
	close(p.done)
}

func (p *importProgress) print() {
	completed := p.completed.Load()
	elapsed := time.Since(p.startedAt).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	fmt.Fprintf(
		p.writer,
		"导入进度: %d/%d (%.1f%%), %.1f 条/秒\n",
		completed,
		p.total,
		float64(completed)*100/float64(p.total),
		float64(completed)/elapsed,
	)
}
