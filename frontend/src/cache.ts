import { GetDictionaryCache, GetHistoryCache, SaveDictionaryCache, SaveHistoryCache } from '../wailsjs/go/main/App';
import { service, typeless } from '../wailsjs/go/models';

export async function readDictionaryCache() {
  const cache = await GetDictionaryCache();
  return {
    words: cache.words ?? [],
    pendingWords: cache.pendingWords ?? [],
  };
}

export async function writeDictionaryCache(
  words: typeless.DictionaryWord[],
  pendingWords: typeless.PendingDictionaryWord[],
) {
  await SaveDictionaryCache(new service.DictionaryCache({
    words,
    pendingWords,
  }));
}

export async function readHistoryCache(query: service.HistoryQuery) {
  return await GetHistoryCache(query);
}

export async function writeHistoryCache(query: service.HistoryQuery, records: typeless.TranscriptRecord[]) {
  await SaveHistoryCache(query, records);
}
