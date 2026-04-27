import { useEffect, useMemo, useState } from 'react';
import { ConfirmAutoImport, DefaultAutoImportSources, ScanAutoImportSources } from '../../../wailsjs/go/main/App';
import { service, typeless } from '../../../wailsjs/go/models';
import { LogConsole } from '../../components/Dialog';

type AutoImportStep = 'sources' | 'preview' | 'confirm';

type AutoImportPanelProps = {
  busy: boolean;
  logs: string[];
  onError: (message: string) => void;
  onSuccess: (result: service.AutoImportConfirmResult) => void;
};

type SourceDraft = {
  name: string;
  source: typeless.AutoImportSource;
};

export function AutoImportPanel({ busy, logs, onError, onSuccess }: AutoImportPanelProps) {
  const [step, setStep] = useState<AutoImportStep>('sources');
  const [loadingDefaults, setLoadingDefaults] = useState(true);
  const [actionBusy, setActionBusy] = useState(false);
  const [sources, setSources] = useState<SourceDraft[]>([]);
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
          setSources(defaults.map((source) => ({
            name: platformLabel(source.platform),
            source,
          })));
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
    return items.filter((item) => item.term.toLowerCase().includes(keyword) || item.examples.some((example) => example.toLowerCase().includes(keyword)));
  }, [scanResult, search]);

  const selectedItems = useMemo(() => {
    const items = scanResult?.items ?? [];
    return items.filter((item) => selectedTerms[item.normalized_term] !== false);
  }, [scanResult, selectedTerms]);

  async function scanSources() {
    try {
      setActionBusy(true);
      const result = await ScanAutoImportSources(new service.AutoImportScanRequest({
        sources: sources.map((item) => item.source),
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
      name: 'Custom',
    ]);
  }

  function updateSource(index: number, nextSource: SourceDraft) {
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
              <div className="source-card source-card-inline" key={`${source.source.platform}-${index}`}>
                <label className="source-inline-main">
                  <input
                    checked={source.source.enabled}
                    onChange={(event) => updateSource(index, { ...source, source: { ...source.source, enabled: event.target.checked } })}
                    type="checkbox"
                  />
                  <div className="source-label-group">
                    {source.source.platform === 'custom' ? (
                      <input
                        className="source-name-input"
                        value={source.name}
                        onChange={(event) => updateSource(index, { ...source, name: event.target.value })}
                        placeholder="Custom"
                      />
                    ) : (
                      <span className="source-name">{source.name}</span>
                    )}
                  </div>
                </label>
                <input
                  className="source-workdir-input"
                  value={source.source.workdir}
                  onChange={(event) => updateSource(index, { ...source, source: { ...source.source, workdir: event.target.value } })}
                  placeholder="输入目录"
                />
                {source.source.platform === 'custom' ? (
                  <button className="icon-button subtle-icon-button" type="button" onClick={() => removeSource(index)} aria-label="删除类别">×</button>
                ) : (
                  <div className="source-spacer" />
                )}
              </div>
            ))}
          </div>
          <div className="dialog-actions spread-actions">
            <button className="ghost-button add-source-button" type="button" onClick={addCustomSource}>+ Custom</button>
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
          <div className="candidate-list">
            {filteredItems.map((item) => (
              <label className="candidate-row" key={`${item.platform}-${item.normalized_term}`}>
                <input
                  checked={selectedTerms[item.normalized_term] !== false}
                  onChange={(event) => setSelectedTerms((current) => ({ ...current, [item.normalized_term]: event.target.checked }))}
                  type="checkbox"
                />
                <div className="candidate-main">
                  <div className="candidate-header">
                    <strong>{item.term}</strong>
                    <span className="candidate-meta">{platformLabel(item.platform)} · {item.hits} 次</span>
                  </div>
                  <div className="candidate-example">{item.examples[0] ?? '无示例'}</div>
                </div>
              </label>
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
          <div className="candidate-list compact">
            {selectedItems.map((item) => (
              <div className="candidate-row compact" key={`${item.platform}-${item.normalized_term}`}>
                <div className="candidate-main">
                  <div className="candidate-header">
                    <strong>{item.term}</strong>
                    <span className="candidate-meta">{platformLabel(item.platform)}</span>
                  </div>
                  <div className="candidate-example">{item.examples[0] ?? '无示例'}</div>
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

function platformLabel(platform: string) {
  const normalized = platform.trim().toLowerCase();
  if (normalized === 'codex') {
    return 'Codex';
  }
  if (normalized === 'claude') {
    return 'Claude';
  }
  if (normalized === 'custom') {
    return 'Custom';
  }
  return platform.trim() || 'Custom';
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
