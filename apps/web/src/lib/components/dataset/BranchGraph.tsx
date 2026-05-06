import { useMemo, useState } from 'react';

import { CytoscapeCanvas } from '@/lib/components/CytoscapeCanvas';
import type { DatasetBranch } from '@/lib/api/datasets';

type ExtraStats = {
  transactions?: number;
  downstreamPipelines?: number;
};

interface BranchGraphProps {
  branches: DatasetBranch[];
  extras?: Record<string, ExtraStats>;
  selectedBranch?: string | null;
  onSelect?: (branch: DatasetBranch) => void;
  onDoubleClick?: (branch: DatasetBranch) => void;
}

function relativeTime(ts?: string): string {
  if (!ts) return '—';
  const then = new Date(ts).getTime();
  if (Number.isNaN(then)) return ts;
  const delta = Math.max(0, Date.now() - then);
  const seconds = Math.round(delta / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  return `${days}d ago`;
}

function shortId(id: string | null | undefined): string {
  if (!id) return '—';
  if (id.length <= 12) return id;
  return `${id.slice(0, 8)}…`;
}

const STYLESHEET = [
  {
    selector: 'node',
    style: {
      'background-color': '#1f2937',
      'border-color': '#475569',
      'border-width': 2,
      label: 'data(label)',
      color: '#f1f5f9',
      'text-valign': 'center',
      'text-halign': 'center',
      'font-size': 11,
      'font-weight': 600,
      width: 140,
      height: 44,
      shape: 'round-rectangle',
      'text-wrap': 'wrap',
      'text-max-width': '120px',
    },
  },
  {
    selector: 'node[?isDefault]',
    style: { 'background-color': '#1e40af', 'border-color': '#3b82f6' },
  },
  {
    selector: 'node[?hasOpenTransaction]',
    style: { 'border-color': '#f59e0b' },
  },
  {
    selector: 'node[?archived]',
    style: { 'background-color': '#374151', color: '#94a3b8', 'border-style': 'dashed' },
  },
  {
    selector: 'node:selected',
    style: { 'border-color': '#fbbf24', 'border-width': 4 },
  },
  {
    selector: 'edge',
    style: {
      width: 1.5,
      'line-color': '#475569',
      'target-arrow-color': '#475569',
      'target-arrow-shape': 'triangle',
      'curve-style': 'bezier',
    },
  },
] as const;

export function BranchGraph({ branches, extras = {}, selectedBranch, onSelect, onDoubleClick }: BranchGraphProps) {
  const [showRetired, setShowRetired] = useState(false);
  const [labelFilter, setLabelFilter] = useState('');
  const [nameSearch, setNameSearch] = useState('');

  const visible = useMemo(() => {
    return branches.filter((b) => {
      if (!showRetired && b.archived_at) return false;
      if (labelFilter.trim()) {
        const wanted = labelFilter.trim();
        const labels = b.labels ?? {};
        const matches = Object.entries(labels).some(([k, v]) => `${k}=${v}`.includes(wanted) || k.includes(wanted) || v.includes(wanted));
        if (!matches) return false;
      }
      if (nameSearch.trim() && !b.name.toLowerCase().includes(nameSearch.trim().toLowerCase())) return false;
      return true;
    });
  }, [branches, showRetired, labelFilter, nameSearch]);

  const elements = useMemo(() => {
    const ids = new Set(visible.map((b) => b.id));
    const nodes = visible.map((b) => {
      const stats = extras[b.name] ?? {};
      const detail = `${stats.transactions ?? 0} tx · ${relativeTime(b.last_activity_at)}`;
      return {
        data: {
          id: b.id,
          label: `${b.name}\n${detail}`,
          isDefault: b.is_default,
          hasOpenTransaction: Boolean(b.has_open_transaction),
          archived: Boolean(b.archived_at),
          branch: b,
        },
        classes: b.id === selectedBranch || b.name === selectedBranch ? 'selected' : '',
      };
    });
    const edges = visible
      .filter((b) => b.parent_branch_id && ids.has(b.parent_branch_id))
      .map((b) => ({
        data: { id: `${b.parent_branch_id}->${b.id}`, source: b.parent_branch_id!, target: b.id },
      }));
    return [...nodes, ...edges];
  }, [visible, extras, selectedBranch]);

  const layout = { name: 'breadthfirst', directed: true, padding: 16, spacingFactor: 1.4 } as const;

  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
        <input value={nameSearch} onChange={(e) => setNameSearch(e.target.value)} placeholder="Search branch name…" className="of-input" style={{ fontSize: 11, width: 200 }} />
        <input value={labelFilter} onChange={(e) => setLabelFilter(e.target.value)} placeholder="Label filter (key=value or substring)" className="of-input" style={{ fontSize: 11, width: 240 }} />
        <label style={{ fontSize: 11, display: 'flex', alignItems: 'center', gap: 4 }}>
          <input type="checkbox" checked={showRetired} onChange={(e) => setShowRetired(e.target.checked)} />
          Show retired
        </label>
        <span className="of-text-muted" style={{ fontSize: 11, marginLeft: 'auto' }}>
          {visible.length} of {branches.length} branches
        </span>
      </div>

      <CytoscapeCanvas
        elements={elements}
        stylesheet={STYLESHEET as unknown as import('cytoscape').StylesheetStyle[]}
        layout={layout}
        height={420}
        onReady={(cy) => {
          cy.removeListener('tap');
          cy.removeListener('dblclick');
          cy.on('tap', 'node', (evt) => {
            const branch = evt.target.data('branch') as DatasetBranch | undefined;
            if (branch) onSelect?.(branch);
          });
          cy.on('dblclick', 'node', (evt) => {
            const branch = evt.target.data('branch') as DatasetBranch | undefined;
            if (branch) onDoubleClick?.(branch);
          });
        }}
      />

      <div style={{ display: 'flex', gap: 12, fontSize: 10, color: 'var(--text-muted)', flexWrap: 'wrap' }}>
        <span>● <strong style={{ color: '#3b82f6' }}>blue</strong> = default branch</span>
        <span>○ <strong style={{ color: '#f59e0b' }}>amber border</strong> = open transaction</span>
        <span>┄ <strong style={{ color: '#94a3b8' }}>dashed</strong> = archived</span>
        <span>tip: dblclick a node to open its detail · short ids: {shortId(visible[0]?.head_transaction_id)}</span>
      </div>
    </div>
  );
}
