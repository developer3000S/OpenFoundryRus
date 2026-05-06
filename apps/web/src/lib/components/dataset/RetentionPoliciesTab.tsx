import { useEffect, useState } from 'react';

import {
  createRetentionPolicy,
  deleteRetentionPolicy,
  getApplicablePolicies,
  getRetentionPreview,
  type ApplicablePoliciesResponse,
  type RetentionPolicy,
  type RetentionPreviewResponse,
} from '@/lib/api/datasets';

interface RetentionPoliciesTabProps {
  datasetRid: string;
  projectId?: string;
  spaceId?: string;
  orgId?: string;
  canManage?: boolean;
}

function PolicyRow({ policy, onDelete }: { policy: RetentionPolicy; onDelete?: () => void }) {
  return (
    <li
      style={{
        padding: 8,
        background: 'var(--bg-subtle)',
        borderRadius: 6,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        gap: 8,
        fontSize: 12,
      }}
    >
      <div>
        <strong>{policy.name}</strong>
        <span className="of-text-muted" style={{ marginLeft: 8, fontSize: 11 }}>
          · {policy.target_kind} · {policy.retention_days}d · {policy.purge_mode}{policy.legal_hold ? ' · legal hold' : ''}
        </span>
      </div>
      {onDelete && (
        <button type="button" onClick={onDelete} className="of-button" style={{ fontSize: 10, color: '#b91c1c', borderColor: '#fecaca' }}>
          Delete
        </button>
      )}
    </li>
  );
}

export function RetentionPoliciesTab({ datasetRid, projectId, spaceId, orgId, canManage = false }: RetentionPoliciesTabProps) {
  const [applicable, setApplicable] = useState<ApplicablePoliciesResponse | null>(null);
  const [preview, setPreview] = useState<RetentionPreviewResponse | null>(null);
  const [asOfDays, setAsOfDays] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [creating, setCreating] = useState(false);
  const [policyName, setPolicyName] = useState('');
  const [retentionDays, setRetentionDays] = useState(90);

  async function loadApplicable() {
    setError(null);
    try {
      setApplicable(await getApplicablePolicies(datasetRid, { project_id: projectId, space_id: spaceId, org_id: orgId }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Applicable-policies failed.');
    }
  }
  async function loadPreview() {
    try {
      setPreview(await getRetentionPreview(datasetRid, asOfDays, { project_id: projectId, space_id: spaceId, org_id: orgId }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Preview failed.');
    }
  }

  useEffect(() => { void loadApplicable(); }, [datasetRid, projectId, spaceId, orgId]);
  useEffect(() => { void loadPreview(); }, [datasetRid, asOfDays]);

  async function createPolicy() {
    if (!policyName.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await createRetentionPolicy({
        name: policyName.trim(),
        scope: `dataset:${datasetRid}`,
        target_kind: 'transaction',
        retention_days: retentionDays,
        legal_hold: false,
        purge_mode: 'soft',
        rules: [],
        selector: { dataset_rid: datasetRid } as Parameters<typeof createRetentionPolicy>[0]['selector'],
        criteria: { tx_types: [] } as Parameters<typeof createRetentionPolicy>[0]['criteria'],
        grace_period_minutes: 60,
        updated_by: 'ui',
      });
      setPolicyName('');
      setCreating(false);
      await loadApplicable();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create policy failed.');
    } finally {
      setBusy(false);
    }
  }

  async function deletePolicy(id: string) {
    if (typeof window !== 'undefined' && !window.confirm('Delete policy?')) return;
    setBusy(true);
    try {
      await deleteRetentionPolicy(id);
      await loadApplicable();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <div style={{ padding: '8px 12px', background: '#1e3a8a', color: '#bfdbfe', borderRadius: 6, fontSize: 11 }}>
        <strong>Beta:</strong> retention policies. Inherited bottom-up: org → space → project → explicit.
      </div>

      {error && <div className="of-status-danger" style={{ padding: '8px 12px', borderRadius: 8, fontSize: 12 }}>{error}</div>}

      {applicable && (
        <>
          {(['org', 'space', 'project'] as const).map((scope) => {
            const list = applicable.inherited[scope];
            return (
              <section key={scope} className="of-panel" style={{ padding: 12 }}>
                <p className="of-eyebrow" style={{ fontSize: 10 }}>{scope.toUpperCase()} ({list.length})</p>
                <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
                  {list.map((p) => <PolicyRow key={p.id} policy={p} />)}
                  {list.length === 0 && <li className="of-text-muted" style={{ fontSize: 11 }}>None.</li>}
                </ul>
              </section>
            );
          })}

          <section className="of-panel" style={{ padding: 12 }}>
            <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
              <p className="of-eyebrow" style={{ fontSize: 10 }}>Explicit on this dataset ({applicable.explicit.length})</p>
              {canManage && !creating && (
                <button type="button" onClick={() => setCreating(true)} className="of-button" style={{ fontSize: 11 }}>+ New policy</button>
              )}
            </header>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
              {applicable.explicit.map((p) => (
                <PolicyRow key={p.id} policy={p} onDelete={canManage ? () => void deletePolicy(p.id) : undefined} />
              ))}
              {applicable.explicit.length === 0 && <li className="of-text-muted" style={{ fontSize: 11 }}>No explicit policies on this dataset.</li>}
            </ul>
            {creating && (
              <div style={{ marginTop: 12, display: 'grid', gap: 6 }}>
                <input value={policyName} onChange={(e) => setPolicyName(e.target.value)} placeholder="Policy name" className="of-input" />
                <label style={{ fontSize: 12 }}>
                  Retention days
                  <input type="number" min={1} value={retentionDays} onChange={(e) => setRetentionDays(Number(e.target.value) || 0)} className="of-input" style={{ marginTop: 4, width: 120 }} />
                </label>
                <div style={{ display: 'flex', gap: 6 }}>
                  <button type="button" onClick={() => void createPolicy()} disabled={busy} className="of-button of-button--primary" style={{ fontSize: 11 }}>Create</button>
                  <button type="button" onClick={() => setCreating(false)} className="of-button" style={{ fontSize: 11 }}>Cancel</button>
                </div>
              </div>
            )}
          </section>

          <section className="of-panel" style={{ padding: 12 }}>
            <p className="of-eyebrow" style={{ fontSize: 10 }}>Effective policy</p>
            {applicable.effective ? <PolicyRow policy={applicable.effective} /> : <p className="of-text-muted" style={{ fontSize: 11 }}>No policy applies — data retained indefinitely.</p>}
            {applicable.conflicts.length > 0 && (
              <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 11 }}>
                {applicable.conflicts.map((c, i) => (
                  <li key={i} style={{ color: '#fbbf24' }}>
                    Conflict: {c.winner_id} won over {c.loser_id} — {c.reason}
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      )}

      <section className="of-panel" style={{ padding: 12, display: 'grid', gap: 8 }}>
        <p className="of-eyebrow" style={{ fontSize: 10 }}>Preview deletions</p>
        <label style={{ fontSize: 12 }}>
          As of (days from now): {asOfDays}
          <input
            type="range"
            min={0}
            max={365}
            value={asOfDays}
            onChange={(e) => setAsOfDays(Number(e.target.value))}
            style={{ width: '100%', marginTop: 4 }}
          />
        </label>
        {preview && (
          <ul className="of-text-muted" style={{ paddingLeft: 18, fontSize: 12, marginTop: 0 }}>
            <li>{preview.summary.transactions_would_delete} of {preview.summary.transactions_total} transactions would be purged</li>
            <li>{preview.summary.files_total} files · {preview.summary.bytes_total.toLocaleString()} bytes</li>
            {preview.warnings.length > 0 && preview.warnings.map((w, i) => <li key={i} style={{ color: '#fbbf24' }}>{w}</li>)}
          </ul>
        )}
      </section>
    </section>
  );
}
