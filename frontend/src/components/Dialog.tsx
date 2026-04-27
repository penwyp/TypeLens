import { ReactNode } from 'react';

export function Dialog({ title, children, onClose, className = '' }: { title: string; children: ReactNode; onClose: () => void; className?: string }) {
  return (
    <div className="dialog-backdrop" role="presentation" onClick={onClose}>
      <section className={`dialog ${className}`.trim()} role="dialog" aria-modal="true" aria-label={title} onClick={(event) => event.stopPropagation()}>
        <div className="dialog-header">
          <h2>{title}</h2>
          <button className="icon-button" onClick={onClose} type="button" aria-label="关闭">×</button>
        </div>
        {children}
      </section>
    </div>
  );
}

export function LogConsole({ logs, busy, idleText = '暂无日志' }: { logs: string[]; busy: boolean; idleText?: string }) {
  return (
    <div className="log-console">
      {busy ? <div className="log-line active">运行中...</div> : null}
      {logs.length > 0 ? logs.map((line, index) => (
        <div className="log-line" key={`${index}-${line}`}>{line}</div>
      )) : <div className="log-line muted">{idleText}</div>}
    </div>
  );
}
