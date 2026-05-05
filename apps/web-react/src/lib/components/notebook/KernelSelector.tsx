import type { NotebookKernel } from '@/lib/api/notebooks';

import { KernelStatus } from './KernelStatus';

interface KernelSelectorProps {
  value: NotebookKernel;
  status?: string | null;
  disabled?: boolean;
  onChange?: (kernel: NotebookKernel) => void;
  onStart?: () => void;
  onStop?: () => void;
}

export function KernelSelector({
  value,
  status = null,
  disabled = false,
  onChange,
  onStart,
  onStop,
}: KernelSelectorProps) {
  const sessionLive = Boolean(status) && status !== 'dead';

  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        gap: 12,
        background: '#fff',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-md)',
        padding: '8px 12px',
        boxShadow: 'var(--shadow-panel)',
      }}
    >
      <div className="of-eyebrow">Kernel</div>

      <select
        className="of-select"
        value={value}
        onChange={(e) => onChange?.(e.target.value as NotebookKernel)}
        disabled={disabled}
        style={{ width: 'auto', minHeight: 32, fontSize: 13 }}
      >
        <option value="python">Python</option>
        <option value="sql">SQL</option>
        <option value="llm">LLM</option>
        <option value="r">R</option>
      </select>

      <KernelStatus kernel={value} status={status} />

      {!sessionLive ? (
        <button type="button" className="of-btn of-btn-primary" onClick={() => onStart?.()} style={{ minHeight: 32, fontSize: 13 }}>
          Start session
        </button>
      ) : (
        <button type="button" className="of-btn" onClick={() => onStop?.()} style={{ minHeight: 32, fontSize: 13 }}>
          Stop session
        </button>
      )}
    </div>
  );
}
