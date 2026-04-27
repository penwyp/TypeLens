package main

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/penwyp/typelens/internal/service"
	"github.com/penwyp/typelens/pkg/typeless"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	service *service.Service
}

func NewApp() *App {
	defaultConfig, err := service.DefaultConfig()
	if err != nil {
		defaultConfig = service.Config{
			APIHost:    typeless.DefaultAPIHost,
			TimeoutSec: 15,
		}
	}
	return &App{
		service: service.New(defaultConfig),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.service.ResumeAutoImportSync(ctx, newAutoImportEventWriter(ctx))
}

func (a *App) GetConfig() service.Config {
	return a.service.Config()
}

func (a *App) GetDictionaryCache() (service.DictionaryCache, error) {
	return a.service.LoadDictionaryCache()
}

func (a *App) SaveDictionaryCache(cache service.DictionaryCache) error {
	return a.service.SaveDictionaryCache(cache)
}

func (a *App) GetHistoryCache(query service.HistoryQuery) ([]typeless.TranscriptRecord, error) {
	return a.service.LoadHistoryCache(query)
}

func (a *App) SaveHistoryCache(query service.HistoryQuery, records []typeless.TranscriptRecord) error {
	return a.service.SaveHistoryCache(query, records)
}

func (a *App) SetConfig(config service.Config) {
	a.service.SetConfig(config)
}

func (a *App) ListDictionaryWords() ([]typeless.DictionaryWord, error) {
	return a.service.ListDictionary(a.ctx)
}

func (a *App) AddDictionaryTerm(term string) error {
	return a.service.AddDictionaryTerm(a.ctx, term)
}

func (a *App) ImportDictionaryFile(filePath string, concurrency int, dryRun bool) (typeless.ImportResult, error) {
	return a.service.ImportDictionary(a.ctx, service.ImportRequest{
		FilePath:    filePath,
		Concurrency: concurrency,
		DryRun:      dryRun,
		LogWriter:   newEventWriter(a.ctx, "typelens:dictionary-log"),
	})
}

func (a *App) DeleteDictionaryWord(id string) error {
	return a.service.DeleteDictionaryWord(a.ctx, id)
}

func (a *App) ClearDictionary() (int, error) {
	return a.service.ClearDictionary(a.ctx, service.ClearRequest{
		Concurrency: 10,
		LogWriter:   newEventWriter(a.ctx, "typelens:dictionary-log"),
	})
}

func (a *App) ResetDictionary(defaultsFile string, concurrency int) (typeless.ResetResult, error) {
	return a.service.ResetDictionary(a.ctx, service.ResetRequest{
		DefaultsFile: defaultsFile,
		Concurrency:  concurrency,
		LogWriter:    newEventWriter(a.ctx, "typelens:dictionary-log"),
	})
}

func (a *App) QueryHistory(query service.HistoryQuery) ([]typeless.TranscriptRecord, error) {
	return a.service.QueryHistory(a.ctx, query)
}

func (a *App) CopyText(text string) error {
	return a.service.CopyText(a.ctx, text)
}

func (a *App) SelectTextFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择文本文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Text Files (*.txt)",
				Pattern:     "*.txt",
			},
			{
				DisplayName: "All Files (*.*)",
				Pattern:     "*.*",
			},
		},
	})
}

func (a *App) SelectDictionaryExportFile() (string, error) {
	defaultDir, err := typeless.DefaultDictionaryExportDir()
	if err != nil {
		return "", err
	}
	defaultName := typeless.DefaultDictionaryExportFilename(time.Now())
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:            "导出词典",
		DefaultDirectory: defaultDir,
		DefaultFilename:  defaultName,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Text Files (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})
}

func (a *App) ExportDictionaryFile(filePath string) (int, error) {
	return a.service.ExportDictionary(a.ctx, service.ExportRequest{
		FilePath: filePath,
	})
}

func (a *App) DefaultAutoImportSources() ([]typeless.AutoImportSource, error) {
	return typeless.DefaultAutoImportSources()
}

func (a *App) ScanAutoImportSources(request service.AutoImportScanRequest) (typeless.AutoImportScanResult, error) {
	return a.service.ScanAutoImport(a.ctx, request)
}

func (a *App) ConfirmAutoImport(request service.AutoImportConfirmRequest) (service.AutoImportConfirmResult, error) {
	return a.service.ConfirmAutoImport(a.ctx, request, newAutoImportEventWriter(a.ctx))
}

func (a *App) ListPendingImportedWords() ([]typeless.PendingDictionaryWord, error) {
	return a.service.ListPendingAutoImportWords()
}

type eventWriter struct {
	ctx   context.Context
	event string
	mu    sync.Mutex
}

type autoImportEventWriter struct {
	ctx context.Context
	mu  sync.Mutex
}

func newEventWriter(ctx context.Context, event string) io.Writer {
	return &eventWriter{ctx: ctx, event: event}
}

func newAutoImportEventWriter(ctx context.Context) io.Writer {
	return &autoImportEventWriter{ctx: ctx}
}

func (w *eventWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	text := strings.TrimSpace(string(p))
	if text == "" {
		return len(p), nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runtime.EventsEmit(w.ctx, w.event, line)
	}
	return len(p), nil
}

func (w *autoImportEventWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	text := strings.TrimSpace(string(p))
	if text == "" {
		return len(p), nil
	}
	completed := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runtime.EventsEmit(w.ctx, "typelens:auto-import-log", line)
		if line == "后台自动导入同步已完成。" || line == "没有待同步词条。" {
			completed = true
		}
	}
	if completed {
		runtime.EventsEmit(w.ctx, "typelens:auto-import-finished")
	}
	return len(p), nil
}
