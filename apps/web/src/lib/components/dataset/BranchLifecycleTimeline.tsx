import { useMemo, useState } from 'react';

import type { DatasetBranch, DatasetTransaction } from '@/lib/api/datasets';

interface BranchLifecycleTimelineProps {
  branches: DatasetBranch[];
  transactions: DatasetTransaction[];
}

type Tone = 'committed' | 'aborted' | 'open' | 'jobspec' | 'archived';

interface Marker {
  branch: string;
  tone: Tone;
  at: string;
  label: string;
}

const PALETTE: Record<Tone, { bg: string; label: string }> = {
  committed: { bg: '#10b981', label: 'COMMITTED' },
  aborted: { bg: '#ef4444', label: 'ABORTED' },
  open: { bg: '#f59e0b', label: 'OPEN' },
  jobspec: { bg: '#3b82f6', label: 'JOBSPEC' },
  archived: { bg: '#64748b', label: 'ARCHIVED' },
};

function txTone(status: string): Tone {
  const s = status.toUpperCase();
  if (s === 'COMMITTED') return 'committed';
  if (s === 'ABORTED') return 'aborted';
  if (s === 'OPEN') return 'open';
  return 'committed';
}

export function BranchLifecycleTimeline({ branches, transactions }: BranchLifecycleTimelineProps) {
  const [hovered, setHovered] = useState<Marker | null>(null);

  const allMarkers = useMemo<Marker[]>(() => {
    const out: Marker[] = [];
    for (const tx of transactions) {
      const branch = (tx as DatasetTransaction & { branch_name?: string | null }).branch_name;
      if (!branch) continue;
      const at = (tx.committed_at ?? tx.created_at) || '';
      out.push({ branch, tone: txTone(tx.status), at, label: `${tx.operation || 'TX'} · ${tx.status}` });
      const meta = (tx.metadata ?? {}) as Record<string, unknown>;
      const jobspecAt = meta['published_jobspec_at'];
      if (typeof jobspecAt === 'string') {
        out.push({ branch, tone: 'jobspec', at: jobspecAt, label: 'JobSpec published' });
      }
    }
    for (const b of branches) {
      if (b.archived_at) out.push({ branch: b.name, tone: 'archived', at: b.archived_at, label: 'Branch archived' });
    }
    return out;
  }, [branches, transactions]);

  const range = useMemo(() => {
    const times = allMarkers.map((m) => Date.parse(m.at)).filter((n) => Number.isFinite(n));
    if (times.length === 0) {
      const now = Date.now();
      return { start: now - 24 * 3600 * 1000, end: now };
    }
    return { start: Math.min(...times), end: Math.max(...times) };
  }, [allMarkers]);

  function pct(at: string): number {
    const ts = Date.parse(at);
    if (!Number.isFinite(ts)) return 0;
    const span = range.end - range.start;
    if (span <= 0) return 50;
    return Math.max(0, Math.min(100, ((ts - range.start) / span) * 100));
  }

  const markersByBranch = useMemo(() => {
    const out: Record<string, Marker[]> = {};
    for (const m of allMarkers) {
      if (!out[m.branch]) out[m.branch] = [];
      out[m.branch].push(m);
    }
    for (const arr of Object.values(out)) arr.sort((a, b) => Date.parse(a.at) - Date.parse(b.at));
    return out;
  }, [allMarkers]);

  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 style={{ margin: 0, fontSize: 13, fontWeight: 600 }}>Branch lifecycle</h2>
        <ul style={{ display: 'flex', flexWrap: 'wrap', gap: 8, paddingLeft: 0, listStyle: 'none', fontSize: 10, color: 'var(--text-muted)' }}>
          {(['jobspec', 'committed', 'aborted', 'open', 'archived'] as Tone[]).map((t) => (
            <li key={t} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: '50%', background: PALETTE[t].bg }} />
              <span>{PALETTE[t].label}</span>
            </li>
          ))}
        </ul>
      </header>
      <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
        {branches.map((branch) => {
          const markers = markersByBranch[branch.name] ?? [];
          return (
            <li key={branch.id} style={{ display: 'grid', gridTemplateColumns: '140px 1fr', alignItems: 'center', gap: 8, fontSize: 11 }}>
              <span style={{ fontFamily: 'var(--font-mono)' }}>
                {branch.name}{branch.is_default ? ' ★' : ''}
              </span>
              <div style={{ position: 'relative', height: 16, borderRadius: 4, background: 'var(--bg-subtle)' }}>
                {markers.map((m, i) => (
                  <button
                    key={`${m.branch}-${m.tone}-${m.at}-${i}`}
                    type="button"
                    onMouseEnter={() => setHovered(m)}
                    onMouseLeave={() => setHovered(null)}
                    aria-label={m.label}
                    style={{
                      position: 'absolute',
                      top: '50%',
                      left: `${pct(m.at)}%`,
                      transform: 'translate(-50%, -50%)',
                      width: 12,
                      height: 12,
                      borderRadius: '50%',
                      background: PALETTE[m.tone].bg,
                      border: 'none',
                      cursor: 'pointer',
                      boxShadow: '0 0 0 2px rgba(255,255,255,0.15)',
                      padding: 0,
                    }}
                  />
                ))}
              </div>
            </li>
          );
        })}
        {branches.length === 0 && <li className="of-text-muted" style={{ fontSize: 11 }}>No branches.</li>}
      </ul>
      {hovered && (
        <aside style={{ padding: 8, borderRadius: 6, background: 'var(--bg-subtle)', fontSize: 11 }}>
          <p style={{ margin: 0, fontWeight: 600 }}>{hovered.branch} · {hovered.label}</p>
          <p style={{ margin: 0, color: 'var(--text-muted)' }}>{hovered.at}</p>
        </aside>
      )}
    </section>
  );
}
