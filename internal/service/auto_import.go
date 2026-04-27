package service

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

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

	pendingWords, err := typeless.LoadPendingDictionaryWords(s.autoImportStatePath())
	if err != nil {
		return AutoImportConfirmResult{}, err
	}
	nextWords, acceptedCount := typeless.MergePendingCandidates(pendingWords, request.Items)
	if acceptedCount == 0 {
		return AutoImportConfirmResult{
			AcceptedCount: 0,
			Words:         typeless.FilterVisiblePendingWords(nextWords),
		}, nil
	}
	if err := typeless.SavePendingDictionaryWords(s.autoImportStatePath(), nextWords); err != nil {
		return AutoImportConfirmResult{}, err
	}
	if err := s.savePendingWordsCache(nextWords); err != nil {
		return AutoImportConfirmResult{}, err
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

	pendingWords, err := typeless.LoadPendingDictionaryWords(s.autoImportStatePath())
	if err != nil {
		return AutoImportConfirmResult{}, err
	}
	nextWords, acceptedCount := typeless.MergePendingCandidates(pendingWords, request.Items)
	if acceptedCount == 0 {
		return AutoImportConfirmResult{
			AcceptedCount: 0,
			Words:         typeless.FilterVisiblePendingWords(nextWords),
		}, nil
	}
	if err := typeless.SavePendingDictionaryWords(s.autoImportStatePath(), nextWords); err != nil {
		return AutoImportConfirmResult{}, err
	}
	if err := s.savePendingWordsCache(nextWords); err != nil {
		return AutoImportConfirmResult{}, err
	}

	emitAutoImportLog(logWriter, "已写入本地待同步词条 %d 个，开始同步。", acceptedCount)
	s.syncPendingAutoImportWords(ctx, logWriter)

	words, err := typeless.LoadPendingDictionaryWords(s.autoImportStatePath())
	if err != nil {
		return AutoImportConfirmResult{}, err
	}
	return AutoImportConfirmResult{
		AcceptedCount: acceptedCount,
		Words:         typeless.FilterVisiblePendingWords(words),
	}, nil
}

func (s *Service) ListPendingAutoImportWords() ([]typeless.PendingDictionaryWord, error) {
	words, err := typeless.LoadPendingDictionaryWords(s.autoImportStatePath())
	if err != nil {
		return nil, err
	}
	visible := typeless.FilterVisiblePendingWords(words)
	if err := s.savePendingWordsCache(words); err != nil {
		return nil, err
	}
	return visible, nil
}

func (s *Service) ResumeAutoImportSync(ctx context.Context, logWriter io.Writer) {
	s.startAutoImportSync(ctx, logWriter)
}

func (s *Service) startAutoImportSync(ctx context.Context, logWriter io.Writer) {
	s.autoImportMu.Lock()
	if s.autoImportRunning {
		s.autoImportMu.Unlock()
		return
	}
	s.autoImportRunning = true
	s.autoImportMu.Unlock()

	go func() {
		defer func() {
			s.autoImportMu.Lock()
			s.autoImportRunning = false
			s.autoImportMu.Unlock()
		}()
		s.syncPendingAutoImportWords(ctx, logWriter)
	}()
}

func (s *Service) syncPendingAutoImportWords(ctx context.Context, logWriter io.Writer) {
	emitAutoImportLog(logWriter, "后台自动导入同步已启动。")
	words, err := typeless.LoadPendingDictionaryWords(s.autoImportStatePath())
	if err != nil {
		emitAutoImportLog(logWriter, "读取本地待同步词失败：%v", err)
		return
	}
	if len(words) == 0 {
		emitAutoImportLog(logWriter, "没有待同步词条。")
		return
	}
	client := s.newDictionaryClient()
	for _, word := range words {
		if err := ctx.Err(); err != nil {
			return
		}
		if word.Status != typeless.AutoImportStatusPending && word.Status != typeless.AutoImportStatusFailed {
			continue
		}
		words = typeless.UpdatePendingDictionaryWordStatus(words, word.Term, typeless.AutoImportStatusSyncing, "")
		if err := typeless.SavePendingDictionaryWords(s.autoImportStatePath(), words); err != nil {
			emitAutoImportLog(logWriter, "更新词 %q 状态失败：%v", word.Term, err)
			return
		}
		if err := s.savePendingWordsCache(words); err != nil {
			emitAutoImportLog(logWriter, "更新本地缓存失败：%v", err)
			return
		}
		if err := client.Add(ctx, word.Term); err != nil {
			if typeless.IsSkippableDictionaryAddError(err) {
				words = typeless.UpdatePendingDictionaryWordStatus(words, word.Term, typeless.AutoImportStatusSynced, "")
				emitAutoImportLog(logWriter, "词 %q 已存在，标记为已同步。", word.Term)
			} else {
				words = typeless.UpdatePendingDictionaryWordStatus(words, word.Term, typeless.AutoImportStatusFailed, err.Error())
				emitAutoImportLog(logWriter, "词 %q 同步失败：%v", word.Term, err)
			}
		} else {
			words = typeless.UpdatePendingDictionaryWordStatus(words, word.Term, typeless.AutoImportStatusSynced, "")
			emitAutoImportLog(logWriter, "词 %q 已同步。", word.Term)
		}
		if err := typeless.SavePendingDictionaryWords(s.autoImportStatePath(), words); err != nil {
			emitAutoImportLog(logWriter, "保存词 %q 状态失败：%v", word.Term, err)
			return
		}
		if err := s.savePendingWordsCache(words); err != nil {
			emitAutoImportLog(logWriter, "保存本地缓存失败：%v", err)
			return
		}
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

func emitAutoImportLog(writer io.Writer, format string, args ...any) {
	if writer == nil {
		return
	}
	fmt.Fprintf(writer, format+"\n", args...)
}
