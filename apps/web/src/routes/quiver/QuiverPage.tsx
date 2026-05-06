import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';

import { EChartView } from '@/lib/components/analytics/EChartView';
import {
  createQuiverVisualFunction,
  deleteQuiverVisualFunction,
  getOntologyGraph,
  listObjects,
  listObjectTypes,
  listQuiverVisualFunctions,
  updateQuiverVisualFunction,
  type GraphResponse,
  type ObjectInstance,
  type ObjectType,
  type QuiverChartKind,
  type QuiverVisualFunction,
} from '@/lib/api/ontology';
import { buildQuiverVegaSpec, downloadJsonDocument } from '@/lib/utils/quiver';

function numericValue(value: unknown) {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value.replace(/,/g, ''));
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

async function loadAllObjects(typeId: string): Promise<ObjectInstance[]> {
  const rows: ObjectInstance[] = [];
  let nextPage = 1;
  let total = 0;
  do {
    const response = await listObjects(typeId, { page: nextPage, per_page: 100 });
    rows.push(...response.data);
    total = response.total;
    nextPage += 1;
  } while (rows.length < total);
  return rows;
}

export function QuiverPage() {
  const [searchParams] = useSearchParams();
  const embedded = searchParams.get('embedded') === '1';
  const requestedVisualFunctionId = searchParams.get('visual_function_id') ?? '';

  const [types, setTypes] = useState<ObjectType[]>([]);
  const [primaryTypeId, setPrimaryTypeId] = useState(searchParams.get('primary_type_id') ?? '');
  const [secondaryTypeId, setSecondaryTypeId] = useState(searchParams.get('secondary_type_id') ?? '');
  const [primaryObjects, setPrimaryObjects] = useState<ObjectInstance[]>([]);
  const [secondaryObjects, setSecondaryObjects] = useState<ObjectInstance[]>([]);
  const [graph, setGraph] = useState<GraphResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingVisualFunctions, setLoadingVisualFunctions] = useState(true);
  const [savingVisualFunction, setSavingVisualFunction] = useState(false);
  const [deletingVisualFunction, setDeletingVisualFunction] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const [dateField, setDateField] = useState(searchParams.get('date_field') ?? '');
  const [metricField, setMetricField] = useState(searchParams.get('metric_field') ?? '');
  const [groupField, setGroupField] = useState(searchParams.get('group_field') ?? '');
  const [joinField, setJoinField] = useState(searchParams.get('join_field') ?? '');
  const [secondaryJoinField, setSecondaryJoinField] = useState(
    searchParams.get('secondary_join_field') ?? '',
  );
  const [selectedGroup, setSelectedGroup] = useState(searchParams.get('selected_group') ?? '');

  const [visualFunctions, setVisualFunctions] = useState<QuiverVisualFunction[]>([]);
  const [selectedVisualFunctionId, setSelectedVisualFunctionId] = useState('');
  const [visualFunctionName, setVisualFunctionName] = useState('');
  const [visualFunctionDescription, setVisualFunctionDescription] = useState('');
  const [chartKind, setChartKind] = useState<QuiverChartKind>('line');
  const [sharedVisualFunction, setSharedVisualFunction] = useState(false);

  function primaryTypeLabel() {
    return types.find((type) => type.id === primaryTypeId)?.display_name ?? 'Quiver';
  }

  function secondaryTypeLabel() {
    return secondaryTypeId
      ? types.find((type) => type.id === secondaryTypeId)?.display_name ?? 'Joined type'
      : 'No join';
  }

  // ── Initial load ──
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      setLoadingVisualFunctions(true);
      try {
        const response = await listQuiverVisualFunctions({ per_page: 100, include_shared: true });
        if (!cancelled) setVisualFunctions(response.data);
      } catch (cause) {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Failed to load saved Quiver lenses');
          setVisualFunctions([]);
        }
      } finally {
        if (!cancelled) setLoadingVisualFunctions(false);
      }
    })();

    void (async () => {
      setLoading(true);
      setError('');
      try {
        const response = await listObjectTypes({ per_page: 100 });
        if (cancelled) return;
        setTypes(response.data);
        setPrimaryTypeId((current) => current || response.data[0]?.id || '');
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load ontology types');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  // Once visual functions are loaded, auto-apply the requested one (URL parameter).
  useEffect(() => {
    if (!requestedVisualFunctionId || visualFunctions.length === 0) return;
    const vf = visualFunctions.find((entry) => entry.id === requestedVisualFunctionId);
    if (vf) void applyVisualFunction(vf);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visualFunctions, requestedVisualFunctionId]);

  // Load primary objects + graph whenever primary type changes.
  useEffect(() => {
    if (!primaryTypeId) {
      setPrimaryObjects([]);
      setGraph(null);
      return;
    }
    let cancelled = false;
    setError('');
    (async () => {
      try {
        const rows = await loadAllObjects(primaryTypeId);
        if (cancelled) return;
        setPrimaryObjects(rows);
        try {
          const graphResponse = await getOntologyGraph({
            root_type_id: primaryTypeId,
            depth: 2,
            limit: 120,
          });
          if (!cancelled) setGraph(graphResponse);
        } catch {
          if (!cancelled) setGraph(null);
        }
      } catch (cause) {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Failed to load primary object set');
          setPrimaryObjects([]);
          setGraph(null);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [primaryTypeId]);

  // Load secondary objects whenever secondary type changes.
  useEffect(() => {
    if (!secondaryTypeId) {
      setSecondaryObjects([]);
      return;
    }
    let cancelled = false;
    setError('');
    (async () => {
      try {
        const rows = await loadAllObjects(secondaryTypeId);
        if (!cancelled) setSecondaryObjects(rows);
      } catch (cause) {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Failed to load secondary object set');
          setSecondaryObjects([]);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [secondaryTypeId]);

  // Hydrate field defaults whenever objects arrive.
  useEffect(() => {
    if (primaryObjects.length === 0) return;
    const sample = primaryObjects[0]?.properties ?? {};
    const keys = Object.keys(sample);
    setDateField((current) => current || keys.find((key) => /date|time|day|month/i.test(key)) || '');
    setMetricField(
      (current) =>
        current || keys.find((key) => typeof sample[key] === 'number') || keys[0] || '',
    );
    setGroupField((current) => current || keys[0] || '');
    setJoinField((current) => current || keys[0] || '');
    setSecondaryJoinField(
      (current) => current || Object.keys(secondaryObjects[0]?.properties ?? {})[0] || keys[0] || '',
    );
    setVisualFunctionName((current) => current || `${primaryTypeLabel()} lens`);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [primaryObjects, secondaryObjects]);

  // ── Derived data ──
  const joinedObjects = useMemo(() => {
    if (!secondaryTypeId || !joinField || !secondaryJoinField || secondaryObjects.length === 0) {
      return primaryObjects.map((object) => object.properties);
    }
    const secondaryIndex = Object.fromEntries(
      secondaryObjects.map((object) => [String(object.properties[secondaryJoinField] ?? ''), object]),
    );
    return primaryObjects.map((object) => {
      const secondary = secondaryIndex[String(object.properties[joinField] ?? '')];
      const prefixed = secondary
        ? Object.fromEntries(
            Object.entries(secondary.properties).map(([key, value]) => [`linked_${key}`, value]),
          )
        : {};
      return { ...object.properties, ...prefixed };
    });
  }, [primaryObjects, secondaryObjects, secondaryTypeId, joinField, secondaryJoinField]);

  const sampleKeys = useMemo(() => Object.keys(joinedObjects[0] ?? {}), [joinedObjects]);
  const primaryKeys = useMemo(
    () => Object.keys(primaryObjects[0]?.properties ?? {}),
    [primaryObjects],
  );
  const secondaryKeys = useMemo(
    () => Object.keys(secondaryObjects[0]?.properties ?? {}),
    [secondaryObjects],
  );

  const timeSeriesRows = useMemo(() => {
    const buckets: Record<string, { total: number; count: number }> = {};
    for (const row of joinedObjects) {
      const group = String(row[dateField] ?? '').slice(0, 10);
      if (!group) continue;
      const current = buckets[group] ?? { total: 0, count: 0 };
      current.total += numericValue(row[metricField]);
      current.count += 1;
      buckets[group] = current;
    }
    return Object.entries(buckets)
      .map(([date, stats]) => ({ date, value: Number(stats.total.toFixed(2)), count: stats.count }))
      .sort((left, right) => left.date.localeCompare(right.date));
  }, [joinedObjects, dateField, metricField]);

  const groupedRows = useMemo(() => {
    const buckets: Record<string, { total: number; count: number }> = {};
    for (const row of joinedObjects) {
      const group = String(row[groupField] ?? 'Unknown');
      if (selectedGroup && group !== selectedGroup) continue;
      const current = buckets[group] ?? { total: 0, count: 0 };
      current.total += numericValue(row[metricField]);
      current.count += 1;
      buckets[group] = current;
    }
    return Object.entries(buckets)
      .map(([group, stats]) => ({ group, value: Number(stats.total.toFixed(2)), count: stats.count }))
      .sort((left, right) => right.value - left.value)
      .slice(0, 20);
  }, [joinedObjects, groupField, selectedGroup, metricField]);

  const currentVegaSpec = useMemo(
    () =>
      buildQuiverVegaSpec(
        {
          title: visualFunctionName.trim() || `${primaryTypeLabel()} lens`,
          description: visualFunctionDescription.trim(),
          primaryTypeId,
          secondaryTypeId,
          joinField,
          secondaryJoinField,
          dateField,
          metricField,
          groupField,
          selectedGroup,
          chartKind,
          shared: sharedVisualFunction,
        },
        timeSeriesRows,
        groupedRows,
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
      visualFunctionName,
      visualFunctionDescription,
      primaryTypeId,
      secondaryTypeId,
      joinField,
      secondaryJoinField,
      dateField,
      metricField,
      groupField,
      selectedGroup,
      chartKind,
      sharedVisualFunction,
      timeSeriesRows,
      groupedRows,
      types,
    ],
  );

  // ── Visual function lifecycle ──

  function currentPayload() {
    return {
      name: visualFunctionName.trim() || `${primaryTypeLabel()} lens`,
      description: visualFunctionDescription.trim(),
      primary_type_id: primaryTypeId,
      secondary_type_id: secondaryTypeId || null,
      join_field: joinField,
      secondary_join_field: secondaryJoinField,
      date_field: dateField,
      metric_field: metricField,
      group_field: groupField,
      selected_group: selectedGroup || null,
      chart_kind: chartKind,
      shared: sharedVisualFunction,
    };
  }

  async function reloadVisualFunctions(preferredId?: string) {
    setLoadingVisualFunctions(true);
    try {
      const response = await listQuiverVisualFunctions({ per_page: 100, include_shared: true });
      setVisualFunctions(response.data);
      if (preferredId && !response.data.some((entry) => entry.id === preferredId)) {
        setSelectedVisualFunctionId('');
      }
    } finally {
      setLoadingVisualFunctions(false);
    }
  }

  async function saveVisualFunction() {
    if (!primaryTypeId || !joinField || !dateField || !metricField || !groupField) {
      setError('Choose the primary type and the join/date/metric/group fields before saving the lens.');
      return;
    }
    setSavingVisualFunction(true);
    setError('');
    setNotice('');
    try {
      const payload = currentPayload();
      const isUpdate = Boolean(selectedVisualFunctionId);
      const saved = isUpdate
        ? await updateQuiverVisualFunction(selectedVisualFunctionId, payload)
        : await createQuiverVisualFunction(payload);
      setSelectedVisualFunctionId(saved.id);
      setVisualFunctionName(saved.name);
      setVisualFunctionDescription(saved.description);
      setChartKind(saved.chart_kind);
      setSharedVisualFunction(saved.shared);
      await reloadVisualFunctions(saved.id);
      setNotice(
        isUpdate
          ? `Updated ${saved.name} in the Quiver workspace library.`
          : `Saved ${saved.name} to the Quiver workspace library.`,
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to save the Quiver lens');
    } finally {
      setSavingVisualFunction(false);
    }
  }

  async function deleteVisualFunction(id: string) {
    setDeletingVisualFunction(true);
    setError('');
    setNotice('');
    try {
      await deleteQuiverVisualFunction(id);
      if (selectedVisualFunctionId === id) resetVisualFunctionDraft();
      await reloadVisualFunctions();
      setNotice('Removed the saved Quiver lens from the workspace library.');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to delete the Quiver lens');
    } finally {
      setDeletingVisualFunction(false);
    }
  }

  function resetVisualFunctionDraft() {
    setSelectedVisualFunctionId('');
    setVisualFunctionName(`${primaryTypeLabel()} lens`);
    setVisualFunctionDescription('');
    setChartKind('line');
    setSharedVisualFunction(false);
  }

  async function applyVisualFunction(vf: QuiverVisualFunction) {
    setSelectedVisualFunctionId(vf.id);
    setVisualFunctionName(vf.name);
    setVisualFunctionDescription(vf.description);
    setPrimaryTypeId(vf.primary_type_id);
    setSecondaryTypeId(vf.secondary_type_id ?? '');
    setJoinField(vf.join_field);
    setSecondaryJoinField(vf.secondary_join_field);
    setDateField(vf.date_field);
    setMetricField(vf.metric_field);
    setGroupField(vf.group_field);
    setSelectedGroup(vf.selected_group ?? '');
    setChartKind(vf.chart_kind);
    setSharedVisualFunction(vf.shared);
    setNotice(`Loaded ${vf.name}.`);
  }

  async function copyVegaSpec() {
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      setError('Clipboard access is not available in this browser context.');
      return;
    }
    try {
      await navigator.clipboard.writeText(JSON.stringify(currentVegaSpec, null, 2));
      setNotice('Copied the Vega-Lite spec to the clipboard.');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to copy the Vega spec');
    }
  }

  function downloadVegaSpec() {
    downloadJsonDocument(
      `${(visualFunctionName.trim() || primaryTypeLabel()).toLowerCase().replace(/[^a-z0-9]+/g, '-')}-vega.json`,
      currentVegaSpec,
    );
    setNotice('Downloaded the Vega-Lite spec JSON.');
  }

  // ── Render ──

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: embedded ? 16 : 24 }}>
        <p className="of-eyebrow" style={{ color: '#0284c7' }}>
          {embedded ? 'Quiver embed' : 'Quiver'}
        </p>
        <h1 className={embedded ? 'of-heading-lg' : 'of-heading-xl'} style={{ marginTop: 8 }}>
          Time-series and ontology analytics with reusable visual functions
        </h1>
        <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
          Explore object sets without code, join related domains, navigate the ontology graph,
          persist reusable lenses in the workspace catalog, and export advanced Vega-Lite specs for
          downstream use.
        </p>

        {error && (
          <div className="of-status-danger" style={{ marginTop: 16, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
            {error}
          </div>
        )}
        {notice && (
          <div className="of-status-success" style={{ marginTop: 16, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
            {notice}
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.95fr) minmax(0, 1.05fr)' }}>
        <div style={{ display: 'grid', gap: 16 }}>
          {/* Object sets panel */}
          <section className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">Object sets</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Point-and-click joins
            </h2>

            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 16 }}>
              <FieldSelect
                label="Primary type"
                value={primaryTypeId}
                onChange={setPrimaryTypeId}
                options={types.map((type) => ({ value: type.id, label: type.display_name }))}
              />
              <FieldSelect
                label="Secondary type"
                value={secondaryTypeId}
                onChange={setSecondaryTypeId}
                options={[
                  { value: '', label: 'No join' },
                  ...types
                    .filter((type) => type.id !== primaryTypeId)
                    .map((type) => ({ value: type.id, label: type.display_name })),
                ]}
              />
              <FieldSelect
                label="Join field"
                value={joinField}
                onChange={setJoinField}
                options={primaryKeys.map((key) => ({ value: key, label: key }))}
              />
              <FieldSelect
                label="Linked field"
                value={secondaryJoinField}
                onChange={setSecondaryJoinField}
                options={secondaryKeys.map((key) => ({ value: key, label: key }))}
              />
              <FieldSelect
                label="Date field"
                value={dateField}
                onChange={setDateField}
                options={sampleKeys.map((key) => ({ value: key, label: key }))}
              />
              <FieldSelect
                label="Metric field"
                value={metricField}
                onChange={setMetricField}
                options={sampleKeys.map((key) => ({ value: key, label: key }))}
              />
              <FieldSelect
                label="Group field"
                value={groupField}
                onChange={setGroupField}
                options={sampleKeys.map((key) => ({ value: key, label: key }))}
                fullWidth
              />
            </div>
          </section>

          {/* Visual functions panel */}
          <section className="of-panel" style={{ padding: 24 }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
              <div>
                <p className="of-eyebrow">Visual functions</p>
                <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                  Workspace-persisted analysis presets
                </h2>
              </div>
              <span className="of-text-muted" style={{ fontSize: 12 }}>
                {loadingVisualFunctions ? 'Syncing…' : `${visualFunctions.length} saved lens(es)`}
              </span>
            </div>

            <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
              <input
                className="of-input"
                value={visualFunctionName}
                onChange={(e) => setVisualFunctionName(e.target.value)}
                placeholder="Name this Quiver lens"
              />
              <textarea
                className="of-textarea"
                rows={3}
                value={visualFunctionDescription}
                onChange={(e) => setVisualFunctionDescription(e.target.value)}
                placeholder="Describe the lens and intended audience"
              />

              <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 1fr) auto' }}>
                <FieldSelect
                  label="Vega chart preset"
                  value={chartKind}
                  onChange={(v) => setChartKind(v as QuiverChartKind)}
                  options={[
                    { value: 'line', label: 'line' },
                    { value: 'area', label: 'area' },
                    { value: 'bar', label: 'bar' },
                    { value: 'point', label: 'point' },
                  ]}
                />
                <label
                  className="of-panel-muted"
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 12,
                    padding: '12px 16px',
                    fontSize: 13,
                  }}
                >
                  <input
                    type="checkbox"
                    checked={sharedVisualFunction}
                    onChange={(e) => setSharedVisualFunction(e.target.checked)}
                  />
                  Share with workspace
                </label>
              </div>

              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                <button type="button" className="of-btn" onClick={resetVisualFunctionDraft}>
                  New draft
                </button>
                <button
                  type="button"
                  className="of-btn of-btn-primary"
                  onClick={() => void saveVisualFunction()}
                  disabled={savingVisualFunction}
                >
                  {savingVisualFunction
                    ? 'Saving…'
                    : selectedVisualFunctionId
                      ? 'Update saved lens'
                      : 'Save lens'}
                </button>
                {selectedVisualFunctionId && (
                  <button
                    type="button"
                    className="of-btn of-btn-danger"
                    onClick={() => void deleteVisualFunction(selectedVisualFunctionId)}
                    disabled={deletingVisualFunction}
                  >
                    {deletingVisualFunction ? 'Deleting…' : 'Delete saved lens'}
                  </button>
                )}
              </div>

              <div className="of-panel-muted" style={{ padding: '10px 14px', fontSize: 13, color: 'var(--text-muted)' }}>
                Saved Quiver lenses now persist in <code>ontology-service</code> with their
                canonical Vega-Lite template, instead of living only in browser storage.
              </div>
            </div>

            <div style={{ display: 'grid', gap: 12, marginTop: 20 }}>
              {visualFunctions.length === 0 ? (
                <div
                  style={{
                    border: '1px dashed var(--border-default)',
                    borderRadius: 'var(--radius-md)',
                    padding: '20px 16px',
                    fontSize: 13,
                    color: 'var(--text-muted)',
                  }}
                >
                  Save the current lens to reuse it in dashboards, Workshop, or object views later.
                </div>
              ) : (
                visualFunctions.map((vf) => {
                  const active = selectedVisualFunctionId === vf.id;
                  return (
                    <div
                      key={vf.id}
                      style={{
                        border: `1px solid ${active ? '#0ea5e9' : 'var(--border-default)'}`,
                        background: active ? '#f0f9ff' : 'var(--bg-panel-muted)',
                        borderRadius: 'var(--radius-md)',
                        padding: 16,
                      }}
                    >
                      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                        <button
                          type="button"
                          onClick={() => void applyVisualFunction(vf)}
                          style={{
                            minWidth: 0,
                            flex: 1,
                            textAlign: 'left',
                            background: 'transparent',
                            border: 0,
                            padding: 0,
                            cursor: 'pointer',
                          }}
                        >
                          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
                            <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-strong)' }}>
                              {vf.name}
                            </div>
                            <span className="of-chip" style={{ textTransform: 'uppercase', letterSpacing: '0.18em', fontSize: 10, fontWeight: 600 }}>
                              {vf.chart_kind}
                            </span>
                            {vf.shared && (
                              <span className="of-chip of-status-success" style={{ textTransform: 'uppercase', letterSpacing: '0.18em', fontSize: 10, fontWeight: 600 }}>
                                shared
                              </span>
                            )}
                          </div>
                          <div className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                            {vf.metric_field} by {vf.group_field} •{' '}
                            {new Date(vf.updated_at).toLocaleString()}
                          </div>
                          {vf.description && (
                            <div className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
                              {vf.description}
                            </div>
                          )}
                        </button>
                        {active && (
                          <button
                            type="button"
                            className="of-btn of-btn-danger"
                            onClick={() => void deleteVisualFunction(vf.id)}
                            disabled={deletingVisualFunction}
                            style={{ minHeight: 30, fontSize: 12 }}
                          >
                            Delete
                          </button>
                        )}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </section>
        </div>

        {/* Right column: charts + vega + graph */}
        <div style={{ display: 'grid', gap: 16 }}>
          <section className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">Time series</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Metric progression over time
            </h2>
            <div style={{ marginTop: 16, height: 320 }}>
              <EChartView
                rows={timeSeriesRows.map((row) => ({ date: row.date, value: row.value }))}
                categoryKey="date"
                valueKeys={['value']}
                mode={chartKind === 'point' ? 'line' : chartKind}
                emptyLabel={
                  loading
                    ? 'Loading ontology objects…'
                    : 'Pick a type with date and numeric properties to render the time series.'
                }
              />
            </div>
          </section>

          <section className="of-panel" style={{ padding: 24 }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
              <div>
                <p className="of-eyebrow">Object analytics</p>
                <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                  Interactive grouped lens
                </h2>
              </div>
              {selectedGroup && (
                <button
                  type="button"
                  className="of-btn"
                  onClick={() => setSelectedGroup('')}
                  style={{ minHeight: 30, fontSize: 13 }}
                >
                  Clear {selectedGroup}
                </button>
              )}
            </div>
            <div style={{ marginTop: 16, height: 320 }}>
              <EChartView
                rows={groupedRows.map((row) => ({ group: row.group, value: row.value }))}
                categoryKey="group"
                valueKeys={['value']}
                mode="bar"
                emptyLabel="Choose group and metric fields to build the object lens."
                onCategoryClick={(value) => setSelectedGroup(value)}
              />
            </div>
          </section>

          <section className="of-panel" style={{ padding: 24 }}>
            <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
              <div style={{ maxWidth: 540 }}>
                <p className="of-eyebrow">Vega plots</p>
                <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                  Hydrated Vega-Lite export
                </h2>
                <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13, lineHeight: 1.6 }}>
                  Quiver emits a live Vega-Lite spec for the current lens, including time-series
                  and grouped analytics datasets, so the same analysis can move into external
                  notebooks, published dashboards, or other frontend surfaces.
                </p>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                <button type="button" className="of-btn" onClick={() => void copyVegaSpec()}>
                  Copy spec
                </button>
                <button type="button" className="of-btn" onClick={downloadVegaSpec}>
                  Download JSON
                </button>
              </div>
            </div>

            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(3, 1fr)', marginTop: 16 }}>
              {[
                { label: 'Preset', value: chartKind },
                { label: 'Time-series rows', value: timeSeriesRows.length },
                { label: 'Grouped rows', value: groupedRows.length },
              ].map((card) => (
                <div key={card.label} className="of-panel-muted" style={{ padding: 16 }}>
                  <p className="of-eyebrow">{card.label}</p>
                  <p style={{ marginTop: 8, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>
                    {card.value}
                  </p>
                </div>
              ))}
            </div>

            <div
              style={{
                marginTop: 16,
                background: '#0f172a',
                borderRadius: 'var(--radius-md)',
                padding: 16,
              }}
            >
              <pre
                className="of-scrollbar"
                style={{
                  margin: 0,
                  overflowX: 'auto',
                  whiteSpace: 'pre-wrap',
                  fontSize: 11,
                  lineHeight: 1.6,
                  color: '#f1f5f9',
                  fontFamily: 'var(--font-mono)',
                }}
              >
                {JSON.stringify(currentVegaSpec, null, 2)}
              </pre>
            </div>
          </section>

          <section className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">Graph navigation</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Relationship overview for the current object type
            </h2>

            {graph ? (
              <>
                <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(3, 1fr)', marginTop: 16 }}>
                  <div className="of-panel-muted" style={{ padding: 16 }}>
                    <p className="of-eyebrow">Nodes</p>
                    <p style={{ marginTop: 8, fontSize: 28, fontWeight: 600, color: 'var(--text-strong)' }}>
                      {graph.total_nodes}
                    </p>
                  </div>
                  <div className="of-panel-muted" style={{ padding: 16 }}>
                    <p className="of-eyebrow">Edges</p>
                    <p style={{ marginTop: 8, fontSize: 28, fontWeight: 600, color: 'var(--text-strong)' }}>
                      {graph.total_edges}
                    </p>
                  </div>
                  <div className="of-panel-muted" style={{ padding: 16 }}>
                    <p className="of-eyebrow">Join scope</p>
                    <p style={{ marginTop: 8, fontSize: 14, fontWeight: 600, color: 'var(--text-strong)' }}>
                      {primaryTypeLabel()} → {secondaryTypeLabel()}
                    </p>
                  </div>
                </div>

                <div className="of-panel-muted" style={{ marginTop: 16, padding: 16 }}>
                  <div style={{ fontWeight: 600, color: 'var(--text-strong)', fontSize: 14 }}>
                    Related nodes
                  </div>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
                    {graph.nodes.slice(0, 18).map((node) => (
                      <span key={node.id} className="of-chip" style={{ fontSize: 11 }}>
                        {node.label}
                      </span>
                    ))}
                  </div>
                </div>
              </>
            ) : (
              <div
                style={{
                  marginTop: 16,
                  border: '1px dashed var(--border-default)',
                  borderRadius: 'var(--radius-md)',
                  padding: '20px 16px',
                  fontSize: 13,
                  color: 'var(--text-muted)',
                }}
              >
                Load a primary object type to inspect its relationship graph.
              </div>
            )}
          </section>
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
        value={value}
        onChange={(e) => onChange(e.target.value)}
        style={{
          marginTop: 6,
          width: '100%',
          background: 'transparent',
          border: 0,
          padding: 0,
          minHeight: 0,
          fontSize: 13,
          outline: 'none',
        }}
      >
        {options.map((option) => (
          <option key={`${option.value}-${option.label}`} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}
