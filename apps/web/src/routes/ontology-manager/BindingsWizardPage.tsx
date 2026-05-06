import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { listDatasets, previewDataset, type Dataset, type DatasetPreviewResponse } from '@/lib/api/datasets';
import {
  createObjectTypeBinding,
  listObjectTypes,
  listProperties,
  materializeObjectTypeBinding,
  type MaterializeBindingResponse,
  type ObjectType,
  type ObjectTypeBinding,
  type ObjectTypeBindingPropertyMapping,
  type ObjectTypeBindingSyncMode,
  type Property,
} from '@/lib/api/ontology';

type Step = 1 | 2 | 3 | 4;

function autoMapping(columns: { name: string }[], props: Property[]): ObjectTypeBindingPropertyMapping[] {
  const result: ObjectTypeBindingPropertyMapping[] = [];
  const used = new Set<string>();
  for (const col of columns) {
    const match = props.find((p) => !used.has(p.name) && (p.name === col.name || p.name.toLowerCase() === col.name.toLowerCase()));
    if (match) {
      used.add(match.name);
      result.push({ source_column: col.name, target_property: match.name });
    }
  }
  return result;
}

export function BindingsWizardPage() {
  const [step, setStep] = useState<Step>(1);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [datasetSearch, setDatasetSearch] = useState('');
  const [selectedTypeId, setSelectedTypeId] = useState('');
  const [selectedDatasetId, setSelectedDatasetId] = useState('');

  const [preview, setPreview] = useState<DatasetPreviewResponse | null>(null);
  const [typeProperties, setTypeProperties] = useState<Property[]>([]);

  const [mapping, setMapping] = useState<ObjectTypeBindingPropertyMapping[]>([]);
  const [primaryKeyColumn, setPrimaryKeyColumn] = useState('');

  const [syncMode, setSyncMode] = useState<ObjectTypeBindingSyncMode>('snapshot');
  const [defaultMarking, setDefaultMarking] = useState('public');
  const [previewLimit, setPreviewLimit] = useState(1000);
  const [datasetBranch, setDatasetBranch] = useState('');
  const [datasetVersion, setDatasetVersion] = useState<number | ''>('');

  const [createdBinding, setCreatedBinding] = useState<ObjectTypeBinding | null>(null);
  const [materializeResult, setMaterializeResult] = useState<MaterializeBindingResponse | null>(null);

  useEffect(() => {
    setError('');
    Promise.all([
      listObjectTypes({ per_page: 200 }),
      listDatasets({ page: 1, per_page: 200 }),
    ])
      .then(([t, d]) => {
        setObjectTypes(t.data);
        setDatasets(d.data);
      })
      .catch((cause: unknown) => setError(cause instanceof Error ? cause.message : 'Failed to load'));
  }, []);

  const filteredDatasets = useMemo(() => {
    if (!datasetSearch.trim()) return datasets;
    const s = datasetSearch.toLowerCase();
    return datasets.filter((d) => d.name.toLowerCase().includes(s));
  }, [datasets, datasetSearch]);

  const selectedType = objectTypes.find((t) => t.id === selectedTypeId) ?? null;
  const previewColumns = preview?.columns ?? [];

  async function goToStep2() {
    if (!selectedTypeId || !selectedDatasetId) {
      setError('Pick an object type and dataset.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      const [pv, props] = await Promise.all([
        previewDataset(selectedDatasetId, { limit: 25 }),
        listProperties(selectedTypeId),
      ]);
      setPreview(pv);
      setTypeProperties(props);
      const auto = autoMapping(pv.columns ?? [], props);
      setMapping(auto);
      if (selectedType?.primary_key_property) {
        const match = (pv.columns ?? []).find((c) => c.name === selectedType.primary_key_property);
        if (match) setPrimaryKeyColumn(match.name);
      }
      setStep(2);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load preview');
    } finally {
      setBusy(false);
    }
  }

  async function createBinding() {
    setBusy(true);
    setError('');
    try {
      const binding = await createObjectTypeBinding(selectedTypeId, {
        dataset_id: selectedDatasetId,
        dataset_branch: datasetBranch || undefined,
        dataset_version: typeof datasetVersion === 'number' ? datasetVersion : undefined,
        primary_key_column: primaryKeyColumn,
        property_mapping: mapping,
        sync_mode: syncMode,
        default_marking: defaultMarking,
        preview_limit: previewLimit,
      });
      setCreatedBinding(binding);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create binding failed');
    } finally {
      setBusy(false);
    }
  }

  async function materialize(dryRun: boolean) {
    if (!createdBinding) return;
    setBusy(true);
    try {
      const result = await materializeObjectTypeBinding(selectedTypeId, createdBinding.id, {
        dry_run: dryRun,
      });
      setMaterializeResult(result);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Materialize failed');
    } finally {
      setBusy(false);
    }
  }

  function setMappingForColumn(col: string, prop: string) {
    setMapping((prev) => {
      const without = prev.filter((m) => m.source_column !== col);
      return prop ? [...without, { source_column: col, target_property: prop }] : without;
    });
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/ontology-manager" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Ontology manager</Link>
      <header>
        <h1 className="of-heading-xl">Bind dataset → object type</h1>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>Step {step} of 4</p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {step === 1 && (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          <p className="of-eyebrow">1. Pick object type + dataset</p>
          <label style={{ fontSize: 13 }}>
            Object type
            <select value={selectedTypeId} onChange={(e) => setSelectedTypeId(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
              <option value="">— pick —</option>
              {objectTypes.map((t) => (
                <option key={t.id} value={t.id}>{t.display_name} ({t.name})</option>
              ))}
            </select>
          </label>
          <label style={{ fontSize: 13 }}>
            Dataset search
            <input value={datasetSearch} onChange={(e) => setDatasetSearch(e.target.value)} placeholder="search datasets…" className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Dataset ({filteredDatasets.length} matches)
            <select value={selectedDatasetId} onChange={(e) => setSelectedDatasetId(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
              <option value="">— pick —</option>
              {filteredDatasets.map((d) => (
                <option key={d.id} value={d.id}>{d.name} · {d.format}</option>
              ))}
            </select>
          </label>
          <button type="button" onClick={() => void goToStep2()} disabled={busy || !selectedTypeId || !selectedDatasetId} className="of-button of-button--primary">
            Next →
          </button>
        </section>
      )}

      {step === 2 && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">2. Preview ({previewColumns.length} columns)</p>
          <div style={{ display: 'flex', gap: 8, fontSize: 12, marginTop: 8, flexWrap: 'wrap' }}>
            {previewColumns.map((c) => (
              <span key={c.name} style={{ padding: '4px 10px', borderRadius: 999, background: 'var(--bg-subtle)' }}>
                {c.name} <span className="of-text-muted">{c.field_type ?? c.data_type ?? ''}</span>
              </span>
            ))}
          </div>
          <div style={{ display: 'flex', gap: 6, marginTop: 12 }}>
            <button type="button" onClick={() => setStep(1)} className="of-button">← Back</button>
            <button type="button" onClick={() => setStep(3)} className="of-button of-button--primary">Next →</button>
          </div>
        </section>
      )}

      {step === 3 && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">3. Map columns → properties</p>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12, marginTop: 8 }}>
            <thead>
              <tr>
                <th style={{ textAlign: 'left', padding: 6, borderBottom: '1px solid var(--border-default)' }}>Column</th>
                <th style={{ textAlign: 'left', padding: 6, borderBottom: '1px solid var(--border-default)' }}>Property</th>
                <th style={{ textAlign: 'left', padding: 6, borderBottom: '1px solid var(--border-default)' }}>PK</th>
              </tr>
            </thead>
            <tbody>
              {previewColumns.map((c) => {
                const m = mapping.find((mm) => mm.source_column === c.name);
                return (
                  <tr key={c.name} style={{ borderBottom: '1px solid var(--border-subtle)' }}>
                    <td style={{ padding: 6, fontFamily: 'var(--font-mono)' }}>{c.name}</td>
                    <td style={{ padding: 6 }}>
                      <select value={m?.target_property ?? ''} onChange={(e) => setMappingForColumn(c.name, e.target.value)} className="of-input" style={{ fontSize: 12 }}>
                        <option value="">— skip —</option>
                        {typeProperties.map((p) => (
                          <option key={p.id} value={p.name}>{p.name} ({p.property_type})</option>
                        ))}
                      </select>
                    </td>
                    <td style={{ padding: 6 }}>
                      <input type="radio" name="pk" checked={primaryKeyColumn === c.name} onChange={() => setPrimaryKeyColumn(c.name)} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          <div style={{ display: 'flex', gap: 6, marginTop: 12 }}>
            <button type="button" onClick={() => setStep(2)} className="of-button">← Back</button>
            <button type="button" onClick={() => setStep(4)} disabled={!primaryKeyColumn} className="of-button of-button--primary">Next →</button>
          </div>
        </section>
      )}

      {step === 4 && (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          <p className="of-eyebrow">4. Sync mode + create</p>
          <label style={{ fontSize: 13 }}>
            Sync mode
            <select value={syncMode} onChange={(e) => setSyncMode(e.target.value as ObjectTypeBindingSyncMode)} className="of-input" style={{ marginTop: 4 }}>
              <option value="snapshot">snapshot</option>
              <option value="incremental">incremental</option>
              <option value="view">view</option>
            </select>
          </label>
          <label style={{ fontSize: 13 }}>
            Default marking
            <input value={defaultMarking} onChange={(e) => setDefaultMarking(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Preview limit
            <input type="number" value={previewLimit} onChange={(e) => setPreviewLimit(Number(e.target.value) || 0)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Dataset branch (optional)
            <input value={datasetBranch} onChange={(e) => setDatasetBranch(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Dataset version (optional)
            <input type="number" value={datasetVersion} onChange={(e) => setDatasetVersion(e.target.value ? Number(e.target.value) : '')} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <div style={{ display: 'flex', gap: 6 }}>
            <button type="button" onClick={() => setStep(3)} className="of-button">← Back</button>
            {!createdBinding && (
              <button type="button" onClick={() => void createBinding()} disabled={busy} className="of-button of-button--primary">
                Create binding
              </button>
            )}
            {createdBinding && (
              <>
                <button type="button" onClick={() => void materialize(true)} disabled={busy} className="of-button">Dry-run materialize</button>
                <button type="button" onClick={() => void materialize(false)} disabled={busy} className="of-button of-button--primary">Materialize now</button>
              </>
            )}
          </div>
          {createdBinding && (
            <pre style={{ marginTop: 8, padding: 10, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
              {JSON.stringify(createdBinding, null, 2)}
            </pre>
          )}
          {materializeResult && (
            <pre style={{ marginTop: 8, padding: 10, background: '#0c0a09', color: '#a5f3fc', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
              {JSON.stringify(materializeResult, null, 2)}
            </pre>
          )}
        </section>
      )}
    </section>
  );
}
