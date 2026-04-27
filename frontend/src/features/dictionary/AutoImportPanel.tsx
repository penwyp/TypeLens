import { useEffect, useMemo, useState } from 'react';
import { ConfirmAutoImport, DefaultAutoImportSources, ScanAutoImportSources } from '../../../wailsjs/go/main/App';
import { service, typeless } from '../../../wailsjs/go/models';
import { LogConsole } from '../../components/Dialog';

type AutoImportStep = 'sources' | 'preview' | 'confirm';

type AutoImportPanelProps = {
  busy: boolean;
  logs: string[];
  onError: (message: string) => void;
  onScanStart: () => void;
  onSuccess: (result: service.AutoImportConfirmResult) => void;
};

export function AutoImportPanel({ busy, logs, onError, onScanStart, onSuccess }: AutoImportPanelProps) {
  const [step, setStep] = useState<AutoImportStep>('sources');
  const [loadingDefaults, setLoadingDefaults] = useState(true);
  const [actionBusy, setActionBusy] = useState(false);
  const [sources, setSources] = useState<typeless.AutoImportSource[]>([]);
  const [scanResult, setScanResult] = useState<typeless.AutoImportScanResult | null>(null);
  const [selectedTerms, setSelectedTerms] = useState<Record<string, boolean>>({});
  const [search, setSearch] = useState('');

  useEffect(() => {
    let canceled = false;
    async function loadDefaults() {
      try {
        setLoadingDefaults(true);
        const defaults = await DefaultAutoImportSources();
        if (!canceled) {
          setSources(defaults);
        }
      } catch (error) {
        if (!canceled) {
          onError(stringifyError(error));
        }
      } finally {
        if (!canceled) {
          setLoadingDefaults(false);
        }
      }
    }
    void loadDefaults();
    return () => {
      canceled = true;
    };
  }, [onError]);

  const filteredItems = useMemo(() => {
    const items = scanResult?.items ?? [];
    const keyword = search.trim().toLowerCase();
    if (!keyword) {
      return items;
    }
    return items.filter((item) => item.term.toLowerCase().includes(keyword));
  }, [scanResult, search]);

  const selectedItems = useMemo(() => {
    const items = scanResult?.items ?? [];
    return items.filter((item) => selectedTerms[item.normalized_term] !== false);
  }, [scanResult, selectedTerms]);

  async function scanSources() {
    try {
      setActionBusy(true);
      onScanStart();
      const result = await ScanAutoImportSources(new service.AutoImportScanRequest({
        sources,
      }));
      setScanResult(result);
      setSearch('');
      const defaults: Record<string, boolean> = {};
      for (const item of result.items) {
        defaults[item.normalized_term] = true;
      }
      setSelectedTerms(defaults);
      setStep('preview');
    } catch (error) {
      onError(stringifyError(error));
    } finally {
      setActionBusy(false);
    }
  }

  async function confirmImport() {
    try {
      setActionBusy(true);
      const result = await ConfirmAutoImport(new service.AutoImportConfirmRequest({
        items: selectedItems,
      }));
      onSuccess(result);
      setStep('sources');
      setScanResult(null);
      setSearch('');
    } catch (error) {
      onError(stringifyError(error));
    } finally {
      setActionBusy(false);
    }
  }

  function toggleAll(selected: boolean) {
    const next: Record<string, boolean> = {};
    for (const item of scanResult?.items ?? []) {
      next[item.normalized_term] = selected;
    }
    setSelectedTerms(next);
  }

  function addCustomSource() {
    setSources((current) => [
      ...current,
      {
        platform: 'custom',
        enabled: true,
        workdir: '',
      },
    ]);
  }

  function updateSource(index: number, nextSource: typeless.AutoImportSource) {
    setSources((current) => current.map((source, sourceIndex) => sourceIndex === index ? nextSource : source));
  }

  function removeSource(index: number) {
    setSources((current) => current.filter((_, sourceIndex) => sourceIndex !== index));
  }

  return (
    <div className="dialog-form auto-import-form">
      {step === 'sources' ? (
        <>
          <div className="field-hint auto-import-hint">扫描 `history.jsonl` 以及目录下的 `*.jsonl` 记录，提取候选词。</div>
          <div className="auto-import-source-list">
            {sources.map((source, index) => (
              <div className="source-card source-card-inline" key={`${source.platform}-${index}`}>
                <div className="source-inline-main">
                  <input
                    className="source-checkbox"
                    checked={source.enabled}
                    onChange={(event) => updateSource(index, { ...source, enabled: event.target.checked })}
                    type="checkbox"
                  />
                  <div className="source-label-group">
                    <span className="source-name">{sourceLabel(source.platform)}</span>
                  </div>
                </div>
                <input
                  className="source-workdir-input"
                  value={source.workdir}
                  onChange={(event) => updateSource(index, { ...source, workdir: event.target.value })}
                  placeholder="输入目录"
                />
                {source.platform === 'custom' ? (
                  <button className="icon-button subtle-icon-button" type="button" onClick={() => removeSource(index)} aria-label="删除类别">×</button>
                ) : (
                  <div className="source-spacer" />
                )}
              </div>
            ))}
          </div>
          <div className="dialog-actions spread-actions">
            <button className="ghost-button add-source-button" type="button" onClick={addCustomSource}>+ 添加其他目录</button>
            <button
              className="primary-button"
              disabled={loadingDefaults || busy || actionBusy || !sources.some((item) => item.enabled && item.workdir.trim())}
              type="button"
              onClick={() => void scanSources()}
            >
              开始扫描
            </button>
          </div>
          <LogConsole logs={logs} busy={busy || loadingDefaults || actionBusy} idleText="等待开始扫描" />
        </>
      ) : null}

      {step === 'preview' && scanResult ? (
        <>
          <div className="auto-import-summary-grid">
            <SummaryMetric label="扫描文件" value={scanResult.scanned_files} />
            <SummaryMetric label="用户输入" value={scanResult.parsed_messages} />
            <SummaryMetric label="候选词" value={scanResult.raw_candidates} />
            <SummaryMetric label="待导入" value={selectedItems.length} />
          </div>
          <div className="preview-toolbar">
            <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索候选词" />
            <div className="button-row">
              <button className="ghost-button" type="button" onClick={() => toggleAll(true)}>全选</button>
              <button className="ghost-button" type="button" onClick={() => toggleAll(false)}>全不选</button>
            </div>
          </div>
          <div className="list word-grid auto-import-word-grid">
            {filteredItems.map((item) => (
              <button
                className={`word-chip auto-import-chip ${selectedTerms[item.normalized_term] !== false ? 'auto-import-chip-selected' : 'auto-import-chip-unselected'}`}
                key={`${item.platform}-${item.normalized_term}`}
                type="button"
                aria-pressed={selectedTerms[item.normalized_term] !== false}
                onClick={() => setSelectedTerms((current) => ({ ...current, [item.normalized_term]: current[item.normalized_term] === false }))}
              >
                <div className="word-primary">
                  <span className="word-text">{item.term}</span>
                </div>
                {selectedTerms[item.normalized_term] !== false ? <span className="auto-import-chip-check" aria-hidden="true">✓</span> : null}
                <span className="auto-import-chip-count" aria-hidden="true">{item.hits} 次</span>
              </button>
            ))}
            {filteredItems.length === 0 ? <div className="empty-state">没有可展示的候选词</div> : null}
          </div>
          <div className="dialog-actions">
            <button className="ghost-button" type="button" onClick={() => setStep('sources')}>返回</button>
            <button className="primary-button" disabled={actionBusy || selectedItems.length === 0} type="button" onClick={() => setStep('confirm')}>确认候选词</button>
          </div>
        </>
      ) : null}

      {step === 'confirm' && scanResult ? (
        <>
          <div className="summary-box">
            即将导入 {selectedItems.length} 个词。确认后会先写入本地词典视图，再在后台逐步同步到远端词典。
          </div>
          <div className="list word-grid auto-import-word-grid auto-import-word-grid-compact">
            {selectedItems.map((item) => (
              <div className="word-chip auto-import-chip auto-import-chip-readonly auto-import-chip-selected" key={`${item.platform}-${item.normalized_term}`}>
                <div className="word-primary">
                  <span className="word-text">{item.term}</span>
                </div>
              </div>
            ))}
          </div>
          <div className="dialog-actions">
            <button className="ghost-button" type="button" onClick={() => setStep('preview')}>返回预览</button>
            <button className="primary-button" disabled={busy || actionBusy || selectedItems.length === 0} type="button" onClick={() => void confirmImport()}>
              确认导入
            </button>
          </div>
          <LogConsole logs={logs} busy={busy || actionBusy} idleText="确认后开始后台同步" />
        </>
      ) : null}
    </div>
  );
}

function SummaryMetric({ label, value }: { label: string; value: number }) {
  return (
    <div className="summary-metric">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function sourceLabel(platform: string) {
  const normalized = platform.trim().toLowerCase();
  if (normalized === 'codex') {
    return 'Codex';
  }
  if (normalized === 'claude') {
    return 'Claude';
  }
  return '其他目录';
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
