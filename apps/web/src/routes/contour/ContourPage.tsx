import { useEffect, useMemo, useState } from 'react';

import { EChartView } from '@/lib/components/analytics/EChartView';
import {
  createDataset,
  listDatasets,
  previewDataset,
  uploadData,
  type Dataset,
} from '@/lib/api/datasets';
import {
  buildObjectTableLines,
  downloadStructuredPdf,
  type PdfSection,
} from '@/lib/utils/pdf';
import { notifications } from '@stores/notifications';

type Aggregation = 'sum' | 'avg' | 'count' | 'max';

function numericValue(value: unknown) {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value.replace(/,/g, ''));
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

async function loadAllRows(datasetId: string) {
  const firstPage = await previewDataset(datasetId, { limit: 1000, offset: 0 });
  const total = firstPage.total_rows ?? firstPage.rows?.length ?? 0;
  const rows = [...(firstPage.rows ?? [])];
  for (let offset = rows.length; offset < total; offset += 1000) {
    const next = await previewDataset(datasetId, { limit: 1000, offset });
    rows.push(...(next.rows ?? []));
  }
  return rows;
}

export function ContourPage() {
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [primaryDatasetId, setPrimaryDatasetId] = useState('');
  const [secondaryDatasetId, setSecondaryDatasetId] = useState('');
  const [primaryRows, setPrimaryRows] = useState<Array<Record<string, unknown>>>([]);
  const [secondaryRows, setSecondaryRows] = useState<Array<Record<string, unknown>>>([]);
  const [loadingPrimary, setLoadingPrimary] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [exportingPdf, setExportingPdf] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [error, setError] = useState('');

  const [primaryJoinKey, setPrimaryJoinKey] = useState('');
  const [secondaryJoinKey, setSecondaryJoinKey] = useState('');
  const [search, setSearch] = useState('');
  const [dateField, setDateField] = useState('');
  const [dateFrom, setDateFrom] = useState('');
  const [dateTo, setDateTo] = useState('');
  const [dimension, setDimension] = useState('');
  const [secondaryDimension, setSecondaryDimension] = useState('');
  const [metric, setMetric] = useState('');
  const [aggregation, setAggregation] = useState<Aggregation>('sum');
  const [selectedCategory, setSelectedCategory] = useState('');

  // ── Initial dataset load ──
  useEffect(() => {
    void (async () => {
      try {
        const response = await listDatasets({ per_page: 100 });
        setDatasets(response.data);
        const initial = response.data[0]?.id ?? '';
        setPrimaryDatasetId(initial);
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'Failed to load datasets');
      }
    })();
  }, []);

  // ── Primary rows ──
  useEffect(() => {
    if (!primaryDatasetId) {
      setPrimaryRows([]);
      return;
    }
    let cancelled = false;
    setLoadingPrimary(true);
    setError('');
    (async () => {
      try {
        const rows = await loadAllRows(primaryDatasetId);
        if (!cancelled) setPrimaryRows(rows);
      } catch (cause) {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Failed to load primary dataset');
          setPrimaryRows([]);
        }
      } finally {
        if (!cancelled) setLoadingPrimary(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [primaryDatasetId]);

  // ── Secondary rows ──
  useEffect(() => {
    if (!secondaryDatasetId) {
      setSecondaryRows([]);
      return;
    }
    let cancelled = false;
    setError('');
    (async () => {
      try {
        const rows = await loadAllRows(secondaryDatasetId);
        if (!cancelled) setSecondaryRows(rows);
      } catch (cause) {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Failed to load secondary dataset');
          setSecondaryRows([]);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [secondaryDatasetId]);

  const sourceRows = useMemo(() => {
    if (!secondaryDatasetId || !primaryJoinKey || !secondaryJoinKey || secondaryRows.length === 0) {
      return primaryRows;
    }
    const secondaryIndex = Object.fromEntries(
      secondaryRows.map((row) => [String(row[secondaryJoinKey] ?? ''), row]),
    );
    return primaryRows.map((row) => {
      const joined = secondaryIndex[String(row[primaryJoinKey] ?? '')];
      if (!joined) return row;
      const prefixed = Object.fromEntries(
        Object.entries(joined).map(([key, value]) => [`joined_${key}`, value]),
      );
      return { ...row, ...prefixed };
    });
  }, [primaryRows, secondaryRows, secondaryDatasetId, primaryJoinKey, secondaryJoinKey]);

  const sampleKeys = useMemo(() => Object.keys(sourceRows[0] ?? {}), [sourceRows]);

  // Hydrate dimension/metric/dateField defaults once data is available.
  useEffect(() => {
    if (sampleKeys.length === 0) return;
    if (!dimension) setDimension(sampleKeys[0] ?? '');
    if (!secondaryDimension) setSecondaryDimension(sampleKeys[1] ?? sampleKeys[0] ?? '');
    if (!metric) {
      const sample = sourceRows[0] ?? {};
      const numericKey = sampleKeys.find((key) => typeof sample[key] === 'number');
      setMetric(numericKey ?? sampleKeys[1] ?? '');
    }
    if (!dateField) {
      const dateKey = sampleKeys.find((key) => /date|time|day|month/i.test(key));
      if (dateKey) setDateField(dateKey);
    }
    if (!primaryJoinKey) setPrimaryJoinKey(Object.keys(primaryRows[0] ?? {})[0] ?? '');
    if (!secondaryJoinKey) setSecondaryJoinKey(Object.keys(secondaryRows[0] ?? {})[0] ?? '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sampleKeys, primaryRows, secondaryRows]);

  function searchableText(row: Record<string, unknown>) {
    return Object.values(row)
      .map((value) => String(value ?? ''))
      .join(' ')
      .toLowerCase();
  }

  function matchesDateFilter(row: Record<string, unknown>) {
    if (!dateField || (!dateFrom && !dateTo)) return true;
    const raw = row[dateField];
    const date = raw ? new Date(String(raw)) : null;
    if (!date || Number.isNaN(date.getTime())) return true;
    if (dateFrom && date < new Date(dateFrom)) return false;
    if (dateTo) {
      const rowDate = date.toISOString().slice(0, 10);
      if (rowDate > dateTo) return false;
    }
    return true;
  }

  const filteredRows = useMemo(() => {
    return sourceRows.filter((row) => {
      if (search.trim() && !searchableText(row).includes(search.trim().toLowerCase())) return false;
      if (selectedCategory && String(row[dimension] ?? '') !== selectedCategory) return false;
      return matchesDateFilter(row);
    });
  }, [sourceRows, search, selectedCategory, dimension, dateField, dateFrom, dateTo]);

  const aggregateRows = (rows: Array<Record<string, unknown>>, groupField: string) => {
    const bucket: Record<string, { count: number; total: number; max: number }> = {};
    for (const row of rows) {
      const key = String(row[groupField] ?? 'Unknown');
      const nextValue = numericValue(row[metric]);
      const current = bucket[key] ?? { count: 0, total: 0, max: Number.NEGATIVE_INFINITY };
      current.count += 1;
      current.total += nextValue;
      current.max = Math.max(current.max, nextValue);
      bucket[key] = current;
    }
    return Object.entries(bucket)
      .map(([group, stats]) => {
        const value =
          aggregation === 'count'
            ? stats.count
            : aggregation === 'avg'
              ? stats.count === 0
                ? 0
                : stats.total / stats.count
              : aggregation === 'max'
                ? stats.max === Number.NEGATIVE_INFINITY
                  ? 0
                  : stats.max
                : stats.total;
        return { group, value: Number(value.toFixed(2)), count: stats.count };
      })
      .sort((left, right) => Number(right.value) - Number(left.value))
      .slice(0, 24);
  };

  const analysisRows = useMemo(
    () => aggregateRows(filteredRows, dimension || (sampleKeys[0] ?? '')),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [filteredRows, dimension, metric, aggregation, sampleKeys],
  );
  const breakdownRows = useMemo(
    () => aggregateRows(filteredRows, secondaryDimension || dimension || (sampleKeys[0] ?? '')),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [filteredRows, secondaryDimension, dimension, metric, aggregation, sampleKeys],
  );

  const analysisPath = useMemo(
    () =>
      [
        `dataset:${datasets.find((dataset) => dataset.id === primaryDatasetId)?.name ?? 'none'}`,
        secondaryDatasetId
          ? `join:${datasets.find((dataset) => dataset.id === secondaryDatasetId)?.name ?? 'secondary'}`
          : null,
        search.trim() ? `search:${search.trim()}` : null,
        selectedCategory ? `drill:${selectedCategory}` : null,
      ].filter(Boolean) as string[],
    [datasets, primaryDatasetId, secondaryDatasetId, search, selectedCategory],
  );

  function datasetName(datasetId: string) {
    return datasets.find((dataset) => dataset.id === datasetId)?.name ?? 'Unselected dataset';
  }

  async function exportCurrentView() {
    if (analysisRows.length === 0) return;
    setExporting(true);
    setError('');
    try {
      const dataset = await createDataset({
        name: `Contour Export ${new Date().toISOString().slice(0, 16)}`,
        description: 'Materialized from the Contour analysis board.',
        format: 'json',
        tags: ['contour', 'analysis-export'],
      });
      const payload = JSON.stringify(analysisRows, null, 2);
      const file = new File([payload], 'contour-export.json', { type: 'application/json' });
      await uploadData(dataset.id, file);
      notifications.success(`Exported to dataset ${dataset.name}`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to export analysis');
    } finally {
      setExporting(false);
    }
  }

  async function exportCurrentPdf() {
    if (analysisRows.length === 0) return;
    setExportingPdf(true);
    setError('');
    try {
      const filtered = filteredRows;
      const sections: PdfSection[] = [
        {
          heading: 'Analysis scope',
          lines: [
            `Primary dataset: ${datasetName(primaryDatasetId)}`,
            `Join dataset: ${secondaryDatasetId ? datasetName(secondaryDatasetId) : 'None'}`,
            `Rows in source view: ${sourceRows.length}`,
            `Rows after filters: ${filtered.length}`,
            `Dimension: ${dimension || 'Not selected'}`,
            `Secondary dimension: ${secondaryDimension || 'Not selected'}`,
            `Metric: ${metric || 'Not selected'} (${aggregation})`,
            `Date field: ${dateField || 'No date filter'}`,
          ],
        },
        {
          heading: 'Path and filters',
          lines: [
            `Search: ${search.trim() || 'None'}`,
            `Drill category: ${selectedCategory || 'Global view'}`,
            `Date window: ${dateFrom || 'Start'} -> ${dateTo || 'Now'}`,
            `Analysis path: ${analysisPath.join(' -> ') || 'No path state captured'}`,
          ],
        },
        {
          heading: 'Primary analysis',
          lines: [
            `${analysisRows.length} grouped row(s) ready for materialization.`,
            ...buildObjectTableLines(analysisRows as Array<Record<string, unknown>>, 12, 3),
          ],
        },
        {
          heading: 'Linked breakdown',
          lines: [
            `${breakdownRows.length} breakdown row(s) in the secondary board.`,
            ...buildObjectTableLines(breakdownRows as Array<Record<string, unknown>>, 12, 3),
          ],
        },
      ];

      downloadStructuredPdf({
        fileName: `contour-${datasetName(primaryDatasetId).toLowerCase().replace(/[^a-z0-9]+/g, '-')}.pdf`,
        title: 'Contour Analysis Export',
        subtitle: datasetName(primaryDatasetId),
        metadata: [
          `Generated at ${new Date().toISOString()}`,
          `Fullscreen mode: ${fullscreen ? 'enabled' : 'disabled'}`,
          'OpenFoundry Contour PDF snapshot',
        ],
        sections,
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to export PDF snapshot');
    } finally {
      setExportingPdf(false);
    }
  }

  const wrapperStyle: React.CSSProperties = fullscreen
    ? {
        position: 'fixed',
        inset: 0,
        zIndex: 50,
        overflow: 'auto',
        background: 'var(--bg-app)',
        padding: 24,
        display: 'grid',
        gap: 16,
      }
    : { display: 'grid', gap: 16 };

  return (
    <section className={fullscreen ? '' : 'of-page'} style={wrapperStyle}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div style={{ maxWidth: 720 }}>
            <p className="of-eyebrow" style={{ color: '#0d9488' }}>
              Contour
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              Top-down analysis with transform boards and materialized exports
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
              Join datasets, drill through paths, filter chart-to-chart, and persist the resulting
              analysis as a new dataset.
            </p>
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            <button type="button" className="of-btn" onClick={() => setFullscreen((v) => !v)}>
              {fullscreen ? 'Exit fullscreen' : 'Fullscreen'}
            </button>
            <button
              type="button"
              className="of-btn"
              onClick={() => void exportCurrentPdf()}
              disabled={exportingPdf || analysisRows.length === 0}
            >
              {exportingPdf ? 'Exporting PDF…' : 'Export PDF'}
            </button>
            <button
              type="button"
              className="of-btn of-btn-primary"
              onClick={() => void exportCurrentView()}
              disabled={exporting || analysisRows.length === 0}
            >
              {exporting ? 'Exporting…' : 'Export to dataset'}
            </button>
          </div>
        </div>

        {error && (
          <div
            className="of-status-danger"
            style={{ marginTop: 16, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}
          >
            {error}
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.95fr) minmax(0, 1.05fr)' }}>
        <div style={{ display: 'grid', gap: 16 }}>
          <div className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">Transform board</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Shape the analysis
            </h2>

            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 16 }}>
              <FieldSelect
                label="Primary dataset"
                value={primaryDatasetId}
                onChange={(value) => {
                  setPrimaryDatasetId(value);
                  setSelectedCategory('');
                }}
                options={datasets.map((d) => ({ value: d.id, label: d.name }))}
              />
              <FieldSelect
                label="Join dataset"
                value={secondaryDatasetId}
                onChange={setSecondaryDatasetId}
                options={[
                  { value: '', label: 'No join' },
                  ...datasets
                    .filter((d) => d.id !== primaryDatasetId)
                    .map((d) => ({ value: d.id, label: d.name })),
                ]}
              />
              {secondaryDatasetId && (
                <>
                  <FieldSelect
                    label="Primary key"
                    value={primaryJoinKey}
                    onChange={setPrimaryJoinKey}
                    options={Object.keys(primaryRows[0] ?? {}).map((key) => ({ value: key, label: key }))}
                  />
                  <FieldSelect
                    label="Join key"
                    value={secondaryJoinKey}
                    onChange={setSecondaryJoinKey}
                    options={Object.keys(secondaryRows[0] ?? {}).map((key) => ({ value: key, label: key }))}
                  />
                </>
              )}
              <FieldSelect
                label="Dimension"
                value={dimension}
                onChange={setDimension}
                options={sampleKeys.map((key) => ({ value: key, label: key }))}
              />
              <FieldSelect
                label="Secondary dimension"
                value={secondaryDimension}
                onChange={setSecondaryDimension}
                options={sampleKeys.map((key) => ({ value: key, label: key }))}
              />
              <FieldSelect
                label="Metric"
                value={metric}
                onChange={setMetric}
                options={sampleKeys.map((key) => ({ value: key, label: key }))}
              />
              <FieldSelect
                label="Aggregation"
                value={aggregation}
                onChange={(value) => setAggregation(value as Aggregation)}
                options={[
                  { value: 'sum', label: 'sum' },
                  { value: 'avg', label: 'avg' },
                  { value: 'count', label: 'count' },
                  { value: 'max', label: 'max' },
                ]}
              />

              <FieldInput
                label="Search parameter"
                value={search}
                onChange={setSearch}
                placeholder="Search across the joined rows"
                fullWidth
              />

              <FieldSelect
                label="Date field"
                value={dateField}
                onChange={setDateField}
                options={[{ value: '', label: 'No date filter' }, ...sampleKeys.map((k) => ({ value: k, label: k }))]}
              />
              <FieldInput label="From" value={dateFrom} onChange={setDateFrom} type="date" />
              <FieldInput label="To" value={dateTo} onChange={setDateTo} type="date" />
            </div>
          </div>

          <div className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">Analysis path</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Sequence and drill breadcrumbs
            </h2>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 16 }}>
              {analysisPath.map((step) => (
                <span key={step} className="of-chip">
                  {step}
                </span>
              ))}
            </div>
            {selectedCategory && (
              <button
                type="button"
                className="of-btn"
                style={{ marginTop: 16, minHeight: 32, fontSize: 13 }}
                onClick={() => setSelectedCategory('')}
              >
                Clear drill into {selectedCategory}
              </button>
            )}
          </div>
        </div>

        <div style={{ display: 'grid', gap: 16 }}>
          <div className="of-panel" style={{ padding: 24 }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
              <div>
                <p className="of-eyebrow">Display board</p>
                <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                  Primary analysis chart
                </h2>
              </div>
              <span className="of-text-muted" style={{ fontSize: 12 }}>
                {analysisRows.length} grouped rows
              </span>
            </div>
            <div style={{ marginTop: 16, height: 320 }}>
              <EChartView
                rows={analysisRows.map((row) => ({ category: row.group, value: row.value }))}
                categoryKey="category"
                valueKeys={['value']}
                mode="bar"
                emptyLabel={
                  loadingPrimary
                    ? 'Loading dataset…'
                    : 'Pick a dataset and metric to start the board.'
                }
                onCategoryClick={(value) => setSelectedCategory(value)}
              />
            </div>
          </div>

          <div className="of-panel" style={{ padding: 24 }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
              <div>
                <p className="of-eyebrow">Linked board</p>
                <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                  Chart-to-chart filtering
                </h2>
              </div>
              <span className="of-text-muted" style={{ fontSize: 12 }}>
                {selectedCategory ? `Scoped to ${selectedCategory}` : 'Global view'}
              </span>
            </div>
            <div style={{ marginTop: 16, height: 320 }}>
              <EChartView
                rows={breakdownRows.map((row) => ({ category: row.group, value: row.value }))}
                categoryKey="category"
                valueKeys={['value']}
                mode="pie"
                emptyLabel="Choose a secondary dimension to break the analysis down."
              />
            </div>
          </div>

          <div className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">Result table</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Materializable rows
            </h2>
            <div style={{ marginTop: 16, overflowX: 'auto' }}>
              <table className="of-table">
                <thead>
                  <tr>
                    <th>{dimension || 'group'}</th>
                    <th>
                      {aggregation}({metric || 'value'})
                    </th>
                    <th>count</th>
                  </tr>
                </thead>
                <tbody>
                  {analysisRows.map((row) => (
                    <tr key={row.group}>
                      <td>{row.group}</td>
                      <td>{row.value}</td>
                      <td className="of-text-muted">{row.count}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

interface FieldSelectProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: Array<{ value: string; label: string }>;
  fullWidth?: boolean;
}

function FieldSelect({ label, value, onChange, options, fullWidth }: FieldSelectProps) {
  return (
    <label
      className="of-panel-muted"
      style={{
        padding: '8px 12px',
        display: 'block',
        gridColumn: fullWidth ? '1 / -1' : undefined,
      }}
    >
      <div className="of-eyebrow">{label}</div>
      <select
        className="of-select"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        style={{ marginTop: 6, background: 'transparent', border: 0, padding: 0, minHeight: 0, fontSize: 13 }}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

interface FieldInputProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: string;
  fullWidth?: boolean;
}

function FieldInput({ label, value, onChange, placeholder, type = 'text', fullWidth }: FieldInputProps) {
  return (
    <label
      className="of-panel-muted"
      style={{
        padding: '8px 12px',
        display: 'block',
        gridColumn: fullWidth ? '1 / -1' : undefined,
      }}
    >
      <div className="of-eyebrow">{label}</div>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        style={{
          marginTop: 6,
          width: '100%',
          background: 'transparent',
          border: 0,
          outline: 'none',
          fontSize: 13,
          color: 'var(--text-strong)',
        }}
      />
    </label>
  );
}
