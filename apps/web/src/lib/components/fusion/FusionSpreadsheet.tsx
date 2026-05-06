import { useEffect, useState } from 'react';

import {
  listDatasets,
  previewDataset,
  uploadData,
  type Dataset,
} from '@/lib/api/datasets';
import {
  listObjects,
  listObjectTypes,
  updateObject,
  type ObjectInstance,
  type ObjectType,
} from '@/lib/api/ontology';

type Mode = 'dataset' | 'ontology';

async function loadDatasetRows(id: string) {
  const first = await previewDataset(id, { limit: 1000, offset: 0 });
  const total = first.total_rows ?? first.rows?.length ?? 0;
  const rows = [...(first.rows ?? [])];
  for (let offset = rows.length; offset < total; offset += 1000) {
    const next = await previewDataset(id, { limit: 1000, offset });
    rows.push(...(next.rows ?? []));
  }
  return rows;
}

async function loadObjectRows(typeId: string) {
  const rows: ObjectInstance[] = [];
  let page = 1;
  let total = 0;
  do {
    const response = await listObjects(typeId, { page, per_page: 100 });
    rows.push(...response.data);
    total = response.total;
    page += 1;
  } while (rows.length < total);
  return rows;
}

export function FusionSpreadsheet() {
  const [mode, setMode] = useState<Mode>('dataset');
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [datasetId, setDatasetId] = useState('');
  const [datasetRows, setDatasetRows] = useState<Array<Record<string, unknown>>>([]);
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [objectTypeId, setObjectTypeId] = useState('');
  const [objectRows, setObjectRows] = useState<ObjectInstance[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setBusy(true);
      setError('');
      try {
        const [datasetResponse, typeResponse] = await Promise.all([
          listDatasets({ per_page: 100 }),
          listObjectTypes({ per_page: 100 }),
        ]);
        if (cancelled) return;
        setDatasets(datasetResponse.data);
        setObjectTypes(typeResponse.data);
        const nextDatasetId = datasetResponse.data[0]?.id || '';
        const nextTypeId = typeResponse.data[0]?.id || '';
        setDatasetId(nextDatasetId);
        setObjectTypeId(nextTypeId);
        if (nextDatasetId) {
          const rows = await loadDatasetRows(nextDatasetId);
          if (!cancelled) setDatasetRows(rows);
        }
        if (nextTypeId) {
          const rows = await loadObjectRows(nextTypeId);
          if (!cancelled) setObjectRows(rows);
        }
      } catch (cause) {
        if (cancelled) return;
        setError(cause instanceof Error ? cause.message : 'Failed to load spreadsheet sources');
      } finally {
        if (!cancelled) setBusy(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  async function refreshDatasetSheet(id = datasetId) {
    if (!id) return;
    setBusy(true);
    try {
      const rows = await loadDatasetRows(id);
      setDatasetRows(rows);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to refresh dataset sheet');
    } finally {
      setBusy(false);
    }
  }

  async function refreshObjectSheet(id = objectTypeId) {
    if (!id) return;
    setBusy(true);
    try {
      const rows = await loadObjectRows(id);
      setObjectRows(rows);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to refresh ontology sheet');
    } finally {
      setBusy(false);
    }
  }

  function datasetColumns() {
    return Object.keys(datasetRows[0] ?? {}).slice(0, 12);
  }

  function objectColumns() {
    return Object.keys(objectRows[0]?.properties ?? {}).slice(0, 12);
  }

  function updateDatasetCell(rowIndex: number, column: string, value: string) {
    setDatasetRows((current) => current.map((row, index) => (index === rowIndex ? { ...row, [column]: value } : row)));
  }

  async function saveDatasetSheet() {
    if (!datasetId || datasetRows.length === 0) return;
    setBusy(true);
    setError('');
    try {
      const payload = JSON.stringify(datasetRows, null, 2);
      const file = new File([payload], 'fusion-spreadsheet.json', { type: 'application/json' });
      await uploadData(datasetId, file);
      await refreshDatasetSheet();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to persist dataset spreadsheet');
    } finally {
      setBusy(false);
    }
  }

  function updateObjectCell(objectId: string, column: string, value: string) {
    setObjectRows((current) =>
      current.map((row) => (row.id === objectId ? { ...row, properties: { ...row.properties, [column]: value } } : row)),
    );
  }

  async function saveObjectRow(objectId: string) {
    if (!objectTypeId) return;
    const row = objectRows.find((entry) => entry.id === objectId);
    if (!row) return;
    setBusy(true);
    setError('');
    try {
      const updated = await updateObject(objectTypeId, objectId, { properties: row.properties });
      setObjectRows((current) => current.map((entry) => (entry.id === objectId ? updated : entry)));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to persist ontology row');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div>
          <p className="of-eyebrow">Fusion Spreadsheet</p>
          <h2 className="of-heading-md" style={{ marginTop: 6 }}>
            Bidirectional grid for datasets and ontology objects
          </h2>
          <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13, lineHeight: 1.7, maxWidth: 720 }}>
            Edit dataset rows in bulk, or patch ontology objects inline, without leaving the Fusion workspace.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button
            type="button"
            onClick={() => setMode('dataset')}
            className={mode === 'dataset' ? 'of-button of-button--primary' : 'of-button'}
          >
            Dataset sheet
          </button>
          <button
            type="button"
            onClick={() => setMode('ontology')}
            className={mode === 'ontology' ? 'of-button of-button--primary' : 'of-button'}
          >
            Ontology sheet
          </button>
        </div>
      </div>

      {error && (
        <div className="of-status-danger" style={{ marginTop: 14, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {mode === 'dataset' ? (
        <div style={{ display: 'grid', gap: 14, marginTop: 18 }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
            <select
              value={datasetId}
              onChange={(e) => {
                setDatasetId(e.target.value);
                void refreshDatasetSheet(e.target.value);
              }}
              className="of-input"
              style={{ width: 'auto', fontSize: 13 }}
            >
              {datasets.map((dataset) => (
                <option key={dataset.id} value={dataset.id}>
                  {dataset.name}
                </option>
              ))}
            </select>
            <button type="button" onClick={() => void refreshDatasetSheet()} disabled={busy} className="of-button">
              Refresh
            </button>
            <button
              type="button"
              onClick={() => void saveDatasetSheet()}
              disabled={busy || datasetRows.length === 0}
              className="of-button of-button--primary"
            >
              {busy ? 'Working…' : 'Save dataset'}
            </button>
          </div>

          <div style={{ overflowX: 'auto', border: '1px solid var(--border-default)', borderRadius: 16 }}>
            <table style={{ minWidth: '100%', fontSize: 12 }}>
              <thead style={{ background: 'var(--bg-subtle)' }}>
                <tr>
                  {datasetColumns().map((column) => (
                    <th key={column} style={{ padding: '8px 10px', textAlign: 'left', fontWeight: 600, color: 'var(--text-muted)' }}>
                      {column}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {datasetRows.slice(0, 40).map((row, rowIndex) => (
                  <tr key={rowIndex} style={{ borderTop: '1px solid var(--border-default)' }}>
                    {datasetColumns().map((column) => (
                      <td key={column} style={{ padding: '8px 10px' }}>
                        <input
                          value={String(row[column] ?? '')}
                          onChange={(e) => updateDatasetCell(rowIndex, column, e.target.value)}
                          className="of-input"
                          style={{ fontSize: 12 }}
                        />
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 14, marginTop: 18 }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
            <select
              value={objectTypeId}
              onChange={(e) => {
                setObjectTypeId(e.target.value);
                void refreshObjectSheet(e.target.value);
              }}
              className="of-input"
              style={{ width: 'auto', fontSize: 13 }}
            >
              {objectTypes.map((type) => (
                <option key={type.id} value={type.id}>
                  {type.display_name}
                </option>
              ))}
            </select>
            <button type="button" onClick={() => void refreshObjectSheet()} disabled={busy} className="of-button">
              Refresh
            </button>
          </div>

          <div style={{ overflowX: 'auto', border: '1px solid var(--border-default)', borderRadius: 16 }}>
            <table style={{ minWidth: '100%', fontSize: 12 }}>
              <thead style={{ background: 'var(--bg-subtle)' }}>
                <tr>
                  {objectColumns().map((column) => (
                    <th key={column} style={{ padding: '8px 10px', textAlign: 'left', fontWeight: 600, color: 'var(--text-muted)' }}>
                      {column}
                    </th>
                  ))}
                  <th style={{ padding: '8px 10px', textAlign: 'left', fontWeight: 600, color: 'var(--text-muted)' }}>action</th>
                </tr>
              </thead>
              <tbody>
                {objectRows.slice(0, 40).map((row) => (
                  <tr key={row.id} style={{ borderTop: '1px solid var(--border-default)' }}>
                    {objectColumns().map((column) => (
                      <td key={column} style={{ padding: '8px 10px' }}>
                        <input
                          value={String(row.properties[column] ?? '')}
                          onChange={(e) => updateObjectCell(row.id, column, e.target.value)}
                          className="of-input"
                          style={{ fontSize: 12 }}
                        />
                      </td>
                    ))}
                    <td style={{ padding: '8px 10px' }}>
                      <button type="button" onClick={() => void saveObjectRow(row.id)} disabled={busy} className="of-button" style={{ fontSize: 12 }}>
                        Save row
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
}
