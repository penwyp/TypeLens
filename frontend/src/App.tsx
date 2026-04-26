import { FormEvent, useEffect, useState } from 'react';
import {
  AddDictionaryTerm,
  ClearDictionary,
  CopyText,
  DeleteDictionaryWord,
  GetConfig,
  ImportDictionaryFile,
  ListDictionaryWords,
  QueryHistory,
  ResetDictionary,
  SelectTextFile,
} from '../wailsjs/go/main/App';
import { service, typeless } from '../wailsjs/go/models';
import { EventsOn } from '../wailsjs/runtime/runtime';
import './App.css';

type Notice = {
  kind: 'success' | 'error' | 'info';
  text: string;
};

function App() {
  const [activeView, setActiveView] = useState<'dictionary' | 'history'>('dictionary');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<Notice | null>(null);

  const [config, setConfig] = useState<service.Config>({
    userDataPath: '',
    dbPath: '',
    apiHost: '',
    timeoutSec: 15,
  });

  const [words, setWords] = useState<typeless.DictionaryWord[]>([]);
  const [newTerm, setNewTerm] = useState('');
  const [importPath, setImportPath] = useState('');
  const [importConcurrency, setImportConcurrency] = useState(10);
  const [importDryRun, setImportDryRun] = useState(false);
  const [importSummary, setImportSummary] = useState<typeless.ImportResult | null>(null);
  const [dictionaryLogs, setDictionaryLogs] = useState<string[]>([]);

  const [resetPath, setResetPath] = useState('');
  const [resetConcurrency, setResetConcurrency] = useState(10);
  const [resetSummary, setResetSummary] = useState<typeless.ResetResult | null>(null);

  const [historyQuery, setHistoryQuery] = useState<service.HistoryQuery>({
    limit: 20,
    keyword: '',
    regex: '',
    contextMode: 'frontmost',
  });
  const [records, setRecords] = useState<typeless.TranscriptRecord[]>([]);

  useEffect(() => {
    void bootstrap();
  }, []);

  useEffect(() => {
    return EventsOn('typelens:dictionary-log', (line: string) => {
      setDictionaryLogs((current) => [...current.slice(-199), line]);
    });
  }, []);

  async function bootstrap() {
    try {
      setBusy(true);
      const nextConfig = await GetConfig();
      setConfig(nextConfig);
      const nextWords = await loadWords();
      setWords(nextWords);
      setNotice({ kind: 'info', text: `已连接 Typeless，当前词典 ${nextWords.length} 条。` });
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function refreshWords() {
    try {
      setBusy(true);
      const nextWords = await loadWords();
      setWords(nextWords);
      setNotice({ kind: 'info', text: `词典已刷新，共 ${nextWords.length} 条。` });
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function addTerm(event: FormEvent) {
    event.preventDefault();
    try {
      setBusy(true);
      setImportSummary(null);
      setResetSummary(null);
      await AddDictionaryTerm(newTerm);
      setNewTerm('');
      const nextWords = await loadWords();
      setWords(nextWords);
      setNotice({ kind: 'success', text: '词条已新增。' });
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function importWords(event: FormEvent) {
    event.preventDefault();
    try {
      setBusy(true);
      setDictionaryLogs([]);
      setResetSummary(null);
      const summary = await ImportDictionaryFile(importPath, importConcurrency, importDryRun);
      setImportSummary(summary);
      const nextWords = await loadWords();
      setWords(nextWords);
      setNotice({ kind: 'success', text: importDryRun ? '导入预览完成。' : '词典导入完成。' });
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
      setDictionaryLogs([]);
      setImportSummary(null);
      const summary = await ResetDictionary(resetPath, resetConcurrency);
      setResetSummary(summary);
      const nextWords = await loadWords();
      setWords(nextWords);
      setNotice({ kind: 'success', text: '差量重置完成。' });
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function clearWords() {
    if (!window.confirm('确认清空全部词典词条吗？')) {
      return;
    }
    try {
      setBusy(true);
      setDictionaryLogs([]);
      setImportSummary(null);
      setResetSummary(null);
      const deleted = await ClearDictionary();
      const nextWords = await loadWords();
      setWords(nextWords);
      setNotice({ kind: 'success', text: `已删除 ${deleted} 个词条。` });
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function deleteWord(id: string) {
    try {
      setBusy(true);
      setImportSummary(null);
      setResetSummary(null);
      await DeleteDictionaryWord(id);
      setWords((current) => current.filter((item) => item.user_dictionary_id !== id));
      setNotice({ kind: 'success', text: '词条已删除。' });
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function queryHistory(event: FormEvent) {
    event.preventDefault();
    try {
      setBusy(true);
      const nextRecords = await QueryHistory(historyQuery);
      setRecords(nextRecords);
      setNotice({ kind: 'info', text: `历史查询完成，共 ${nextRecords.length} 条。` });
    } catch (error) {
      setNotice({ kind: 'error', text: stringifyError(error) });
    } finally {
      setBusy(false);
    }
  }

  async function copyText(text: string) {
    try {
      await CopyText(text);
      setNotice({ kind: 'success', text: '已复制到剪贴板。' });
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

  const statusText = busy ? '正在处理请求…' : '本机数据已接入。';
  const importFileLabel = summarizePath(importPath, '选择文件');
  const resetFileLabel = summarizePath(resetPath, '使用内置词表');
  const readyState = config.userDataPath && config.dbPath ? '自动连接已就绪' : '等待本机环境准备完成';
  const historyContextLabel = historyQuery.contextMode === 'all'
    ? '全部应用'
    : historyQuery.contextMode === 'latest'
      ? '最近一次来源'
      : '当前前台应用';

  return (
    <div id="app-shell">
      <aside className="sidebar">
        <div className="brand-block">
          <div className="brand-kicker">TypeLens</div>
          <h1>Typeless 工作台</h1>
          <p>{readyState}</p>
        </div>

        <section className="config-panel hero-panel">
          <div className="section-title">概览</div>
          <div className="metric-grid">
            <article className="metric-card">
              <strong>{words.length}</strong>
              <span>当前词条</span>
            </article>
            <article className="metric-card">
              <strong>{records.length}</strong>
              <span>已载入历史</span>
            </article>
            <article className="metric-card">
              <strong>{dictionaryLogs.length}</strong>
              <span>运行日志</span>
            </article>
          </div>
        </section>

        {notice ? <div className={`notice notice-${notice.kind}`}>{notice.text}</div> : null}
      </aside>

      <main className="workspace">
        <header className="topbar">
          <div className="topbar-copy">
            <div className="eyebrow">Desktop Console</div>
            <div className="topbar-title">所见即所得</div>
            <div className="status-line">{statusText}</div>
          </div>

          <div className="topbar-actions">
            <div className="summary-pill">
              <span className="summary-label">当前模式</span>
              <strong>{activeView === 'dictionary' ? '词典管理' : `历史检索 · ${historyContextLabel}`}</strong>
            </div>
            <div className="tabs">
              <button className={activeView === 'dictionary' ? 'tab active' : 'tab'} onClick={() => setActiveView('dictionary')}>
                词典
              </button>
              <button className={activeView === 'history' ? 'tab active' : 'tab'} onClick={() => setActiveView('history')}>
                历史
              </button>
            </div>
          </div>
        </header>

        {activeView === 'dictionary' ? (
          <section className="content-grid">
            <div className="panel panel-wide">
              <div className="panel-header">
                <div>
                  <h2>词典总览</h2>
                  <p>共 {words.length} 条</p>
                </div>
                <div className="button-row">
                  <button className="ghost-button" disabled={busy} onClick={() => void refreshWords()}>
                    刷新
                  </button>
                  <button className="danger-button" disabled={busy} onClick={() => void clearWords()}>
                    清空
                  </button>
                </div>
              </div>

              <div className="word-table">
                {words.map((word) => (
                  <div className="word-row" key={word.user_dictionary_id}>
                    <div>
                      <div className="word-term">{word.term}</div>
                      <div className="word-meta">{word.category || 'custom'} · {word.updated_at || word.created_at || 'unknown time'}</div>
                    </div>
                    <button className="danger-link" disabled={busy} onClick={() => void deleteWord(word.user_dictionary_id)}>
                      删除
                    </button>
                  </div>
                ))}
                {words.length === 0 ? <div className="empty-state">暂无词条。</div> : null}
              </div>
            </div>

            <div className="panel-stack">
              <form className="panel" onSubmit={addTerm}>
                <div className="panel-header narrow">
                  <div><h2>新增词条</h2></div>
                </div>
                <div className="form-row">
                  <input value={newTerm} onChange={(event) => setNewTerm(event.target.value)} placeholder="例如: Claude Code" />
                  <button className="primary-button" disabled={busy} type="submit">新增</button>
                </div>
              </form>

              <form className="panel" onSubmit={importWords}>
                <div className="panel-header narrow">
                  <div><h2>导入文件</h2></div>
                </div>
                <div className="file-card">
                  <div>
                    <div className="file-card-label">待导入文件</div>
                    <div className="file-card-value">{importFileLabel}</div>
                  </div>
                  <button className="ghost-button" disabled={busy} onClick={() => void selectPath('import')} type="button">选择文件</button>
                </div>
                <div className="form-grid two">
                  <label>
                    <span>并发</span>
                    <input type="number" min={1} value={importConcurrency} onChange={(event) => setImportConcurrency(Number(event.target.value) || 10)} />
                  </label>
                  <label className="check-row">
                    <input checked={importDryRun} onChange={(event) => setImportDryRun(event.target.checked)} type="checkbox" />
                    <span>仅预览</span>
                  </label>
                </div>
                <button className="primary-button" disabled={busy} type="submit">开始导入</button>
                {importSummary ? (
                  <div className="summary-box">
                    输入 {importSummary.TotalInput}，去重后 {importSummary.Unique}，跳过 {importSummary.Skipped}，导入 {importSummary.Imported}
                  </div>
                ) : null}
              </form>

              <form className="panel" onSubmit={resetWords}>
                <div className="panel-header narrow">
                  <div><h2>差量重置</h2></div>
                </div>
                <div className="file-card">
                  <div>
                    <div className="file-card-label">目标词表</div>
                    <div className="file-card-value">{resetFileLabel}</div>
                  </div>
                  <button className="ghost-button" disabled={busy} onClick={() => void selectPath('reset')} type="button">选择文件</button>
                </div>
                <label>
                  <span>并发</span>
                  <input type="number" min={1} value={resetConcurrency} onChange={(event) => setResetConcurrency(Number(event.target.value) || 10)} />
                </label>
                <button className="primary-button" disabled={busy} type="submit">执行重置</button>
                {resetSummary ? (
                  <div className="summary-box">
                    目标 {resetSummary.Unique}，保留 {resetSummary.Kept}，删除 {resetSummary.Deleted}，新增 {resetSummary.Imported}
                  </div>
                ) : null}
              </form>

              <div className="panel">
                <div className="panel-header narrow">
                  <div><h2>运行日志</h2></div>
                </div>
                <div className="log-console">
                  {dictionaryLogs.length > 0 ? dictionaryLogs.map((line, index) => (
                    <div className="log-line" key={`${index}-${line}`}>{line}</div>
                  )) : <div className="empty-state">还没有运行日志。</div>}
                </div>
              </div>
            </div>
          </section>
        ) : (
          <section className="content-grid history-layout">
            <form className="panel history-search" onSubmit={queryHistory}>
              <div className="panel-header">
                <div>
                  <h2>历史查询</h2>
                  <p>按范围筛选</p>
                </div>
                <button className="primary-button" disabled={busy} type="submit">查询</button>
              </div>
              <div className="form-grid two">
                <label>
                  <span>数量</span>
                  <input
                    type="number"
                    min={1}
                    value={historyQuery.limit}
                    onChange={(event) => setHistoryQuery({ ...historyQuery, limit: Number(event.target.value) || 20 })}
                  />
                </label>
                <label>
                  <span>范围</span>
                  <select value={historyQuery.contextMode} onChange={(event) => setHistoryQuery({ ...historyQuery, contextMode: event.target.value })}>
                    <option value="frontmost">当前前台应用</option>
                    <option value="latest">最近一次来源</option>
                    <option value="all">全部应用</option>
                  </select>
                </label>
              </div>
              <label>
                <span>关键字</span>
                <input value={historyQuery.keyword} onChange={(event) => setHistoryQuery({ ...historyQuery, keyword: event.target.value })} placeholder="输入想定位的词或短语" />
              </label>
              <label>
                <span>正则</span>
                <input value={historyQuery.regex} onChange={(event) => setHistoryQuery({ ...historyQuery, regex: event.target.value })} placeholder="需要高级匹配时再填写" />
              </label>
            </form>

            <div className="panel panel-wide">
              <div className="panel-header">
                <div>
                  <h2>结果列表</h2>
                  <p>{records.length} 条结果</p>
                </div>
              </div>
              <div className="history-list">
                {records.map((record) => (
                  <article className="history-card" key={record.ID}>
                    <div className="history-card-top">
                      <div>
                        <div className="history-title">{record.AppName || 'Unknown App'}</div>
                        <div className="history-meta">
                          {record.CreatedAt} · {record.BundleID || record.WebDomain || 'global'}
                        </div>
                      </div>
                      <button className="primary-button small" onClick={() => void copyText(record.Text)}>复制</button>
                    </div>
                    {record.Title ? <div className="history-window">{record.Title}</div> : null}
                    <pre className="history-text">{record.Text}</pre>
                  </article>
                ))}
                {records.length === 0 ? <div className="empty-state">还没有查询结果。</div> : null}
              </div>
            </div>
          </section>
        )}
      </main>
    </div>
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
