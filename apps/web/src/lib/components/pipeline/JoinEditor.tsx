import { useEffect, useState } from 'react';

import type {
  PipelineIRField,
  PipelineJoinSchemaGuidance,
  PipelineSchemaGuidanceDiagnostic,
} from '@/lib/api/pipelines';
import { Glyph } from '@/lib/components/ui/Glyph';
import {
  JOIN_TYPES,
  type JoinDraft,
  type JoinMatch,
  type JoinType,
} from './joinDraft';

interface JoinEditorProps {
  open: boolean;
  draft: JoinDraft | null;
  leftSchema?: string[];
  rightSchema?: string[];
  guidance?: PipelineJoinSchemaGuidance | null;
  onClose: () => void;
  onApply: (next: JoinDraft) => void;
}

export function JoinEditor({ open, draft, leftSchema = [], rightSchema = [], guidance = null, onClose, onApply }: JoinEditorProps) {
  const [working, setWorking] = useState<JoinDraft | null>(null);

  useEffect(() => {
    if (!open) return;
    setWorking(draft ? { ...draft, matches: draft.matches.map((m) => ({ ...m })) } : null);
  }, [open, draft]);

  if (!open || !working) return null;

  const leftFields = guidance?.left_schema ?? fieldsFromNames(leftSchema);
  const rightFields = guidance?.right_schema ?? fieldsFromNames(rightSchema);
  const leftColumns = leftFields.map((field) => field.name);
  const rightColumns = rightFields.map((field) => field.name);
  const activeDiagnostics = joinDiagnosticsForWorking(working, leftFields, rightFields);
  const compatibleCandidates = (guidance?.candidate_keys ?? []).filter((candidate) => candidate.compatible).slice(0, 6);

  function patch<K extends keyof JoinDraft>(key: K, value: JoinDraft[K]) {
    setWorking((current) => (current ? { ...current, [key]: value } : current));
  }

  function patchMatch(index: number, p: Partial<JoinMatch>) {
    setWorking((current) => {
      if (!current) return current;
      const matches = current.matches.map((entry, i) => (i === index ? { ...entry, ...p } : entry));
      return { ...current, matches };
    });
  }

  function addMatch() {
    setWorking((current) => (current ? { ...current, matches: [...current.matches, { left_column: '', right_column: '' }] } : current));
  }

  function removeMatch(index: number) {
    setWorking((current) => {
      if (!current || current.matches.length <= 1) return current;
      return { ...current, matches: current.matches.filter((_, i) => i !== index) };
    });
  }

  function swap() {
    setWorking((current) => {
      if (!current) return current;
      return {
        ...current,
        left_node_id: current.right_node_id,
        left_node_label: current.right_node_label,
        right_node_id: current.left_node_id,
        right_node_label: current.left_node_label,
        matches: current.matches.map((entry) => ({ left_column: entry.right_column, right_column: entry.left_column })),
        left_columns: current.right_columns,
        right_columns: current.left_columns,
        auto_select_left: current.auto_select_right,
        auto_select_right: current.auto_select_left,
      };
    });
  }

  function toggleColumn(side: 'left' | 'right', column: string) {
    setWorking((current) => {
      if (!current) return current;
      const key = side === 'left' ? 'left_columns' : 'right_columns';
      const list = current[key];
      const next = list.includes(column) ? list.filter((entry) => entry !== column) : [...list, column];
      return { ...current, [key]: next };
    });
  }

  function selectAll(side: 'left' | 'right', schema: string[]) {
    setWorking((current) => {
      if (!current) return current;
      if (side === 'left') return { ...current, left_columns: [...schema], auto_select_left: false };
      return { ...current, right_columns: [...schema], auto_select_right: false };
    });
  }

  function deselectAll(side: 'left' | 'right') {
    setWorking((current) => {
      if (!current) return current;
      if (side === 'left') return { ...current, left_columns: [], auto_select_left: false };
      return { ...current, right_columns: [], auto_select_right: false };
    });
  }

  function commit() {
    if (working) onApply(working);
    onClose();
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="join-editor-title"
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 95,
        background: '#f4f6f9',
        display: 'grid',
        gridTemplateRows: 'auto 1fr',
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
              background: 'rgba(34, 139, 34, 0.12)',
              color: '#15803d',
            }}
          >
            <JoinGlyph />
          </span>
          <input
            id="join-editor-title"
            value={working.display_name}
            onChange={(event) => patch('display_name', event.target.value)}
            placeholder="Join name"
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
          <button
            type="button"
            onClick={() => {
              const next = window.prompt('Join description', working.description ?? '');
              if (next !== null) patch('description', next);
            }}
            className="of-button"
            style={{ fontSize: 12 }}
          >
            <Glyph name="pencil" size={12} /> Description
          </button>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <button
            type="button"
            onClick={commit}
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
              cursor: 'pointer',
            }}
          >
            <Glyph name="check" size={13} tone="#fff" />
            Applied
          </button>
          <button type="button" className="of-button" onClick={onClose}>
            <Glyph name="x" size={12} />
            Close
          </button>
        </div>
      </header>

      <div style={{ overflowY: 'auto', padding: '20px 24px' }}>
        <div style={{ maxWidth: 1080, margin: '0 auto', display: 'grid', gap: 18 }}>
          <Row label="Join type">
            <select
              value={working.join_type}
              onChange={(event) => patch('join_type', event.target.value as JoinType)}
              style={inputStyle()}
            >
              {JOIN_TYPES.map((entry) => (
                <option key={entry.id} value={entry.id}>{entry.label}</option>
              ))}
            </select>
            <span style={{ marginLeft: 14, fontSize: 12.5, color: 'var(--text-muted)' }}>
              {JOIN_TYPES.find((entry) => entry.id === working.join_type)?.description}
            </span>
          </Row>

          <Row label="Input tables">
            <span style={inputChipStyle()}>
              <Glyph name="folder" size={13} tone="#cf923f" /> {working.left_node_label}
            </span>
            <button type="button" className="of-button" onClick={swap} style={{ fontSize: 12 }}>
              <Glyph name="move" size={12} /> Swap
            </button>
            <span style={inputChipStyle()}>
              <Glyph name="database" size={13} tone="#2d72d2" /> {working.right_node_label}
            </span>
          </Row>

          <Row label="Match condition">
            <div style={{ display: 'grid', gap: 6, flex: 1 }}>
              {compatibleCandidates.length > 0 ? (
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
                  <span className="of-text-muted" style={{ fontSize: 12 }}>Suggested keys</span>
                  {compatibleCandidates.map((candidate) => (
                    <button
                      key={`${candidate.left_column}:${candidate.right_column}`}
                      type="button"
                      className="of-button"
                      onClick={() => patchMatch(0, { left_column: candidate.left_column, right_column: candidate.right_column })}
                      title={`${candidate.reason} · ${candidate.left_type} = ${candidate.right_type}`}
                      style={{ fontSize: 11 }}
                    >
                      {candidate.left_column} = {candidate.right_column}
                    </button>
                  ))}
                </div>
              ) : null}
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: 'var(--text-muted)' }}>
                <span>rows that match</span>
                <select disabled style={inputStyle()}>
                  <option>all conditions</option>
                </select>
                <div style={{ marginLeft: 'auto', display: 'inline-flex', borderRadius: 4, overflow: 'hidden', border: '1px solid var(--border-default)' }}>
                  <span style={{ padding: '6px 12px', background: '#1c2127', color: '#fff', fontSize: 12 }}>Basic</span>
                  <span style={{ padding: '6px 12px', background: '#fff', color: 'var(--text-muted)', fontSize: 12 }}>Advanced</span>
                </div>
              </div>
              {working.matches.map((match, index) => (
                <div key={index} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <ColumnPicker
                    schema={leftColumns}
                    value={match.left_column}
                    onChange={(value) => patchMatch(index, { left_column: value })}
                  />
                  <span style={{ color: 'var(--text-muted)', fontSize: 12 }}>is equal to</span>
                  <ColumnPicker
                    schema={rightColumns}
                    value={match.right_column}
                    onChange={(value) => patchMatch(index, { right_column: value })}
                  />
                  {working.matches.length > 1 ? (
                    <button type="button" aria-label="Remove match" className="of-button" onClick={() => removeMatch(index)} style={{ fontSize: 12 }}>
                      <Glyph name="x" size={12} />
                    </button>
                  ) : null}
                </div>
              ))}
              <div>
                <button type="button" className="of-button" onClick={addMatch} style={{ fontSize: 12 }}>
                  <Glyph name="plus" size={12} /> Add match condition
                </button>
              </div>
              <GuidanceDiagnostics diagnostics={activeDiagnostics} />
            </div>
          </Row>

          <Row label="Select columns">
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 18, flex: 1 }}>
              <ColumnSelector
                title={`Left: ${working.left_node_label}`}
                schema={leftColumns}
                selected={working.left_columns}
                autoSelectAll={working.auto_select_left}
                onAutoToggle={(checked) => patch('auto_select_left', checked)}
                onToggle={(column) => toggleColumn('left', column)}
                onSelectAll={() => selectAll('left', leftColumns)}
                onDeselectAll={() => deselectAll('left')}
              />
              <ColumnSelector
                title={`Right: ${working.right_node_label}`}
                schema={rightColumns}
                selected={working.right_columns}
                autoSelectAll={working.auto_select_right}
                onAutoToggle={(checked) => patch('auto_select_right', checked)}
                onToggle={(column) => toggleColumn('right', column)}
                onSelectAll={() => selectAll('right', rightColumns)}
                onDeselectAll={() => deselectAll('right')}
                prefix={working.right_prefix ?? ''}
                onPrefixChange={(value) => patch('right_prefix', value)}
              />
            </div>
          </Row>
        </div>
      </div>
    </div>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '160px minmax(0, 1fr)', gap: 18, alignItems: 'flex-start' }}>
      <p className="of-text-muted" style={{ margin: 0, fontSize: 13 }}>{label}</p>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>{children}</div>
    </div>
  );
}

function ColumnPicker({ schema, value, onChange }: { schema: string[]; value: string; onChange: (value: string) => void }) {
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, background: '#fff', flex: 1 }}>
      <Glyph name="cube" size={12} tone="#5c7080" />
      <input
        list={`column-list-${value}`}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="Column"
        style={{ flex: 1, border: 0, outline: 'none', fontSize: 13 }}
      />
      <datalist id={`column-list-${value}`}>
        {schema.map((name) => <option key={name} value={name} />)}
      </datalist>
      <Glyph name="chevron-down" size={12} tone="#5c7080" />
    </span>
  );
}

function ColumnSelector({
  title,
  schema,
  selected,
  autoSelectAll,
  onAutoToggle,
  onToggle,
  onSelectAll,
  onDeselectAll,
  prefix,
  onPrefixChange,
}: {
  title: string;
  schema: string[];
  selected: string[];
  autoSelectAll: boolean;
  onAutoToggle: (checked: boolean) => void;
  onToggle: (column: string) => void;
  onSelectAll: () => void;
  onDeselectAll: () => void;
  prefix?: string;
  onPrefixChange?: (value: string) => void;
}) {
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<'all' | 'selected' | 'not'>('all');
  const visible = schema.filter((name) => {
    if (filter === 'selected' && !selected.includes(name)) return false;
    if (filter === 'not' && selected.includes(name)) return false;
    if (search && !name.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });
  const showsPrefix = onPrefixChange !== undefined;
  return (
    <section style={{ display: 'grid', gap: 8 }}>
      <p style={{ margin: 0, fontSize: 13, fontWeight: 600 }}>{title}</p>
      {showsPrefix ? (
        <input
          value={prefix ?? ''}
          onChange={(event) => onPrefixChange?.(event.target.value)}
          placeholder="Prefix right columns"
          style={inputStyle()}
        />
      ) : null}
      <label style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
        <input
          type="checkbox"
          checked={autoSelectAll}
          onChange={(event) => onAutoToggle(event.target.checked)}
          style={{ accentColor: 'var(--status-info)' }}
        />
        Auto-select all {showsPrefix ? 'right' : 'left'} columns
      </label>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, background: '#fff', flex: 1 }}>
          <Glyph name="search" size={12} tone="#5c7080" />
          <input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search for columns..."
            style={{ flex: 1, border: 0, outline: 'none', fontSize: 13 }}
          />
        </span>
        <select value={filter} onChange={(event) => setFilter(event.target.value as 'all' | 'selected' | 'not')} style={inputStyle()}>
          <option value="all">All types</option>
          <option value="selected">Selected</option>
          <option value="not">Not selected</option>
        </select>
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: 12, color: 'var(--text-muted)' }}>
        <span>{selected.length} of {schema.length} columns selected</span>
        <span>
          <button type="button" onClick={onSelectAll} className="of-link" style={linkBtnStyle()} disabled={schema.length === 0}>Select all</button>
          {' | '}
          <button type="button" onClick={onDeselectAll} className="of-link" style={linkBtnStyle()}>Deselect all</button>
        </span>
      </div>
      <div style={{ border: '1px solid var(--border-subtle)', borderRadius: 4, maxHeight: 280, overflowY: 'auto', background: '#fff' }}>
        {visible.length === 0 ? (
          <p className="of-text-muted" style={{ padding: 16, textAlign: 'center', margin: 0, fontSize: 12 }}>No columns.</p>
        ) : (
          visible.map((column) => {
            const checked = autoSelectAll || selected.includes(column);
            return (
              <label
                key={column}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '6px 10px',
                  fontSize: 13,
                  color: 'var(--text-strong)',
                  borderBottom: '1px solid var(--border-subtle)',
                  cursor: autoSelectAll ? 'not-allowed' : 'pointer',
                  background: checked ? 'rgba(45, 114, 210, 0.04)' : 'transparent',
                }}
              >
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={autoSelectAll}
                  onChange={() => onToggle(column)}
                  style={{ accentColor: 'var(--status-info)' }}
                />
                <span style={{ flex: 1 }}>{column}</span>
                <span className="of-text-muted" style={{ fontSize: 11 }}>String</span>
              </label>
            );
          })
        )}
      </div>
    </section>
  );
}

function GuidanceDiagnostics({ diagnostics }: { diagnostics: PipelineSchemaGuidanceDiagnostic[] }) {
  if (diagnostics.length === 0) return null;
  return (
    <div style={{ display: 'grid', gap: 4 }}>
      {diagnostics.map((diagnostic, index) => (
        <div
          key={`${diagnostic.code}:${index}`}
          style={{
            padding: '6px 8px',
            borderRadius: 4,
            fontSize: 12,
            color: diagnostic.severity === 'error' ? '#7f1d1d' : '#713f12',
            background: diagnostic.severity === 'error' ? '#fee2e2' : '#fef3c7',
            border: `1px solid ${diagnostic.severity === 'error' ? '#fecaca' : '#fde68a'}`,
          }}
        >
          <strong style={{ marginRight: 6 }}>{diagnostic.code}</strong>
          {diagnostic.message}
        </div>
      ))}
    </div>
  );
}

function fieldsFromNames(names: string[]): PipelineIRField[] {
  return names.map((name) => ({ name, field_type: 'STRING', nullable: true }));
}

function joinDiagnosticsForWorking(draft: JoinDraft, leftFields: PipelineIRField[], rightFields: PipelineIRField[]): PipelineSchemaGuidanceDiagnostic[] {
  if (draft.join_type !== 'cross' && draft.matches.every((match) => !match.left_column || !match.right_column)) {
    return [{ severity: 'error', code: 'join_missing_match_conditions', message: 'Join requires at least one match condition.' }];
  }
  const diagnostics: PipelineSchemaGuidanceDiagnostic[] = [];
  for (const match of draft.matches) {
    if (!match.left_column || !match.right_column) continue;
    const left = leftFields.find((field) => field.name === match.left_column);
    const right = rightFields.find((field) => field.name === match.right_column);
    if (!left) {
      diagnostics.push({ severity: 'error', code: 'missing_join_column', left_column: match.left_column, right_column: match.right_column, message: `Left column "${match.left_column}" is not available.` });
      continue;
    }
    if (!right) {
      diagnostics.push({ severity: 'error', code: 'missing_join_column', left_column: match.left_column, right_column: match.right_column, message: `Right column "${match.right_column}" is not available.` });
      continue;
    }
    if (!compatibleJoinFieldTypes(left.field_type, right.field_type)) {
      diagnostics.push({
        severity: 'error',
        code: 'incompatible_join_key_types',
        left_column: match.left_column,
        right_column: match.right_column,
        left_type: left.field_type,
        right_type: right.field_type,
        message: `${match.left_column} (${left.field_type}) is not compatible with ${match.right_column} (${right.field_type}).`,
      });
    }
  }
  return diagnostics;
}

function compatibleJoinFieldTypes(leftRaw: string | undefined, rightRaw: string | undefined): boolean {
  const left = normalizeType(leftRaw);
  const right = normalizeType(rightRaw);
  if (left === right || left === 'unknown' || right === 'unknown') return true;
  const numeric = new Set(['integer', 'long', 'double', 'float', 'number']);
  const textual = new Set(['string', 'varchar']);
  const temporal = new Set(['date', 'timestamp']);
  return (numeric.has(left) && numeric.has(right))
    || (textual.has(left) && textual.has(right))
    || (temporal.has(left) && temporal.has(right));
}

function normalizeType(raw: string | undefined): string {
  return (raw ?? 'unknown').trim().toLowerCase();
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

function inputChipStyle(): React.CSSProperties {
  return {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    padding: '6px 10px',
    border: '1px solid var(--border-subtle)',
    borderRadius: 4,
    fontSize: 13,
    background: '#fff',
  };
}

function linkBtnStyle(): React.CSSProperties {
  return {
    background: 'none',
    border: 0,
    padding: 0,
    color: 'var(--status-info)',
    cursor: 'pointer',
    fontSize: 12,
  };
}

function JoinGlyph() {
  return (
    <svg width={16} height={16} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="6" width="9" height="12" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
      <rect x="11" y="6" width="9" height="12" rx="1.5" stroke="currentColor" strokeWidth="1.6" />
      <path d="M11 11h2" stroke="currentColor" strokeWidth="1.6" />
    </svg>
  );
}
