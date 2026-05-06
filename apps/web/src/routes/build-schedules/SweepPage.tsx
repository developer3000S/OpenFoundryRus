import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { applySweep, runSweep, type LinterFinding, type SweepReport } from '@/lib/api/schedules';

const SEVERITY_TONE: Record<LinterFinding['severity'], string> = {
  Info: '#334155',
  Warning: '#92400e',
  Error: '#b91c1c',
};

export function SweepPage() {
  const [report, setReport] = useState<SweepReport | null>(null);
  const [project, setProject] = useState('');
  const [production, setProduction] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [applyResult, setApplyResult] = useState<Array<Record<string, unknown>> | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function run() {
    setBusy(true);
    setError('');
    setApplyResult(null);
    try {
      const params: { project?: string; production?: boolean } = { production };
      if (project.trim()) params.project = project.trim();
      const res = await runSweep(params);
      setReport({ findings: res.findings });
      setSelected(new Set(res.findings.map((f) => f.id)));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Sweep failed');
      setReport(null);
    } finally {
      setBusy(false);
    }
  }

  async function applySelection() {
    if (!report) return;
    setBusy(true);
    setError('');
    try {
      const res = await applySweep({
        finding_ids: Array.from(selected),
        report,
      });
      setApplyResult(res.applied);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Apply failed');
    } finally {
      setBusy(false);
    }
  }

  function toggle(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  const grouped = useMemo(() => {
    const m = new Map<string, LinterFinding[]>();
    if (!report) return m;
    for (const f of report.findings) {
      const list = m.get(f.rule_id) ?? [];
      list.push(f);
      m.set(f.rule_id, list);
    }
    return m;
  }, [report]);

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16, maxWidth: 1024 }}>
      <Link to="/build-schedules" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Build schedules</Link>
      <header>
        <h1 className="of-heading-xl">Sweep schedules</h1>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
          Run rules SCH-001 through SCH-007 against the schedule inventory. Findings are bucketed by rule with a bulk apply.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
        <label style={{ fontSize: 13 }}>
          Project RID (optional)
          <input
            value={project}
            onChange={(e) => setProject(e.target.value)}
            placeholder="ri.foundry.main.project.alpha"
            className="of-input"
            style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }}
          />
        </label>
        <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
          <input type="checkbox" checked={production} onChange={(e) => setProduction(e.target.checked)} />
          Production environment (gates SCH-006 high-frequency rule)
        </label>
        <div>
          <button type="button" onClick={() => void run()} disabled={busy} className="of-button of-button--primary">
            {busy ? 'Running…' : 'Run sweep'}
          </button>
        </div>
      </section>

      {report && (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 12 }}>
          {report.findings.length === 0 ? (
            <p className="of-text-muted" style={{ fontStyle: 'italic' }}>No findings — every schedule looks healthy.</p>
          ) : (
            <>
              {Array.from(grouped.entries()).map(([ruleId, findings]) => (
                <article key={ruleId} className="of-panel" style={{ padding: 12 }}>
                  <header style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: 6 }}>
                    <h2 className="of-heading-md" style={{ margin: 0 }}>{ruleId}</h2>
                    <span className="of-text-muted" style={{ fontSize: 11 }}>
                      {findings.length} finding{findings.length === 1 ? '' : 's'}
                    </span>
                  </header>
                  <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
                    {findings.map((f) => (
                      <li key={f.id} style={{ background: 'var(--bg-subtle)', padding: '6px 8px', borderRadius: 4 }}>
                        <label style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 12, cursor: 'pointer' }}>
                          <input type="checkbox" checked={selected.has(f.id)} onChange={() => toggle(f.id)} />
                          <span style={{ padding: '1px 6px', borderRadius: 3, fontSize: 10, fontWeight: 600, background: SEVERITY_TONE[f.severity], color: '#fff' }}>
                            {f.severity}
                          </span>
                          <code style={{ fontSize: 11 }}>{f.schedule_rid}</code>
                          <span style={{ flex: 1 }}>{f.message}</span>
                          <span className="of-text-muted" style={{ fontStyle: 'italic' }}>→ {f.recommended_action}</span>
                        </label>
                      </li>
                    ))}
                  </ul>
                </article>
              ))}
              <button
                type="button"
                onClick={() => void applySelection()}
                disabled={selected.size === 0 || busy}
                className="of-button of-button--primary"
                style={{ alignSelf: 'flex-start', background: '#b45309', borderColor: '#b45309' }}
              >
                Apply ({selected.size})
              </button>
            </>
          )}
        </section>
      )}

      {applyResult && (
        <section className="of-panel" style={{ padding: 16 }}>
          <h2 className="of-heading-md" style={{ margin: 0 }}>Apply result</h2>
          <pre style={{ marginTop: 8, padding: 10, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
            {JSON.stringify(applyResult, null, 2)}
          </pre>
        </section>
      )}
    </section>
  );
}
