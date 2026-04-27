package service

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/penwyp/typelens/pkg/typeless"
)

type Config struct {
	UserDataPath        string `json:"userDataPath"`
	DBPath              string `json:"dbPath"`
	APIHost             string `json:"apiHost"`
	TimeoutSec          int    `json:"timeoutSec"`
	AutoImportStatePath string `json:"autoImportStatePath"`
	CachePath           string `json:"cachePath"`
}

type HistoryQuery struct {
	Limit       int    `json:"limit"`
	Keyword     string `json:"keyword"`
	Regex       string `json:"regex"`
	ContextMode string `json:"contextMode"`
}

type ImportRequest struct {
	FilePath    string
	DryRun      bool
	Concurrency int
	LogWriter   io.Writer
}

type ResetRequest struct {
	DefaultsFile string
	Concurrency  int
	LogWriter    io.Writer
}

type ClearRequest struct {
	Concurrency int
	LogWriter   io.Writer
}

type ExportRequest struct {
	FilePath  string
	LogWriter io.Writer
}

type Service struct {
	config            Config
	autoImportMu      sync.Mutex
	autoImportRunning bool
}

func DefaultConfig() (Config, error) {
	userDataPath, err := typeless.DefaultUserDataPath()
	if err != nil {
		return Config{}, err
	}
	dbPath, err := typeless.DefaultHistoryDBPath()
	if err != nil {
		return Config{}, err
	}
	autoImportStatePath, err := typeless.DefaultAutoImportStatePath()
	if err != nil {
		return Config{}, err
	}
	cachePath, err := typeless.DefaultCachePath()
	if err != nil {
		return Config{}, err
	}
	return normalizeConfig(Config{
		UserDataPath:        userDataPath,
		DBPath:              dbPath,
		APIHost:             typeless.DefaultAPIHost,
		TimeoutSec:          15,
		AutoImportStatePath: autoImportStatePath,
		CachePath:           cachePath,
	}), nil
}

func New(config Config) *Service {
	return &Service{config: normalizeConfig(config)}
}

func (s *Service) Config() Config {
	return s.config
}

func (s *Service) SetConfig(config Config) {
	s.config = normalizeConfig(config)
}

func (s *Service) ListDictionary(ctx context.Context) ([]typeless.DictionaryWord, error) {
	return s.newDictionaryClient().ListAll(ctx)
}

func (s *Service) AddDictionaryTerm(ctx context.Context, term string) error {
	term = strings.TrimSpace(term)
	if term == "" {
		return fmt.Errorf("词条不能为空")
	}
	return s.newDictionaryClient().Add(ctx, term)
}

func (s *Service) DeleteDictionaryWord(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("词条 ID 不能为空")
	}
	return s.newDictionaryClient().Delete(ctx, id)
}

func (s *Service) ImportDictionary(ctx context.Context, request ImportRequest) (typeless.ImportResult, error) {
	if strings.TrimSpace(request.FilePath) == "" {
		return typeless.ImportResult{}, fmt.Errorf("导入文件不能为空")
	}
	writeProgressLog(request.LogWriter, "开始解析文件。")
	terms, err := typeless.ReadTermsFile(request.FilePath)
	if err != nil {
		return typeless.ImportResult{}, err
	}
	writeProgressLog(request.LogWriter, "解析文件完成，共 %d 行。", len(terms))
	writeProgressLog(request.LogWriter, "开始获取历史所有记录。")
	return s.newDictionaryClient().ImportTerms(ctx, terms, typeless.ImportOptions{
		DryRun:         request.DryRun,
		Concurrency:    request.Concurrency,
		ProgressWriter: request.LogWriter,
	})
}

func (s *Service) ClearDictionary(ctx context.Context, request ClearRequest) (int, error) {
	return s.newDictionaryClient().Clear(ctx, typeless.ClearOptions{
		Concurrency:    request.Concurrency,
		ProgressWriter: request.LogWriter,
	})
}

func (s *Service) ResetDictionary(ctx context.Context, request ResetRequest) (typeless.ResetResult, error) {
	terms := typeless.DefaultDictionaryTerms
	if strings.TrimSpace(request.DefaultsFile) != "" {
		writeProgressLog(request.LogWriter, "开始解析文件。")
		fileTerms, err := typeless.ReadTermsFile(request.DefaultsFile)
		if err != nil {
			return typeless.ResetResult{}, err
		}
		terms = fileTerms
		writeProgressLog(request.LogWriter, "解析文件完成，共 %d 行。", len(terms))
	} else {
		writeProgressLog(request.LogWriter, "使用内置词表，共 %d 行。", len(terms))
	}
	writeProgressLog(request.LogWriter, "开始获取历史所有记录。")
	return s.newDictionaryClient().Reset(ctx, terms, typeless.ResetOptions{
		Concurrency:    request.Concurrency,
		ProgressWriter: request.LogWriter,
	})
}

func (s *Service) QueryHistory(ctx context.Context, query HistoryQuery) ([]typeless.TranscriptRecord, error) {
	user, err := typeless.LoadCurrentUser(ctx, s.config.UserDataPath)
	if err != nil {
		return nil, err
	}
	appCtx, err := s.resolveHistoryContext(ctx, user.UserID, query.ContextMode)
	if err != nil {
		return nil, err
	}
	options, err := s.buildHistoryOptions(query)
	if err != nil {
		return nil, err
	}
	return typeless.QueryRecentTranscripts(ctx, s.config.DBPath, user.UserID, appCtx, options)
}

func (s *Service) CopyText(ctx context.Context, text string) error {
	return typeless.CopyToClipboard(ctx, text)
}

func (s *Service) ExportDictionary(ctx context.Context, request ExportRequest) (int, error) {
	filePath := strings.TrimSpace(request.FilePath)
	if filePath == "" {
		return 0, fmt.Errorf("导出文件不能为空")
	}
	writeProgressLog(request.LogWriter, "10%% 准备导出词典。")
	writeProgressLog(request.LogWriter, "25%% 正在读取远端词典。")
	words, err := s.newDictionaryClient().ListAll(ctx)
	if err != nil {
		return 0, err
	}
	writeProgressLog(request.LogWriter, "45%% 远端词典读取完成，共 %d 个词条。", len(words))
	writeProgressLog(request.LogWriter, "55%% 正在读取本地待同步词条。")
	pending, err := typeless.LoadPendingDictionaryWords(s.config.AutoImportStatePath)
	if err != nil {
		return 0, err
	}
	writeProgressLog(request.LogWriter, "70%% 本地待同步词条读取完成，共 %d 个。", len(pending))
	terms := typeless.MergeDictionaryExportTerms(words, pending)
	writeProgressLog(request.LogWriter, "85%% 合并去重完成，待写入 1/1 个文件，共 %d 行。", len(terms))
	if err := typeless.WriteDictionaryTermsFile(filePath, terms); err != nil {
		return 0, err
	}
	writeProgressLog(request.LogWriter, "100%% 导出完成，输出文件 1/1：%s", filePath)
	return len(terms), nil
}

func (s *Service) resolveHistoryContext(ctx context.Context, userID, mode string) (typeless.AppContext, error) {
	switch strings.TrimSpace(mode) {
	case "", "frontmost":
		return typeless.CurrentAppContext(ctx)
	case "latest":
		return typeless.LatestTranscriptContext(ctx, s.config.DBPath, userID)
	case "all":
		return typeless.AppContext{}, nil
	default:
		return typeless.AppContext{}, fmt.Errorf("未知上下文来源 %q，可选: frontmost/latest/all", mode)
	}
}

func (s *Service) buildHistoryOptions(query HistoryQuery) (typeless.TranscriptQueryOptions, error) {
	options := typeless.TranscriptQueryOptions{
		Limit:   query.Limit,
		Keyword: strings.TrimSpace(query.Keyword),
	}
	if options.Limit <= 0 {
		options.Limit = 20
	}
	if strings.TrimSpace(query.Regex) != "" {
		compiled, err := regexp.Compile(query.Regex)
		if err != nil {
			return typeless.TranscriptQueryOptions{}, fmt.Errorf("编译正则失败: %w", err)
		}
		options.Regex = compiled
	}
	return options, nil
}

func (s *Service) newDictionaryClient() *typeless.DictionaryClient {
	return typeless.NewDictionaryClient(
		s.config.APIHost,
		s.config.UserDataPath,
		time.Duration(s.config.TimeoutSec)*time.Second,
	)
}

func normalizeConfig(config Config) Config {
	config.UserDataPath = strings.TrimSpace(config.UserDataPath)
	config.DBPath = strings.TrimSpace(config.DBPath)
	config.APIHost = strings.TrimSpace(config.APIHost)
	config.AutoImportStatePath = strings.TrimSpace(config.AutoImportStatePath)
	config.CachePath = strings.TrimSpace(config.CachePath)
	if config.APIHost == "" {
		config.APIHost = typeless.DefaultAPIHost
	}
	if config.TimeoutSec <= 0 {
		config.TimeoutSec = 15
	}
	if config.AutoImportStatePath == "" {
		if defaultPath, err := typeless.DefaultAutoImportStatePath(); err == nil {
			config.AutoImportStatePath = defaultPath
		}
	}
	if config.CachePath == "" {
		if defaultPath, err := typeless.DefaultCachePath(); err == nil {
			config.CachePath = defaultPath
		}
	}
	return config
}

func writeProgressLog(writer io.Writer, format string, args ...any) {
	if writer == nil {
		return
	}
	fmt.Fprintf(writer, format+"\n", args...)
}
