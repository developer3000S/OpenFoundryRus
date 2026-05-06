import { useMemo } from 'react';

import type { DatasetBranch, DatasetFilesystemEntry, DatasetTransaction } from '@/lib/api/datasets';

export interface SchemaField {
  name: string;
  type?: string;
  nullable?: boolean;
}

export interface CompareSide {
  label: string;
  schema: SchemaField[];
  files: DatasetFilesystemEntry[];
}

export type CompareSelector = { kind: 'transaction' | 'branch'; value: string };

interface CompareTabProps {
  transactions: DatasetTransaction[];
  branches: DatasetBranch[];
  sideA: CompareSide | null;
  sideB: CompareSide | null;
  selectorA: CompareSelector;
  selectorB: CompareSelector;
  loading?: boolean;
  error?: string;
  onChangeSelector: (which: 'A' | 'B', selector: CompareSelector) => void;
}

function diffSchemas(a: SchemaField[], b: SchemaField[]) {
  const aMap = new Map(a.map((f) => [f.name, f]));
  const bMap = new Map(b.map((f) => [f.name, f]));
  const added: SchemaField[] = [];
  const removed: SchemaField[] = [];
  const modified: Array<{ name: string; before: SchemaField; after: SchemaField }> = [];
  for (const [name, after] of bMap) {
    const before = aMap.get(name);
    if (!before) added.push(after);
    else if (before.type !== after.type || before.nullable !== after.nullable) modified.push({ name, before, after });
  }
  for (const [name, before] of aMap) if (!bMap.has(name)) removed.push(before);
  return { added, removed, modified };
}

function diffFiles(a: DatasetFilesystemEntry[], b: DatasetFilesystemEntry[]) {
  const aMap = new Map(a.map((f) => [f.path, f]));
  const bMap = new Map(b.map((f) => [f.path, f]));
  const added: DatasetFilesystemEntry[] = [];
  const removed: DatasetFilesystemEntry[] = [];
  const modified: Array<{ path: string; before: DatasetFilesystemEntry; after: DatasetFilesystemEntry }> = [];
  for (const [path, after] of bMap) {
    const before = aMap.get(path);
    if (!before) added.push(after);
    else if ((before.size_bytes ?? 0) !== (after.size_bytes ?? 0)) modified.push({ path, before, after });
  }
  for (const [path, before] of aMap) if (!bMap.has(path)) removed.push(before);
  return { added, removed, modified };
}

const TONE = {
  add: { background: '#022c22', color: '#86efac' },
  remove: { background: '#7f1d1d', color: '#fecaca' },
  mod: { background: '#78350f', color: '#fde68a' },
} as const;

export function CompareTab({
  transactions,
  branches,
  sideA,
  sideB,
  selectorA,
  selectorB,
  loading = false,
  error = '',
  onChangeSelector,
}: CompareTabProps) {
  const schemaDiff = useMemo(() => diffSchemas(sideA?.schema ?? [], sideB?.schema ?? []), [sideA, sideB]);
  const fileDiff = useMemo(() => diffFiles(sideA?.files ?? [], sideB?.files ?? []), [sideA, sideB]);

  const options = useMemo(() => {
    const out: Array<{ value: string; label: string; selector: CompareSelector }> = [];
    for (const b of branches) {
      out.push({ value: `branch:${b.name}`, label: `Branch ${b.name}`, selector: { kind: 'branch', value: b.name } });
    }
    for (const tx of transactions.slice(0, 50)) {
      out.push({
        value: `transaction:${tx.id}`,
        label: `${tx.operation} ${tx.id.slice(0, 8)} · ${new Date(tx.created_at).toLocaleDateString()}`,
        selector: { kind: 'transaction', value: tx.id },
      });
    }
    return out;
  }, [branches, transactions]);

  function selectorValue(s: CompareSelector) {
    return `${s.kind}:${s.value}`;
  }

  function onPickerChange(which: 'A' | 'B', value: string) {
    const opt = options.find((o) => o.value === value);
    if (opt) onChangeSelector(which, opt.selector);
  }

  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <header>
        <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.22em' }}>Compare</div>
        <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>Schema and file diff</h2>
        <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>Pick two transactions or branches to compare.</p>
      </header>

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))' }}>
        {([
          { key: 'A' as const, selector: selectorA, side: sideA },
          { key: 'B' as const, selector: selectorB, side: sideB },
        ]).map((picker) => (
          <div key={picker.key} className="of-panel" style={{ padding: 12 }}>
            <label style={{ fontSize: 11, textTransform: 'uppercase', color: 'var(--text-muted)' }}>Side {picker.key}</label>
            <select
              value={selectorValue(picker.selector)}
              onChange={(e) => onPickerChange(picker.key, e.target.value)}
              className="of-input"
              style={{ marginTop: 4, width: '100%' }}
            >
              {options.map((opt) => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
            </select>
            {picker.side && (
              <div className="of-text-muted" style={{ marginTop: 4, fontSize: 11 }}>
                {picker.side.schema.length} field{picker.side.schema.length === 1 ? '' : 's'} · {picker.side.files.length} file{picker.side.files.length === 1 ? '' : 's'}
              </div>
            )}
          </div>
        ))}
      </div>

      {error && <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 12, fontSize: 13 }}>{error}</div>}
      {loading && <div className="of-panel" style={{ padding: 24, textAlign: 'center', fontSize: 13 }}>Loading comparison…</div>}

      {!loading && sideA && sideB && (
        <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))' }}>
          <DiffPanel title="Schema diff">
            {schemaDiff.added.map((f) => (<DiffRow key={`a-${f.name}`} tone="add" sign="+" label={f.name} hint={f.type} />))}
            {schemaDiff.removed.map((f) => (<DiffRow key={`r-${f.name}`} tone="remove" sign="−" label={f.name} hint={f.type} />))}
            {schemaDiff.modified.map((m) => (<DiffRow key={`m-${m.name}`} tone="mod" sign="~" label={m.name} hint={`${m.before.type ?? '—'} → ${m.after.type ?? '—'}`} />))}
            {schemaDiff.added.length + schemaDiff.removed.length + schemaDiff.modified.length === 0 && (
              <li className="of-text-muted" style={{ fontSize: 11 }}>No schema changes.</li>
            )}
          </DiffPanel>
          <DiffPanel title="Files diff">
            {fileDiff.added.map((f) => (<DiffRow key={`a-${f.path}`} tone="add" sign="+" label={f.path} />))}
            {fileDiff.removed.map((f) => (<DiffRow key={`r-${f.path}`} tone="remove" sign="−" label={f.path} />))}
            {fileDiff.modified.map((m) => (
              <DiffRow
                key={`m-${m.path}`}
                tone="mod"
                sign="~"
                label={m.path}
                hint={`${(m.before.size_bytes ?? 0).toLocaleString()} → ${(m.after.size_bytes ?? 0).toLocaleString()} B`}
              />
            ))}
            {fileDiff.added.length + fileDiff.removed.length + fileDiff.modified.length === 0 && (
              <li className="of-text-muted" style={{ fontSize: 11 }}>No file changes.</li>
            )}
          </DiffPanel>
        </div>
      )}
    </section>
  );
}

function DiffPanel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="of-panel" style={{ padding: 12 }}>
      <p className="of-eyebrow" style={{ fontSize: 10 }}>{title}</p>
      <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, maxHeight: 360, overflow: 'auto' }}>
        {children}
      </ul>
    </div>
  );
}

function DiffRow({ tone, sign, label, hint }: { tone: keyof typeof TONE; sign: string; label: string; hint?: string }) {
  return (
    <li style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 8px', borderRadius: 4, ...TONE[tone] }}>
      <span style={{ fontWeight: 700 }}>{sign}</span>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis' }}>{label}</span>
      {hint && <span style={{ fontSize: 11, opacity: 0.7 }}>{hint}</span>}
    </li>
  );
}
