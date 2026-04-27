import { GetDictionaryCache, GetHistoryCache, SaveHistoryCache } from '../wailsjs/go/main/App';
import { service, typeless } from '../wailsjs/go/models';

export async function readDictionaryCache() {
  const cache = await GetDictionaryCache();
  return {
    words: cache.words ?? [],
    pendingWords: (cache.pendingWords ?? []).filter((word) => word.status === 'pending' || word.status === 'syncing'),
  };
}

export async function readHistoryCache(query: service.HistoryQuery) {
  return await GetHistoryCache(query);
}

export async function writeHistoryCache(query: service.HistoryQuery, records: typeless.TranscriptRecord[]) {
  await SaveHistoryCache(query, records);
}
