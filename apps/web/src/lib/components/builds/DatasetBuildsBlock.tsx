import { useEffect, useState } from 'react';

import { listDatasetBuildsV1, type Build } from '@/lib/api/buildsV1';

import { StateBadge } from './StateBadge';

interface Props {
  datasetRid: string;
  limit?: number;
}

function shortRid(rid: string): string {
  const dot = rid.lastIndexOf('.');
  return dot >= 0 ? rid.slice(dot + 1).slice(0, 8) : rid.slice(0, 8);
}

function durationLabel(b: Build): string {
  const start = b.started_at ?? b.queued_at ?? b.created_at;
  const end = b.finished_at ?? new Date().toISOString();
  const ms = Math.max(0, Date.parse(end) - Date.parse(start));
  if (ms < 1000) return `${ms}ms`;
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  return `${m}m ${s % 60}s`;
}

export function DatasetBuildsBlock({ datasetRid, limit = 10 }: Props) {
  const [builds, setBuilds] = useState<Build[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function reload() {
      setLoading(true);
      setError(null);
      try {
        const response = await listDatasetBuildsV1(datasetRid);
        if (!cancelled) setBuilds(response.data.slice(0, limit));
      } catch (e) {
        if (!cancelled) setError(String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void reload();
    const handle = window.setInterval(() => {
      if (document.visibilityState === 'visible') void reload();
    }, 10_000);
    return () => { cancelled = true; clearInterval(handle); };
  }, [datasetRid, limit]);

  return (
    <section style={sectionStyle}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h4 style={{ margin: 0, fontSize: 13, color: '#f1f5f9' }}>Recent builds touching this dataset</h4>
        <a href="/builds?pipeline_rid=&since=&until=&state=&branch=" style={{ color: '#60a5fa', fontSize: 11, textDecoration: 'none' }}>See all in /builds →</a>
      </header>

      {error ? (
        <p style={{ color: '#ef4444', fontSize: 12, margin: 0 }}>{error}</p>
      ) : loading && builds.length === 0 ? (
        <p style={hintStyle}>Loading…</p>
      ) : builds.length === 0 ? (
        <p style={hintStyle}>No builds have touched this dataset yet.</p>
      ) : (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 4 }}>
          {builds.map((b) => (
            <li key={b.rid}>
              <a href={`/builds/${encodeURIComponent(b.rid)}`} style={rowStyle}>
                <code style={{ fontFamily: 'ui-monospace, monospace' }}>{shortRid(b.rid)}</code>
                <StateBadge kind="build" state={b.state} size="sm" />
                <span style={{ color: '#cbd5e1' }}>{b.build_branch}</span>
                <span style={{ color: '#94a3b8' }}>{durationLabel(b)}</span>
                <span style={{ color: '#94a3b8' }}>by <code>{b.requested_by}</code></span>
              </a>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

const sectionStyle: React.CSSProperties = { display: 'flex', flexDirection: 'column', gap: 8, padding: '12px 14px', background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0' };
const rowStyle: React.CSSProperties = { display: 'grid', gridTemplateColumns: 'auto auto 1fr auto auto', gap: 10, alignItems: 'center', padding: '6px 8px', borderRadius: 4, fontSize: 12, color: '#e2e8f0', textDecoration: 'none' };
const hintStyle: React.CSSProperties = { color: '#94a3b8', fontSize: 12, margin: 0, fontStyle: 'italic' };
