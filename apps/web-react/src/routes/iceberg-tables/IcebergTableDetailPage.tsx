import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import {
  createIcebergApiToken,
  getIcebergMetadata,
  getIcebergTable,
  getIcebergTableMarkingsByPath,
  listIcebergBranches,
  listIcebergSnapshots,
  runIcebergDiagnose,
  updateIcebergTableMarkings,
  type IcebergBranchesResponse,
  type IcebergDiagnoseResponse,
  type IcebergMetadataResponse,
  type IcebergSnapshotsResponse,
  type IcebergTableDetail,
  type IcebergTableMarkings,
} from '@/lib/api/icebergTables';

type Tab = 'overview' | 'schema' | 'snapshots' | 'metadata' | 'branches' | 'permissions' | 'activity' | 'catalog-access';

const TABS: Array<{ key: Tab; label: string }> = [
  { key: 'overview', label: 'Overview' },
  { key: 'schema', label: 'Schema' },
  { key: 'snapshots', label: 'Snapshots' },
  { key: 'metadata', label: 'Metadata' },
  { key: 'branches', label: 'Branches' },
  { key: 'permissions', label: 'Permissions' },
  { key: 'activity', label: 'Activity' },
  { key: 'catalog-access', label: 'Catalog Access' },
];

export function IcebergTableDetailPage() {
  const { id = '' } = useParams();
  const [activeTab, setActiveTab] = useState<Tab>('overview');
  const [detail, setDetail] = useState<IcebergTableDetail | null>(null);
  const [snapshots, setSnapshots] = useState<IcebergSnapshotsResponse | null>(null);
  const [metadata, setMetadata] = useState<IcebergMetadataResponse | null>(null);
  const [branches, setBranches] = useState<IcebergBranchesResponse | null>(null);
  const [tableMarkings, setTableMarkings] = useState<IcebergTableMarkings | null>(null);
  const [markingsError, setMarkingsError] = useState('');
  const [diagnoseRunning, setDiagnoseRunning] = useState(false);
  const [diagnoseResult, setDiagnoseResult] = useState<IcebergDiagnoseResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [issuedToken, setIssuedToken] = useState<string | null>(null);
  const [issuingToken, setIssuingToken] = useState(false);
  const [markingsDraft, setMarkingsDraft] = useState('');

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const res = await getIcebergTable(id);
        if (!cancelled) setDetail(res);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load table');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [id]);

  useEffect(() => {
    async function loadTab() {
      try {
        if (activeTab === 'snapshots' && !snapshots) setSnapshots(await listIcebergSnapshots(id));
        if (activeTab === 'metadata' && !metadata) setMetadata(await getIcebergMetadata(id));
        if (activeTab === 'branches' && !branches) setBranches(await listIcebergBranches(id));
        if (activeTab === 'permissions' && !tableMarkings && detail) {
          const namespace = detail.summary.namespace.join('.');
          try {
            const m = await getIcebergTableMarkingsByPath(namespace, detail.summary.name);
            setTableMarkings(m);
            setMarkingsDraft(JSON.stringify(m.effective));
          } catch (err) {
            setMarkingsError(err instanceof Error ? err.message : 'Failed to load markings');
          }
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load tab');
      }
    }
    void loadTab();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab, id, detail]);

  async function saveMarkings() {
    if (!detail) return;
    const namespace = detail.summary.namespace.join('.');
    const next = markingsDraft.split(',').map((s) => s.trim()).filter(Boolean);
    try {
      const res = await updateIcebergTableMarkings(namespace, detail.summary.name, next);
      setTableMarkings(res);
    } catch (err) {
      setMarkingsError(err instanceof Error ? err.message : 'Failed to save markings');
    }
  }

  async function runDiagnose(client: string) {
    setDiagnoseRunning(true);
    setDiagnoseResult(null);
    try {
      const res = await runIcebergDiagnose(client, detail?.summary.project_rid);
      setDiagnoseResult(res);
    } catch (err) {
      setDiagnoseResult({
        client,
        success: false,
        steps: [{ name: 'request', ok: false, latency_ms: 0, detail: err instanceof Error ? err.message : 'Unknown error' }],
        total_latency_ms: 0,
      });
    } finally {
      setDiagnoseRunning(false);
    }
  }

  async function generateToken() {
    setIssuingToken(true);
    try {
      const res = await createIcebergApiToken('PyIceberg client');
      setIssuedToken(res.raw_token);
    } catch (err) {
      setIssuedToken(`Error: ${err instanceof Error ? err.message : err}`);
    } finally {
      setIssuingToken(false);
    }
  }

  function downloadMetadata() {
    if (!metadata) return;
    const blob = new Blob([JSON.stringify(metadata.metadata, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${detail?.summary.name ?? 'table'}.metadata.json`;
    a.click();
    URL.revokeObjectURL(url);
  }

  if (loading) return <p className="of-text-muted" style={{ padding: 24 }}>Loading…</p>;
  if (error) return <p role="alert" className="of-status-danger" style={{ padding: 24 }}>{error}</p>;
  if (!detail) return null;

  const namespace = detail.summary.namespace.join('.') || '(root)';

  return (
    <section className="of-page" style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 16 }}>
      <header style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <Link to="/iceberg-tables" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
          ← Iceberg tables
        </Link>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <h1 className="of-heading-xl" style={{ margin: 0 }}>{detail.summary.name}</h1>
          <span className="of-chip" style={{ background: '#fef3c7', color: '#92400e' }}>Beta</span>
        </div>
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', fontSize: 13 }}>
          <span className="of-text-muted">Namespace: <code>{namespace}</code></span>
          <span className="of-text-muted">Format: v{detail.summary.format_version}</span>
          <span className="of-text-muted">Location: <code>{detail.summary.location}</code></span>
        </div>
      </header>

      <nav style={{ display: 'flex', flexWrap: 'wrap', gap: 4, borderBottom: '1px solid var(--border-default)' }}>
        {TABS.map((tab) => {
          const active = activeTab === tab.key;
          return (
            <button
              key={tab.key}
              type="button"
              onClick={() => setActiveTab(tab.key)}
              style={{
                padding: '8px 14px',
                background: 'transparent',
                border: 'none',
                borderBottom: `2px solid ${active ? '#1d4ed8' : 'transparent'}`,
                color: active ? 'var(--text-strong)' : 'var(--text-muted)',
                cursor: 'pointer',
                fontSize: 13,
                fontWeight: active ? 600 : 400,
              }}
            >
              {tab.label}
            </button>
          );
        })}
      </nav>

      {activeTab === 'overview' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Overview</p>
          <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
            {JSON.stringify(detail.summary, null, 2)}
          </pre>
        </section>
      )}

      {activeTab === 'schema' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Current schema</p>
          <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 480 }}>
            {JSON.stringify(detail.schema, null, 2)}
          </pre>
        </section>
      )}

      {activeTab === 'snapshots' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Snapshots</p>
          {snapshots ? (
            <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 13 }}>
              {snapshots.snapshots.map((s) => (
                <li key={s.snapshot_id}>
                  <strong>{s.snapshot_id}</strong> · {s.operation} · {s.timestamp ?? '—'}
                </li>
              ))}
            </ul>
          ) : (
            <p className="of-text-muted">Loading snapshots…</p>
          )}
        </section>
      )}

      {activeTab === 'metadata' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Metadata</p>
          {metadata && (
            <>
              <button type="button" onClick={downloadMetadata} className="of-button" style={{ marginTop: 8 }}>
                Download metadata.json
              </button>
              <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 480 }}>
                {JSON.stringify(metadata.metadata, null, 2)}
              </pre>
            </>
          )}
        </section>
      )}

      {activeTab === 'branches' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Branches and tags</p>
          {branches && (
            <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
              {JSON.stringify(branches.branches, null, 2)}
            </pre>
          )}
        </section>
      )}

      {activeTab === 'permissions' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Markings</p>
          {markingsError && <p role="alert" className="of-status-danger" style={{ padding: 8, fontSize: 13 }}>{markingsError}</p>}
          {tableMarkings && (
            <>
              <p className="of-text-muted" style={{ fontSize: 13, marginTop: 6 }}>Current: {tableMarkings.effective.map((m) => m.name || m.marking_id).join(', ') || '—'}</p>
              <input
                value={markingsDraft}
                onChange={(e) => setMarkingsDraft(e.target.value)}
                placeholder="Comma-separated markings"
                className="of-input"
                style={{ marginTop: 8 }}
              />
              <button type="button" onClick={() => void saveMarkings()} className="of-button of-button--primary" style={{ marginTop: 8 }}>
                Save markings
              </button>
            </>
          )}
        </section>
      )}

      {activeTab === 'activity' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Activity</p>
          <p className="of-text-muted" style={{ marginTop: 8 }}>
            Audit feed for this Iceberg table is sourced from the audit-compliance-service.
          </p>
        </section>
      )}

      {activeTab === 'catalog-access' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Catalog access</p>
          <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
            Snippets for connecting external Iceberg clients (PyIceberg, Spark, Trino, Snowflake).
          </p>
          <div style={{ display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' }}>
            <button type="button" onClick={() => void runDiagnose('pyiceberg')} disabled={diagnoseRunning} className="of-button">
              Diagnose PyIceberg
            </button>
            <button type="button" onClick={() => void runDiagnose('spark')} disabled={diagnoseRunning} className="of-button">
              Diagnose Spark
            </button>
            <button type="button" onClick={() => void generateToken()} disabled={issuingToken} className="of-button of-button--primary">
              {issuingToken ? 'Issuing…' : 'Issue API token'}
            </button>
          </div>
          {issuedToken && (
            <pre style={{ marginTop: 12, padding: 12, background: '#0c0a09', color: '#fde68a', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
              {issuedToken}
            </pre>
          )}
          {diagnoseResult && (
            <pre style={{ marginTop: 12, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
              {JSON.stringify(diagnoseResult, null, 2)}
            </pre>
          )}
        </section>
      )}
    </section>
  );
}
