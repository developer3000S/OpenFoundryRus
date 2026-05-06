import type { VersionDiff } from '@/lib/api/schedules';

interface ScheduleDiffProps {
  diff: VersionDiff | null;
  fromVersion: number;
  toVersion: number;
}

function formatValue(value: unknown) {
  if (value === null || value === undefined) return '∅';
  if (typeof value === 'string') return value;
  return JSON.stringify(value);
}

function Entry({ path, before, after }: { path: string; before: unknown; after: unknown }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr auto auto auto', alignItems: 'center', gap: 6, fontFamily: 'var(--font-mono)', fontSize: 12, padding: '4px 6px', background: '#111827', borderRadius: 4 }}>
      <span style={{ color: '#93c5fd' }}>{path}</span>
      <span style={{ color: '#fca5a5', textDecoration: 'line-through' }}>{formatValue(before)}</span>
      <span style={{ color: '#64748b' }}>→</span>
      <span style={{ color: '#86efac' }}>{formatValue(after)}</span>
    </div>
  );
}

export function ScheduleDiff({ diff, fromVersion, toVersion }: ScheduleDiffProps) {
  const hasChanges =
    !!diff &&
    (!!diff.name_diff || !!diff.description_diff || diff.trigger_diff.length > 0 || diff.target_diff.length > 0);

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 4, padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0' }}>
      <header>
        <h4 style={{ margin: 0, fontSize: 13, color: '#cbd5e1' }}>Diff: v{fromVersion} → v{toVersion}</h4>
      </header>
      {!diff ? (
        <p style={{ color: '#94a3b8', fontStyle: 'italic', fontSize: 12, margin: 0 }}>Loading diff…</p>
      ) : !hasChanges ? (
        <p style={{ color: '#94a3b8', fontStyle: 'italic', fontSize: 12, margin: 0 }}>No changes between these versions.</p>
      ) : (
        <>
          {diff.name_diff && <Entry path="name" before={diff.name_diff.before} after={diff.name_diff.after} />}
          {diff.description_diff && <Entry path="description" before={diff.description_diff.before} after={diff.description_diff.after} />}
          {diff.trigger_diff.length > 0 && (
            <>
              <h5 style={{ margin: '8px 0 2px', fontSize: 11, textTransform: 'uppercase', color: '#94a3b8', letterSpacing: '0.05em' }}>Trigger</h5>
              {diff.trigger_diff.map((e) => <Entry key={e.path} path={e.path} before={e.before} after={e.after} />)}
            </>
          )}
          {diff.target_diff.length > 0 && (
            <>
              <h5 style={{ margin: '8px 0 2px', fontSize: 11, textTransform: 'uppercase', color: '#94a3b8', letterSpacing: '0.05em' }}>Target</h5>
              {diff.target_diff.map((e) => <Entry key={e.path} path={e.path} before={e.before} after={e.after} />)}
            </>
          )}
        </>
      )}
    </section>
  );
}
