package service

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/penwyp/typelens/pkg/typeless"
	"golang.org/x/sync/errgroup"
)

type AutoImportScanRequest struct {
	Sources []typeless.AutoImportSource `json:"sources"`
}

type AutoImportConfirmRequest struct {
	Items []typeless.AutoImportCandidate `json:"items"`
}

type AutoImportConfirmResult struct {
	AcceptedCount int                              `json:"accepted_count"`
	Words         []typeless.PendingDictionaryWord `json:"words"`
}

const failedAutoImportRetryDelay = 2 * time.Minute

func (s *Service) ScanAutoImport(ctx context.Context, request AutoImportScanRequest, logWriter io.Writer) (typeless.AutoImportScanResult, error) {
	sources, err := s.normalizeAutoImportSources(request.Sources)
	if err != nil {
		return typeless.AutoImportScanResult{}, err
	}
	enabledSources := 0
	for _, source := range sources {
		if source.Enabled {
			enabledSources++
		}
	}
	emitAutoImportLog(logWriter, "已接收到 %d 个目录，开始准备自动导入。", enabledSources)

	var (
		cache      DictionaryCache
		scanResult typeless.AutoImportScanResult
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		result, err := s.LoadDictionaryCache()
		if err != nil {
			return err
		}
		cache = result
		return nil
	})
	group.Go(func() error {
		result, err := typeless.ScanAutoImportCandidatesWithProgress(
			groupCtx,
			sources,
			nil,
			nil,
			logWriter,
		)
		if err != nil {
			return err
		}
		scanResult = result
		return nil
	})
	if err := group.Wait(); err != nil {
		return typeless.AutoImportScanResult{}, err
	}

	scanResult.Items = typeless.FilterAutoImportCandidates(
		scanResult.Items,
		typeless.DictionaryTermSet(cache.Words),
		typeless.PendingDictionaryTermSet(cache.PendingWords),
	)
	scanResult.FilteredCandidates = len(scanResult.Items)
	return scanResult, nil
}

func (s *Service) ConfirmAutoImport(ctx context.Context, request AutoImportConfirmRequest, logWriter io.Writer) (AutoImportConfirmResult, error) {
	if len(request.Items) == 0 {
		return AutoImportConfirmResult{}, fmt.Errorf("没有可导入的词")
	}

	var acceptedCount int
	nextWords, err := s.updatePendingAutoImportWords(func(words []typeless.PendingDictionaryWord) []typeless.PendingDictionaryWord {
		var added int
		words, added = typeless.MergePendingCandidates(words, request.Items)
		acceptedCount = added
		return words
	})
	if err != nil {
		return AutoImportConfirmResult{}, err
	}
	if acceptedCount == 0 {
		if hasRetryableAutoImportWords(nextWords, time.Now()) {
			s.startAutoImportSync(ctx, logWriter)
		}
		return AutoImportConfirmResult{
			AcceptedCount: 0,
			Words:         typeless.FilterVisiblePendingWords(nextWords),
		}, nil
	}

	emitAutoImportLog(logWriter, "已写入本地待同步词条 %d 个，开始后台同步。", acceptedCount)
	s.startAutoImportSync(ctx, logWriter)

	return AutoImportConfirmResult{
		AcceptedCount: acceptedCount,
		Words:         typeless.FilterVisiblePendingWords(nextWords),
	}, nil
}

func (s *Service) ConfirmAutoImportSync(ctx context.Context, request AutoImportConfirmRequest, logWriter io.Writer) (AutoImportConfirmResult, error) {
	if len(request.Items) == 0 {
		return AutoImportConfirmResult{}, fmt.Errorf("没有可导入的词")
	}

	var acceptedCount int
	nextWords, err := s.updatePendingAutoImportWords(func(words []typeless.PendingDictionaryWord) []typeless.PendingDictionaryWord {
		var added int
		words, added = typeless.MergePendingCandidates(words, request.Items)
		acceptedCount = added
		return words
	})
	if err != nil {
		return AutoImportConfirmResult{}, err
	}
	if acceptedCount == 0 {
		if hasSyncableAutoImportWords(nextWords) {
			s.syncPendingAutoImportWords(ctx, logWriter)
			words, err := s.loadPendingAutoImportWords()
			if err != nil {
				return AutoImportConfirmResult{}, err
			}
			nextWords = words
		}
		return AutoImportConfirmResult{
			AcceptedCount: 0,
			Words:         typeless.FilterVisiblePendingWords(nextWords),
		}, nil
	}

	emitAutoImportLog(logWriter, "已写入本地待同步词条 %d 个，开始同步。", acceptedCount)
	s.syncPendingAutoImportWords(ctx, logWriter)

	words, err := s.loadPendingAutoImportWords()
	if err != nil {
		return AutoImportConfirmResult{}, err
	}
	return AutoImportConfirmResult{
		AcceptedCount: acceptedCount,
		Words:         typeless.FilterVisiblePendingWords(words),
	}, nil
}

func (s *Service) ListPendingAutoImportWords(ctx context.Context, logWriter io.Writer) ([]typeless.PendingDictionaryWord, error) {
	words, err := s.loadPendingAutoImportWords()
	if err != nil {
		return nil, err
	}
	visible := typeless.FilterVisiblePendingWords(words)
	if err := s.savePendingWordsCache(words); err != nil {
		return nil, err
	}
	if hasRetryableAutoImportWords(words, time.Now()) {
		s.startAutoImportSync(ctx, logWriter)
	}
	return visible, nil
}

func (s *Service) ResumeAutoImportSync(ctx context.Context, logWriter io.Writer) {
	words, err := s.loadPendingAutoImportWords()
	if err != nil {
		emitAutoImportLog(logWriter, "读取本地待同步词失败：%v", err)
		return
	}
	if hasSyncableAutoImportWords(words) {
		s.startAutoImportSync(ctx, logWriter)
	}
}

func (s *Service) startAutoImportSync(ctx context.Context, logWriter io.Writer) {
	s.autoImportMu.Lock()
	if s.autoImportRunning {
		s.autoImportMu.Unlock()
		return
	}
	if s.autoImportRetry != nil {
		s.autoImportRetry.Stop()
		s.autoImportRetry = nil
	}
	s.autoImportRunning = true
	s.autoImportMu.Unlock()

	go func() {
		defer func() {
			s.autoImportMu.Lock()
			s.autoImportRunning = false
			s.autoImportMu.Unlock()
			s.scheduleFailedAutoImportRetry(ctx, logWriter)
		}()
		s.syncPendingAutoImportWords(ctx, logWriter)
	}()
}

func (s *Service) syncPendingAutoImportWords(ctx context.Context, logWriter io.Writer) {
	emitAutoImportLog(logWriter, "后台自动导入同步已启动。")
	runStartedAt := time.Now()
	client := s.newDictionaryClient()
	processed := false
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		words, err := s.loadPendingAutoImportWords()
		if err != nil {
			emitAutoImportLog(logWriter, "读取本地待同步词失败：%v", err)
			return
		}
		if len(words) == 0 {
			emitAutoImportLog(logWriter, "没有待同步词条。")
			return
		}
		word, ok := nextSyncableAutoImportWord(words, runStartedAt)
		if !ok {
			break
		}
		processed = true
		if err := s.updatePendingAutoImportWordStatus(word.Term, typeless.AutoImportStatusSyncing, ""); err != nil {
			emitAutoImportLog(logWriter, "更新词 %q 状态失败：%v", word.Term, err)
			return
		}
		if err := client.Add(ctx, word.Term); err != nil {
			if typeless.IsSkippableDictionaryAddError(err) {
				if err := s.updatePendingAutoImportWordStatus(word.Term, typeless.AutoImportStatusSynced, ""); err != nil {
					emitAutoImportLog(logWriter, "保存词 %q 状态失败：%v", word.Term, err)
					return
				}
				emitAutoImportLog(logWriter, "词 %q 已存在，标记为已同步。", word.Term)
			} else {
				if err := s.updatePendingAutoImportWordStatus(word.Term, typeless.AutoImportStatusFailed, err.Error()); err != nil {
					emitAutoImportLog(logWriter, "保存词 %q 状态失败：%v", word.Term, err)
					return
				}
				emitAutoImportLog(logWriter, "词 %q 同步失败：%v", word.Term, err)
			}
		} else {
			if err := s.updatePendingAutoImportWordStatus(word.Term, typeless.AutoImportStatusSynced, ""); err != nil {
				emitAutoImportLog(logWriter, "保存词 %q 状态失败：%v", word.Term, err)
				return
			}
			emitAutoImportLog(logWriter, "词 %q 已同步。", word.Term)
		}
	}
	if !processed {
		emitAutoImportLog(logWriter, "当前没有可同步词条。")
	}
	if _, err := s.refreshDictionaryCache(ctx); err != nil {
		emitAutoImportLog(logWriter, "刷新本地缓存失败：%v", err)
		return
	}
	emitAutoImportLog(logWriter, "后台自动导入同步已完成。")
}

func (s *Service) autoImportStatePath() string {
	return s.config.AutoImportStatePath
}

func (s *Service) normalizeAutoImportSources(sources []typeless.AutoImportSource) ([]typeless.AutoImportSource, error) {
	if len(sources) == 0 {
		defaults, err := typeless.DefaultAutoImportSources()
		if err != nil {
			return nil, err
		}
		return defaults, nil
	}

	normalized := make([]typeless.AutoImportSource, 0, len(sources))
	for _, source := range sources {
		platform := strings.ToLower(strings.TrimSpace(source.Platform))
		switch platform {
		case typeless.AutoImportPlatformCodex, typeless.AutoImportPlatformClaude, typeless.AutoImportPlatformCustom:
		default:
			return nil, fmt.Errorf("未知平台 %q", source.Platform)
		}
		normalized = append(normalized, typeless.AutoImportSource{
			Platform: platform,
			Enabled:  source.Enabled,
			Workdir:  strings.TrimSpace(source.Workdir),
		})
	}
	slices.SortFunc(normalized, func(left, right typeless.AutoImportSource) int {
		return strings.Compare(left.Platform, right.Platform)
	})
	return normalized, nil
}

func (s *Service) loadPendingAutoImportWords() ([]typeless.PendingDictionaryWord, error) {
	s.autoImportStateMu.Lock()
	defer s.autoImportStateMu.Unlock()
	return typeless.LoadPendingDictionaryWords(s.autoImportStatePath())
}

func (s *Service) updatePendingAutoImportWords(update func([]typeless.PendingDictionaryWord) []typeless.PendingDictionaryWord) ([]typeless.PendingDictionaryWord, error) {
	s.autoImportStateMu.Lock()
	defer s.autoImportStateMu.Unlock()

	words, err := typeless.LoadPendingDictionaryWords(s.autoImportStatePath())
	if err != nil {
		return nil, err
	}
	nextWords := update(slices.Clone(words))
	if err := typeless.SavePendingDictionaryWords(s.autoImportStatePath(), nextWords); err != nil {
		return nil, err
	}
	if err := s.savePendingWordsCache(nextWords); err != nil {
		return nil, err
	}
	return nextWords, nil
}

func (s *Service) updatePendingAutoImportWordStatus(term, status, errorText string) error {
	_, err := s.updatePendingAutoImportWords(func(words []typeless.PendingDictionaryWord) []typeless.PendingDictionaryWord {
		return typeless.UpdatePendingDictionaryWordStatus(words, term, status, errorText)
	})
	return err
}

func hasRetryableAutoImportWords(words []typeless.PendingDictionaryWord, now time.Time) bool {
	for _, word := range words {
		if isRetryableAutoImportWord(word, now) {
			return true
		}
	}
	return false
}

func (s *Service) scheduleFailedAutoImportRetry(ctx context.Context, logWriter io.Writer) {
	if ctx.Err() != nil {
		return
	}
	words, err := s.loadPendingAutoImportWords()
	if err != nil {
		emitAutoImportLog(logWriter, "读取本地待同步词失败：%v", err)
		return
	}
	delay, ok := nextFailedAutoImportRetryDelay(words, time.Now())
	if !ok {
		return
	}
	s.autoImportMu.Lock()
	if s.autoImportRetry != nil {
		s.autoImportRetry.Stop()
	}
	s.autoImportRetry = time.AfterFunc(delay, func() {
		s.autoImportMu.Lock()
		s.autoImportRetry = nil
		s.autoImportMu.Unlock()
		s.startAutoImportSync(ctx, logWriter)
	})
	s.autoImportMu.Unlock()
}

func hasSyncableAutoImportWords(words []typeless.PendingDictionaryWord) bool {
	for _, word := range words {
		if isSyncableAutoImportWord(word) {
			return true
		}
	}
	return false
}

func nextSyncableAutoImportWord(words []typeless.PendingDictionaryWord, runStartedAt time.Time) (typeless.PendingDictionaryWord, bool) {
	for _, word := range words {
		if isSyncableAutoImportWordForRun(word, runStartedAt) {
			return word, true
		}
	}
	return typeless.PendingDictionaryWord{}, false
}

func nextFailedAutoImportRetryDelay(words []typeless.PendingDictionaryWord, now time.Time) (time.Duration, bool) {
	var (
		next time.Duration
		ok   bool
	)
	for _, word := range words {
		if word.Status != typeless.AutoImportStatusFailed {
			continue
		}
		updatedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(word.UpdatedAt))
		if err != nil {
			return 0, true
		}
		delay := updatedAt.Add(failedAutoImportRetryDelay).Sub(now)
		if delay <= 0 {
			return 0, true
		}
		if !ok || delay < next {
			next = delay
			ok = true
		}
	}
	return next, ok
}

func isRetryableAutoImportWord(word typeless.PendingDictionaryWord, now time.Time) bool {
	switch word.Status {
	case typeless.AutoImportStatusPending, typeless.AutoImportStatusSyncing:
		return true
	case typeless.AutoImportStatusFailed:
		updatedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(word.UpdatedAt))
		if err != nil {
			return true
		}
		return !updatedAt.Add(failedAutoImportRetryDelay).After(now)
	default:
		return false
	}
}

func isSyncableAutoImportWordForRun(word typeless.PendingDictionaryWord, runStartedAt time.Time) bool {
	switch word.Status {
	case typeless.AutoImportStatusPending, typeless.AutoImportStatusSyncing:
		return true
	case typeless.AutoImportStatusFailed:
		updatedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(word.UpdatedAt))
		if err != nil {
			return true
		}
		return !updatedAt.After(runStartedAt)
	default:
		return false
	}
}

func isSyncableAutoImportWord(word typeless.PendingDictionaryWord) bool {
	switch word.Status {
	case typeless.AutoImportStatusPending, typeless.AutoImportStatusSyncing, typeless.AutoImportStatusFailed:
		return true
	default:
		return false
	}
}

func emitAutoImportLog(writer io.Writer, format string, args ...any) {
	if writer == nil {
		return
	}
	fmt.Fprintf(writer, format+"\n", args...)
}
