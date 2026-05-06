import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';

import {
  capabilityChips,
  providerLabel,
  tableTypeLabel,
  virtualTables,
  type Capabilities,
  type TableType,
  type VirtualTable,
  type VirtualTableProvider,
} from '@/lib/api/virtual-tables';

const TABLE_TYPES: TableType[] = [
  'TABLE',
  'VIEW',
  'MATERIALIZED_VIEW',
  'EXTERNAL_DELTA',
  'MANAGED_DELTA',
  'MANAGED_ICEBERG',
  'PARQUET_FILES',
  'AVRO_FILES',
  'CSV_FILES',
  'OTHER',
];

function provider(row: VirtualTable): VirtualTableProvider | null {
  return (row.properties?.provider as VirtualTableProvider | undefined) ?? null;
}

function chips(caps: Capabilities | undefined): string[] {
  return caps ? capabilityChips(caps) : [];
}

export function VirtualTablesPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [items, setItems] = useState<VirtualTable[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [nextCursor, setNextCursor] = useState<string | null>(null);

  const [projectFilter, setProjectFilter] = useState(() => searchParams.get('project') ?? '');
  const [sourceFilter, setSourceFilter] = useState(() => searchParams.get('source') ?? '');
  const [nameFilter, setNameFilter] = useState(() => searchParams.get('name') ?? '');
  const [typeFilter, setTypeFilter] = useState<TableType | ''>(() => (searchParams.get('type') as TableType | '') ?? '');
  const [writableOnly, setWritableOnly] = useState(() => searchParams.get('writable') === '1');
  const [updateDetectionOnly, setUpdateDetectionOnly] = useState(() => searchParams.get('updates') === '1');

  function syncFiltersToUrl() {
    const params = new URLSearchParams();
    if (projectFilter) params.set('project', projectFilter);
    if (sourceFilter) params.set('source', sourceFilter);
    if (nameFilter) params.set('name', nameFilter);
    if (typeFilter) params.set('type', typeFilter);
    if (writableOnly) params.set('writable', '1');
    if (updateDetectionOnly) params.set('updates', '1');
    setSearchParams(params, { replace: true });
  }

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const response = await virtualTables.listVirtualTables({
        project: projectFilter || undefined,
        source: sourceFilter || undefined,
        name: nameFilter || undefined,
        type: (typeFilter || undefined) as TableType | undefined,
        limit: 100,
      });
      setItems(
        response.items.filter((row) => {
          if (writableOnly && !row.capabilities?.write) return false;
          if (updateDetectionOnly && !row.update_detection_enabled) return false;
          return true;
        }),
      );
      setNextCursor(response.next_cursor);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load virtual tables');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function applyFilters() {
    syncFiltersToUrl();
    void refresh();
  }

  return (
    <section
      className="of-page"
      data-testid="virtual-tables-page"
      style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 20 }}
    >
      <header>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 16 }}>
          <h1 className="of-heading-xl">Virtual tables</h1>
          <Link to="/data-connection" style={{ fontSize: 13, color: 'var(--text-muted)' }}>
            → Configure on a source
          </Link>
        </div>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Pointers to tables in supported data platforms (BigQuery, Snowflake, Databricks, S3 / GCS / ADLS, Foundry
          Iceberg). Foundry queries the source on demand — no copy.
        </p>
      </header>

      <div data-testid="virtual-tables-filters" style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center' }}>
        <input
          type="text"
          placeholder="Filter by project rid"
          value={projectFilter}
          onChange={(e) => setProjectFilter(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && applyFilters()}
          data-testid="vt-filter-project"
          className="of-input"
          style={{ width: 'auto' }}
        />
        <input
          type="text"
          placeholder="Filter by source rid"
          value={sourceFilter}
          onChange={(e) => setSourceFilter(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && applyFilters()}
          data-testid="vt-filter-source"
          className="of-input"
          style={{ width: 'auto' }}
        />
        <input
          type="text"
          placeholder="Filter by name"
          value={nameFilter}
          onChange={(e) => setNameFilter(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && applyFilters()}
          data-testid="vt-filter-name"
          className="of-input"
          style={{ width: 'auto' }}
        />
        <select
          value={typeFilter}
          onChange={(e) => {
            setTypeFilter(e.target.value as TableType | '');
            applyFilters();
          }}
          data-testid="vt-filter-type"
          className="of-input"
          style={{ width: 'auto' }}
        >
          <option value="">All table types</option>
          {TABLE_TYPES.map((type) => (
            <option key={type} value={type}>
              {tableTypeLabel(type)}
            </option>
          ))}
        </select>
        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 13 }}>
          <input
            type="checkbox"
            checked={writableOnly}
            onChange={(e) => {
              setWritableOnly(e.target.checked);
              applyFilters();
            }}
          />
          Writable only
        </label>
        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 13 }}>
          <input
            type="checkbox"
            checked={updateDetectionOnly}
            onChange={(e) => {
              setUpdateDetectionOnly(e.target.checked);
              applyFilters();
            }}
            data-testid="vt-filter-updates"
          />
          Update detection on
        </label>
        <button type="button" onClick={applyFilters} className="of-button">
          Apply
        </button>
      </div>

      {loading ? (
        <div className="of-panel" style={{ padding: 16, color: 'var(--text-muted)' }}>
          Loading…
        </div>
      ) : error ? (
        <div className="of-status-danger" role="alert" data-testid="vt-error" style={{ padding: 16, borderRadius: 'var(--radius-md)' }}>
          {error}
        </div>
      ) : items.length === 0 ? (
        <div className="of-panel" data-testid="vt-empty" style={{ padding: 24, textAlign: 'center' }}>
          <p>No virtual tables yet.</p>
          <p className="of-text-muted">
            Open a Data Connection source from a supported provider (BigQuery, Snowflake, Databricks, S3 / GCS / ADLS)
            and use the <strong>Virtual tables</strong> tab to register one.
          </p>
          <Link to="/data-connection" style={{ marginTop: 8, color: '#1d4ed8', display: 'inline-block' }}>
            Go to Data Connection sources →
          </Link>
        </div>
      ) : (
        <>
          <table data-testid="virtual-tables-grid" style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead style={{ background: 'var(--bg-subtle)' }}>
              <tr>
                {['Name', 'Source', 'Provider', 'Type', 'Capabilities', 'Project', 'Markings', 'Created'].map((h) => (
                  <th key={h} style={{ textAlign: 'left', padding: '10px 14px', borderBottom: '1px solid var(--border-default)', fontWeight: 600 }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {items.map((row) => (
                <tr key={row.rid}>
                  <td style={{ padding: '10px 14px', borderBottom: '1px solid var(--border-default)' }}>
                    <Link to={`/virtual-tables/${encodeURIComponent(row.rid)}`} data-testid="vt-row-link">
                      {row.name}
                    </Link>
                  </td>
                  <td style={{ padding: '10px 14px', borderBottom: '1px solid var(--border-default)' }}>
                    <Link
                      to={`/data-connection/sources/${encodeURIComponent(row.source_rid)}`}
                      style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}
                      title={row.source_rid}
                    >
                      {row.source_rid}
                    </Link>
                  </td>
                  <td style={{ padding: '10px 14px', borderBottom: '1px solid var(--border-default)' }}>
                    {provider(row) ? <span className="of-chip">{providerLabel(provider(row)!)}</span> : <span className="of-text-muted">—</span>}
                  </td>
                  <td style={{ padding: '10px 14px', borderBottom: '1px solid var(--border-default)' }}>
                    {tableTypeLabel(row.table_type)}
                  </td>
                  <td style={{ padding: '10px 14px', borderBottom: '1px solid var(--border-default)' }}>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                      {chips(row.capabilities).map((chip) => (
                        <span key={chip} className="of-chip" data-testid="vt-cap-chip">
                          {chip}
                        </span>
                      ))}
                      {row.update_detection_enabled && (
                        <span className="of-chip" style={{ background: '#fef9c3', color: '#854d0e' }}>
                          Update detection
                        </span>
                      )}
                    </div>
                  </td>
                  <td style={{ padding: '10px 14px', borderBottom: '1px solid var(--border-default)', fontFamily: 'var(--font-mono)', fontSize: 11 }}>
                    {row.project_rid}
                  </td>
                  <td style={{ padding: '10px 14px', borderBottom: '1px solid var(--border-default)' }}>
                    {row.markings.map((marking) => (
                      <span key={marking} className="of-chip" style={{ marginRight: 4 }}>
                        {marking}
                      </span>
                    ))}
                  </td>
                  <td style={{ padding: '10px 14px', borderBottom: '1px solid var(--border-default)', fontSize: 11 }}>
                    {new Date(row.created_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {nextCursor && (
            <div style={{ textAlign: 'center', padding: '8px 0' }}>
              <span className="of-text-muted">More results available — refine filters to load.</span>
            </div>
          )}
        </>
      )}
    </section>
  );
}
