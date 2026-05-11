import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { CreatePipelineModal } from '@/lib/components/pipeline/CreatePipelineModal';
import { RunHistory } from '@/lib/components/pipeline/RunHistory';
import { ConfirmDialog } from '@/lib/components/ConfirmDialog';
import { Glyph } from '@/lib/components/ui/Glyph';
import { deletePipeline, listPipelines, pipelineNodesFromDAG, runDuePipelines, type Pipeline } from '@/lib/api/pipelines';

type StatusTab = 'all' | 'draft' | 'active' | 'paused' | 'archived';
type ScheduleFacet = 'all' | 'scheduled' | 'manual';
type SortKey = 'updated_desc' | 'updated_asc' | 'created_desc' | 'name_asc' | 'name_desc';

const STATUS_TABS: Array<{ id: StatusTab; label: string }> = [
  { id: 'all', label: 'All pipelines' },
  { id: 'draft', label: 'Drafts' },
  { id: 'active', label: 'Active' },
  { id: 'paused', label: 'Paused' },
  { id: 'archived', label: 'Archived' },
];

const PIPELINE_TYPES = ['BATCH', 'FASTER', 'DISTRIBUTED', 'INCREMENTAL', 'STREAMING', 'EXTERNAL'] as const;
type PipelineTypeFacet = (typeof PIPELINE_TYPES)[number];

const SORT_OPTIONS: Array<{ id: SortKey; label: string }> = [
  { id: 'updated_desc', label: 'Sorted by most recently updated' },
  { id: 'updated_asc', label: 'Sorted by least recently updated' },
  { id: 'created_desc', label: 'Sorted by most recently created' },
  { id: 'name_asc', label: 'Sorted by name (A → Z)' },
  { id: 'name_desc', label: 'Sorted by name (Z → A)' },
];

function fmtDate(value: string | null | undefined) {
  if (!value) return '—';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString();
}

function fmtSchedule(pipeline: Pipeline) {
  if (!pipeline.schedule_config?.enabled) return 'Manual';
  return pipeline.schedule_config.cron ? `Cron · ${pipeline.schedule_config.cron}` : 'Scheduled';
}

function statusTone(status: string) {
  const normalized = status.toLowerCase();
  if (normalized === 'active' || normalized === 'validated' || normalized === 'deployed') return 'of-status-success';
  if (normalized === 'paused' || normalized === 'draft') return 'of-status-warning';
  if (normalized === 'archived' || normalized === 'failed') return 'of-status-danger';
  return 'of-status-info';
}

function pipelineTypeOf(pipeline: Pipeline) {
  return (pipeline.pipeline_type ?? 'BATCH').toUpperCase();
}

function typeLabel(t: string) {
  return t.charAt(0) + t.slice(1).toLowerCase();
}

function tsOf(value: string | null | undefined) {
  if (!value) return 0;
  const t = new Date(value).getTime();
  return Number.isNaN(t) ? 0 : t;
}

export function PipelinesPage() {
  const [pipelines, setPipelines] = useState<Pipeline[]>([]);
  const [statusTab, setStatusTab] = useState<StatusTab>('all');
  const [search, setSearch] = useState('');
  const [pendingSearch, setPendingSearch] = useState('');
  const [typeFacet, setTypeFacet] = useState<PipelineTypeFacet | ''>('');
  const [scheduleFacet, setScheduleFacet] = useState<ScheduleFacet>('all');
  const [sortKey, setSortKey] = useState<SortKey>('updated_desc');
  const [sortOpen, setSortOpen] = useState(false);
  const [filterOpen, setFilterOpen] = useState(false);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkSelected, setBulkSelected] = useState<Set<string>>(new Set());
  const [total, setTotal] = useState(0);
  const [selectedPipelineId, setSelectedPipelineId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [feedback, setFeedback] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<Pipeline | null>(null);
  const navigate = useNavigate();

  const sortRef = useRef<HTMLDivElement | null>(null);
  const filterRef = useRef<HTMLDivElement | null>(null);

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const res = await listPipelines({
        search: search || undefined,
        status: statusTab === 'all' ? undefined : statusTab,
        per_page: 100,
      });
      setPipelines(res.data);
      setTotal(res.total ?? res.data.length);
      setSelectedPipelineId((current) => {
        if (current && res.data.some((pipeline) => pipeline.id === current)) return current;
        return res.data[0]?.id ?? null;
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load pipelines');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusTab, search]);

  useEffect(() => {
    if (!sortOpen && !filterOpen) return;
    function onClick(event: MouseEvent) {
      const target = event.target as Node | null;
      if (sortOpen && sortRef.current && target && !sortRef.current.contains(target)) {
        setSortOpen(false);
      }
      if (filterOpen && filterRef.current && target && !filterRef.current.contains(target)) {
        setFilterOpen(false);
      }
    }
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, [sortOpen, filterOpen]);

  function applySearch() {
    setSearch(pendingSearch.trim());
  }

  function resetFilters() {
    setPendingSearch('');
    setSearch('');
    setTypeFacet('');
    setScheduleFacet('all');
    setStatusTab('all');
  }

  function clearFilter(kind: 'type' | 'schedule' | 'status' | 'q') {
    if (kind === 'type') setTypeFacet('');
    if (kind === 'schedule') setScheduleFacet('all');
    if (kind === 'status') setStatusTab('all');
    if (kind === 'q') {
      setPendingSearch('');
      setSearch('');
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    setBusy(true);
    setError('');
    setFeedback('');
    try {
      await deletePipeline(deleteTarget.id);
      setDeleteTarget(null);
      setFeedback(`Deleted pipeline “${deleteTarget.name}”.`);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  async function handleRunDue() {
    setBusy(true);
    setError('');
    setFeedback('');
    try {
      const res = await runDuePipelines();
      setFeedback(`Dispatched ${res.triggered_runs} due run${res.triggered_runs === 1 ? '' : 's'}.`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to dispatch due runs');
    } finally {
      setBusy(false);
    }
  }

  async function deleteSelected() {
    if (bulkSelected.size === 0) return;
    setBusy(true);
    setError('');
    setFeedback('');
    try {
      const ids = Array.from(bulkSelected);
      for (const id of ids) {
        await deletePipeline(id);
      }
      setFeedback(`Deleted ${ids.length} pipeline${ids.length === 1 ? '' : 's'}.`);
      setBulkSelected(new Set());
      setBulkOpen(false);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Bulk delete failed');
    } finally {
      setBusy(false);
    }
  }

  const visiblePipelines = useMemo(() => {
    const filtered = pipelines.filter((pipeline) => {
      if (typeFacet && pipelineTypeOf(pipeline) !== typeFacet) return false;
      if (scheduleFacet === 'scheduled' && !pipeline.schedule_config?.enabled) return false;
      if (scheduleFacet === 'manual' && pipeline.schedule_config?.enabled) return false;
      return true;
    });
    const sorted = filtered.slice();
    sorted.sort((a, b) => {
      switch (sortKey) {
        case 'updated_asc':
          return tsOf(a.updated_at) - tsOf(b.updated_at);
        case 'created_desc':
          return tsOf(b.created_at) - tsOf(a.created_at);
        case 'name_asc':
          return a.name.localeCompare(b.name);
        case 'name_desc':
          return b.name.localeCompare(a.name);
        case 'updated_desc':
        default:
          return tsOf(b.updated_at) - tsOf(a.updated_at);
      }
    });
    return sorted;
  }, [pipelines, typeFacet, scheduleFacet, sortKey]);

  const typeCounts = useMemo(() => {
    const counts = new Map<PipelineTypeFacet, number>();
    PIPELINE_TYPES.forEach((t) => counts.set(t, 0));
    pipelines.forEach((pipeline) => {
      const t = pipelineTypeOf(pipeline) as PipelineTypeFacet;
      if (counts.has(t)) counts.set(t, (counts.get(t) ?? 0) + 1);
    });
    return counts;
  }, [pipelines]);

  const scheduleCounts = useMemo(() => {
    let scheduled = 0;
    let manual = 0;
    pipelines.forEach((pipeline) => {
      if (pipeline.schedule_config?.enabled) scheduled += 1;
      else manual += 1;
    });
    return { scheduled, manual };
  }, [pipelines]);

  const statusCounts = useMemo(() => {
    let active = 0;
    let paused = 0;
    let draft = 0;
    let archived = 0;
    pipelines.forEach((pipeline) => {
      const s = pipeline.status.toLowerCase();
      if (s === 'active') active += 1;
      else if (s === 'paused') paused += 1;
      else if (s === 'draft') draft += 1;
      else if (s === 'archived') archived += 1;
    });
    return { active, paused, draft, archived };
  }, [pipelines]);

  const selectedPipeline = visiblePipelines.find((pipeline) => pipeline.id === selectedPipelineId)
    ?? pipelines.find((pipeline) => pipeline.id === selectedPipelineId)
    ?? null;

  const activeFilters: Array<{ kind: 'type' | 'schedule' | 'status' | 'q'; label: string }> = [];
  if (typeFacet) activeFilters.push({ kind: 'type', label: `Type · ${typeLabel(typeFacet)}` });
  if (scheduleFacet !== 'all') {
    activeFilters.push({ kind: 'schedule', label: `Schedule · ${scheduleFacet === 'scheduled' ? 'Scheduled' : 'Manual'}` });
  }
  if (statusTab !== 'all') {
    activeFilters.push({ kind: 'status', label: `Status · ${STATUS_TABS.find((t) => t.id === statusTab)?.label ?? statusTab}` });
  }
  if (search) activeFilters.push({ kind: 'q', label: `Name · ${search}` });

  const sortLabel = SORT_OPTIONS.find((opt) => opt.id === sortKey)?.label ?? 'Sort';

  function toggleBulk(id: string) {
    setBulkSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }
  function selectAllVisible() {
    setBulkSelected(new Set(visiblePipelines.map((p) => p.id)));
  }
  function clearBulk() {
    setBulkSelected(new Set());
  }

  return (
    <section className="of-page" style={{ padding: 0, display: 'grid', gap: 0 }}>
      {/* Title strip — Foundry "Build schedules" header */}
      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          padding: '10px 18px',
          background: 'var(--bg-panel)',
          borderBottom: '1px solid var(--border-subtle)',
          borderTop: '2px solid var(--status-info)',
        }}
      >
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: 26,
            height: 26,
            borderRadius: 'var(--radius-sm)',
            background: 'var(--bg-panel-muted)',
            border: '1px solid var(--border-subtle)',
            color: 'var(--text-muted)',
          }}
        >
          <Glyph name="run" size={14} />
        </span>
        <h1 className="of-heading-lg" style={{ margin: 0, fontSize: 16 }}>
          Pipelines
        </h1>
        <span className="of-text-muted" style={{ fontSize: 12, marginLeft: 6 }}>
          Author hybrid batch and streaming pipelines.
        </span>
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
          <StatPill tone="info" icon="history" value={statusCounts.draft} label={`${statusCounts.draft} draft${statusCounts.draft === 1 ? '' : 's'}`} />
          <StatPill tone="success" icon="check" value={statusCounts.active} label={`${statusCounts.active} active`} />
          <StatPill tone="danger" icon="x" value={statusCounts.paused + statusCounts.archived} label={`${statusCounts.paused + statusCounts.archived} paused or archived`} />
        </div>
      </header>

      {/* Tab bar — Compass pattern */}
      <div
        className="of-tabbar"
        role="tablist"
        aria-label="Pipeline status"
        style={{
          background: 'var(--bg-panel)',
          padding: '0 12px',
          borderBottom: '1px solid var(--border-subtle)',
        }}
      >
        {STATUS_TABS.map((tab) => {
          const count = tab.id === 'all'
            ? pipelines.length
            : pipelines.filter((p) => p.status.toLowerCase() === tab.id).length;
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={statusTab === tab.id}
              className={`of-tab ${statusTab === tab.id ? 'of-tab-active' : ''}`}
              onClick={() => setStatusTab(tab.id)}
              style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
            >
              <span>{tab.label}</span>
              <span
                className="of-badge"
                style={{
                  fontSize: 10,
                  background: statusTab === tab.id ? 'var(--bg-chip-active)' : 'var(--bg-chip)',
                }}
              >
                {count}
              </span>
            </button>
          );
        })}
      </div>

      {/* Sub-header — count + search params + actions */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          padding: '10px 18px',
          background: 'var(--bg-panel)',
          borderBottom: '1px solid var(--border-subtle)',
          flexWrap: 'wrap',
        }}
      >
        <p className="of-heading-md" style={{ margin: 0, fontSize: 14 }}>
          {loading ? 'Loading…' : `${visiblePipelines.length} pipeline${visiblePipelines.length === 1 ? '' : 's'}`}
        </p>
        <span className="of-text-muted" style={{ fontSize: 12 }}>
          Current search parameters
        </span>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', minWidth: 0 }}>
          {activeFilters.length === 0 ? (
            <span className="of-text-soft" style={{ fontSize: 12, fontStyle: 'italic' }}>
              none
            </span>
          ) : (
            activeFilters.map((filter) => (
              <span
                key={filter.kind}
                className="of-chip"
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 4,
                  fontSize: 11,
                  paddingRight: 4,
                }}
              >
                {filter.label}
                <button
                  type="button"
                  aria-label={`Remove filter ${filter.label}`}
                  className="of-button of-button--ghost"
                  onClick={() => clearFilter(filter.kind)}
                  style={{ minHeight: 16, height: 16, width: 16, padding: 0, borderRadius: '50%' }}
                >
                  <Glyph name="x" size={10} />
                </button>
              </span>
            ))
          )}
          {activeFilters.length > 0 ? (
            <button
              type="button"
              className="of-button of-button--ghost"
              onClick={resetFilters}
              style={{ fontSize: 11, minHeight: 22, padding: '0 6px' }}
              title="Reset all filters"
            >
              Reset
            </button>
          ) : null}
        </div>
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 6, alignItems: 'center' }}>
          <button
            type="button"
            onClick={() => void handleRunDue()}
            disabled={busy}
            className="of-button"
            title="Trigger due scheduled runs"
          >
            <Glyph name="run" size={13} />
            {busy ? 'Dispatching…' : 'Run due'}
          </button>
          <Link to="/pipelines/new" className="of-button" title="Create pipeline from JSON definition">
            <Glyph name="code" size={13} />
            JSON create
          </Link>
          <button
            type="button"
            onClick={() => setCreateOpen(true)}
            className="of-button of-button--success"
          >
            <Glyph name="plus" size={13} />
            New pipeline
          </button>
        </div>
      </div>

      {/* Center toolbar — Sort / Select / Filter / Filter input */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          padding: '10px 18px',
          background: 'var(--bg-panel-muted)',
          borderBottom: '1px solid var(--border-subtle)',
          flexWrap: 'wrap',
        }}
      >
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', margin: '0 auto' }}>
          {/* Sort dropdown */}
          <div ref={sortRef} style={{ position: 'relative' }}>
            <button
              type="button"
              className="of-button"
              onClick={() => setSortOpen((open) => !open)}
              aria-haspopup="listbox"
              aria-expanded={sortOpen}
              style={{ minWidth: 240, justifyContent: 'space-between' }}
            >
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                <Glyph name="list" size={13} />
                {sortLabel}
              </span>
              <Glyph name="chevron-down" size={12} />
            </button>
            {sortOpen ? (
              <div
                role="listbox"
                className="of-popover"
                style={{
                  position: 'absolute',
                  top: 'calc(100% + 4px)',
                  left: 0,
                  minWidth: 260,
                  zIndex: 30,
                  display: 'grid',
                  padding: 4,
                }}
              >
                {SORT_OPTIONS.map((opt) => (
                  <button
                    key={opt.id}
                    type="button"
                    role="option"
                    aria-selected={sortKey === opt.id}
                    className={`of-button of-button--ghost`}
                    onClick={() => {
                      setSortKey(opt.id);
                      setSortOpen(false);
                    }}
                    style={{
                      justifyContent: 'flex-start',
                      width: '100%',
                      fontWeight: sortKey === opt.id ? 700 : 500,
                      background: sortKey === opt.id ? 'var(--bg-chip-active)' : undefined,
                    }}
                  >
                    {sortKey === opt.id ? <Glyph name="check" size={12} /> : <span style={{ width: 12 }} />}
                    {opt.label}
                  </button>
                ))}
              </div>
            ) : null}
          </div>

          {/* Select pipelines (bulk toggle) */}
          <button
            type="button"
            className={`of-button ${bulkOpen ? 'of-chip-active' : ''}`}
            onClick={() => {
              setBulkOpen((open) => !open);
              if (bulkOpen) clearBulk();
            }}
            style={{ minWidth: 160, justifyContent: 'space-between' }}
          >
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
              <Glyph name="bookmark" size={13} />
              {bulkOpen ? `${bulkSelected.size} selected` : 'Select pipelines…'}
            </span>
            {bulkOpen ? <Glyph name="x" size={12} /> : <Glyph name="chevron-down" size={12} />}
          </button>

          {/* Filter dropdown — facets */}
          <div ref={filterRef} style={{ position: 'relative' }}>
            <button
              type="button"
              className={`of-button ${filterOpen || typeFacet || scheduleFacet !== 'all' ? 'of-chip-active' : ''}`}
              onClick={() => setFilterOpen((open) => !open)}
              aria-haspopup="dialog"
              aria-expanded={filterOpen}
              title="Filter"
              style={{ minWidth: 100 }}
            >
              <Glyph name="settings" size={13} />
              Filter
              {typeFacet || scheduleFacet !== 'all' ? (
                <span className="of-badge" style={{ marginLeft: 4 }}>
                  {(typeFacet ? 1 : 0) + (scheduleFacet !== 'all' ? 1 : 0)}
                </span>
              ) : null}
            </button>
            {filterOpen ? (
              <div
                className="of-popover"
                role="dialog"
                aria-label="Pipeline filters"
                style={{
                  position: 'absolute',
                  top: 'calc(100% + 4px)',
                  left: 0,
                  width: 280,
                  zIndex: 30,
                  padding: 12,
                  display: 'grid',
                  gap: 12,
                }}
              >
                <FacetGroup title="Pipeline type">
                  <FacetButton
                    active={typeFacet === ''}
                    count={pipelines.length}
                    disabled={loading}
                    onClick={() => setTypeFacet('')}
                  >
                    All types
                  </FacetButton>
                  {PIPELINE_TYPES.map((t) => (
                    <FacetButton
                      key={t}
                      active={typeFacet === t}
                      count={typeCounts.get(t) ?? 0}
                      disabled={loading}
                      onClick={() => setTypeFacet(t)}
                    >
                      {typeLabel(t)}
                    </FacetButton>
                  ))}
                </FacetGroup>

                <FacetGroup title="Schedule">
                  <FacetButton
                    active={scheduleFacet === 'all'}
                    count={pipelines.length}
                    disabled={loading}
                    onClick={() => setScheduleFacet('all')}
                  >
                    All
                  </FacetButton>
                  <FacetButton
                    active={scheduleFacet === 'scheduled'}
                    count={scheduleCounts.scheduled}
                    disabled={loading}
                    onClick={() => setScheduleFacet('scheduled')}
                  >
                    Scheduled
                  </FacetButton>
                  <FacetButton
                    active={scheduleFacet === 'manual'}
                    count={scheduleCounts.manual}
                    disabled={loading}
                    onClick={() => setScheduleFacet('manual')}
                  >
                    Manual
                  </FacetButton>
                </FacetGroup>

                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 6 }}>
                  <button
                    type="button"
                    className="of-button of-button--ghost"
                    onClick={() => {
                      setTypeFacet('');
                      setScheduleFacet('all');
                    }}
                    style={{ fontSize: 11 }}
                  >
                    Clear
                  </button>
                  <button
                    type="button"
                    className="of-button"
                    onClick={() => setFilterOpen(false)}
                    style={{ fontSize: 11 }}
                  >
                    Done
                  </button>
                </div>
              </div>
            ) : null}
          </div>

          {/* Filter input */}
          <form
            onSubmit={(event) => {
              event.preventDefault();
              applySearch();
            }}
            style={{ position: 'relative', display: 'flex', alignItems: 'center' }}
          >
            <span
              style={{
                position: 'absolute',
                left: 8,
                color: 'var(--text-soft)',
                pointerEvents: 'none',
                display: 'inline-flex',
              }}
            >
              <Glyph name="search" size={13} />
            </span>
            <input
              value={pendingSearch}
              onChange={(event) => setPendingSearch(event.target.value)}
              onBlur={applySearch}
              placeholder="Filter by name…"
              className="of-input"
              style={{ paddingLeft: 26, minHeight: 28, minWidth: 240 }}
              aria-label="Filter by name"
            />
            {pendingSearch ? (
              <button
                type="button"
                className="of-button of-button--ghost"
                onClick={() => {
                  setPendingSearch('');
                  setSearch('');
                }}
                style={{
                  position: 'absolute',
                  right: 4,
                  minHeight: 22,
                  height: 22,
                  width: 22,
                  padding: 0,
                  borderRadius: '50%',
                }}
                aria-label="Clear filter"
              >
                <Glyph name="x" size={11} />
              </button>
            ) : null}
          </form>
        </div>
      </div>

      {/* Bulk action toolbar */}
      {bulkOpen ? (
        <div
          style={{
            display: 'flex',
            gap: 8,
            alignItems: 'center',
            padding: '8px 18px',
            background: 'var(--bg-chip-active)',
            borderBottom: '1px solid var(--border-default)',
          }}
        >
          <span className="of-heading-sm" style={{ margin: 0, fontSize: 12 }}>
            {bulkSelected.size} of {visiblePipelines.length} selected
          </span>
          <button
            type="button"
            className="of-button of-button--ghost"
            onClick={selectAllVisible}
            style={{ fontSize: 11 }}
          >
            Select all visible
          </button>
          <button
            type="button"
            className="of-button of-button--ghost"
            onClick={clearBulk}
            disabled={bulkSelected.size === 0}
            style={{ fontSize: 11 }}
          >
            Clear
          </button>
          <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
            <button
              type="button"
              className="of-button"
              disabled={bulkSelected.size === 0 || busy}
              onClick={() => void deleteSelected()}
              style={{ fontSize: 11, color: 'var(--status-danger)', borderColor: '#e0b4b1' }}
            >
              <Glyph name="trash" size={12} />
              Delete selected
            </button>
          </div>
        </div>
      ) : null}

      {/* Status messages */}
      {error ? (
        <div className="of-status-danger" style={{ padding: '8px 18px', fontSize: 12 }}>
          {error}
        </div>
      ) : null}
      {feedback ? (
        <div className="of-status-success" style={{ padding: '8px 18px', fontSize: 12 }}>
          {feedback}
        </div>
      ) : null}

      {/* Main list — Foundry table */}
      <div className="of-scrollbar" style={{ overflowX: 'auto', background: 'var(--bg-panel)' }}>
        <table className="of-table" style={{ minWidth: 960 }}>
          <thead>
            <tr>
              {bulkOpen ? (
                <th style={{ width: 32 }}>
                  <input
                    type="checkbox"
                    aria-label="Select all visible pipelines"
                    checked={
                      visiblePipelines.length > 0 &&
                      visiblePipelines.every((p) => bulkSelected.has(p.id))
                    }
                    onChange={(event) => {
                      if (event.target.checked) selectAllVisible();
                      else clearBulk();
                    }}
                  />
                </th>
              ) : null}
              <th style={{ minWidth: 280 }}>Name</th>
              <th>Type</th>
              <th>Status</th>
              <th>Schedule</th>
              <th>Lifecycle</th>
              <th>Last updated</th>
              <th>Nodes</th>
              <th style={{ textAlign: 'right' }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr>
                <td colSpan={bulkOpen ? 9 : 8} style={{ padding: 36, textAlign: 'center' }}>
                  <span className="of-text-muted">Loading pipelines…</span>
                </td>
              </tr>
            ) : visiblePipelines.length === 0 ? (
              <tr>
                <td colSpan={bulkOpen ? 9 : 8} style={{ padding: 36, textAlign: 'center' }}>
                  <div style={{ display: 'grid', justifyItems: 'center', gap: 10 }}>
                    <Glyph name="run" size={28} tone="var(--text-soft)" />
                    <span className="of-text-muted">No pipelines match these filters.</span>
                    <button
                      type="button"
                      className="of-button of-button--success"
                      onClick={() => setCreateOpen(true)}
                    >
                      <Glyph name="plus" size={14} />
                      New pipeline
                    </button>
                  </div>
                </td>
              </tr>
            ) : (
              visiblePipelines.map((pipeline) => {
                const isSelected = pipeline.id === selectedPipelineId;
                const type = pipelineTypeOf(pipeline);
                const checked = bulkSelected.has(pipeline.id);
                return (
                  <tr
                    key={pipeline.id}
                    onClick={() => setSelectedPipelineId(pipeline.id)}
                    style={{
                      background: isSelected ? 'var(--bg-hover)' : undefined,
                      cursor: 'pointer',
                    }}
                  >
                    {bulkOpen ? (
                      <td style={{ width: 32 }} onClick={(event) => event.stopPropagation()}>
                        <input
                          type="checkbox"
                          aria-label={`Select ${pipeline.name}`}
                          checked={checked}
                          onChange={() => toggleBulk(pipeline.id)}
                        />
                      </td>
                    ) : null}
                    <td style={{ minWidth: 280 }}>
                      <div style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
                        <span style={{ marginTop: 2, color: 'var(--status-info)' }}>
                          <Glyph name="run" size={14} />
                        </span>
                        <div style={{ minWidth: 0 }}>
                          <Link
                            to={`/pipelines/${pipeline.id}/edit`}
                            className="of-link"
                            style={{ fontWeight: 600 }}
                            onClick={(event) => event.stopPropagation()}
                          >
                            {pipeline.name}
                          </Link>
                          <div
                            className="of-text-soft"
                            style={{ fontFamily: 'var(--font-mono)', fontSize: 10, marginTop: 2 }}
                          >
                            {pipeline.id}
                          </div>
                          {pipeline.description ? (
                            <div
                              className="of-text-muted"
                              style={{ maxWidth: 480, marginTop: 4, fontSize: 11 }}
                            >
                              {pipeline.description}
                            </div>
                          ) : null}
                        </div>
                      </div>
                    </td>
                    <td>
                      <span className="of-chip" style={{ fontSize: 11 }}>
                        {typeLabel(type)}
                      </span>
                    </td>
                    <td>
                      <span
                        className={statusTone(pipeline.status)}
                        style={{
                          display: 'inline-flex',
                          padding: '2px 7px',
                          borderRadius: 'var(--radius-sm)',
                          fontSize: 11,
                          fontWeight: 700,
                        }}
                      >
                        {pipeline.status}
                      </span>
                    </td>
                    <td>{fmtSchedule(pipeline)}</td>
                    <td className="of-text-muted">{pipeline.lifecycle ?? '—'}</td>
                    <td className="of-text-muted">{fmtDate(pipeline.updated_at)}</td>
                    <td>{pipelineNodesFromDAG(pipeline.dag).length}</td>
                    <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                      <div style={{ display: 'inline-flex', gap: 6 }}>
                        <button
                          type="button"
                          onClick={(event) => {
                            event.stopPropagation();
                            navigate(`/pipelines/${pipeline.id}/edit`);
                          }}
                          className="of-button"
                          style={{ fontSize: 11 }}
                        >
                          Builder
                        </button>
                        <button
                          type="button"
                          onClick={(event) => {
                            event.stopPropagation();
                            setSelectedPipelineId(pipeline.id);
                          }}
                          className="of-button"
                          style={{ fontSize: 11 }}
                        >
                          Runs
                        </button>
                        <button
                          type="button"
                          onClick={(event) => {
                            event.stopPropagation();
                            setDeleteTarget(pipeline);
                          }}
                          disabled={busy}
                          className="of-button"
                          style={{
                            fontSize: 11,
                            color: 'var(--status-danger)',
                            borderColor: '#e0b4b1',
                          }}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Footer counts */}
      <div
        style={{
          display: 'flex',
          gap: 12,
          alignItems: 'center',
          padding: '8px 18px',
          background: 'var(--bg-panel-muted)',
          borderTop: '1px solid var(--border-subtle)',
          fontSize: 11,
          color: 'var(--text-muted)',
        }}
      >
        <span>
          {visiblePipelines.length.toLocaleString()} shown · {total.toLocaleString()} total · {scheduleCounts.scheduled} scheduled · {scheduleCounts.manual} manual
        </span>
      </div>

      {/* Run history detail panel */}
      <section
        className="of-panel"
        style={{ overflow: 'hidden', margin: '12px 18px', borderRadius: 'var(--radius-md)' }}
      >
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'flex-start',
            gap: 10,
            padding: '10px 12px',
            borderBottom: '1px solid var(--border-subtle)',
            background: 'var(--bg-panel-muted)',
          }}
        >
          <div style={{ minWidth: 0 }}>
            <p className="of-eyebrow" style={{ margin: 0 }}>
              Run history
            </p>
            <h2 className="of-heading-sm" style={{ margin: '2px 0 0' }}>
              {selectedPipeline ? selectedPipeline.name : 'Select a pipeline'}
            </h2>
            {selectedPipeline ? (
              <p
                className="of-text-muted"
                style={{ margin: '2px 0 0', fontSize: 11, fontFamily: 'var(--font-mono)' }}
              >
                {selectedPipeline.id}
              </p>
            ) : null}
          </div>
          {selectedPipeline ? (
            <div style={{ display: 'flex', gap: 6 }}>
              <button
                type="button"
                onClick={() => navigate(`/pipelines/${selectedPipeline.id}/edit`)}
                className="of-button"
                style={{ fontSize: 11 }}
              >
                Open builder
              </button>
            </div>
          ) : null}
        </div>
        {selectedPipeline ? (
          <div style={{ padding: 10 }}>
            <RunHistory pipelineId={selectedPipeline.id} />
          </div>
        ) : (
          <p className="of-text-muted" style={{ margin: 0, padding: 18, fontSize: 12 }}>
            Click a pipeline row above to inspect its run history.
          </p>
        )}
      </section>

      <CreatePipelineModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(pipelineId) => navigate(`/pipelines/${pipelineId}/edit`)}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="Delete pipeline"
        message={deleteTarget ? `Delete pipeline “${deleteTarget.name}”? This cannot be undone.` : ''}
        confirmLabel="Delete"
        danger
        busy={busy}
        onConfirm={() => void confirmDelete()}
        onCancel={() => setDeleteTarget(null)}
      />
    </section>
  );
}

function StatPill({
  tone,
  icon,
  value,
  label,
}: {
  tone: 'info' | 'success' | 'danger';
  icon: 'history' | 'check' | 'x';
  value: number;
  label: string;
}) {
  const colorByTone: Record<string, string> = {
    info: 'var(--status-info)',
    success: 'var(--status-success)',
    danger: 'var(--status-danger)',
  };
  const bgByTone: Record<string, string> = {
    info: 'var(--status-info-bg)',
    success: 'var(--status-success-bg)',
    danger: 'var(--status-danger-bg)',
  };
  return (
    <span
      title={label}
      aria-label={label}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
        padding: '2px 8px',
        background: bgByTone[tone],
        color: colorByTone[tone],
        border: `1px solid ${colorByTone[tone]}33`,
        borderRadius: 'var(--radius-sm)',
        fontSize: 11,
        fontWeight: 700,
      }}
    >
      <Glyph name={icon} size={12} tone={colorByTone[tone]} />
      {value}
    </span>
  );
}

function FacetGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section style={{ display: 'grid', gap: 6 }}>
      <p className="of-eyebrow" style={{ margin: 0 }}>
        {title}
      </p>
      <div style={{ display: 'grid', gap: 3 }}>{children}</div>
    </section>
  );
}

function FacetButton({
  active,
  count,
  disabled,
  onClick,
  children,
}: {
  active: boolean;
  count: number;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      className={`of-button ${active ? 'of-chip-active' : 'of-button--ghost'}`}
      onClick={onClick}
      disabled={disabled}
      style={{
        justifyContent: 'space-between',
        minHeight: 26,
        width: '100%',
        borderColor: active ? '#bdd2f0' : 'transparent',
        fontWeight: active ? 700 : 500,
        padding: '0 8px',
      }}
    >
      <span
        style={{
          minWidth: 0,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          textAlign: 'left',
        }}
      >
        {children}
      </span>
      <span className="of-badge">{count}</span>
    </button>
  );
}
