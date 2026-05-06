import { useState } from 'react';

import type { Dataset, DatasetBranch, DatasetJobSpecStatus } from '@/lib/api/datasets';
import { BranchPicker } from './BranchPicker';
import { JobSpecBadge } from './JobSpecBadge';
import { MarkingBadge, type MarkingLevel, type MarkingSource } from './MarkingBadge';

export type OpenInTarget =
  | 'pipeline_builder'
  | 'notebook'
  | 'sql_workbench'
  | 'contour'
  | 'fusion'
  | 'workshop'
  | 'code_workspaces';

export interface EffectiveMarking {
  id: string;
  label: string;
  level?: MarkingLevel;
  source: MarkingSource;
}

interface DatasetHeaderProps {
  dataset: Dataset;
  branches: DatasetBranch[];
  markings: EffectiveMarking[];
  busy?: boolean;
  jobSpecStatus?: DatasetJobSpecStatus;
  canManage?: boolean;
  onSwitchBranch: (name: string) => void | Promise<void>;
  onCreateBranch: (params: { name: string; from: string; description?: string }) => void | Promise<void>;
  onAllActions?: (action: string) => void;
  onBuild?: (target: string) => void;
  onAnalyze?: (target: string) => void;
  onExplorePipeline?: (target: string) => void;
  onOpenIn?: (target: OpenInTarget) => void | Promise<void>;
  onPublishToMarketplace?: () => void;
}

const OPEN_IN_TARGETS: { target: OpenInTarget; label: string; comingSoon: boolean }[] = [
  { target: 'pipeline_builder', label: 'Pipeline Builder', comingSoon: false },
  { target: 'notebook', label: 'Notebook', comingSoon: false },
  { target: 'sql_workbench', label: 'SQL Workbench', comingSoon: false },
  { target: 'contour', label: 'Contour', comingSoon: false },
  { target: 'fusion', label: 'Fusion', comingSoon: false },
  { target: 'workshop', label: 'Workshop', comingSoon: true },
  { target: 'code_workspaces', label: 'Code Workspaces', comingSoon: true },
];

const ALL_ACTIONS = ['Star', 'Move', 'Permissions', 'Delete'];

function MenuButton({ label, items, onPick }: { label: string; items: string[]; onPick: (item: string) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <div style={{ position: 'relative' }}>
      <button type="button" onClick={() => setOpen((o) => !o)} className="of-button" style={{ fontSize: 12 }} aria-haspopup="menu" aria-expanded={open}>
        {label} ▾
      </button>
      {open && (
        <ul
          role="menu"
          onMouseLeave={() => setOpen(false)}
          style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            left: 0,
            zIndex: 40,
            background: 'var(--bg-default)',
            border: '1px solid var(--border-default)',
            borderRadius: 8,
            padding: 4,
            margin: 0,
            listStyle: 'none',
            minWidth: 180,
            boxShadow: '0 8px 24px rgba(0,0,0,0.2)',
          }}
        >
          {items.map((item) => (
            <li key={item}>
              <button
                type="button"
                onClick={() => {
                  setOpen(false);
                  onPick(item);
                }}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: '6px 10px',
                  border: 'none',
                  background: 'transparent',
                  fontSize: 12,
                  cursor: 'pointer',
                  color: 'inherit',
                }}
              >
                {item}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function DatasetHeader({
  dataset,
  branches,
  markings,
  busy = false,
  jobSpecStatus,
  canManage = false,
  onSwitchBranch,
  onCreateBranch,
  onAllActions,
  onBuild,
  onAnalyze,
  onExplorePipeline,
  onOpenIn,
  onPublishToMarketplace,
}: DatasetHeaderProps) {
  const breadcrumbs = (dataset.storage_path ?? '').split('/').filter(Boolean);

  return (
    <header style={{ display: 'grid', gap: 8 }}>
      {breadcrumbs.length > 0 && (
        <nav className="of-text-muted" style={{ fontSize: 11, fontFamily: 'var(--font-mono)' }} aria-label="Storage breadcrumbs">
          {breadcrumbs.join(' / ')}
        </nav>
      )}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <h1 className="of-heading-xl" style={{ margin: 0 }}>{dataset.name}</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 11, fontFamily: 'var(--font-mono)' }}>{dataset.id}</p>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginTop: 8 }}>
            <BranchPicker
              branches={branches}
              currentBranch={dataset.active_branch}
              busy={busy}
              onSwitch={onSwitchBranch}
              onCreate={onCreateBranch}
            />
            <JobSpecBadge
              hasMasterJobspec={Boolean(jobSpecStatus?.has_master_jobspec)}
              branchesWithJobspec={jobSpecStatus?.branches_with_jobspec}
            />
            {markings.map((m) => (
              <MarkingBadge key={m.id} id={m.id} label={m.label} level={m.level} source={m.source} compact />
            ))}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          <MenuButton label="All actions" items={ALL_ACTIONS} onPick={(a) => onAllActions?.(a)} />
          <MenuButton label="Build" items={['Build now', 'Run schedule']} onPick={(t) => onBuild?.(t)} />
          <MenuButton label="Analyze in Contour" items={['Open in Contour']} onPick={(t) => onAnalyze?.(t)} />
          <MenuButton label="Explore pipeline" items={['Lineage', 'Pipeline graph']} onPick={(t) => onExplorePipeline?.(t)} />
          <MenuButton
            label="Open in…"
            items={OPEN_IN_TARGETS.map((t) => `${t.label}${t.comingSoon ? ' (coming soon)' : ''}`)}
            onPick={(item) => {
              const match = OPEN_IN_TARGETS.find((t) => item.startsWith(t.label));
              if (match && !match.comingSoon) void onOpenIn?.(match.target);
            }}
          />
          {canManage && onPublishToMarketplace && (
            <button type="button" onClick={onPublishToMarketplace} className="of-button of-button--primary" style={{ fontSize: 12 }}>
              Publish to Marketplace
            </button>
          )}
        </div>
      </div>
    </header>
  );
}
