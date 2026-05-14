import { useEffect, useMemo, useState } from 'react';
import { Link, NavLink, useNavigate } from 'react-router-dom';

import { ConfirmDialog } from '@/lib/components/ConfirmDialog';
import { Pagination } from '@/lib/components/Pagination';
import { CreateDatasetModal } from '@/lib/components/dataset/CreateDatasetModal';
import { DatasetCatalogFacets } from '@/lib/components/dataset/DatasetCatalogFacets';
import { Glyph } from '@/lib/components/ui/Glyph';
import {
  deleteDataset,
  getCatalogFacets,
  listDatasets,
  type Dataset,
  type DatasetCatalogFacets as DatasetCatalogFacetsResponse,
} from '@/lib/api/datasets';

const PAGE_SIZE = 20;

type CatalogTab = 'collections' | 'files';

interface DatasetFilters {
  search: string;
  tag: string;
  owner: string;
  format: string;
}

const EMPTY_FACETS: DatasetCatalogFacetsResponse = { tags: [], owners: [] };

export function DatasetsListPage() {
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [facets, setFacets] = useState<DatasetCatalogFacetsResponse>(EMPTY_FACETS);
  const [search, setSearch] = useState('');
  const [tag, setTag] = useState('');
  const [owner, setOwner] = useState('');
  const [format, setFormat] = useState('');
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Dataset | null>(null);
  const [activeTab, setActiveTab] = useState<CatalogTab>('files');
  const navigate = useNavigate();

  const filters = useMemo<DatasetFilters>(() => ({ search, tag, owner, format }), [search, tag, owner, format]);

  const formatFacets = useMemo(() => {
    const counts = new Map<string, number>();
    datasets.forEach((dataset) => {
      const key = (dataset.format || 'unknown').toLowerCase();
      counts.set(key, (counts.get(key) ?? 0) + 1);
    });
    return Array.from(counts.entries())
      .map(([value, count]) => ({ value, count }))
      .sort((a, b) => b.count - a.count);
  }, [datasets]);

  const summary = useMemo(() => {
    const visibleRows = datasets.reduce((sum, dataset) => sum + (dataset.row_count ?? 0), 0);
    const visibleBytes = datasets.reduce((sum, dataset) => sum + (dataset.size_bytes ?? 0), 0);
    const healthyVisible = datasets.filter((dataset) => dataset.health_status === 'healthy').length;
    return [
      { label: 'Datasets', value: total.toLocaleString(), hint: `${datasets.length.toLocaleString()} visible` },
      { label: 'Tags indexed', value: facets.tags.length.toLocaleString(), hint: `${facets.owners.length.toLocaleString()} owners` },
      { label: 'Healthy visible', value: healthyVisible.toLocaleString(), hint: 'Latest health snapshot' },
      { label: 'Rows visible', value: compactNumber(visibleRows), hint: formatBytes(visibleBytes) },
    ];
  }, [datasets, facets.owners.length, facets.tags.length, total]);

  async function load(requestedPage = page, requestedFilters = filters) {
    setLoading(true);
    setError('');
    try {
      const [res, catalogFacets] = await Promise.all([
        listDatasets({
          page: requestedPage,
          per_page: PAGE_SIZE,
          search: requestedFilters.search || undefined,
          tag: requestedFilters.tag || undefined,
          owner_id: requestedFilters.owner || undefined,
        }),
        getCatalogFacets().catch(() => EMPTY_FACETS),
      ]);
      setDatasets(res.data);
      setTotal(res.total);
      setFacets(catalogFacets);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load datasets');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load(page, filters);
  }, [page]);

  function applyFilters(nextFilters = filters) {
    setSearch(nextFilters.search);
    setTag(nextFilters.tag);
    setOwner(nextFilters.owner);
    setFormat(nextFilters.format);
    if (page === 1) {
      void load(1, nextFilters);
    } else {
      setPage(1);
    }
  }

  function resetFilters() {
    applyFilters({ search: '', tag: '', owner: '', format: '' });
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    setBusy(true);
    setError('');
    try {
      await deleteDataset(deleteTarget.id);
      setDeleteTarget(null);
      const nextPage = datasets.length === 1 && page > 1 ? page - 1 : page;
      if (nextPage !== page) {
        setPage(nextPage);
      } else {
        await load(nextPage, filters);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  const filteredByFormat = useMemo(() => {
    if (!format) return datasets;
    return datasets.filter((dataset) => (dataset.format || 'unknown').toLowerCase() === format);
  }, [datasets, format]);

  const collections = useMemo(() => groupByPrimaryTag(filteredByFormat), [filteredByFormat]);
  const activeFilterCount = [search, tag, owner, format].filter(Boolean).length;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <section className="of-catalog-page">
      <nav className="of-catalog-subnav" aria-label="Catalog sections">
        <CatalogSubnavItem to="/datasets" icon="folder-open" label="Files" active={false} title="All files" />
        <CatalogSubnavItem to="/datasets" icon="badge-check" label="Data Catalog" active title="Data Catalog" highlightIcon />
        <CatalogSubnavItem to="/projects" icon="bookmark" label="Portfolios" active={false} />
        <CatalogSubnavItem to="/projects" icon="project" label="Projects" active={false} />
        <CatalogSubnavItem to="/workspace" icon="users" label="Your files" active={false} />
        <CatalogSubnavItem to="/workspace" icon="add-user" label="Shared with you" active={false} />
        <span className="of-catalog-subnav__spacer" />
        <Link to="/settings" className="of-button" style={{ marginBottom: 6 }}>
          Manage spaces
          <Glyph name="settings" size={14} />
        </Link>
      </nav>

      <header className="of-catalog-header">
        <h1 className="of-catalog-header__title">Data Catalog</h1>
        <div className="of-catalog-header__actions">
          <Link to="/data-connection/new" className="of-button">
            Request data
          </Link>
          <button type="button" className="of-button of-button--success" onClick={() => setCreateOpen(true)}>
            <Glyph name="plus" size={14} />
            New
          </button>
        </div>
      </header>

      <div className="of-catalog-tabbar">
        <button
          type="button"
          className={`of-catalog-tab ${activeTab === 'collections' ? 'of-catalog-tab--active' : ''}`}
          onClick={() => setActiveTab('collections')}
        >
          Collections
        </button>
        <button
          type="button"
          className={`of-catalog-tab ${activeTab === 'files' ? 'of-catalog-tab--active' : ''}`}
          onClick={() => setActiveTab('files')}
        >
          Files
        </button>
      </div>

      {error ? (
        <div className="of-catalog-banner of-status-danger">{error}</div>
      ) : null}

      <div className="of-catalog-body">
        <DatasetCatalogFacets
          tags={facets.tags}
          owners={facets.owners}
          formats={formatFacets}
          selectedTag={tag}
          selectedOwner={owner}
          selectedFormat={format}
          disabled={loading}
          onTagChange={(nextTag) => applyFilters({ ...filters, tag: nextTag })}
          onOwnerChange={(nextOwner) => applyFilters({ ...filters, owner: nextOwner })}
          onFormatChange={(nextFormat) => applyFilters({ ...filters, format: nextFormat })}
        />

        <section className="of-catalog-content">
          <div className="of-catalog-toolbar">
            <div className="of-catalog-breadcrumb">
              <span className="of-catalog-breadcrumb__current">Data Catalog</span>
              <Glyph name="chevron-right" size={14} />
              <span style={{ color: 'var(--text-strong)', fontSize: 13, fontWeight: 600 }}>
                {activeTab === 'collections' ? 'Collections' : 'All files'}
              </span>
              <span className="of-catalog-section-chip" aria-label="count">
                <Glyph name="cube" size={12} />
                <span style={{ marginLeft: 4 }}>{total.toLocaleString()}</span>
              </span>
            </div>
            <div className="of-catalog-toolbar__group">
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  applyFilters();
                }}
                className="of-catalog-search"
                role="search"
              >
                <Glyph name="search" size={14} />
                <input
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  placeholder="Search datasets"
                  aria-label="Search datasets"
                />
              </form>
              {activeFilterCount > 0 ? (
                <button type="button" className="of-button of-button--ghost" onClick={resetFilters}>
                  Reset
                </button>
              ) : null}
              <Link to="/datasets/upload" className="of-button">
                <Glyph name="database" size={14} />
                Upload data
              </Link>
            </div>
          </div>

          <div className="of-catalog-stats">
            {summary.map((item) => (
              <div key={item.label} className="of-catalog-stat">
                <p className="of-catalog-stat__label">{item.label}</p>
                <p className="of-catalog-stat__value">
                  {loading && item.label === 'Datasets' ? '...' : item.value}
                </p>
                <p className="of-catalog-stat__hint">{item.hint}</p>
              </div>
            ))}
          </div>

          <div style={{ overflowX: 'auto' }}>
            <table className="of-catalog-table">
              <thead>
                <tr>
                  <th style={{ width: '50%' }}>
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                      Name <Glyph name="chevron-down" size={11} />
                    </span>
                  </th>
                  <th style={{ width: 200 }}>Last updated</th>
                  <th>Tags</th>
                  <th aria-label="actions" style={{ width: 100 }} />
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  <tr>
                    <td colSpan={4} className="of-catalog-empty">
                      Loading datasets…
                    </td>
                  </tr>
                ) : filteredByFormat.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="of-catalog-empty">
                      No datasets matched the current filters.
                    </td>
                  </tr>
                ) : activeTab === 'collections' ? (
                  collections.map((group) => (
                    <FragmentGroup
                      key={group.label}
                      label={group.label}
                      datasets={group.datasets}
                      activeTag={tag}
                      onTagClick={(value) => applyFilters({ ...filters, tag: value })}
                      onDelete={setDeleteTarget}
                      busy={busy}
                    />
                  ))
                ) : (
                  filteredByFormat.map((dataset) => (
                    <DatasetRow
                      key={dataset.id}
                      dataset={dataset}
                      activeTag={tag}
                      onTagClick={(value) => applyFilters({ ...filters, tag: value })}
                      onDelete={() => setDeleteTarget(dataset)}
                      busy={busy}
                    />
                  ))
                )}
              </tbody>
            </table>
          </div>

          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              gap: 12,
              flexWrap: 'wrap',
              padding: '14px 0 0',
              borderTop: '1px solid var(--border-subtle)',
              marginTop: 12,
            }}
          >
            <span className="of-text-muted" style={{ fontSize: 11 }}>
              Page {page} of {totalPages}
            </span>
            <Pagination page={page} perPage={PAGE_SIZE} total={total} onChange={setPage} />
          </div>
        </section>
      </div>

      <CreateDatasetModal
        open={createOpen}
        initialTag={tag}
        onClose={() => setCreateOpen(false)}
        onCreated={(dataset) => {
          setCreateOpen(false);
          navigate(`/datasets/${encodeURIComponent(dataset.id)}`);
        }}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="Delete dataset"
        message={deleteTarget ? `Move ${deleteTarget.name} to deleted datasets? It can be restored through the dataset API until it is hard-deleted.` : ''}
        confirmLabel="Soft-delete"
        danger
        busy={busy}
        onConfirm={() => void confirmDelete()}
        onCancel={() => setDeleteTarget(null)}
      />
    </section>
  );
}

function CatalogSubnavItem({
  to,
  icon,
  label,
  active,
  title,
  highlightIcon,
}: {
  to: string;
  icon: 'folder-open' | 'badge-check' | 'bookmark' | 'project' | 'users' | 'add-user';
  label: string;
  active: boolean;
  title?: string;
  highlightIcon?: boolean;
}) {
  return (
    <NavLink
      to={to}
      title={title ?? label}
      className={({ isActive }) =>
        `of-catalog-subnav__item ${active || isActive ? 'of-catalog-subnav__item--active' : ''}`
      }
      end
    >
      <span className={`of-catalog-subnav__icon ${highlightIcon ? 'of-catalog-subnav__icon--active' : ''}`}>
        <Glyph name={icon} size={16} />
      </span>
      <span>{label}</span>
    </NavLink>
  );
}

function DatasetRow({
  dataset,
  activeTag,
  onTagClick,
  onDelete,
  busy,
}: {
  dataset: Dataset;
  activeTag: string;
  onTagClick: (tag: string) => void;
  onDelete: () => void;
  busy: boolean;
}) {
  const navigate = useNavigate();
  const tags = dataset.tags ?? [];
  const branchPath = dataset.storage_path
    ? (dataset.path || `/${dataset.storage_path.replace(/^\/+/, '').replace(/\/+$/, '')}`)
    : `/datasets/${dataset.name}`;
  const datasetCount = Math.max(1, dataset.current_version || 1);

  return (
    <tr
      onClick={() => navigate(`/datasets/${encodeURIComponent(dataset.id)}`)}
      onKeyDown={(event) => {
        if (event.key === 'Enter') navigate(`/datasets/${encodeURIComponent(dataset.id)}`);
      }}
    >
      <td>
        <div className="of-catalog-row__name">
          <span className="of-catalog-row__icon" aria-hidden>
            <Glyph name="spreadsheet" size={14} />
          </span>
          <div style={{ minWidth: 0 }}>
            <Link
              to={`/datasets/${encodeURIComponent(dataset.id)}`}
              className="of-catalog-row__title"
              onClick={(event) => event.stopPropagation()}
            >
              {dataset.name}
              {dataset.format ? <span className="of-catalog-row__alias">({dataset.format})</span> : null}
            </Link>
            <div className="of-catalog-row__path">{branchPath}</div>
          </div>
        </div>
      </td>
      <td>
        <span className="of-catalog-row__counts">
          <span className="of-catalog-row__count" title="Versions">
            <Glyph name="cube" size={12} />
            {datasetCount}
          </span>
          {dataset.health_status ? <HealthDot status={dataset.health_status} /> : null}
          <span className="of-catalog-row__date">{formatDate(dataset.updated_at)}</span>
        </span>
      </td>
      <td>
        <div className="of-catalog-row__tagcell">
          {tags.length === 0 ? (
            <span className="of-catalog-untagged">Not tagged</span>
          ) : (
            tags.slice(0, 3).map((value) => (
              <button
                key={value}
                type="button"
                className={`of-chip ${activeTag === value ? 'of-chip-active' : ''}`}
                onClick={(event) => {
                  event.stopPropagation();
                  onTagClick(value);
                }}
                style={{ border: 0 }}
              >
                {value}
              </button>
            ))
          )}
          {tags.length > 3 ? <span className="of-text-muted" style={{ fontSize: 11 }}>+{tags.length - 3}</span> : null}
        </div>
      </td>
      <td onClick={(event) => event.stopPropagation()}>
        <div style={{ display: 'flex', gap: 4, justifyContent: 'flex-end' }}>
          <Link
            to={`/datasets/${encodeURIComponent(dataset.id)}/branches`}
            className="of-button of-button--ghost"
            style={{ fontSize: 11, minHeight: 26 }}
            title="Branches"
          >
            <Glyph name="graph" size={13} />
          </Link>
          <button
            type="button"
            onClick={onDelete}
            disabled={busy}
            className="of-button of-button--ghost"
            style={{ fontSize: 11, minHeight: 26, color: 'var(--status-danger)' }}
            title="Delete"
          >
            <Glyph name="trash" size={13} />
          </button>
        </div>
      </td>
    </tr>
  );
}

function FragmentGroup({
  label,
  datasets,
  activeTag,
  onTagClick,
  onDelete,
  busy,
}: {
  label: string;
  datasets: Dataset[];
  activeTag: string;
  onTagClick: (tag: string) => void;
  onDelete: (dataset: Dataset) => void;
  busy: boolean;
}) {
  return (
    <>
      <tr className="of-catalog-section-row">
        <td colSpan={4}>
          <span className="of-catalog-section-chip">{label}</span>
        </td>
      </tr>
      {datasets.map((dataset) => (
        <DatasetRow
          key={dataset.id}
          dataset={dataset}
          activeTag={activeTag}
          onTagClick={onTagClick}
          onDelete={() => onDelete(dataset)}
          busy={busy}
        />
      ))}
    </>
  );
}

function HealthDot({ status }: { status: string }) {
  const color = status === 'healthy'
    ? 'var(--status-success)'
    : status === 'warning'
    ? 'var(--status-warning)'
    : status === 'degraded' || status === 'critical'
    ? 'var(--status-danger)'
    : 'var(--text-soft)';
  return (
    <span
      title={`Health: ${status}`}
      style={{
        display: 'inline-block',
        width: 8,
        height: 8,
        borderRadius: '50%',
        background: color,
      }}
    />
  );
}

function groupByPrimaryTag(datasets: Dataset[]) {
  const buckets = new Map<string, Dataset[]>();
  datasets.forEach((dataset) => {
    const primary = (dataset.tags && dataset.tags[0]) || 'Not tagged';
    const list = buckets.get(primary) ?? [];
    list.push(dataset);
    buckets.set(primary, list);
  });
  return Array.from(buckets.entries())
    .sort((a, b) => {
      if (a[0] === 'Not tagged') return 1;
      if (b[0] === 'Not tagged') return -1;
      return a[0].localeCompare(b[0]);
    })
    .map(([label, items]) => ({ label, datasets: items }));
}

function formatDate(value?: string | null) {
  if (!value) return '-';
  const time = new Date(value);
  if (Number.isNaN(time.getTime())) return '-';
  return time.toLocaleString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes.toLocaleString()} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function compactNumber(value: number) {
  return Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(value);
}
