import { useEffect, useMemo, useState } from 'react';

import {
  listPipelineTransformCatalog,
  type PipelineTransformCatalogEntry,
} from '@/lib/api/pipelines';
import { Glyph } from '@/lib/components/ui/Glyph';
import {
  CAST_TYPES,
  FILTER_OPERATORS,
  blockSectionLabel,
  newCastBlock,
  newDropColumnsBlock,
  newFilterBlock,
  newHaversineDistanceBlock,
  newNormalizeColumnsBlock,
  newRenameColumnsBlock,
  newSelectColumnsBlock,
  type CastBlock,
  type DropColumnsBlock,
  type FilterBlock,
  type FilterCondition,
  type HaversineDistanceBlock,
  type NormalizeColumnsBlock,
  type RenameColumnsBlock,
  type SelectColumnsBlock,
  type TransformBlock,
  type TransformStack,
} from './transformStack';

interface TransformStackEditorProps {
  open: boolean;
  stack: TransformStack | null;
  schema?: string[];
  onClose: () => void;
  onApplyAll: (next: TransformStack) => void;
}

interface AddBlockOption {
  id: string;
  label: string;
  description: string;
  status: string;
  create?: () => TransformBlock;
}

const STACK_BLOCK_CREATORS: Record<string, () => TransformBlock> = {
  select: () => newSelectColumnsBlock(),
  cast: () => newCastBlock(),
  filter: () => newFilterBlock(),
  drop: () => newDropColumnsBlock(),
  rename: () => newRenameColumnsBlock(),
  normalize_columns: () => newNormalizeColumnsBlock(),
  haversine_distance: () => newHaversineDistanceBlock(),
};

const FALLBACK_ADD_BLOCK_OPTIONS: AddBlockOption[] = [
  { id: 'select', label: 'Select columns', description: 'Project an ordered set of columns.', status: 'available', create: () => newSelectColumnsBlock() },
  { id: 'cast', label: 'Cast column', description: 'Convert a column to another scalar type.', status: 'available', create: () => newCastBlock() },
  { id: 'filter', label: 'Filter rows', description: 'Keep or drop rows by condition.', status: 'available', create: () => newFilterBlock() },
  { id: 'drop', label: 'Drop columns', description: 'Remove columns from the output.', status: 'available', create: () => newDropColumnsBlock() },
  { id: 'rename', label: 'Rename columns', description: 'Rename columns while preserving values.', status: 'available', create: () => newRenameColumnsBlock() },
  { id: 'normalize_columns', label: 'Normalize column names', description: 'Convert names to snake_case.', status: 'available', create: () => newNormalizeColumnsBlock() },
  { id: 'haversine_distance', label: 'Haversine distance', description: 'Calculate distance between two coordinate pairs.', status: 'available', create: () => newHaversineDistanceBlock() },
];

export function TransformStackEditor({ open, stack, schema = [], onClose, onApplyAll }: TransformStackEditorProps) {
  const [draft, setDraft] = useState<TransformStack | null>(null);
  const [search, setSearch] = useState('');
  const [showSearchMenu, setShowSearchMenu] = useState(false);
  const [catalogEntries, setCatalogEntries] = useState<PipelineTransformCatalogEntry[]>([]);
  const [catalogVersion, setCatalogVersion] = useState('');
  const [catalogError, setCatalogError] = useState('');

  useEffect(() => {
    if (!open) return;
    setDraft(stack ? { ...stack, blocks: stack.blocks.map((block) => ({ ...block })) } : null);
    setSearch('');
    setShowSearchMenu(false);
  }, [open, stack]);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    listPipelineTransformCatalog()
      .then((catalog) => {
        if (cancelled) return;
        setCatalogEntries(catalog.transforms);
        setCatalogVersion(catalog.schema_version);
        setCatalogError('');
      })
      .catch((cause) => {
        if (cancelled) return;
        setCatalogEntries([]);
        setCatalogVersion('');
        setCatalogError(cause instanceof Error ? cause.message : 'catalog unavailable');
      });
    return () => {
      cancelled = true;
    };
  }, [open]);

  const allApplied = useMemo(() => Boolean(draft && draft.blocks.every((block) => block.applied)), [draft]);
  const addBlockOptions = useMemo<AddBlockOption[]>(() => {
    const stackEntries = catalogEntries.filter((entry) => entry.builder_surface === 'transform_stack');
    if (stackEntries.length === 0) return FALLBACK_ADD_BLOCK_OPTIONS;
    return stackEntries.map((entry) => ({
      id: entry.id,
      label: entry.label,
      description: entry.description,
      status: entry.execution_status,
      create: entry.execution_status === 'available' ? STACK_BLOCK_CREATORS[entry.id] : undefined,
    }));
  }, [catalogEntries]);
  const catalogByID = useMemo(() => new Map(catalogEntries.map((entry) => [entry.id, entry])), [catalogEntries]);
  const filteredAddOptions = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return addBlockOptions;
    return addBlockOptions.filter((entry) => `${entry.label} ${entry.description} ${entry.id}`.toLowerCase().includes(q));
  }, [addBlockOptions, search]);

  if (!open || !draft) return null;

  function patchBlock<T extends TransformBlock>(id: string, patch: Partial<T>) {
    setDraft((current) => {
      if (!current) return current;
      return {
        ...current,
        blocks: current.blocks.map((block) => (block.id === id ? ({ ...block, ...patch } as TransformBlock) : block)),
      };
    });
  }

  function removeBlock(id: string) {
    setDraft((current) => {
      if (!current) return current;
      return { ...current, blocks: current.blocks.filter((block) => block.id !== id) };
    });
  }

  function addBlock(creator?: () => TransformBlock) {
    if (!creator) return;
    setDraft((current) => {
      if (!current) return current;
      return { ...current, blocks: [...current.blocks, creator()] };
    });
    setSearch('');
    setShowSearchMenu(false);
  }

  function applyAll() {
    if (!draft) return;
    const next: TransformStack = {
      ...draft,
      blocks: draft.blocks.map((block) => ({ ...block, applied: true })),
    };
    onApplyAll(next);
    onClose();
  }

  function setName(value: string) {
    setDraft((current) => (current ? { ...current, display_name: value } : current));
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="transform-editor-title"
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 95,
        background: '#f4f6f9',
        display: 'grid',
        gridTemplateRows: 'auto 1fr auto',
      }}
    >
      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 14,
          padding: '10px 18px',
          borderBottom: '1px solid var(--border-subtle)',
          background: '#fff',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0, flex: 1 }}>
          <span
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: 28,
              height: 28,
              borderRadius: 4,
              background: 'rgba(45, 114, 210, 0.12)',
              color: 'var(--status-info)',
            }}
          >
            <TransformGlyph />
          </span>
          <input
            id="transform-editor-title"
            value={draft.display_name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Transform name"
            style={{
              border: 0,
              outline: 'none',
              fontSize: 15,
              fontWeight: 600,
              color: 'var(--text-strong)',
              background: 'transparent',
              flex: 1,
              minWidth: 0,
            }}
          />
          <Glyph name="sparkles" size={14} tone="#7c5dd6" />
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, color: 'var(--text-muted)' }}>
            <button type="button" aria-label="Edit" style={iconBtnStyle()}><Glyph name="pencil" size={13} /></button>
            <button type="button" aria-label="Find" style={iconBtnStyle()}><Glyph name="search" size={13} /></button>
            <button type="button" aria-label="View source" style={iconBtnStyle()}>{'</>'}</button>
            <button type="button" aria-label="Functions" style={iconBtnStyle()}><span style={{ fontFamily: 'serif', fontStyle: 'italic', fontSize: 14 }}>fx</span></button>
            <button type="button" aria-label="Add" style={iconBtnStyle()}><Glyph name="plus" size={13} /></button>
            <span style={{ width: 1, height: 16, background: 'var(--border-subtle)', margin: '0 4px' }} />
            <button type="button" aria-label="Copy" style={iconBtnStyle()}><Glyph name="duplicate" size={13} /></button>
            <button type="button" aria-label="AI" style={iconBtnStyle()}><Glyph name="sparkles" size={13} tone="#7c5dd6" /></button>
            <button type="button" aria-label="Expand" style={iconBtnStyle()}><Glyph name="chevron-down" size={13} /></button>
            <button type="button" aria-label="Collapse" style={iconBtnStyle()}><Glyph name="chevron-down" size={13} /></button>
            <button type="button" aria-label="Delete stack" style={iconBtnStyle()}><Glyph name="trash" size={13} tone="#b42318" /></button>
          </span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <button type="button" className="of-button" onClick={() => onClose()}>
            <Glyph name="chevron-down" size={12} />
            Expand all
          </button>
          <button
            type="button"
            onClick={applyAll}
            disabled={allApplied || draft.blocks.length === 0}
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 6,
              padding: '8px 14px',
              border: 0,
              borderRadius: 4,
              background: '#2d72d2',
              color: '#fff',
              fontSize: 13,
              fontWeight: 600,
              cursor: allApplied || draft.blocks.length === 0 ? 'not-allowed' : 'pointer',
              opacity: allApplied || draft.blocks.length === 0 ? 0.55 : 1,
            }}
          >
            <Glyph name="check" size={13} tone="#fff" />
            Apply all
          </button>
          <button type="button" className="of-button" onClick={onClose}>
            <Glyph name="x" size={12} />
            Close
          </button>
        </div>
      </header>

      <div style={{ overflowY: 'auto', padding: '16px 18px' }}>
        <div style={{ maxWidth: 960, margin: '0 auto', display: 'grid', gap: 12 }}>
          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 10,
              padding: '8px 14px',
              background: '#fff',
              border: '1px solid var(--border-subtle)',
              borderRadius: 6,
              alignSelf: 'center',
            }}
          >
            <Glyph name="database" size={14} tone="#2d72d2" />
            <strong style={{ fontSize: 13 }}>{draft.source_dataset_name}</strong>
            <span className="of-text-muted" style={{ fontSize: 12 }}>{schema.length || ''} columns</span>
          </div>

          {draft.blocks.length === 0 ? (
            <p className="of-text-muted" style={{ textAlign: 'center', padding: 32 }}>
              Use the search bar at the bottom to add a transform block.
            </p>
          ) : (
            draft.blocks.map((block) => (
              <BlockCard
                key={block.id}
                block={block}
                schema={schema}
                catalogEntry={catalogByID.get(catalogIDForBlock(block))}
                onPatch={(patch) => patchBlock(block.id, patch as Partial<TransformBlock>)}
                onApply={() => patchBlock(block.id, { applied: true })}
                onCancel={() => patchBlock(block.id, { applied: false })}
                onDelete={() => removeBlock(block.id)}
              />
            ))
          )}
        </div>
      </div>

      <footer style={{ padding: '10px 18px', background: '#fff', borderTop: '1px solid var(--border-subtle)' }}>
        <div style={{ maxWidth: 960, margin: '0 auto', display: 'flex', alignItems: 'center', gap: 10, position: 'relative' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 10px', flex: 1, border: '1px solid var(--border-default)', borderRadius: 4, background: '#f4f6f9' }}>
            <Glyph name="search" size={14} tone="#5c7080" />
            <input
              type="search"
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
                setShowSearchMenu(true);
              }}
              onFocus={() => setShowSearchMenu(true)}
              onBlur={() => setTimeout(() => setShowSearchMenu(false), 120)}
              placeholder="Search transforms and columns..."
              style={{ flex: 1, background: 'transparent', border: 0, outline: 'none', fontSize: 13 }}
            />
            <span className="of-text-muted" style={{ fontSize: 11 }}>/</span>
          </div>
          {showSearchMenu && filteredAddOptions.length > 0 ? (
            <div
              role="menu"
              style={{
                position: 'absolute',
                bottom: 'calc(100% + 6px)',
                left: 0,
                right: 130,
                background: '#fff',
                border: '1px solid var(--border-default)',
                borderRadius: 4,
                boxShadow: '0 8px 24px rgba(15, 23, 42, 0.16)',
                maxHeight: 240,
                overflowY: 'auto',
                zIndex: 4,
              }}
            >
              {filteredAddOptions.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  onMouseDown={() => addBlock(option.create)}
                  disabled={!option.create}
                  title={option.create ? option.description : `${option.description} (${option.status})`}
                  style={{
                    display: 'block',
                    width: '100%',
                    padding: '8px 12px',
                    border: 0,
                    background: 'transparent',
                    fontSize: 13,
                    textAlign: 'left',
                    cursor: option.create ? 'pointer' : 'not-allowed',
                    color: option.create ? 'var(--text-strong)' : 'var(--text-muted)',
                  }}
                  onMouseEnter={(event) => (event.currentTarget.style.background = 'rgba(45, 114, 210, 0.06)')}
                  onMouseLeave={(event) => (event.currentTarget.style.background = 'transparent')}
                >
                  <span style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                    <span>{option.label}</span>
                    <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{option.status}</span>
                  </span>
                </button>
              ))}
            </div>
          ) : null}
          <button type="button" className="of-button" disabled>
            <Glyph name="sparkles" size={13} tone="#7c5dd6" />
            Generate
          </button>
          <span className="of-text-muted" style={{ fontSize: 11 }}>
            {catalogVersion || (catalogError ? 'local catalog' : 'catalog')}
          </span>
        </div>
      </footer>
    </div>
  );
}

function BlockCard({
  block,
  schema,
  catalogEntry,
  onPatch,
  onApply,
  onCancel,
  onDelete,
}: {
  block: TransformBlock;
  schema: string[];
  catalogEntry?: PipelineTransformCatalogEntry;
  onPatch: (patch: Partial<TransformBlock>) => void;
  onApply: () => void;
  onCancel: () => void;
  onDelete: () => void;
}) {
  const accent = blockAccent(block.kind);
  return (
    <section
      style={{
        background: '#fff',
        border: '1px solid var(--border-subtle)',
        borderLeft: `3px solid ${accent}`,
        borderRadius: 6,
        overflow: 'hidden',
      }}
    >
      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 8,
          padding: '8px 14px',
          borderBottom: '1px solid var(--border-subtle)',
          background: 'rgba(45, 114, 210, 0.02)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.06em', color: accent, textTransform: 'uppercase' }}>
            {catalogEntry?.label ?? blockSectionLabel(block.kind)}
          </span>
          {catalogEntry ? <span className="of-chip" style={{ fontSize: 10 }}>{catalogEntry.form.kind}</span> : null}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <button type="button" className="of-button" onClick={onCancel} style={{ fontSize: 12 }}>
            <Glyph name="x" size={12} /> Cancel
          </button>
          <button type="button" className="of-button" disabled style={{ fontSize: 12 }}>
            <Glyph name="plus" size={12} /> Preview
          </button>
          <button type="button" aria-label="AI suggest" style={iconBtnStyle()}><Glyph name="sparkles" size={13} tone="#7c5dd6" /></button>
          <button type="button" aria-label="Copy" style={iconBtnStyle()}><Glyph name="duplicate" size={13} /></button>
          <button type="button" aria-label="Reorder" style={iconBtnStyle()}><Glyph name="move" size={13} /></button>
          <button type="button" aria-label="Delete" style={iconBtnStyle()} onClick={onDelete}><Glyph name="trash" size={13} tone="#b42318" /></button>
        </div>
      </header>
      <div style={{ padding: '12px 14px', display: 'grid', gap: 10 }}>
        {block.kind === 'cast' ? (
          <CastBody block={block} schema={schema} onPatch={(patch) => onPatch(patch)} />
        ) : null}
        {block.kind === 'filter' ? (
          <FilterBody block={block} schema={schema} onPatch={(patch) => onPatch(patch)} />
        ) : null}
        {block.kind === 'select' ? (
          <SelectBody block={block} schema={schema} onPatch={(patch) => onPatch(patch)} />
        ) : null}
        {block.kind === 'drop' ? (
          <DropBody block={block} schema={schema} onPatch={(patch) => onPatch(patch)} />
        ) : null}
        {block.kind === 'rename' ? (
          <RenameBody block={block} schema={schema} onPatch={(patch) => onPatch(patch)} />
        ) : null}
        {block.kind === 'normalize' ? (
          <NormalizeBody block={block} onPatch={(patch) => onPatch(patch)} />
        ) : null}
        {block.kind === 'haversine_distance' ? (
          <HaversineBody block={block} schema={schema} onPatch={(patch) => onPatch(patch)} />
        ) : null}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 4 }}>
          <button type="button" className="of-button" onClick={onCancel} style={{ fontSize: 12 }} disabled={!block.applied}>
            Cancel
          </button>
          <button
            type="button"
            onClick={onApply}
            disabled={block.applied}
            style={{
              padding: '6px 12px',
              border: 0,
              borderRadius: 4,
              background: '#2d72d2',
              color: '#fff',
              fontSize: 12,
              fontWeight: 600,
              cursor: block.applied ? 'not-allowed' : 'pointer',
              opacity: block.applied ? 0.55 : 1,
            }}
          >
            {block.applied ? 'Applied' : 'Apply'}
          </button>
        </div>
      </div>
    </section>
  );
}

function CastBody({
  block,
  schema,
  onPatch,
}: {
  block: CastBlock;
  schema: string[];
  onPatch: (patch: Partial<CastBlock>) => void;
}) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '120px 1fr 60px 1fr', gap: 10, alignItems: 'center' }}>
      <span className="of-text-muted" style={{ fontSize: 12 }}>Expression *</span>
      <ColumnInput value={block.source_column} onChange={(value) => onPatch({ source_column: value, target_column: block.target_column || value })} schema={schema} />
      <span style={{ textAlign: 'center', color: 'var(--text-muted)' }}>→</span>
      <input
        value={block.target_column}
        onChange={(event) => onPatch({ target_column: event.target.value })}
        placeholder="Target column"
        style={inputStyle()}
      />
      <span className="of-text-muted" style={{ fontSize: 12 }}>Type *</span>
      <select value={block.target_type} onChange={(event) => onPatch({ target_type: event.target.value as CastBlock['target_type'] })} style={inputStyle()}>
        {CAST_TYPES.map((type) => (
          <option key={type} value={type}>{type}</option>
        ))}
      </select>
      <span />
      <span style={{ fontSize: 11, color: 'var(--text-muted)', alignSelf: 'center' }}>Replace</span>
    </div>
  );
}

function FilterBody({
  block,
  schema,
  onPatch,
}: {
  block: FilterBlock;
  schema: string[];
  onPatch: (patch: Partial<FilterBlock>) => void;
}) {
  function patchCondition(index: number, patch: Partial<FilterCondition>) {
    const conditions = block.conditions.map((cond, i) => (i === index ? { ...cond, ...patch } : cond));
    onPatch({ conditions });
  }
  function removeCondition(index: number) {
    if (block.conditions.length <= 1) return;
    onPatch({ conditions: block.conditions.filter((_, i) => i !== index) });
  }
  function addCondition() {
    onPatch({
      conditions: [...block.conditions, { column: '', operator: 'is_not_null', value: '', treat_empty_as_null: true }],
    });
  }
  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
        <select value={block.mode} onChange={(event) => onPatch({ mode: event.target.value as FilterBlock['mode'] })} style={inputStyle()}>
          <option value="keep">Keep rows</option>
          <option value="drop">Drop rows</option>
        </select>
        <span className="of-text-muted">that match</span>
        <select value={block.match} onChange={(event) => onPatch({ match: event.target.value as FilterBlock['match'] })} style={inputStyle()}>
          <option value="all">all conditions</option>
          <option value="any">any condition</option>
        </select>
      </div>
      {block.conditions.map((condition, index) => (
        <div key={index} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <ColumnInput
            value={condition.column}
            onChange={(value) => patchCondition(index, { column: value })}
            schema={schema}
          />
          <select
            value={condition.operator}
            onChange={(event) => patchCondition(index, { operator: event.target.value as FilterCondition['operator'] })}
            style={inputStyle()}
          >
            {FILTER_OPERATORS.map((op) => (
              <option key={op.id} value={op.id}>{op.label}</option>
            ))}
          </select>
          {(condition.operator === 'equals' || condition.operator === 'not_equals' || condition.operator === 'greater_than' || condition.operator === 'less_than') ? (
            <input
              value={condition.value}
              onChange={(event) => patchCondition(index, { value: event.target.value })}
              placeholder="Value"
              style={{ ...inputStyle(), flex: 1 }}
            />
          ) : null}
          <label style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--text-muted)' }}>
            <input
              type="checkbox"
              checked={condition.treat_empty_as_null}
              onChange={(event) => patchCondition(index, { treat_empty_as_null: event.target.checked })}
            />
            Treat empty string as null
          </label>
          {block.conditions.length > 1 ? (
            <button type="button" aria-label="Remove condition" style={iconBtnStyle()} onClick={() => removeCondition(index)}>
              <Glyph name="x" size={12} />
            </button>
          ) : null}
        </div>
      ))}
      <div style={{ display: 'flex', gap: 8 }}>
        <button type="button" className="of-button" style={{ fontSize: 12 }} onClick={addCondition}>
          <Glyph name="plus" size={12} /> Add condition
        </button>
        <button type="button" className="of-button" style={{ fontSize: 12 }} disabled>
          <Glyph name="plus" size={12} /> Add condition group
        </button>
      </div>
    </div>
  );
}

function DropBody({
  block,
  schema,
  onPatch,
}: {
  block: DropColumnsBlock;
  schema: string[];
  onPatch: (patch: Partial<DropColumnsBlock>) => void;
}) {
  const [draftEntry, setDraftEntry] = useState('');

  function commitDraft() {
    const value = draftEntry.trim();
    if (!value) return;
    if (block.columns.includes(value)) return;
    onPatch({ columns: [...block.columns, value] });
    setDraftEntry('');
  }

  function removeColumn(name: string) {
    onPatch({ columns: block.columns.filter((entry) => entry !== name) });
  }

  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <span className="of-text-muted" style={{ fontSize: 12 }}>Columns to drop *</span>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 6, padding: 6, border: '1px solid var(--border-default)', borderRadius: 4, background: '#fff', minHeight: 36 }}>
        {block.columns.map((name) => (
          <span key={name} style={chipStyle()}>
            {name}
            <button type="button" aria-label="Remove" style={chipDeleteStyle()} onClick={() => removeColumn(name)}>
              <Glyph name="x" size={10} />
            </button>
          </span>
        ))}
        <input
          list={`drop-cols-${block.id}`}
          value={draftEntry}
          onChange={(event) => setDraftEntry(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ',') {
              event.preventDefault();
              commitDraft();
            }
          }}
          onBlur={commitDraft}
          placeholder="Search for columns..."
          style={{ flex: 1, minWidth: 120, border: 0, outline: 'none', fontSize: 13 }}
        />
        <datalist id={`drop-cols-${block.id}`}>
          {schema.map((name) => <option key={name} value={name} />)}
        </datalist>
      </div>
    </div>
  );
}

function SelectBody({
  block,
  schema,
  onPatch,
}: {
  block: SelectColumnsBlock;
  schema: string[];
  onPatch: (patch: Partial<SelectColumnsBlock>) => void;
}) {
  const [draftEntry, setDraftEntry] = useState('');

  function commitDraft() {
    const value = draftEntry.trim();
    if (!value) return;
    if (block.columns.includes(value)) return;
    onPatch({ columns: [...block.columns, value] });
    setDraftEntry('');
  }

  function removeColumn(name: string) {
    onPatch({ columns: block.columns.filter((entry) => entry !== name) });
  }

  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <span className="of-text-muted" style={{ fontSize: 12 }}>Columns to keep *</span>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 6, padding: 6, border: '1px solid var(--border-default)', borderRadius: 4, background: '#fff', minHeight: 36 }}>
        {block.columns.map((name) => (
          <span key={name} style={chipStyle()}>
            {name}
            <button type="button" aria-label="Remove" style={chipDeleteStyle()} onClick={() => removeColumn(name)}>
              <Glyph name="x" size={10} />
            </button>
          </span>
        ))}
        <input
          list={`select-cols-${block.id}`}
          value={draftEntry}
          onChange={(event) => setDraftEntry(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ',') {
              event.preventDefault();
              commitDraft();
            }
          }}
          onBlur={commitDraft}
          placeholder="Search for columns..."
          style={{ flex: 1, minWidth: 120, border: 0, outline: 'none', fontSize: 13 }}
        />
        <datalist id={`select-cols-${block.id}`}>
          {schema.map((name) => <option key={name} value={name} />)}
        </datalist>
      </div>
    </div>
  );
}

function RenameBody({
  block,
  schema,
  onPatch,
}: {
  block: RenameColumnsBlock;
  schema: string[];
  onPatch: (patch: Partial<RenameColumnsBlock>) => void;
}) {
  function patchEntry(index: number, patch: Partial<{ from: string; to: string }>) {
    const renames = block.renames.map((entry, i) => (i === index ? { ...entry, ...patch } : entry));
    onPatch({ renames });
  }
  function addEntry() {
    onPatch({ renames: [...block.renames, { from: '', to: '' }] });
  }
  function removeEntry(index: number) {
    if (block.renames.length <= 1) return;
    onPatch({ renames: block.renames.filter((_, i) => i !== index) });
  }
  return (
    <div style={{ display: 'grid', gap: 8 }}>
      {block.renames.map((entry, index) => (
        <div key={index} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <ColumnInput value={entry.from} onChange={(value) => patchEntry(index, { from: value })} schema={schema} />
          <span style={{ flex: 1 }}>
            <input
              value={entry.to}
              onChange={(event) => patchEntry(index, { to: event.target.value })}
              placeholder="New name"
              style={{ ...inputStyle(), width: '100%' }}
            />
          </span>
          {block.renames.length > 1 ? (
            <button type="button" aria-label="Remove rename" style={iconBtnStyle()} onClick={() => removeEntry(index)}>
              <Glyph name="x" size={12} />
            </button>
          ) : null}
        </div>
      ))}
      <div style={{ display: 'flex', gap: 8 }}>
        <button type="button" className="of-button" onClick={addEntry} style={{ fontSize: 12 }}>
          <Glyph name="plus" size={12} /> Add rename
        </button>
        <button type="button" className="of-button" disabled style={{ fontSize: 12 }}>
          <Glyph name="pencil" size={12} /> Add multiple…
        </button>
      </div>
    </div>
  );
}

function NormalizeBody({
  block,
  onPatch,
}: {
  block: NormalizeColumnsBlock;
  onPatch: (patch: Partial<NormalizeColumnsBlock>) => void;
}) {
  return (
    <div style={{ display: 'grid', gap: 10 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 12px', background: 'rgba(45, 114, 210, 0.06)', borderRadius: 4, fontSize: 12 }}>
        <Glyph name="info" size={14} tone="#2d72d2" />
        Normalizes column names to use lower_snake_case.
      </div>
      <label style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
        <input
          type="checkbox"
          checked={block.remove_special_characters}
          onChange={(event) => onPatch({ remove_special_characters: event.target.checked })}
          style={{ width: 32, height: 16, accentColor: 'var(--status-info)' }}
        />
        Remove special characters
        <span title="Strips characters outside [a-z0-9_]" style={{ display: 'inline-flex' }}>
          <Glyph name="info" size={12} tone="#5c7080" />
        </span>
      </label>
    </div>
  );
}

function HaversineBody({
  block,
  schema,
  onPatch,
}: {
  block: HaversineDistanceBlock;
  schema: string[];
  onPatch: (patch: Partial<HaversineDistanceBlock>) => void;
}) {
  function patchUnit(unit: HaversineDistanceBlock['unit']) {
    const suffix = unit === 'km' ? 'km' : unit === 'meters' ? 'meters' : 'miles';
    onPatch({ unit, target_column: block.target_column || `distance_${suffix}` });
  }
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '120px 1fr 1fr', gap: 10, alignItems: 'center' }}>
      <span className="of-text-muted" style={{ fontSize: 12 }}>Start point *</span>
      <ColumnInput value={block.start_lat_column} onChange={(value) => onPatch({ start_lat_column: value })} schema={schema} />
      <ColumnInput value={block.start_lon_column} onChange={(value) => onPatch({ start_lon_column: value })} schema={schema} />
      <span className="of-text-muted" style={{ fontSize: 12 }}>End point *</span>
      <ColumnInput value={block.end_lat_column} onChange={(value) => onPatch({ end_lat_column: value })} schema={schema} />
      <ColumnInput value={block.end_lon_column} onChange={(value) => onPatch({ end_lon_column: value })} schema={schema} />
      <span className="of-text-muted" style={{ fontSize: 12 }}>Output *</span>
      <select value={block.unit} onChange={(event) => patchUnit(event.target.value as HaversineDistanceBlock['unit'])} style={inputStyle()}>
        <option value="miles">Miles</option>
        <option value="km">Kilometers</option>
        <option value="meters">Meters</option>
      </select>
      <input
        value={block.target_column}
        onChange={(event) => onPatch({ target_column: event.target.value })}
        placeholder="distance_miles"
        style={inputStyle()}
      />
    </div>
  );
}

function ColumnInput({ value, onChange, schema }: { value: string; onChange: (value: string) => void; schema: string[] }) {
  const datalistId = useMemo(() => `column-list-${Math.random().toString(36).slice(2, 8)}`, []);
  return (
    <span style={{ display: 'inline-flex', flex: 1 }}>
      <input
        list={datalistId}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="Column"
        style={{ ...inputStyle(), width: '100%' }}
      />
      <datalist id={datalistId}>
        {schema.map((name) => <option key={name} value={name} />)}
      </datalist>
    </span>
  );
}

function blockAccent(kind: TransformBlock['kind']): string {
  switch (kind) {
    case 'cast':
      return '#2d72d2';
    case 'filter':
      return '#15803d';
    case 'select':
      return '#2563eb';
    case 'drop':
      return '#b42318';
    case 'rename':
      return '#9a5b00';
    case 'normalize':
      return '#7c5dd6';
    case 'haversine_distance':
      return '#047857';
  }
}

function catalogIDForBlock(block: TransformBlock): string {
  if (block.kind === 'normalize') return 'normalize_columns';
  return block.kind;
}

function inputStyle(): React.CSSProperties {
  return {
    padding: '6px 10px',
    border: '1px solid var(--border-default)',
    borderRadius: 4,
    background: '#fff',
    fontSize: 13,
    color: 'var(--text-strong)',
  };
}

function iconBtnStyle(): React.CSSProperties {
  return {
    border: 0,
    background: 'transparent',
    padding: 4,
    cursor: 'pointer',
    color: 'var(--text-muted)',
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: 4,
  };
}

function chipStyle(): React.CSSProperties {
  return {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    padding: '2px 8px',
    background: 'rgba(45, 114, 210, 0.1)',
    color: 'var(--status-info)',
    border: '1px solid rgba(45, 114, 210, 0.3)',
    borderRadius: 999,
    fontSize: 12,
  };
}

function chipDeleteStyle(): React.CSSProperties {
  return {
    border: 0,
    background: 'transparent',
    padding: 0,
    color: 'inherit',
    cursor: 'pointer',
    display: 'inline-flex',
    alignItems: 'center',
  };
}

function TransformGlyph() {
  return (
    <svg width={14} height={14} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M5 6h14M7 12h10M9 18h6" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  );
}
