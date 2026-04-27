import { UIEvent, useEffect, useMemo, useRef, useState } from 'react';
import { CopyText, QueryHistory } from '../wailsjs/go/main/App';
import { service, typeless } from '../wailsjs/go/models';
import { BrowserOpenURL, WindowSetBackgroundColour } from '../wailsjs/runtime/runtime';
import { readHistoryCache, writeHistoryCache } from './cache';
import { DictionaryView } from './features/dictionary/DictionaryView';
import './App.css';

type Notice = {
  kind: 'success' | 'error' | 'info';
  text: string;
};

type HistorySort = 'desc' | 'asc';

const HISTORY_PAGE_SIZE = 30;
const HISTORY_FETCH_MULTIPLIER = 8;
const DEFAULT_HISTORY_QUERY: service.HistoryQuery = {
  limit: HISTORY_PAGE_SIZE,
  keyword: '',
  regex: '',
  contextMode: 'all',
};

function App() {
  const [activeView, setActiveView] = useState<'dictionary' | 'history'>('dictionary');
  const [notice, setNotice] = useState<Notice | null>(null);
  const [copyNotice, setCopyNotice] = useState<string | null>(null);
  const [dictionaryCount, setDictionaryCount] = useState(0);
  const [historyQuery, setHistoryQuery] = useState<service.HistoryQuery>(DEFAULT_HISTORY_QUERY);
  const [historySort, setHistorySort] = useState<HistorySort>('desc');
  const [records, setRecords] = useState<typeless.TranscriptRecord[]>([]);
  const [historyReady, setHistoryReady] = useState(false);
  const historySearchRef = useRef<HTMLInputElement | null>(null);
  const historyCacheQuery = useMemo(() => ({
    ...historyQuery,
    limit: HISTORY_PAGE_SIZE,
  }), [historyQuery.contextMode, historyQuery.keyword, historyQuery.regex]);

  useEffect(() => {
    WindowSetBackgroundColour(0, 0, 0, 0);
    void (async () => {
      await hydrateHistoryCache(DEFAULT_HISTORY_QUERY);
      await loadHistory(DEFAULT_HISTORY_QUERY, { silent: true, silentError: true });
    })();
  }, []);

  useEffect(() => {
    void hydrateHistoryCache(historyCacheQuery);
  }, [historyCacheQuery]);

  useEffect(() => {
    if (activeView !== 'history') {
      return;
    }
    const timer = window.setTimeout(() => {
      void loadHistory(historyQuery, { silent: true, silentError: true });
    }, 200);
    return () => window.clearTimeout(timer);
  }, [activeView, historyQuery.limit, historyQuery.keyword, historyQuery.regex, historyQuery.contextMode]);

  useEffect(() => {
    if (!notice) {
      return;
    }
    const timer = window.setTimeout(() => setNotice(null), 1800);
    return () => window.clearTimeout(timer);
  }, [notice]);

  useEffect(() => {
    if (!copyNotice) {
      return;
    }
    const timer = window.setTimeout(() => setCopyNotice(null), 2200);
    return () => window.clearTimeout(timer);
  }, [copyNotice]);

  useEffect(() => {
    function handleFindShortcut(event: KeyboardEvent) {
      if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== 'f') {
        return;
      }
      event.preventDefault();
      setActiveView('history');
      window.setTimeout(() => historySearchRef.current?.focus(), 0);
    }

    window.addEventListener('keydown', handleFindShortcut);
    return () => window.removeEventListener('keydown', handleFindShortcut);
  }, []);

  async function loadHistory(
    query: service.HistoryQuery = historyQuery,
    options: { silent?: boolean; silentError?: boolean } = {},
  ) {
    try {
      const nextRecords = await QueryHistory({
        ...query,
        limit: query.limit * HISTORY_FETCH_MULTIPLIER,
      });
      setRecords(nextRecords);
      await writeHistoryCache(query, nextRecords);
      setHistoryReady(true);
      if (!options.silent) {
        setNotice({ kind: 'success', text: '历史记录已刷新。' });
      }
    } catch (error) {
      setHistoryReady(true);
      if (!options.silentError) {
        setNotice({ kind: 'error', text: stringifyError(error) });
      }
    }
  }

  async function hydrateHistoryCache(query: service.HistoryQuery) {
    try {
      const cachedRecords = await readHistoryCache(query);
      setRecords(cachedRecords);
      setHistoryReady(cachedRecords.length > 0);
    } catch {
      setHistoryReady(false);
    }
  }

  async function copyHistoryRecord(record: typeless.TranscriptRecord) {
    try {
      await CopyText(record.Text);
      setCopyNotice(`复制成功 ${historyCopySummary(record.Text)}`);
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    }
  }

  const visibleHistoryRecords = useMemo(() => {
    return [...records].sort((left, right) => {
      const leftTime = Date.parse(left.CreatedAt || '');
      const rightTime = Date.parse(right.CreatedAt || '');
      const fallback = (left.CreatedAt || '').localeCompare(right.CreatedAt || '');
      const diff = Number.isNaN(leftTime) || Number.isNaN(rightTime) ? fallback : leftTime - rightTime;
      return historySort === 'asc' ? diff : -diff;
    }).slice(0, historyQuery.limit);
  }, [records, historyQuery.limit, historySort]);

  function handleHistoryScroll(event: UIEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    if (visibleHistoryRecords.length < historyQuery.limit || element.scrollTop + element.clientHeight < element.scrollHeight - 120) {
      return;
    }
    setHistoryQuery((query) => ({ ...query, limit: query.limit + HISTORY_PAGE_SIZE }));
  }

  return (
    <div id="app-shell">
      <aside className="sidebar">
        <div>
          <button className={activeView === 'dictionary' ? 'side-item active' : 'side-item'} onClick={() => setActiveView('dictionary')}>
            <span className="side-label">
              <BookIcon />
              词典
            </span>
            <strong>{dictionaryCount}</strong>
          </button>
          <button className={activeView === 'history' ? 'side-item active' : 'side-item'} onClick={() => setActiveView('history')}>
            <span className="side-label">
              <HistoryIcon />
              历史记录
            </span>
          </button>
        </div>
        <button className="github-link" type="button" aria-label="GitHub" onClick={() => BrowserOpenURL('https://github.com/penwyp/TypeLens')}>
          <svg aria-hidden="true" viewBox="0 0 24 24">
            <path fill="currentColor" d="M12 .5a12 12 0 0 0-3.79 23.39c.6.11.82-.26.82-.58v-2.05c-3.34.73-4.04-1.42-4.04-1.42-.55-1.38-1.34-1.75-1.34-1.75-1.09-.75.08-.73.08-.73 1.2.08 1.84 1.24 1.84 1.24 1.07 1.83 2.81 1.3 3.49.99.11-.78.42-1.3.76-1.6-2.67-.3-5.47-1.33-5.47-5.93 0-1.31.47-2.38 1.24-3.22-.12-.3-.54-1.52.12-3.18 0 0 1.01-.32 3.3 1.23a11.5 11.5 0 0 1 6 0c2.29-1.55 3.3-1.23 3.3-1.23.66 1.66.24 2.88.12 3.18.77.84 1.24 1.91 1.24 3.22 0 4.61-2.81 5.63-5.49 5.93.43.37.82 1.1.82 2.22v3.29c0 .32.22.7.83.58A12 12 0 0 0 12 .5Z" />
          </svg>
        </button>
      </aside>

      <main className="workspace">
        {activeView === 'dictionary' ? (
          <DictionaryView
            onCountChange={setDictionaryCount}
            onNotice={setNotice}
            onCopyNotice={setCopyNotice}
          />
        ) : (
          <section className="view">
            <div className="toolbar">
              <div className="search-box">
                <input
                  ref={historySearchRef}
                  className="search-input"
                  value={historyQuery.keyword}
                  onChange={(event) => setHistoryQuery({ ...historyQuery, keyword: event.target.value, limit: HISTORY_PAGE_SIZE })}
                  placeholder="搜索历史记录"
                />
                {historyQuery.keyword ? (
                  <button
                    className="search-clear"
                    type="button"
                    aria-label="清空搜索"
                    onClick={() => {
                      setHistoryQuery({ ...historyQuery, keyword: '', limit: HISTORY_PAGE_SIZE });
                      historySearchRef.current?.focus();
                    }}
                  >
                    ×
                  </button>
                ) : null}
              </div>
              <div className="button-row">
                <button className="ghost-button" onClick={() => void loadHistory(historyQuery)}>刷新</button>
                <select value={historyQuery.contextMode} onChange={(event) => setHistoryQuery({ ...historyQuery, contextMode: event.target.value, limit: HISTORY_PAGE_SIZE })}>
                  <option value="all">全部</option>
                  <option value="frontmost">当前应用</option>
                  <option value="latest">最近来源</option>
                </select>
                <select value={historySort} onChange={(event) => setHistorySort(event.target.value as HistorySort)}>
                  <option value="desc">最新优先</option>
                  <option value="asc">最早优先</option>
                </select>
              </div>
            </div>

            <div className="list history-list" onScroll={handleHistoryScroll}>
              {!historyReady && visibleHistoryRecords.length === 0 ? <LoadingState /> : null}
              {visibleHistoryRecords.map((record) => (
                <article className="history-card" key={record.ID} onClick={() => void copyHistoryRecord(record)}>
                  <time className="history-time">{formatShanghaiTime(record.CreatedAt)}</time>
                  <pre className="history-text">{record.Text}</pre>
                </article>
              ))}
              {historyReady && visibleHistoryRecords.length === 0 ? <div className="empty-state">暂无历史记录</div> : null}
            </div>
          </section>
        )}
      </main>

      {notice ? <div className={`toast toast-${notice.kind}`}>{notice.text}</div> : null}
      {copyNotice ? <div className="copy-toast">{copyNotice}</div> : null}
    </div>
  );
}

function BookIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24">
      <path fill="currentColor" d="M5.5 3.5h10.25A2.75 2.75 0 0 1 18.5 6.25V21H7a2.5 2.5 0 0 1-2.5-2.5V4.5a1 1 0 0 1 1-1Zm1 13.5v1.5A.5.5 0 0 0 7 19h9.5v-2H6.5Zm0-2h10V6.25c0-.41-.34-.75-.75-.75H6.5V15Z" />
    </svg>
  );
}

function HistoryIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24">
      <path fill="currentColor" d="M12 4a8 8 0 1 1-7.45 5.09.95.95 0 1 1 1.77.69A6.1 6.1 0 1 0 8.2 6.9H10a.95.95 0 1 1 0 1.9H6.05A.95.95 0 0 1 5.1 7.85V3.9a.95.95 0 0 1 1.9 0v1.62A7.96 7.96 0 0 1 12 4Zm.95 3.85v3.75l2.55 1.52a.95.95 0 1 1-.98 1.63l-3.02-1.8a.95.95 0 0 1-.45-.82V7.85a.95.95 0 1 1 1.9 0Z" />
    </svg>
  );
}

function LoadingState() {
  return (
    <div className="loading-state">
      <div className="loading-spinner" />
      <div>当前正在载入中，请稍等</div>
    </div>
  );
}

function formatShanghaiTime(value: string) {
  const date = new Date(value);
  if (!Number.isNaN(date.getTime())) {
    const parts = new Intl.DateTimeFormat('zh-CN', {
      timeZone: 'Asia/Shanghai',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    }).formatToParts(date);
    const get = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value ?? '';
    return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}:${get('second')}`;
  }
  return value.replace('T', ' ').replace('Z', '').replace(/\.\d+/, '');
}

function historyCopySummary(text: string) {
  const normalized = text.trim().replace(/\s+/g, ' ');
  const chars = Array.from(normalized);
  const preview = chars.slice(0, 10).join('');
  const suffix = chars.length > 10 ? '……' : '';
  return `${preview}${suffix}（${Array.from(text).length}）`;
}

function stringifyError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }
  if (typeof error === 'string') {
    return error;
  }
  return '发生未知错误。';
}

export default App;
