package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/penwyp/typelens/pkg/typeless"
)

type DictionaryCache struct {
	Words        []typeless.DictionaryWord        `json:"words"`
	PendingWords []typeless.PendingDictionaryWord `json:"pendingWords"`
}

type appCacheStore struct {
	Dictionary   []typeless.DictionaryWord              `json:"dictionary"`
	PendingWords []typeless.PendingDictionaryWord       `json:"pending_words"`
	Histories    map[string][]typeless.TranscriptRecord `json:"histories"`
}

var cacheFileMu sync.Mutex

func (s *Service) LoadDictionaryCache() (DictionaryCache, error) {
	store, err := s.readCacheStore()
	if err != nil {
		return DictionaryCache{}, err
	}
	return DictionaryCache{
		Words:        store.Dictionary,
		PendingWords: store.PendingWords,
	}, nil
}

func (s *Service) SaveDictionaryCache(cache DictionaryCache) error {
	return s.updateCacheStore(func(store *appCacheStore) {
		store.Dictionary = cloneSlice(cache.Words)
		store.PendingWords = cloneSlice(cache.PendingWords)
	})
}

func (s *Service) LoadHistoryCache(query HistoryQuery) ([]typeless.TranscriptRecord, error) {
	store, err := s.readCacheStore()
	if err != nil {
		return nil, err
	}
	return cloneSlice(store.Histories[historyCacheKey(query)]), nil
}

func (s *Service) SaveHistoryCache(query HistoryQuery, records []typeless.TranscriptRecord) error {
	return s.updateCacheStore(func(store *appCacheStore) {
		if store.Histories == nil {
			store.Histories = make(map[string][]typeless.TranscriptRecord)
		}
		store.Histories[historyCacheKey(query)] = cloneSlice(records)
	})
}

func (s *Service) readCacheStore() (appCacheStore, error) {
	cacheFileMu.Lock()
	defer cacheFileMu.Unlock()
	return s.readCacheStoreLocked()
}

func (s *Service) readCacheStoreLocked() (appCacheStore, error) {
	path := s.config.CachePath
	if strings.TrimSpace(path) == "" {
		return appCacheStore{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return appCacheStore{}, nil
		}
		return appCacheStore{}, err
	}
	var store appCacheStore
	if err := json.Unmarshal(raw, &store); err != nil {
		return appCacheStore{}, nil
	}
	if store.Histories == nil {
		store.Histories = make(map[string][]typeless.TranscriptRecord)
	}
	return store, nil
}

func (s *Service) updateCacheStore(update func(store *appCacheStore)) error {
	cacheFileMu.Lock()
	defer cacheFileMu.Unlock()

	store, err := s.readCacheStoreLocked()
	if err != nil {
		return err
	}
	update(&store)
	return s.writeCacheStoreLocked(store)
}

func (s *Service) writeCacheStoreLocked(store appCacheStore) error {
	path := s.config.CachePath
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func historyCacheKey(query HistoryQuery) string {
	return strings.TrimSpace(query.ContextMode) + "::" +
		strings.TrimSpace(query.Keyword) + "::" +
		strings.TrimSpace(query.Regex)
}

func cloneSlice[T any](items []T) []T {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]T, len(items))
	copy(cloned, items)
	return cloned
}
