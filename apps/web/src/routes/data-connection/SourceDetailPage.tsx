import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { Tabs } from '@/lib/components/Tabs';
import {
  capabilityLabel,
  dataConnection,
  FALLBACK_CONNECTOR_CATALOG,
  type BatchSyncDef,
  type ConnectorCatalogEntry,
  type Credential,
  type CredentialKind,
  type MediaSetSyncDef,
  type NetworkEgressPolicy,
  type Source,
  type SyncRun,
  type TestConnectionResult,
} from '@/lib/api/data-connection';

type Tab = 'overview' | 'networking' | 'credentials' | 'capabilities' | 'runs' | 'media-syncs';

const MEDIA_SYNC_CONNECTORS = new Set(['s3', 'onelake', 'abfs']);

export function SourceDetailPage() {
  const { id = '' } = useParams<{ id: string }>();
  const [tab, setTab] = useState<Tab>('overview');
  const [source, setSource] = useState<Source | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // networking
  const [attached, setAttached] = useState<NetworkEgressPolicy[]>([]);
  const [available, setAvailable] = useState<NetworkEgressPolicy[]>([]);
  const [pickPolicyId, setPickPolicyId] = useState('');

  // credentials
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [credKind, setCredKind] = useState<CredentialKind>('api_key');
  const [credValue, setCredValue] = useState('');

  // test
  const [testResult, setTestResult] = useState<TestConnectionResult | null>(null);

  // syncs / runs
  const [syncs, setSyncs] = useState<BatchSyncDef[]>([]);
  const [runsBySync, setRunsBySync] = useState<Record<string, SyncRun[]>>({});
  const [newOutputDataset, setNewOutputDataset] = useState('');
  const [newFileGlob, setNewFileGlob] = useState('');
  const [newScheduleCron, setNewScheduleCron] = useState('');

  // media-syncs
  const [mediaSyncs, setMediaSyncs] = useState<MediaSetSyncDef[]>([]);

  const catalogEntry: ConnectorCatalogEntry | undefined = source
    ? FALLBACK_CONNECTOR_CATALOG.find((e) => e.type === source.connector_type)
    : undefined;

  async function loadOverview() {
    setLoading(true);
    setError('');
    try {
      setSource(await dataConnection.getSource(id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load source');
    } finally {
      setLoading(false);
    }
  }

  async function loadNetworking() {
    try {
      const [att, all] = await Promise.all([
        dataConnection.listSourcePolicies(id),
        dataConnection.listEgressPolicies(),
      ]);
      setAttached(att);
      const attachedIds = new Set(att.map((p) => p.id));
      setAvailable(all.filter((p) => !attachedIds.has(p.id)));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load networking');
    }
  }

  async function loadCredentials() {
    try {
      setCredentials(await dataConnection.listCredentials(id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load credentials');
    }
  }

  async function loadSyncs() {
    try {
      const list = await dataConnection.listSyncs(id);
      setSyncs(list);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load syncs');
    }
  }

  async function loadRuns(syncId: string) {
    try {
      setRunsBySync((prev) => ({ ...prev, [syncId]: [] }));
      const runs = await dataConnection.listRuns(syncId);
      setRunsBySync((prev) => ({ ...prev, [syncId]: runs }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load runs');
    }
  }

  async function loadMediaSyncs() {
    try {
      setMediaSyncs(await dataConnection.listMediaSetSyncs(id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load media syncs');
    }
  }

  useEffect(() => {
    if (id) void loadOverview();
  }, [id]);

  function selectTab(next: Tab) {
    setTab(next);
    if (next === 'networking') void loadNetworking();
    if (next === 'credentials') void loadCredentials();
    if (next === 'runs') void loadSyncs();
    if (next === 'media-syncs') void loadMediaSyncs();
  }

  async function deleteSource() {
    if (typeof window !== 'undefined' && !window.confirm('Delete source?')) return;
    setBusy(true);
    try {
      await dataConnection.deleteSource(id);
      window.location.href = '/data-connection';
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
      setBusy(false);
    }
  }

  async function testConnection() {
    setBusy(true);
    try {
      setTestResult(await dataConnection.testConnection(id));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Test failed');
    } finally {
      setBusy(false);
    }
  }

  async function attachPolicy() {
    if (!pickPolicyId) return;
    setBusy(true);
    try {
      await dataConnection.attachPolicy(id, pickPolicyId);
      setPickPolicyId('');
      await loadNetworking();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Attach failed');
    } finally {
      setBusy(false);
    }
  }

  async function detachPolicy(policyId: string) {
    setBusy(true);
    try {
      await dataConnection.detachPolicy(id, policyId);
      await loadNetworking();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Detach failed');
    } finally {
      setBusy(false);
    }
  }

  async function setCredential() {
    setBusy(true);
    try {
      await dataConnection.setCredential(id, { kind: credKind, value: credValue });
      setCredValue('');
      await loadCredentials();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Set credential failed');
    } finally {
      setBusy(false);
    }
  }

  async function createSync() {
    setBusy(true);
    try {
      await dataConnection.createSync({
        source_id: id,
        output_dataset_id: newOutputDataset,
        file_glob: newFileGlob || undefined,
        schedule_cron: newScheduleCron || undefined,
      });
      setNewOutputDataset('');
      setNewFileGlob('');
      setNewScheduleCron('');
      await loadSyncs();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create sync failed');
    } finally {
      setBusy(false);
    }
  }

  async function runSync(syncId: string) {
    setBusy(true);
    try {
      await dataConnection.runSync(syncId);
      await loadRuns(syncId);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Run sync failed');
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <p className="of-text-muted">Loading source…</p>
      </section>
    );
  }

  if (!source) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/data-connection" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Sources</Link>
        <p className="of-status-danger" style={{ marginTop: 12 }}>{error || 'Source not found'}</p>
      </section>
    );
  }

  const tabs: Tab[] = ['overview', 'networking', 'credentials', 'capabilities', 'runs'];
  if (MEDIA_SYNC_CONNECTORS.has(source.connector_type)) tabs.push('media-syncs');

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/data-connection" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Sources</Link>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">{source.name}</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
            {source.id} · {source.connector_type} · worker: {source.worker} · status: {source.status}
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button type="button" onClick={() => void testConnection()} disabled={busy} className="of-button">Test connection</button>
          <button type="button" onClick={() => void deleteSource()} disabled={busy} className="of-button" style={{ color: '#b91c1c', borderColor: '#fecaca' }}>
            Delete
          </button>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {testResult && (
        <div style={{ padding: 10, background: testResult.success ? '#d1fae5' : '#fee2e2', borderRadius: 8, fontSize: 12 }}>
          <strong>{testResult.success ? '✓' : '✗'}</strong> {testResult.message}
          {testResult.latency_ms !== null && ` · ${testResult.latency_ms}ms`}
        </div>
      )}

      <Tabs tabs={tabs} active={tab} onChange={selectTab} />

      {tab === 'overview' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <pre style={{ padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
            {JSON.stringify(source, null, 2)}
          </pre>
        </section>
      )}

      {tab === 'networking' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Attached policies ({attached.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
            {attached.map((p) => (
              <li key={p.id} style={{ padding: 8, borderBottom: '1px solid var(--border-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span>
                  <strong>{p.name}</strong> · <code>{p.address.kind}:{p.address.value}</code>
                </span>
                <button type="button" onClick={() => void detachPolicy(p.id)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                  Detach
                </button>
              </li>
            ))}
            {attached.length === 0 && <li className="of-text-muted">No attached policies.</li>}
          </ul>
          <div style={{ marginTop: 12, display: 'flex', gap: 6 }}>
            <select value={pickPolicyId} onChange={(e) => setPickPolicyId(e.target.value)} className="of-input">
              <option value="">— pick policy —</option>
              {available.map((p) => (
                <option key={p.id} value={p.id}>{p.name} · {p.kind}</option>
              ))}
            </select>
            <button type="button" onClick={() => void attachPolicy()} disabled={busy || !pickPolicyId} className="of-button of-button--primary">
              Attach
            </button>
          </div>
        </section>
      )}

      {tab === 'credentials' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Credentials ({credentials.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
            {credentials.map((c) => (
              <li key={c.id}>
                {c.kind} · fingerprint <code>{c.fingerprint}</code> · {new Date(c.created_at).toLocaleString()}
              </li>
            ))}
            {credentials.length === 0 && <li className="of-text-muted">No credentials stored.</li>}
          </ul>
          <div style={{ marginTop: 12, display: 'grid', gap: 6, maxWidth: 480 }}>
            <label style={{ fontSize: 13 }}>
              Kind
              <select value={credKind} onChange={(e) => setCredKind(e.target.value as CredentialKind)} className="of-input" style={{ marginTop: 4 }}>
                {(['password', 'api_key', 'oauth_token', 'aws_keys', 'service_account_json'] as CredentialKind[]).map((k) => (
                  <option key={k} value={k}>{k}</option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13 }}>
              Value (write-only)
              <input type="password" value={credValue} onChange={(e) => setCredValue(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <button type="button" onClick={() => void setCredential()} disabled={busy || !credValue} className="of-button of-button--primary">
              Save credential
            </button>
          </div>
        </section>
      )}

      {tab === 'capabilities' && (
        <section className="of-panel" style={{ padding: 16 }}>
          {catalogEntry ? (
            <>
              <p className="of-eyebrow">{catalogEntry.name}</p>
              <p className="of-text-muted" style={{ fontSize: 12 }}>{catalogEntry.description}</p>
              <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap', marginTop: 8 }}>
                {catalogEntry.capabilities.map((c) => (
                  <span key={c} style={{ fontSize: 10, padding: '2px 6px', background: 'var(--bg-subtle)', borderRadius: 999 }}>{capabilityLabel(c)}</span>
                ))}
              </div>
            </>
          ) : (
            <p className="of-text-muted">No catalog entry for connector type {source.connector_type}.</p>
          )}
        </section>
      )}

      {tab === 'runs' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Create batch sync</p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
              <input value={newOutputDataset} onChange={(e) => setNewOutputDataset(e.target.value)} placeholder="output dataset id" className="of-input" />
              <input value={newFileGlob} onChange={(e) => setNewFileGlob(e.target.value)} placeholder="file_glob (optional)" className="of-input" />
              <input value={newScheduleCron} onChange={(e) => setNewScheduleCron(e.target.value)} placeholder="cron (optional)" className="of-input" />
              <button type="button" onClick={() => void createSync()} disabled={busy || !newOutputDataset} className="of-button of-button--primary">
                Create sync
              </button>
            </div>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Syncs ({syncs.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {syncs.map((s) => (
                <li key={s.id} style={{ padding: 10, borderBottom: '1px solid var(--border-subtle)' }}>
                  <strong>{s.id}</strong> → {s.output_dataset_id}
                  {s.file_glob && <> · glob: <code>{s.file_glob}</code></>}
                  {s.schedule_cron && <> · cron: <code>{s.schedule_cron}</code></>}
                  <div style={{ display: 'flex', gap: 6, marginTop: 6 }}>
                    <button type="button" onClick={() => void runSync(s.id)} disabled={busy} className="of-button" style={{ fontSize: 11 }}>
                      Run now
                    </button>
                    <button type="button" onClick={() => void loadRuns(s.id)} disabled={busy} className="of-button" style={{ fontSize: 11 }}>
                      Refresh runs
                    </button>
                  </div>
                  {runsBySync[s.id] && (
                    <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 11 }}>
                      {runsBySync[s.id].map((r) => (
                        <li key={r.id}>
                          {r.status} · {new Date(r.started_at).toLocaleString()} · {r.bytes_written} bytes · {r.files_written} files
                          {r.error && ` · ${r.error}`}
                        </li>
                      ))}
                      {runsBySync[s.id].length === 0 && <li className="of-text-muted">No runs.</li>}
                    </ul>
                  )}
                </li>
              ))}
              {syncs.length === 0 && <li className="of-text-muted">No syncs yet.</li>}
            </ul>
          </section>
        </>
      )}

      {tab === 'media-syncs' && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Media set syncs ({mediaSyncs.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
            {mediaSyncs.map((m) => (
              <li key={m.id}>
                {m.id} · {m.kind} · target {m.target_media_set_rid} · subfolder <code>{m.subfolder || '/'}</code>
              </li>
            ))}
            {mediaSyncs.length === 0 && <li className="of-text-muted">No media syncs configured.</li>}
          </ul>
        </section>
      )}
    </section>
  );
}
