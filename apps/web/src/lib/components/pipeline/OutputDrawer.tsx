import { useEffect, useState } from 'react';

import { Glyph } from '@/lib/components/ui/Glyph';

export interface OutputDraft {
  kind?: 'dataset' | 'object_type' | 'link_type';
  display_name: string;
  source_node_id: string;
  source_node_label: string;
  columns_total: number;
  columns_mapped: number;
  expectations_open?: boolean;
  write_mode_open?: boolean;
}

interface OutputDrawerProps {
  open: boolean;
  draft: OutputDraft | null;
  onClose: () => void;
  onChangeName: (name: string) => void;
}

export function OutputDrawer({ open, draft, onClose, onChangeName }: OutputDrawerProps) {
  const [expectationsOpen, setExpectationsOpen] = useState(false);
  const [writeModeOpen, setWriteModeOpen] = useState(false);

  useEffect(() => {
    if (!open) return;
    setExpectationsOpen(false);
    setWriteModeOpen(false);
  }, [open]);

  if (!open || !draft) return null;

  return (
    <aside
      style={{
        position: 'fixed',
        top: 0,
        right: 0,
        bottom: 0,
        width: 360,
        zIndex: 80,
        background: '#fff',
        borderLeft: '1px solid var(--border-default)',
        boxShadow: '-12px 0 32px rgba(15, 23, 42, 0.08)',
        display: 'grid',
        gridTemplateRows: 'auto 1fr',
      }}
    >
      <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        <button type="button" onClick={onClose} className="of-button of-button--ghost" style={{ padding: 4, fontSize: 13 }}>
          <Glyph name="chevron-left" size={14} /> Back to outputs
        </button>
        <button type="button" aria-label="Close" onClick={onClose} className="of-button of-button--ghost" style={{ padding: 4 }}>
          <Glyph name="x" size={12} />
        </button>
      </header>

      <div style={{ overflowY: 'auto', padding: 14, display: 'grid', gap: 14 }}>
        <section
          style={{
            border: '1.5px solid #15803d',
            borderRadius: 6,
            padding: '10px 12px',
            display: 'grid',
            gap: 6,
            background: 'rgba(34, 197, 94, 0.04)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10 }}>
            <span
              style={{
                display: 'inline-flex',
                width: 28,
                height: 28,
                alignItems: 'center',
                justifyContent: 'center',
                borderRadius: 4,
                background: 'rgba(45, 114, 210, 0.12)',
                color: 'var(--status-info)',
              }}
            >
              <NewDatasetGlyph />
            </span>
            <input
              value={draft.display_name}
              onChange={(event) => onChangeName(event.target.value)}
              style={{
                flex: 1,
                fontSize: 14,
                fontWeight: 600,
                color: 'var(--text-strong)',
                border: 0,
                outline: 'none',
                background: 'transparent',
                padding: 0,
              }}
            />
          </div>
          <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>
            {draft.kind === 'object_type'
              ? 'Object type and backing dataset will be created after first build'
              : draft.kind === 'link_type'
                ? 'Link type and link rows will be created after first build'
                : 'Output will be created after first build'}
          </p>
        </section>

        <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: 'var(--text-strong)' }}>
          <Glyph name="check" size={14} tone="#15803d" />
          {draft.columns_mapped}/{draft.columns_total} columns mapped
        </div>

        <Collapsible
          icon="autosaved"
          label="Configure expectations"
          open={expectationsOpen}
          onToggle={() => setExpectationsOpen((value) => !value)}
        >
          <p className="of-text-muted" style={{ fontSize: 12, margin: 0 }}>
            Define data quality expectations that gate the deploy. Coming soon.
          </p>
        </Collapsible>

        <Collapsible
          icon="document"
          label="Configure write mode"
          open={writeModeOpen}
          onToggle={() => setWriteModeOpen((value) => !value)}
        >
          <p className="of-text-muted" style={{ fontSize: 12, margin: 0 }}>
            Choose between snapshot, append, or upsert when writing to this output. Coming soon.
          </p>
        </Collapsible>
      </div>
    </aside>
  );
}

function Collapsible({
  icon,
  label,
  open,
  onToggle,
  children,
}: {
  icon: 'autosaved' | 'document';
  label: string;
  open: boolean;
  onToggle: () => void;
  children: React.ReactNode;
}) {
  return (
    <section style={{ border: '1px solid var(--border-subtle)', borderRadius: 6, overflow: 'hidden' }}>
      <button
        type="button"
        onClick={onToggle}
        style={{
          width: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '10px 12px',
          background: 'transparent',
          border: 0,
          cursor: 'pointer',
          fontSize: 13,
          color: 'var(--text-strong)',
          fontWeight: 600,
        }}
      >
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <Glyph name={icon} size={14} tone="#5c7080" />
          {label}
        </span>
        <Glyph name="chevron-down" size={12} tone="#5c7080" />
      </button>
      {open ? <div style={{ padding: '0 12px 12px' }}>{children}</div> : null}
    </section>
  );
}

function NewDatasetGlyph() {
  return (
    <svg width={14} height={14} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="5" width="13" height="14" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
      <path d="M4 9h13" stroke="currentColor" strokeWidth="1.6" />
      <circle cx="19" cy="18" r="3.6" fill="currentColor" />
      <path d="M19 16.4v3.2M17.4 18h3.2" stroke="#fff" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  );
}
