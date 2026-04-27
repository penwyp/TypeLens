import { FormEvent, UIEvent, useEffect, useMemo, useState } from 'react';
import {
  AddDictionaryTerm,
  CopyText,
  DeleteDictionaryWord,
  ExportDictionaryFile,
  ImportDictionaryFile,
  ListDictionaryWords,
  ListPendingImportedWords,
  ResetDictionary,
  SelectDictionaryExportFile,
  SelectTextFile,
} from '../../../wailsjs/go/main/App';
import { typeless } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { readDictionaryCache, writeDictionaryCache } from '../../cache';
import { Dialog, LogConsole } from '../../components/Dialog';
import { AutoImportPanel } from './AutoImportPanel';

type Notice = {
  kind: 'success' | 'error' | 'info';
  text: string;
};

type DialogKind = 'add' | 'edit' | 'import' | 'reset' | 'export' | null;
type ImportTab = 'file' | 'auto';
type WordMenu = {
  x: number;
  y: number;
  entry: DictionaryEntry;
} | null;

type DictionaryEntry =
  | { kind: 'remote'; key: string; word: typeless.DictionaryWord; term: string }
  | { kind: 'pending'; key: string; word: typeless.PendingDictionaryWord; term: string };

const WORD_PAGE_SIZE = 80;

export function DictionaryView({
  onCountChange,
  onNotice,
  onCopyNotice,
}: {
  onCountChange: (count: number) => void;
  onNotice: (notice: Notice) => void;
  onCopyNotice: (text: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [bootstrapping, setBootstrapping] = useState(true);
  const [dialog, setDialog] = useState<DialogKind>(null);
  const [importTab, setImportTab] = useState<ImportTab>('file');
  const [wordMenu, setWordMenu] = useState<WordMenu>(null);

  const [words, setWords] = useState<typeless.DictionaryWord[]>([]);
  const [pendingWords, setPendingWords] = useState<typeless.PendingDictionaryWord[]>([]);
  const [visibleWordCount, setVisibleWordCount] = useState(WORD_PAGE_SIZE);
  const [newTerm, setNewTerm] = useState('');
  const [editingWord, setEditingWord] = useState<typeless.DictionaryWord | null>(null);
  const [editingTerm, setEditingTerm] = useState('');
  const [importPath, setImportPath] = useState('');
  const [importConcurrency, setImportConcurrency] = useState(10);
  const [importSummary, setImportSummary] = useState<typeless.ImportResult | null>(null);
  const [operationLogs, setOperationLogs] = useState<string[]>([]);
  const [exportPath, setExportPath] = useState('');
  const [exportLogs, setExportLogs] = useState<string[]>([]);
  const [exportProgress, setExportProgress] = useState(0);
  const [exportStatusText, setExportStatusText] = useState('等待选择保存路径');
  const [exportResultPath, setExportResultPath] = useState('');

  const [resetPath, setResetPath] = useState('');
  const [resetConcurrency, setResetConcurrency] = useState(10);
  const [resetConfirmed, setResetConfirmed] = useState(false);
  const [resetSummary, setResetSummary] = useState<typeless.ResetResult | null>(null);
  const [autoImportLogs, setAutoImportLogs] = useState<string[]>([]);

  useEffect(() => {
    void bootstrap();
  }, []);

  useEffect(() => {
    return EventsOn('typelens:dictionary-log', (line: string) => {
      setOperationLogs((current) => [...current.slice(-199), line]);
    });
  }, []);

  useEffect(() => {
    return EventsOn('typelens:auto-import-log', (line: string) => {
      setAutoImportLogs((current) => [...current.slice(-199), line]);
    });
  }, []);

  useEffect(() => {
    return EventsOn('typelens:export-log', (line: string) => {
      setExportLogs((current) => [...current.slice(-99), line]);
      const progressMatch = line.match(/^(\d+)%\s+(.*)$/);
      if (!progressMatch) {
        setExportStatusText(line);
        return;
      }
      const percent = Number(progressMatch[1]);
      const text = progressMatch[2];
      if (Number.isFinite(percent)) {
        setExportProgress(percent);
      }
      setExportStatusText(text);
      const pathMatch = text.match(/：(.+)$/);
      if (percent >= 100 && pathMatch) {
        setExportResultPath(pathMatch[1].trim());
      }
    });
  }, []);

  useEffect(() => {
    return EventsOn('typelens:auto-import-finished', () => {
      void refreshAll();
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

  const entries = useMemo<DictionaryEntry[]>(() => {
    const remoteEntries: DictionaryEntry[] = words.map((word) => ({
      kind: 'remote',
      key: `remote-${word.user_dictionary_id}`,
      word,
      term: word.term,
    }));
    const pendingEntries: DictionaryEntry[] = pendingWords.map((word) => ({
      kind: 'pending',
      key: `pending-${normalizeTerm(word.term)}`,
      word,
      term: word.term,
    }));
    return [...pendingEntries, ...remoteEntries].sort((left, right) => left.term.localeCompare(right.term, 'zh-CN'));
  }, [pendingWords, words]);

  useEffect(() => {
    onCountChange(entries.length);
  }, [entries.length, onCountChange]);

  async function bootstrap() {
    try {
      await hydrateCache();
      await refreshAll();
    } finally {
      setBootstrapping(false);
    }
  }

  async function refreshAll(options: { resetVisibleCount?: boolean; silentError?: boolean } = {}) {
    try {
      const [nextWords, nextPending] = await Promise.all([
        ListDictionaryWords(),
        ListPendingImportedWords(),
      ]);
      applyDictionaryState(nextWords, nextPending);
      if (options.resetVisibleCount ?? true) {
        setVisibleWordCount(WORD_PAGE_SIZE);
      }
    } catch (error) {
      if (!options.silentError) {
        throw error;
      }
    }
  }

  function refreshAllInBackground() {
    void refreshAll({
      resetVisibleCount: false,
      silentError: true,
    });
  }

  async function refreshWords() {
    try {
      setBusy(true);
      await refreshAll();
    } catch (error) {
      onNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function addTerm(event: FormEvent) {
    event.preventDefault();
    const term = newTerm.trim();
    if (!term) {
      onNotice({ kind: 'error', text: '词条不能为空' });
      return;
    }
    if (entries.some((entry) => normalizeTerm(entry.term) === normalizeTerm(term))) {
      onNotice({ kind: 'info', text: '词条已存在' });
      return;
    }

    const optimisticWord = createOptimisticWord(term);
    const previousWords = words;
    try {
      setDialog(null);
      setNewTerm('');
      setWords((current) => [optimisticWord, ...current]);
      await AddDictionaryTerm(term);
      onNotice({ kind: 'success', text: '词条已新增。' });
      refreshAllInBackground();
    } catch (error) {
      setWords(previousWords);
      onNotice({ kind: 'error', text: stringifyError(error) });
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
      onNotice({ kind: 'error', text: '词条不能为空' });
      return;
    }
    if (term === oldTerm) {
      setDialog(null);
      return;
    }
    const oldKey = normalizeTerm(oldTerm);
    const nextKey = normalizeTerm(term);
    if (oldKey !== nextKey && entries.some((entry) => normalizeTerm(entry.term) === nextKey)) {
      onNotice({ kind: 'info', text: '词条已存在' });
      return;
    }
    const previousWords = words;
    try {
      setDialog(null);
      setWords((current) => current.map((word) => (
        word.user_dictionary_id === editingWord.user_dictionary_id ? { ...word, term } : word
      )));
      await DeleteDictionaryWord(editingWord.user_dictionary_id);
      await AddDictionaryTerm(term);
      setEditingWord(null);
      setEditingTerm('');
      onNotice({ kind: 'success', text: '词条已更新。' });
      refreshAllInBackground();
    } catch (error) {
      setWords(previousWords);
      onNotice({ kind: 'error', text: stringifyError(error) });
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
      await refreshAll();
      onNotice({ kind: 'success', text: '导入完成。' });
    } catch (error) {
      onNotice({ kind: 'error', text: stringifyError(error) });
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
      setResetConfirmed(false);
      await refreshAll();
      onNotice({ kind: 'success', text: '差量重置完成。' });
    } catch (error) {
      onNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function exportWords(event: FormEvent) {
    event.preventDefault();
    try {
      setBusy(true);
      const count = await ExportDictionaryFile(exportPath);
      setDialog(null);
      onNotice({ kind: 'success', text: `已导出 ${count} 个词。` });
    } catch (error) {
      onNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function deleteWord(entry: DictionaryEntry) {
    if (entry.kind !== 'remote') {
      onNotice({ kind: 'info', text: '待同步词条暂不支持删除。' });
      return;
    }
    const previousWords = words;
    try {
      setWords((current) => current.filter((item) => item.user_dictionary_id !== entry.word.user_dictionary_id));
      await DeleteDictionaryWord(entry.word.user_dictionary_id);
      setWordMenu(null);
      onNotice({ kind: 'success', text: '词条已删除。' });
      refreshAllInBackground();
    } catch (error) {
      setWords(previousWords);
      onNotice({ kind: 'error', text: stringifyError(error) });
    }
  }

  async function copyDictionaryTerm(term: string) {
    try {
      await CopyText(term);
      onCopyNotice(`复制成功 ${term}`);
    } catch (error) {
      onNotice({ kind: 'error', text: stringifyError(error) });
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
      onNotice({ kind: 'error', text: stringifyError(error) });
    }
  }

  async function selectExportPath() {
    try {
      const selected = await SelectDictionaryExportFile();
      if (!selected) {
        return;
      }
      setExportPath(selected);
      setExportLogs([]);
      setExportProgress(0);
      setExportResultPath('');
      setExportStatusText('已确认保存路径，开始导出。');
      setBusy(true);
      const count = await ExportDictionaryFile(selected);
      setExportProgress(100);
      setDialog('export');
      onNotice({ kind: 'success', text: `已导出 ${count} 个词。` });
    } catch (error) {
      onNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  function handleDictionaryScroll(event: UIEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    if (visibleWordCount >= entries.length || element.scrollTop + element.clientHeight < element.scrollHeight - 80) {
      return;
    }
    setVisibleWordCount((count) => Math.min(entries.length, count + WORD_PAGE_SIZE));
  }

  const importFileLabel = summarizePath(importPath, '选择文件');
  const resetFileLabel = summarizePath(resetPath, '使用内置词表');
  const exportFileLabel = summarizePath(exportPath, '选择保存路径');
  const visibleEntries = entries.slice(0, visibleWordCount);

  return (
    <section className="view">
      <div className="toolbar">
        <div />
        <div className="button-row">
          <button className="ghost-button" onClick={() => setDialog('reset')}>重置</button>
          <button className="ghost-button" onClick={() => {
            setExportLogs([]);
            setExportProgress(0);
            setExportResultPath('');
            setExportStatusText('等待选择保存路径');
            setDialog('export');
          }}>导出</button>
          <button className="ghost-button" onClick={() => {
            setImportTab('file');
            setDialog('import');
          }}>导入</button>
          <button className="primary-button" onClick={() => setDialog('add')}>新增</button>
        </div>
      </div>

      <div className="list word-grid" onScroll={handleDictionaryScroll}>
        {bootstrapping && entries.length === 0 ? <LoadingState /> : null}
        {visibleEntries.map((entry) => (
          <div
            className={`word-chip ${entry.kind === 'pending' ? 'pending-chip' : ''}`}
            key={entry.key}
            role="button"
            tabIndex={0}
            onClick={() => void copyDictionaryTerm(entry.term)}
            onContextMenu={(event) => {
              event.preventDefault();
              setWordMenu({ x: event.clientX, y: event.clientY, entry });
            }}
          >
            <div className="word-primary">
              <span className="word-text">{entry.term}</span>
              {entry.kind === 'pending' ? <span className={`status-badge status-${entry.word.status}`}>{pendingStatusLabel(entry.word.status)}</span> : null}
            </div>
            <span className="word-actions">
              {entry.kind === 'remote' ? (
                <>
                  <button
                    className="word-action"
                    type="button"
                    aria-label="编辑"
                    onClick={(event) => {
                      event.stopPropagation();
                      setEditingWord(entry.word);
                      setEditingTerm(entry.word.term);
                      setDialog('edit');
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
                      void deleteWord(entry);
                    }}
                  >
                    <TrashIcon />
                  </button>
                </>
              ) : null}
            </span>
          </div>
        ))}
        {!bootstrapping && entries.length === 0 ? <div className="empty-state">暂无词条</div> : null}
      </div>

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
        <Dialog title="导入文件" className="dialog-wide dialog-import" onClose={() => setDialog(null)}>
          <div className="dialog-tabs">
            <button className={importTab === 'file' ? 'dialog-tab active' : 'dialog-tab'} type="button" onClick={() => setImportTab('file')}>导入文件</button>
            <button className={importTab === 'auto' ? 'dialog-tab active' : 'dialog-tab'} type="button" onClick={() => setImportTab('auto')}>自动导入</button>
          </div>
          <div className={importTab === 'file' ? 'tab-panel active' : 'tab-panel hidden'} aria-hidden={importTab !== 'file'}>
            <form className="dialog-form" onSubmit={importWords}>
              <button className="file-button" disabled={busy} onClick={() => void selectPath('import')} type="button">{importFileLabel}</button>
              <div className="field-hint">文件格式：每行一个词。</div>
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
          </div>
          <div className={importTab === 'auto' ? 'tab-panel active' : 'tab-panel hidden'} aria-hidden={importTab !== 'auto'}>
            <AutoImportPanel
              busy={busy}
              logs={autoImportLogs}
              onError={(text) => onNotice({ kind: 'error', text })}
              onScanStart={() => setAutoImportLogs([])}
              onSuccess={(result) => {
                setPendingWords(result.words);
                void writeDictionaryCache(words, result.words);
                setDialog(null);
                onNotice({ kind: 'success', text: `已成功导入 ${result.accepted_count} 个词，后台同步中。` });
              }}
            />
          </div>
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

      {dialog === 'export' ? (
        <Dialog title="导出词典" onClose={() => setDialog(null)}>
          <form className="dialog-form" onSubmit={exportWords}>
            <button className="file-button" disabled={busy} onClick={() => void selectExportPath()} type="button">{exportFileLabel}</button>
            <div className="field-hint">默认路径在 Downloads，文件名格式为 TypeLens-YYYYMMDD-HHMMSS.txt。</div>
            <div className="field-hint">导出内容为当前所有词典词条，每行一个。</div>
            <div className="summary-box">
              <strong>{exportProgress}%</strong>
              <div>{exportStatusText}</div>
              {exportResultPath ? <div className="field-hint export-path">{exportResultPath}</div> : null}
            </div>
            <LogConsole logs={exportLogs} busy={busy} idleText="确认保存路径后自动开始导出" />
            <div className="dialog-actions">
              <button className="ghost-button" type="button" onClick={() => setDialog(null)}>{busy ? '后台运行中' : '关闭'}</button>
            </div>
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
    </section>
  );

  function applyDictionaryState(nextWords: typeless.DictionaryWord[], nextPending: typeless.PendingDictionaryWord[]) {
    setWords(nextWords);
    setPendingWords(nextPending);
    void writeDictionaryCache(nextWords, nextPending);
  }

  async function hydrateCache() {
    try {
      const cache = await readDictionaryCache();
      setWords(cache.words ?? []);
      setPendingWords(cache.pendingWords ?? []);
    } catch {
      setWords([]);
      setPendingWords([]);
    }
  }
}

function LoadingState() {
  return (
    <div className="loading-state">
      <div className="loading-spinner" />
      <div>当前正在载入中，请稍等</div>
    </div>
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

function pendingStatusLabel(status: string) {
  switch (status) {
    case 'pending':
      return '待同步';
    case 'syncing':
      return '同步中';
    case 'failed':
      return '失败';
    default:
      return status;
  }
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
