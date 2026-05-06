import { useEffect, useState } from 'react';

import {
  capabilityChips,
  defaultCapabilities,
  tableTypeLabel,
  virtualTables,
  type DiscoveredEntry,
  type IncompatibleSourceError,
  type Locator,
  type RegisterVirtualTableRequest,
  type TableType,
  type VirtualTable,
  type VirtualTableProvider,
} from '@/lib/api/virtual-tables';

interface Props {
  open: boolean;
  sourceRid: string;
  provider: VirtualTableProvider;
  entry: DiscoveredEntry | null;
  onClose: () => void;
  onCreated?: (table: VirtualTable) => void;
}

const TABLE_TYPES: TableType[] = [
  'TABLE', 'VIEW', 'MATERIALIZED_VIEW', 'EXTERNAL_DELTA', 'MANAGED_DELTA',
  'MANAGED_ICEBERG', 'PARQUET_FILES', 'AVRO_FILES', 'CSV_FILES', 'OTHER',
];

function deriveLocator(entry: DiscoveredEntry, prov: VirtualTableProvider): Locator {
  const parts = entry.path.split('/').filter(Boolean);
  if (prov === 'AMAZON_S3' || prov === 'AZURE_ABFS' || prov === 'GCS') {
    return { kind: 'file', bucket: parts[0] ?? entry.path, prefix: parts.slice(1).join('/'), format: 'parquet' };
  }
  if (entry.kind === 'iceberg_table' || entry.kind === 'iceberg_namespace') {
    return { kind: 'iceberg', catalog: parts[0] ?? '', namespace: parts[1] ?? 'default', table: parts[parts.length - 1] ?? entry.display_name };
  }
  return { kind: 'tabular', database: parts[0] ?? '', schema: parts[1] ?? '', table: parts[parts.length - 1] ?? entry.display_name };
}

export function CreateVirtualTableModal({ open, sourceRid, provider, entry, onClose, onCreated }: Props) {
  const [projectRid, setProjectRid] = useState('');
  const [parentFolderRid, setParentFolderRid] = useState('');
  const [name, setName] = useState('');
  const [tableType, setTableType] = useState<TableType>('TABLE');
  const [locatorJson, setLocatorJson] = useState('{}');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [remediation, setRemediation] = useState<string | null>(null);

  useEffect(() => {
    if (entry) {
      setName(entry.display_name);
      setTableType(entry.inferred_table_type ?? 'OTHER');
      setLocatorJson(JSON.stringify(deriveLocator(entry, provider), null, 2));
    }
  }, [entry, provider]);

  if (!open) return null;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!projectRid.trim()) { setError('Project rid is required'); return; }
    let locator: Locator;
    try { locator = JSON.parse(locatorJson) as Locator; }
    catch { setError('Locator JSON is not valid'); return; }
    setBusy(true);
    setError(null);
    setRemediation(null);
    try {
      const body: RegisterVirtualTableRequest = {
        project_rid: projectRid.trim(),
        parent_folder_rid: parentFolderRid.trim() || undefined,
        name: name.trim() || undefined,
        locator,
        table_type: tableType,
      };
      const table = await virtualTables.registerVirtualTable(sourceRid, body);
      onCreated?.(table);
      onClose();
    } catch (err) {
      const incompatible = err as IncompatibleSourceError;
      if (incompatible?.error === 'VIRTUAL_TABLES_INCOMPATIBLE_SOURCE_CONFIG') {
        setError(`Source incompatible: ${incompatible.code}`);
        setRemediation(incompatible.reason?.remediation ?? null);
      } else if (err instanceof Error) {
        setError(err.message);
      } else {
        setError('Failed to register virtual table');
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div role="dialog" aria-modal="true" style={backdropStyle}>
      <form onSubmit={submit} style={modalStyle}>
        <header style={fullColStyle}>
          <h3 style={{ margin: 0 }}>Create virtual table</h3>
          <p style={mutedStyle}>From source <code>{sourceRid}</code></p>
        </header>

        <Field label={<>Project rid <span style={{ color: '#b91c1c' }}>*</span></>}>
          <input type="text" value={projectRid} onChange={(e) => setProjectRid(e.target.value)} placeholder="ri.foundry.main.project..." required style={inputStyle} />
        </Field>
        <Field label="Parent folder rid (optional)">
          <input type="text" value={parentFolderRid} onChange={(e) => setParentFolderRid(e.target.value)} placeholder="ri.compass.main.folder..." style={inputStyle} />
        </Field>
        <Field label="Name">
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} style={inputStyle} />
        </Field>
        <Field label="Table type">
          <select value={tableType} onChange={(e) => setTableType(e.target.value as TableType)} style={inputStyle}>
            {TABLE_TYPES.map((t) => <option key={t} value={t}>{tableTypeLabel(t)}</option>)}
          </select>
        </Field>
        <Field label="Locator (JSON)" full>
          <textarea value={locatorJson} onChange={(e) => setLocatorJson(e.target.value)} rows={6} style={{ ...inputStyle, fontFamily: 'ui-monospace, SFMono-Regular, monospace' }} />
        </Field>

        <section style={fullColStyle}>
          <h4 style={{ fontSize: 12, textTransform: 'uppercase', letterSpacing: '0.05em', margin: '0 0 4px', color: '#4b5563' }}>Capabilities (preview)</h4>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
            {capabilityChips(defaultCapabilities(provider, tableType)).map((c) => (
              <span key={c} style={{ background: '#f3f4f6', border: '1px solid #e5e7eb', borderRadius: 4, fontSize: 12, padding: '1px 6px' }}>{c}</span>
            ))}
          </div>
        </section>

        {error && (
          <div role="alert" style={{ ...fullColStyle, background: '#fef2f2', border: '1px solid #fecaca', color: '#b91c1c', borderRadius: 4, padding: '8px 12px', fontSize: 14 }}>
            {error}
            {remediation && <p style={{ margin: '4px 0 0', color: '#7f1d1d', fontSize: 12 }}>{remediation}</p>}
          </div>
        )}

        <footer style={{ ...fullColStyle, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button type="button" onClick={onClose} disabled={busy} style={btnStyle}>Cancel</button>
          <button type="submit" disabled={busy} style={primaryBtnStyle}>{busy ? 'Creating…' : 'Create'}</button>
        </footer>
      </form>
    </div>
  );
}

function Field({ label, children, full = false }: { label: React.ReactNode; children: React.ReactNode; full?: boolean }) {
  return (
    <label style={{ display: 'flex', flexDirection: 'column', fontSize: 12, gap: 4, gridColumn: full ? '1 / -1' : 'auto' }}>
      <span>{label}</span>
      {children}
    </label>
  );
}

const backdropStyle: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100 };
const modalStyle: React.CSSProperties = { background: '#fff', borderRadius: 8, padding: 20, width: 'min(560px, 92vw)', maxHeight: '90vh', overflow: 'auto', display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 };
const fullColStyle: React.CSSProperties = { gridColumn: '1 / -1' };
const mutedStyle: React.CSSProperties = { color: '#6b7280', fontSize: 12 };
const inputStyle: React.CSSProperties = { border: '1px solid #d1d5db', borderRadius: 4, padding: '6px 8px', fontSize: 14, background: '#fff', fontFamily: 'inherit' };
const btnStyle: React.CSSProperties = { padding: '6px 12px', border: '1px solid #d1d5db', borderRadius: 4, background: '#fff', fontSize: 14, cursor: 'pointer' };
const primaryBtnStyle: React.CSSProperties = { ...btnStyle, background: '#1d4ed8', color: '#fff', borderColor: '#1d4ed8' };
