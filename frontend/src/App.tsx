import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import {
  AddDictionaryTerm,
  CopyText,
  DeleteDictionaryWord,
  ImportDictionaryFile,
  ListDictionaryWords,
  QueryHistory,
  ResetDictionary,
  SelectTextFile,
} from '../wailsjs/go/main/App';
import { service, typeless } from '../wailsjs/go/models';
import { BrowserOpenURL, EventsOn, WindowSetBackgroundColour } from '../wailsjs/runtime/runtime';
import './App.css';

type Notice = {
  kind: 'success' | 'error' | 'info';
  text: string;
};

type DialogKind = 'add' | 'edit' | 'import' | 'reset' | null;
type HistorySort = 'desc' | 'asc';
type WordMenu = {
  x: number;
  y: number;
  word: typeless.DictionaryWord;
} | null;

const WORD_PAGE_SIZE = 80;
const HISTORY_PAGE_SIZE = 30;
const HISTORY_FETCH_MULTIPLIER = 8;

function App() {
  const [activeView, setActiveView] = useState<'dictionary' | 'history'>('dictionary');
  const [busy, setBusy] = useState(false);
  const [bootstrapping, setBootstrapping] = useState(true);
  const [notice, setNotice] = useState<Notice | null>(null);
  const [copyNotice, setCopyNotice] = useState<string | null>(null);
  const [dialog, setDialog] = useState<DialogKind>(null);
  const [wordMenu, setWordMenu] = useState<WordMenu>(null);

  const [words, setWords] = useState<typeless.DictionaryWord[]>([]);
  const [visibleWordCount, setVisibleWordCount] = useState(WORD_PAGE_SIZE);
  const [newTerm, setNewTerm] = useState('');
  const [editingWord, setEditingWord] = useState<typeless.DictionaryWord | null>(null);
  const [editingTerm, setEditingTerm] = useState('');
  const [importPath, setImportPath] = useState('');
  const [importConcurrency, setImportConcurrency] = useState(10);
  const [importSummary, setImportSummary] = useState<typeless.ImportResult | null>(null);
  const [operationLogs, setOperationLogs] = useState<string[]>([]);

  const [resetPath, setResetPath] = useState('');
  const [resetConcurrency, setResetConcurrency] = useState(10);
  const [resetConfirmed, setResetConfirmed] = useState(false);
  const [resetSummary, setResetSummary] = useState<typeless.ResetResult | null>(null);

  const [historyQuery, setHistoryQuery] = useState<service.HistoryQuery>({
    limit: HISTORY_PAGE_SIZE,
    keyword: '',
    regex: '',
    contextMode: 'all',
  });
  const [historySort, setHistorySort] = useState<HistorySort>('desc');
  const [records, setRecords] = useState<typeless.TranscriptRecord[]>([]);
  const historySearchRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    WindowSetBackgroundColour(0, 0, 0, 0);
    void bootstrap();
  }, []);

  useEffect(() => {
    return EventsOn('typelens:dictionary-log', (line: string) => {
      setOperationLogs((current) => [...current.slice(-199), line]);
    });
  }, []);

  useEffect(() => {
    const closeMenu = () => setWordMenu(null);
    window.addEventListener('click', closeMenu);
    window.addEventListener('blur', closeMenu);
    return () => {
      window.removeEventListener('click', closeMenu);
      window.removeEventListener('blur', closeMenu);
    };
  }, []);

  useEffect(() => {
    if (activeView !== 'history') {
      return;
    }
    const timer = window.setTimeout(() => {
      void loadHistory();
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

  useEffect(() => {
    if (!dialog) {
      return;
    }

    function handleDialogEscape(event: KeyboardEvent) {
      if (event.key !== 'Escape') {
        return;
      }
      setDialog(null);
    }

    window.addEventListener('keydown', handleDialogEscape);
    return () => window.removeEventListener('keydown', handleDialogEscape);
  }, [dialog]);

  async function bootstrap() {
    try {
      setBusy(true);
      setBootstrapping(true);
      const nextWords = await loadWords();
      setWords(nextWords);
      await loadHistory();
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
      setBootstrapping(false);
    }
  }

  async function refreshWords() {
    try {
      setBusy(true);
      const nextWords = await loadWords();
      setWords(nextWords);
      setVisibleWordCount(WORD_PAGE_SIZE);
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function addTerm(event: FormEvent) {
    event.preventDefault();
    const term = newTerm.trim();
    if (!term) {
      setNotice({ kind: 'error', text: '词条不能为空' });
      return;
    }
    if (words.some((word) => normalizeTerm(word.term) === normalizeTerm(term))) {
      setNotice({ kind: 'info', text: '词条已存在' });
      return;
    }
    const optimisticWord = createOptimisticWord(term);
    try {
      setDialog(null);
      setNewTerm('');
      setNotice({ kind: 'info', text: '正在新增词条。' });
      setImportSummary(null);
      setResetSummary(null);
      setWords((current) => [optimisticWord, ...current]);
      setVisibleWordCount((count) => Math.max(WORD_PAGE_SIZE, count + 1));
      await AddDictionaryTerm(term);
      const nextWords = await loadWords();
      setWords(nextWords);
      setVisibleWordCount(WORD_PAGE_SIZE);
      setNotice({ kind: 'success', text: '词条已新增。' });
    } catch (error) {
      setWords((current) => current.filter((word) => word.user_dictionary_id !== optimisticWord.user_dictionary_id));
      setNotice({ kind: 'error', text: stringifyError(error) });
    }
  }

  async function saveEditedTerm(event: FormEvent) {
    event.preventDefault();
    if (!editingWord) {
      return;
    }
    const term = editingTerm.trim();
    const oldTerm = editingWord.term.trim();
    if (!term) {
      setNotice({ kind: 'error', text: '词条不能为空' });
      return;
    }
    if (term === oldTerm) {
      setDialog(null);
      return;
    }
    const oldKey = normalizeTerm(oldTerm);
    const nextKey = normalizeTerm(term);
    if (oldKey !== nextKey && words.some((word) => normalizeTerm(word.term) === nextKey)) {
      setNotice({ kind: 'info', text: '词条已存在' });
      return;
    }
    const previousWords = words;
    try {
      setDialog(null);
      setEditingWord(null);
      setEditingTerm('');
      setNotice({ kind: 'info', text: '正在保存词条。' });
      setImportSummary(null);
      setResetSummary(null);
      setWords((current) => current.map((word) => (
        word.user_dictionary_id === editingWord.user_dictionary_id ? { ...word, term } : word
      )));
      if (oldKey === nextKey) {
        await DeleteDictionaryWord(editingWord.user_dictionary_id);
        await AddDictionaryTerm(term);
      } else {
        await AddDictionaryTerm(term);
        await DeleteDictionaryWord(editingWord.user_dictionary_id);
      }
      const nextWords = await loadWords();
      setWords(nextWords);
      setNotice({ kind: 'success', text: '词条已保存。' });
    } catch (error) {
      setWords(previousWords);
      setNotice({ kind: 'error', text: stringifyError(error) });
    }
  }

  async function importWords(event: FormEvent) {
    event.preventDefault();
    try {
      setBusy(true);
      setOperationLogs([]);
      setResetSummary(null);
      const concurrency = clampConcurrency(importConcurrency);
      setImportConcurrency(concurrency);
      const summary = await ImportDictionaryFile(importPath, concurrency, false);
      setImportSummary(summary);
      const nextWords = await loadWords();
      setWords(nextWords);
      setVisibleWordCount(WORD_PAGE_SIZE);
      setNotice({ kind: 'success', text: '导入完成。' });
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function resetWords(event: FormEvent) {
    event.preventDefault();
    try {
      setBusy(true);
      setOperationLogs([]);
      setImportSummary(null);
      const concurrency = clampConcurrency(resetConcurrency);
      setResetConcurrency(concurrency);
      const summary = await ResetDictionary(resetPath, concurrency);
      setResetSummary(summary);
      const nextWords = await loadWords();
      setWords(nextWords);
      setVisibleWordCount(WORD_PAGE_SIZE);
      setResetConfirmed(false);
      setNotice({ kind: 'success', text: '差量重置完成。' });
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function deleteWord(id: string) {
    const previousWords = words;
    const target = words.find((item) => item.user_dictionary_id === id);
    if (!target) {
      return;
    }
    try {
      setImportSummary(null);
      setResetSummary(null);
      setWords((current) => current.filter((item) => item.user_dictionary_id !== id));
      await DeleteDictionaryWord(id);
      setWordMenu(null);
      setNotice({ kind: 'success', text: '词条已删除。' });
    } catch (error) {
      setWords(previousWords);
      setNotice({ kind: 'error', text: stringifyError(error) });
    }
  }

  async function loadHistory(showNotice = false) {
    try {
      const nextRecords = await QueryHistory({
        ...historyQuery,
        limit: historyQuery.limit * HISTORY_FETCH_MULTIPLIER,
      });
      setRecords(nextRecords);
      if (showNotice) {
        setNotice({ kind: 'success', text: '历史记录已刷新。' });
      }
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    }
  }

  async function copyDictionaryTerm(term: string) {
    try {
      await CopyText(term);
      setCopyNotice(`复制成功 ${term}`);
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
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

  async function selectPath(kind: 'import' | 'reset') {
    try {
      const selected = await SelectTextFile();
      if (!selected) {
        return;
      }
      if (kind === 'import') {
        setImportPath(selected);
        return;
      }
      setResetPath(selected);
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    }
  }

  async function loadWords() {
    return await ListDictionaryWords();
  }

  function openEditDialog(word: typeless.DictionaryWord) {
    setEditingWord(word);
    setEditingTerm(word.term);
    setDialog('edit');
  }

  function handleDictionaryScroll(event: React.UIEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    if (visibleWordCount >= words.length || element.scrollTop + element.clientHeight < element.scrollHeight - 80) {
      return;
    }
    setVisibleWordCount((count) => Math.min(words.length, count + WORD_PAGE_SIZE));
  }

  function handleHistoryScroll(event: React.UIEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    if (visibleHistoryRecords.length < historyQuery.limit || element.scrollTop + element.clientHeight < element.scrollHeight - 120) {
      return;
    }
    setHistoryQuery((query) => ({ ...query, limit: query.limit + HISTORY_PAGE_SIZE }));
  }

  const importFileLabel = summarizePath(importPath, '选择文件');
  const resetFileLabel = summarizePath(resetPath, '使用内置词表');
  const visibleWords = words.slice(0, visibleWordCount);
  const visibleHistoryRecords = useMemo(() => {
    return [...records].sort((left, right) => {
      const leftTime = Date.parse(left.CreatedAt || '');
      const rightTime = Date.parse(right.CreatedAt || '');
      const fallback = (left.CreatedAt || '').localeCompare(right.CreatedAt || '');
      const diff = Number.isNaN(leftTime) || Number.isNaN(rightTime) ? fallback : leftTime - rightTime;
      return historySort === 'asc' ? diff : -diff;
    }).slice(0, historyQuery.limit);
  }, [records, historyQuery.limit, historySort]);

  return (
    <div id="app-shell">
      <aside className="sidebar">
        <div>
          <button className={activeView === 'dictionary' ? 'side-item active' : 'side-item'} onClick={() => setActiveView('dictionary')}>
            <span className="side-label">
              <BookIcon />
              词典
            </span>
            <strong>{words.length}</strong>
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
          <section className="view">
            <div className="toolbar">
              <div />
              <div className="button-row">
                <button className="ghost-button" onClick={() => setDialog('reset')}>重置</button>
                <button className="ghost-button" onClick={() => setDialog('import')}>导入</button>
                <button className="primary-button" onClick={() => setDialog('add')}>新增</button>
              </div>
            </div>

            <div className="list word-grid" onScroll={handleDictionaryScroll}>
              {bootstrapping && words.length === 0 ? <LoadingState /> : null}
              {visibleWords.map((word) => (
                <div
                  className="word-chip"
                  key={word.user_dictionary_id}
                  role="button"
                  tabIndex={0}
                  onClick={() => void copyDictionaryTerm(word.term)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      void copyDictionaryTerm(word.term);
                    }
                  }}
                  onContextMenu={(event) => {
                    event.preventDefault();
                    setWordMenu({ x: event.clientX, y: event.clientY, word });
                  }}
                >
                  <span className="word-text">{word.term}</span>
                  <span className="word-actions">
                    <button
                      className="word-action"
                      type="button"
                      aria-label="编辑"
                      onClick={(event) => {
                        event.stopPropagation();
                        openEditDialog(word);
                      }}
                    >
                      <EditIcon />
                    </button>
                    <button
                      className="word-action danger"
                      type="button"
                      aria-label="删除"
                      disabled={busy}
                      onClick={(event) => {
                        event.stopPropagation();
                        void deleteWord(word.user_dictionary_id);
                      }}
                    >
                      <TrashIcon />
                    </button>
                  </span>
                </div>
              ))}
              {!bootstrapping && words.length === 0 ? <div className="empty-state">暂无词条</div> : null}
            </div>
          </section>
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
                <button className="ghost-button" onClick={() => void loadHistory(true)}>刷新</button>
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
              {visibleHistoryRecords.map((record) => (
                <article className="history-card" key={record.ID} onClick={() => void copyHistoryRecord(record)}>
                  <time className="history-time">{formatShanghaiTime(record.CreatedAt)}</time>
                  <pre className="history-text">{record.Text}</pre>
                </article>
              ))}
              {!bootstrapping && visibleHistoryRecords.length === 0 ? <div className="empty-state">暂无历史记录</div> : null}
            </div>
          </section>
        )}
      </main>

      {dialog === 'add' ? (
        <Dialog title="新增词条" onClose={() => setDialog(null)}>
          <form className="dialog-form" onSubmit={addTerm}>
            <input autoFocus value={newTerm} onChange={(event) => setNewTerm(event.target.value)} placeholder="词条" />
            <div className="dialog-actions">
              <button className="ghost-button" type="button" onClick={() => setDialog(null)}>取消</button>
              <button className="primary-button" type="submit">新增</button>
            </div>
          </form>
        </Dialog>
      ) : null}

      {dialog === 'edit' && editingWord ? (
        <Dialog title="编辑词汇" onClose={() => setDialog(null)}>
          <form className="dialog-form" onSubmit={saveEditedTerm}>
            <label>
              <span>编辑</span>
              <input autoFocus value={editingTerm} onChange={(event) => setEditingTerm(event.target.value)} />
            </label>
            <div className="dialog-actions">
              <button className="ghost-button" type="button" onClick={() => setDialog(null)}>取消</button>
              <button className="primary-button" type="submit">保存</button>
            </div>
          </form>
        </Dialog>
      ) : null}

      {dialog === 'import' ? (
        <Dialog title="导入文件" onClose={() => setDialog(null)}>
          <form className="dialog-form" onSubmit={importWords}>
            <button className="file-button" disabled={busy} onClick={() => void selectPath('import')} type="button">{importFileLabel}</button>
            <label>
              <span>并发</span>
              <input
                type="number"
                inputMode="numeric"
                min={1}
                max={10}
                value={importConcurrency}
                onChange={(event) => setImportConcurrency(clampConcurrency(event.target.value))}
              />
            </label>
            <div className="dialog-actions">
              <button className="ghost-button" type="button" onClick={() => setDialog(null)}>取消</button>
              <button className="primary-button" disabled={busy || !importPath.trim()} type="submit">导入</button>
            </div>
            {importSummary ? <div className="summary-box">输入 {importSummary.TotalInput}，去重后 {importSummary.Unique}，跳过 {importSummary.Skipped}，导入 {importSummary.Imported}</div> : null}
            <LogConsole logs={operationLogs} busy={busy} />
          </form>
        </Dialog>
      ) : null}

      {dialog === 'reset' ? (
        <Dialog title="重置词典" onClose={() => setDialog(null)}>
          <form className="dialog-form" onSubmit={resetWords}>
            <button className="file-button" disabled={busy} onClick={() => void selectPath('reset')} type="button">{resetFileLabel}</button>
            <label>
              <span>并发</span>
              <input
                type="number"
                inputMode="numeric"
                min={1}
                max={10}
                value={resetConcurrency}
                onChange={(event) => setResetConcurrency(clampConcurrency(event.target.value))}
              />
            </label>
            <label className="check-row">
              <input checked={resetConfirmed} onChange={(event) => setResetConfirmed(event.target.checked)} type="checkbox" />
              <span>确认重置</span>
            </label>
            <div className="dialog-actions">
              <button className="ghost-button" type="button" onClick={() => setDialog(null)}>取消</button>
              <button className="danger-button" disabled={busy || !resetConfirmed} type="submit">重置</button>
            </div>
            {resetSummary ? <div className="summary-box">目标 {resetSummary.Unique}，保留 {resetSummary.Kept}，删除 {resetSummary.Deleted}，新增 {resetSummary.Imported}</div> : null}
            <LogConsole logs={operationLogs} busy={busy} />
          </form>
        </Dialog>
      ) : null}

      {wordMenu ? (
        <div className="context-menu" style={{ left: wordMenu.x, top: wordMenu.y }} onClick={(event) => event.stopPropagation()}>
          <button
            className="context-action"
            onClick={() => {
              setWordMenu(null);
              void refreshWords();
            }}
          >
            刷新
          </button>
        </div>
      ) : null}

      {notice ? <div className={`toast toast-${notice.kind}`}>{notice.text}</div> : null}
      {copyNotice ? <div className="copy-toast">{copyNotice}</div> : null}
    </div>
  );
}

function Dialog({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <div className="dialog-backdrop" role="presentation" onClick={onClose}>
      <section className="dialog" role="dialog" aria-modal="true" aria-label={title} onClick={(event) => event.stopPropagation()}>
        <div className="dialog-header">
          <h2>{title}</h2>
          <button className="icon-button" onClick={onClose} type="button" aria-label="关闭">×</button>
        </div>
        {children}
      </section>
    </div>
  );
}

function LogConsole({ logs, busy }: { logs: string[]; busy: boolean }) {
  return (
    <div className="log-console">
      {busy ? <div className="log-line active">运行中...</div> : null}
      {logs.length > 0 ? logs.map((line, index) => (
        <div className="log-line" key={`${index}-${line}`}>{line}</div>
      )) : <div className="log-line muted">暂无日志</div>}
    </div>
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

function EditIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24">
      <path fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8" d="m5 16.7-.35 2.65L7.3 19l9.85-9.85a2.05 2.05 0 0 0-2.9-2.9L4.4 16.1l.6.6Zm7.95-8.95 3.3 3.3M4.65 19.35 8 16" />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24">
      <path fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.75" d="M6.5 7.25h11M10 7.25V5.8c0-.72.58-1.3 1.3-1.3h1.4c.72 0 1.3.58 1.3 1.3v1.45m2.25 0-.6 10.15a2.25 2.25 0 0 1-2.24 2.1h-3.42a2.25 2.25 0 0 1-2.24-2.1l-.6-10.15M10.5 10.75v5.5m3-5.5v5.5" />
    </svg>
  );
}

function summarizePath(path: string, fallback: string) {
  const value = path.trim();
  if (!value) {
    return fallback;
  }
  const parts = value.split(/[\\/]/).filter(Boolean);
  return parts.at(-1) ?? value;
}

function clampConcurrency(value: number | string) {
  const parsed = typeof value === 'number' ? value : Number(value.replace(/[^\d]/g, ''));
  if (!Number.isFinite(parsed)) {
    return 1;
  }
  return Math.min(10, Math.max(1, Math.trunc(parsed)));
}

function normalizeTerm(value: string) {
  return value.trim().toLowerCase();
}

function createOptimisticWord(term: string): typeless.DictionaryWord {
  return {
    user_dictionary_id: `optimistic-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    term,
    lang: '',
    category: '',
    created_at: '',
    updated_at: '',
    auto: false,
    replace: false,
    replace_targets: [],
  };
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
