import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import { VirtualTableDetailsPanel } from '@/lib/components/data-connection/VirtualTableDetailsPanel';
import {
  capabilityChips,
  providerLabel,
  tableTypeLabel,
  virtualTables,
  type VirtualTable,
  type VirtualTableProvider,
} from '@/lib/api/virtual-tables';

type Tab = 'overview' | 'schema' | 'lineage' | 'permissions' | 'activity' | 'update-detection' | 'imports';

const TABS: Array<{ id: Tab; label: string; deferred?: boolean }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'schema', label: 'Schema' },
  { id: 'lineage', label: 'Lineage', deferred: true },
  { id: 'permissions', label: 'Permissions' },
  { id: 'activity', label: 'Activity', deferred: true },
  { id: 'update-detection', label: 'Update detection' },
  { id: 'imports', label: 'Imports', deferred: true },
];

export function VirtualTableDetailPage() {
  const params = useParams();
  const navigate = useNavigate();
  const rid = decodeURIComponent(params.rid ?? '');

  const [activeTab, setActiveTab] = useState<Tab>('overview');
  const [row, setRow] = useState<VirtualTable | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState<'refresh' | 'delete' | null>(null);
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError('');
      try {
        const table = await virtualTables.getVirtualTable(rid);
        if (!cancelled) setRow(table);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load virtual table');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [rid]);

  function provider(): VirtualTableProvider | null {
    return (row?.properties?.provider as VirtualTableProvider | undefined) ?? null;
  }

  async function refreshSchema() {
    if (!row) return;
    setBusy('refresh');
    try {
      setRow(await virtualTables.refreshSchema(row.rid));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to refresh schema');
    } finally {
      setBusy(null);
    }
  }

  async function confirmDelete() {
    if (!row) return;
    setBusy('delete');
    try {
      await virtualTables.deleteVirtualTable(row.rid);
      navigate('/virtual-tables');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete virtual table');
    } finally {
      setBusy(null);
      setConfirmingDelete(false);
    }
  }

  return (
    <section className="of-page" data-testid="vt-detail-page" style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 16 }}>
      {loading ? (
        <div className="of-panel" style={{ padding: 16, color: 'var(--text-muted)' }}>
          Loading…
        </div>
      ) : error ? (
        <div className="of-status-danger" role="alert" data-testid="vt-detail-error" style={{ padding: 16, borderRadius: 'var(--radius-md)' }}>
          {error}
        </div>
      ) : row ? (
        <>
          <header style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Link to="/virtual-tables" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
              ← All virtual tables
            </Link>
            <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
              <h1 className="of-heading-xl" style={{ margin: 0 }}>
                {row.name}
              </h1>
              <span className="of-chip" style={{ background: '#1d4ed8', color: '#fff' }}>
                Virtual table
              </span>
              {provider() && <span className="of-chip">{providerLabel(provider()!)}</span>}
              <span className="of-chip">{tableTypeLabel(row.table_type)}</span>
              {capabilityChips(row.capabilities).map((chip) => (
                <span key={chip} className="of-chip" data-testid="vt-detail-cap-chip">
                  {chip}
                </span>
              ))}
            </div>
            <div style={{ display: 'flex', gap: 8, marginTop: 4, flexWrap: 'wrap' }}>
              <button
                type="button"
                onClick={() => void refreshSchema()}
                disabled={busy !== null}
                className="of-button"
                data-testid="vt-action-refresh-schema"
              >
                {busy === 'refresh' ? 'Refreshing…' : 'Refresh schema'}
              </button>
              <button type="button" disabled title="Activated in P5" className="of-button">
                Open in Pipeline Builder
              </button>
              <button type="button" disabled title="Activated in P6" className="of-button">
                Open in Contour
              </button>
              <button
                type="button"
                disabled={busy !== null}
                onClick={() => setConfirmingDelete(true)}
                className="of-button"
                style={{ color: '#b91c1c', borderColor: '#fecaca' }}
                data-testid="vt-action-delete"
              >
                Delete
              </button>
            </div>
          </header>

          <nav data-testid="vt-detail-tabs" style={{ display: 'flex', gap: 4, borderBottom: '1px solid var(--border-default)' }}>
            {TABS.map((tab) => {
              const active = tab.id === activeTab;
              return (
                <button
                  key={tab.id}
                  type="button"
                  onClick={() => setActiveTab(tab.id)}
                  data-testid={`vt-tab-${tab.id}`}
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
                  {tab.deferred && <span style={{ marginLeft: 6, fontSize: 10, color: 'var(--text-muted)' }}>soon</span>}
                </button>
              );
            })}
          </nav>

          <div>
            {activeTab === 'overview' && (
              <section style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))' }}>
                <article className="of-panel" style={{ padding: 14 }}>
                  <h3 style={{ margin: 0, fontSize: 14 }}>Source</h3>
                  <Link to={`/data-connection/sources/${encodeURIComponent(row.source_rid)}`} style={{ fontFamily: 'var(--font-mono)', marginTop: 6, display: 'inline-block' }} data-testid="vt-overview-source-link">
                    {row.source_rid}
                  </Link>
                </article>
                <article className="of-panel" style={{ padding: 14 }}>
                  <h3 style={{ margin: 0, fontSize: 14 }}>Project</h3>
                  <span style={{ fontFamily: 'var(--font-mono)', marginTop: 6, display: 'inline-block' }} data-testid="vt-overview-project">
                    {row.project_rid}
                  </span>
                </article>
                <article className="of-panel" style={{ padding: 14 }}>
                  <h3 style={{ margin: 0, fontSize: 14 }}>Locator</h3>
                  <pre data-testid="vt-overview-locator" style={{ marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 11, overflow: 'auto' }}>
                    {JSON.stringify(row.locator, null, 2)}
                  </pre>
                </article>
                <article className="of-panel" style={{ padding: 14 }}>
                  <h3 style={{ margin: 0, fontSize: 14 }}>Capabilities</h3>
                  <ul style={{ marginTop: 6, paddingLeft: 16, fontSize: 13 }}>
                    <li>Read: {row.capabilities.read ? 'yes' : 'no'}</li>
                    <li>Write: {row.capabilities.write ? 'yes' : 'no'}</li>
                    <li>Incremental: {row.capabilities.incremental ? 'yes' : 'no'}</li>
                    <li>Versioning: {row.capabilities.versioning ? 'yes' : 'no'}</li>
                    <li>Compute pushdown: {row.capabilities.compute_pushdown ?? '—'}</li>
                    <li>Foundry compute (Python single-node): {row.capabilities.foundry_compute.python_single_node ? 'yes' : 'no'}</li>
                    <li>Foundry compute (Python Spark): {row.capabilities.foundry_compute.python_spark ? 'yes' : 'no'}</li>
                    <li>Foundry compute (PB Spark): {row.capabilities.foundry_compute.pipeline_builder_spark ? 'yes' : 'no'}</li>
                  </ul>
                </article>
                <article className="of-panel" style={{ padding: 14 }}>
                  <h3 style={{ margin: 0, fontSize: 14 }}>Update detection</h3>
                  <ul style={{ marginTop: 6, paddingLeft: 16, fontSize: 13 }}>
                    <li>Enabled: {row.update_detection_enabled ? 'yes' : 'no'}</li>
                    <li>Interval: {row.update_detection_interval_seconds ?? '—'}{row.update_detection_interval_seconds ? 's' : ''}</li>
                    <li>Last polled at: {row.last_polled_at ?? '—'}</li>
                    <li>Last observed version: {row.last_observed_version ?? '—'}</li>
                  </ul>
                </article>
              </section>
            )}

            {activeTab === 'schema' && (
              <section>
                {row.schema_inferred.length === 0 ? (
                  <p className="of-text-muted">
                    Schema inference returned no columns. Refresh the schema once the source registration completes, or
                    check <code>properties.warnings</code> for upstream messages.
                  </p>
                ) : (
                  <table data-testid="vt-schema-grid" style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
                    <thead style={{ background: 'var(--bg-subtle)' }}>
                      <tr>
                        {['Column', 'Inferred type', 'Source type', 'Nullable'].map((h) => (
                          <th key={h} style={{ textAlign: 'left', padding: '8px 12px', borderBottom: '1px solid var(--border-default)', fontWeight: 600 }}>
                            {h}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {row.schema_inferred.map((col) => (
                        <tr key={col.name}>
                          <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)', fontFamily: 'var(--font-mono)' }}>{col.name}</td>
                          <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)' }}>{col.inferred_type}</td>
                          <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)', fontFamily: 'var(--font-mono)', fontSize: 11 }}>{col.source_type}</td>
                          <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)' }}>{col.nullable ? 'yes' : 'no'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </section>
            )}

            {activeTab === 'permissions' && (
              <section>
                <h3>Markings</h3>
                {row.markings.length === 0 ? (
                  <p className="of-text-muted">
                    No explicit markings. The virtual table inherits the source's markings as a clearance floor (see
                    ADR-NNNN). Update via <code>PATCH /v1/virtual-tables/{row.rid}/markings</code>.
                  </p>
                ) : (
                  <ul>
                    {row.markings.map((marking) => (
                      <li key={marking}>{marking}</li>
                    ))}
                  </ul>
                )}
              </section>
            )}

            {activeTab === 'lineage' && <p className="of-text-muted">Lineage view ships with P6 — pipeline integration.</p>}
            {activeTab === 'activity' && (
              <p className="of-text-muted">
                Audit events for this virtual table are persisted in <code>virtual_table_audit</code> and emitted to{' '}
                <code>audit-compliance-service</code>. The viewer wires up in P3.next.
              </p>
            )}
            {activeTab === 'update-detection' && (
              <VirtualTableDetailsPanel table={row} onChanged={(next) => setRow(next)} />
            )}
            {activeTab === 'imports' && (
              <p className="of-text-muted">
                Imports list ships with P3.next once the <code>virtual_table_imports</code> endpoint is exposed.
              </p>
            )}
          </div>
        </>
      ) : null}

      {confirmingDelete && (
        <div
          role="dialog"
          aria-modal="true"
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(0,0,0,0.4)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 100,
          }}
        >
          <div className="of-panel" style={{ padding: 24, maxWidth: 420 }}>
            <h3 className="of-heading-md">Delete virtual table</h3>
            <p style={{ marginTop: 8, fontSize: 13 }}>
              This removes the pointer in Foundry. The remote source table is not touched. Imports of this virtual
              table into other projects will be removed in cascade.
            </p>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16 }}>
              <button type="button" onClick={() => setConfirmingDelete(false)} className="of-button">
                Cancel
              </button>
              <button
                type="button"
                onClick={() => void confirmDelete()}
                disabled={busy === 'delete'}
                className="of-button"
                style={{ color: '#b91c1c', borderColor: '#fecaca' }}
                data-testid="vt-confirm-delete"
              >
                {busy === 'delete' ? 'Deleting…' : 'Delete'}
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
