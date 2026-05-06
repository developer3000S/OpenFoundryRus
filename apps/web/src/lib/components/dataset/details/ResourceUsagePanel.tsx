interface ResourceUsagePanelProps {
  sizeBytes: number;
  fileCount: number;
  rowCount: number;
  history?: Array<{ ts: string; bytes: number }>;
}

function fmtBytes(b: number) {
  if (b < 1024) return `${b} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  if (b < 1024 * 1024 * 1024) return `${(b / (1024 * 1024)).toFixed(1)} MB`;
  return `${(b / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export function ResourceUsagePanel({ sizeBytes, fileCount, rowCount, history = [] }: ResourceUsagePanelProps) {
  let points = '';
  if (history.length > 0) {
    const max = Math.max(...history.map((h) => h.bytes), 1);
    const w = 200;
    const h = 40;
    points = history
      .map((sample, i) => {
        const x = (i / Math.max(history.length - 1, 1)) * w;
        const y = h - (sample.bytes / max) * h;
        return `${x.toFixed(1)},${y.toFixed(1)}`;
      })
      .join(' ');
  }
  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <header>
        <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>Resource usage metrics</div>
        <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>Storage footprint</h2>
      </header>
      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
        <Stat label="Storage" value={fmtBytes(sizeBytes)} />
        <Stat label="Files" value={fileCount.toLocaleString()} />
        <Stat label="Rows (estimated)" value={rowCount.toLocaleString()} />
      </div>
      {history.length > 1 && (
        <div className="of-panel" style={{ padding: 16 }}>
          <div style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Storage trend</div>
          <svg viewBox="0 0 200 40" style={{ marginTop: 8, height: 40, width: '100%' }} aria-hidden="true">
            <polyline fill="none" stroke="#3b82f6" strokeWidth={1.5} points={points} />
          </svg>
        </div>
      )}
    </section>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="of-panel" style={{ padding: 16 }}>
      <div style={{ fontSize: 10, textTransform: 'uppercase', color: 'var(--text-muted)' }}>{label}</div>
      <div style={{ marginTop: 4, fontSize: 24, fontWeight: 600 }}>{value}</div>
    </div>
  );
}
