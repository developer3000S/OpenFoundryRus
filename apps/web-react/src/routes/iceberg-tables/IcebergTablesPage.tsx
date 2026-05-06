import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { listIcebergTables, type IcebergTableSummary } from '@/lib/api/icebergTables';

function formatNamespace(parts: string[]) {
  return parts.length ? parts.join('.') : '(root)';
}

function formatRowCount(value: number | null): string {
  if (value === null) return '—';
  if (value > 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value > 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return value.toString();
}

function formatRelativeTime(iso: string | null): string {
  if (!iso) return '—';
  const ts = new Date(iso).getTime();
  const diff = Date.now() - ts;
  const minutes = Math.floor(diff / 60_000);
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

export function IcebergTablesPage() {
  const [tables, setTables] = useState<IcebergTableSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [projectFilter, setProjectFilter] = useState('');
  const [namespaceFilter, setNamespaceFilter] = useState('');
  const [nameFilter, setNameFilter] = useState('');
  const [sortField, setSortField] = useState<'name' | 'created_at' | ''>('');

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const response = await listIcebergTables({
        project_rid: projectFilter || undefined,
        namespace: namespaceFilter || undefined,
        name: nameFilter || undefined,
        sort: sortField || undefined,
      });
      setTables(response.tables);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load Iceberg tables');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <section className="of-page" style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 20 }}>
      <header>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <h1 className="of-heading-xl">Iceberg tables</h1>
          <span className="of-chip" data-testid="iceberg-beta-banner" style={{ background: '#fef3c7', color: '#92400e' }}>
            Beta
          </span>
        </div>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Apache Iceberg tables exposed via Foundry's REST Catalog. External clients (PyIceberg, Spark, Trino,
          Snowflake) authenticate via OAuth2 or bearer tokens and read straight from object storage.
        </p>
      </header>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        <input
          type="text"
          placeholder="Filter by project rid"
          value={projectFilter}
          onChange={(e) => setProjectFilter(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && refresh()}
          data-testid="iceberg-filter-project"
          className="of-input"
          style={{ width: 'auto' }}
        />
        <input
          type="text"
          placeholder="Filter by namespace"
          value={namespaceFilter}
          onChange={(e) => setNamespaceFilter(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && refresh()}
          data-testid="iceberg-filter-namespace"
          className="of-input"
          style={{ width: 'auto' }}
        />
        <input
          type="text"
          placeholder="Filter by name"
          value={nameFilter}
          onChange={(e) => setNameFilter(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && refresh()}
          data-testid="iceberg-filter-name"
          className="of-input"
          style={{ width: 'auto' }}
        />
        <select
          value={sortField}
          onChange={(e) => {
            setSortField(e.target.value as 'name' | 'created_at' | '');
            void refresh();
          }}
          data-testid="iceberg-sort"
          className="of-input"
          style={{ width: 'auto' }}
        >
          <option value="">Sort: most recent</option>
          <option value="name">Sort: name</option>
          <option value="created_at">Sort: created at</option>
        </select>
        <button type="button" onClick={() => void refresh()} className="of-button">
          Apply
        </button>
      </div>

      {loading ? (
        <div className="of-panel" style={{ padding: 16, color: 'var(--text-muted)' }}>Loading…</div>
      ) : error ? (
        <div className="of-status-danger" role="alert" style={{ padding: 16 }}>{error}</div>
      ) : tables.length === 0 ? (
        <div className="of-panel" style={{ padding: 16, color: 'var(--text-muted)' }}>
          No Iceberg tables match the current filters.
        </div>
      ) : (
        <table data-testid="iceberg-tables-grid" style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead style={{ background: 'var(--bg-subtle)' }}>
            <tr>
              {['Namespace', 'Name', 'Format', 'Rows (est.)', 'Last snapshot', 'Location', 'Markings'].map((h) => (
                <th key={h} style={{ textAlign: 'left', padding: '8px 12px', borderBottom: '1px solid var(--border-default)', fontWeight: 600 }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {tables.map((table) => (
              <tr key={table.id}>
                <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)' }}>{formatNamespace(table.namespace)}</td>
                <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)' }}>
                  <Link to={`/iceberg-tables/${table.id}`} data-testid="iceberg-table-link">{table.name}</Link>
                </td>
                <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)' }}>v{table.format_version}</td>
                <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)' }}>{formatRowCount(table.row_count_estimate)}</td>
                <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)' }}>{formatRelativeTime(table.last_snapshot_at)}</td>
                <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)', fontFamily: 'var(--font-mono)', maxWidth: '28ch', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={table.location}>{table.location}</td>
                <td style={{ padding: '8px 12px', borderBottom: '1px solid var(--border-default)' }}>
                  {table.markings.map((marking) => (
                    <span key={marking} className="of-chip" style={{ marginRight: 4 }}>{marking}</span>
                  ))}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
