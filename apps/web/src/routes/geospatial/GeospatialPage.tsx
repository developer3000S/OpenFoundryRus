import { useEffect, useMemo, useState } from 'react';

import { HeatmapLayer } from '@/lib/components/map/HeatmapLayer';
import { LayerPanel } from '@/lib/components/map/LayerPanel';
import { MapView } from '@/lib/components/map/MapView';
import { RouteLayer } from '@/lib/components/map/RouteLayer';
import { SpatialAnalysis, type AnalysisDraft } from '@/lib/components/map/SpatialAnalysis';
import {
  clusterFeatures,
  createLayer,
  geocodeAddress,
  getOverview,
  getVectorTile,
  listLayers,
  reverseGeocode,
  routeFeatures,
  runSpatialQuery,
  updateLayer,
  type ClusterResponse,
  type Coordinate,
  type GeocodeResponse,
  type GeospatialOverview,
  type LayerDefinition,
  type LayerStyle,
  type MapFeature,
  type RouteResponse,
  type SpatialQueryResponse,
  type VectorTileResponse,
} from '@/lib/api/geospatial';
import { notifications } from '@stores/notifications';

type TimelineMode = 'up_to' | 'single';

interface TemporalFeatureEntry {
  feature: MapFeature;
  timestamp: number;
  label: string;
}

interface HistogramBucket {
  label: string;
  count: number;
}

const TIMESTAMP_KEYS = [
  'timestamp',
  'event_time',
  'time',
  'datetime',
  'date',
  'created_at',
  'updated_at',
];

function defaultLayerStyle(): LayerStyle {
  return {
    color: '#0f766e',
    opacity: 0.82,
    radius: 8,
    line_width: 3,
    heatmap_intensity: 0.9,
    cluster_color: '#0f766e',
    show_labels: true,
  };
}

function createEmptyAnalysisDraft(): AnalysisDraft {
  return {
    operation: 'nearest',
    point_lat: '40.4168',
    point_lon: '-3.7038',
    radius_km: 18,
    limit: 5,
    cluster_algorithm: 'dbscan',
    cluster_count: 3,
    cluster_radius_km: 30,
    geocode_query: 'Madrid',
    origin_lat: '40.4168',
    origin_lon: '-3.7038',
    destination_lat: '39.4699',
    destination_lon: '-0.3763',
    route_mode: 'drive',
    route_max_minutes: 45,
  };
}

function geometryCoordinates(geometry: LayerDefinition['features'][number]['geometry']): Coordinate[] {
  if (geometry.type === 'point') return [geometry.coordinates];
  return geometry.coordinates;
}

function parseFeatureTimestamp(feature: MapFeature, index: number, layer: LayerDefinition): number {
  for (const key of TIMESTAMP_KEYS) {
    const value = feature.properties[key];
    if (typeof value === 'string' || typeof value === 'number') {
      const parsed = new Date(value).getTime();
      if (!Number.isNaN(parsed)) return parsed;
    }
  }
  const base = new Date(layer.created_at).getTime();
  return base + index * 3_600_000;
}

function featureLabel(feature: MapFeature, index: number) {
  return feature.label || String(feature.properties.name ?? `Feature ${index + 1}`);
}

function temporalEntries(layer: LayerDefinition | null): TemporalFeatureEntry[] {
  if (!layer) return [];
  return layer.features
    .map((feature, index) => ({
      feature,
      timestamp: parseFeatureTimestamp(feature, index, layer),
      label: featureLabel(feature, index),
    }))
    .sort((left, right) => left.timestamp - right.timestamp);
}

function firstNumericField(features: MapFeature[]) {
  const counts = new Map<string, number>();
  for (const feature of features) {
    for (const [key, value] of Object.entries(feature.properties)) {
      if (typeof value === 'number' && Number.isFinite(value)) {
        counts.set(key, (counts.get(key) ?? 0) + 1);
      }
    }
  }
  return [...counts.entries()].sort((left, right) => right[1] - left[1])[0]?.[0] ?? '';
}

function numericFieldRange(features: MapFeature[], field: string) {
  const values = features
    .map((feature) => feature.properties[field])
    .filter((value): value is number => typeof value === 'number' && Number.isFinite(value));
  if (values.length === 0) return { min: 0, max: 0 };
  return { min: Math.min(...values), max: Math.max(...values) };
}

function pointToBounds(point: Coordinate, radiusKm: number) {
  const delta = radiusKm / 111.0;
  return {
    min_lat: point.lat - delta,
    min_lon: point.lon - delta,
    max_lat: point.lat + delta,
    max_lon: point.lon + delta,
  };
}

export function GeospatialPage() {
  const [overview, setOverview] = useState<GeospatialOverview | null>(null);
  const [layers, setLayers] = useState<LayerDefinition[]>([]);
  const [selectedLayerId, setSelectedLayerId] = useState('');
  const [tile, setTile] = useState<VectorTileResponse | null>(null);
  const [queryResponse, setQueryResponse] = useState<SpatialQueryResponse | null>(null);
  const [clusterResponse, setClusterResponse] = useState<ClusterResponse | null>(null);
  const [routeResponse, setRouteResponse] = useState<RouteResponse | null>(null);
  const [geocodeResponse, setGeocodeResponse] = useState<GeocodeResponse | null>(null);
  const [reverseGeocodeResponse, setReverseGeocodeResponse] = useState<GeocodeResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [uiError, setUiError] = useState('');

  const [draft, setDraft] = useState<AnalysisDraft>(createEmptyAnalysisDraft());
  const [styleDraft, setStyleDraft] = useState<LayerStyle>(defaultLayerStyle());
  const [tagText, setTagText] = useState('');
  const [persistIndexed, setPersistIndexed] = useState(true);
  const [selectedNumericField, setSelectedNumericField] = useState('');
  const [numericMin, setNumericMin] = useState(0);
  const [numericMax, setNumericMax] = useState(0);
  const [timelineCursor, setTimelineCursor] = useState(100);
  const [timelineMode, setTimelineMode] = useState<TimelineMode>('up_to');
  const [templateName, setTemplateName] = useState('');
  const [templateDescription, setTemplateDescription] = useState('');

  const busy = loading || busyAction.length > 0;
  const selectedLayer = useMemo(
    () => layers.find((entry) => entry.id === selectedLayerId) ?? null,
    [layers, selectedLayerId],
  );
  const searchResults = useMemo(
    () =>
      [
        ...(geocodeResponse ? [geocodeResponse] : []),
        ...(reverseGeocodeResponse ? [reverseGeocodeResponse] : []),
      ] as GeocodeResponse[],
    [geocodeResponse, reverseGeocodeResponse],
  );
  const templateLayers = useMemo(
    () => layers.filter((layer) => layer.tags.includes('template') || layer.tags.includes('saved-map')),
    [layers],
  );

  function updateDraft(patch: Partial<AnalysisDraft>) {
    setDraft((current) => ({ ...current, ...patch }));
  }

  function updateStyleDraft(patch: Partial<LayerStyle>) {
    setStyleDraft((current) => ({ ...current, ...patch }));
  }

  async function selectLayerById(layerId: string, notify = true, source?: LayerDefinition[]) {
    setSelectedLayerId(layerId);
    const pool = source ?? layers;
    const layer = pool.find((entry) => entry.id === layerId) ?? null;
    if (!layer) return;

    // Seed analysis draft from the layer's first features.
    const coordinates = layer.features.flatMap((feature) => geometryCoordinates(feature.geometry));
    const first = coordinates[0];
    const second = coordinates[1] ?? first;
    if (first && second) {
      setDraft((current) => ({
        ...current,
        point_lat: first.lat.toFixed(4),
        point_lon: first.lon.toFixed(4),
        origin_lat: first.lat.toFixed(4),
        origin_lon: first.lon.toFixed(4),
        destination_lat: second.lat.toFixed(4),
        destination_lon: second.lon.toFixed(4),
      }));
    }

    setStyleDraft({ ...layer.style });
    setTagText(layer.tags.join(', '));
    setPersistIndexed(layer.indexed);
    setTemplateName(`${layer.name} template`);
    setTemplateDescription(`Saved map template derived from ${layer.name}`);

    const numericField = firstNumericField(layer.features);
    setSelectedNumericField(numericField);
    if (numericField) {
      const range = numericFieldRange(layer.features, numericField);
      setNumericMin(range.min);
      setNumericMax(range.max);
    } else {
      setNumericMin(0);
      setNumericMax(0);
    }
    setTimelineCursor(100);

    setQueryResponse(null);
    setClusterResponse(null);
    try {
      const tileResponse = await getVectorTile(layer.id);
      setTile(tileResponse);
    } catch {
      // Soft-fail; the panel renders an empty state.
    }
    if (notify) notifications.info(`Loaded ${layer.name}`);
  }

  async function refreshAll(preferredLayerId?: string) {
    setLoading(true);
    setUiError('');
    try {
      const [overviewResponse, layersResponse] = await Promise.all([getOverview(), listLayers()]);
      setOverview(overviewResponse);
      setLayers(layersResponse.items);
      const nextLayerId = preferredLayerId ?? selectedLayerId ?? layersResponse.items[0]?.id ?? '';
      if (nextLayerId) {
        await selectLayerById(nextLayerId, false, layersResponse.items);
      } else {
        setTile(null);
      }
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Unable to load geospatial surfaces';
      setUiError(message);
      notifications.error(message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refreshAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function currentPoint(): Coordinate {
    return { lat: Number(draft.point_lat), lon: Number(draft.point_lon) };
  }

  // ── Filtered visible layer ──
  const filteredFeatures = useMemo(() => {
    if (!selectedLayer) return [] as MapFeature[];
    let features = selectedLayer.features;

    const temporal = temporalEntries(selectedLayer);
    if (temporal.length > 0) {
      const index = Math.max(0, Math.round((timelineCursor / 100) * Math.max(0, temporal.length - 1)));
      const selectedTimestamp =
        temporal[index]?.timestamp ?? temporal[temporal.length - 1]?.timestamp ?? 0;
      const allowedIds =
        timelineMode === 'single'
          ? new Set([temporal[index]?.feature.id].filter(Boolean) as string[])
          : new Set(
              temporal
                .filter((entry) => entry.timestamp <= selectedTimestamp)
                .map((entry) => entry.feature.id),
            );
      features = features.filter((feature) => allowedIds.has(feature.id));
    }

    if (selectedNumericField) {
      features = features.filter((feature) => {
        const value = feature.properties[selectedNumericField];
        if (typeof value !== 'number' || !Number.isFinite(value)) return false;
        return value >= numericMin && value <= numericMax;
      });
    }

    return features;
  }, [selectedLayer, timelineCursor, timelineMode, selectedNumericField, numericMin, numericMax]);

  const visibleLayer = useMemo<LayerDefinition | null>(() => {
    if (!selectedLayer) return null;
    return {
      ...selectedLayer,
      style: styleDraft,
      indexed: persistIndexed,
      features: filteredFeatures,
    };
  }, [selectedLayer, styleDraft, persistIndexed, filteredFeatures]);

  function timelineIndexLabel() {
    const entries = temporalEntries(selectedLayer);
    if (entries.length === 0) return 'No events';
    const index = Math.max(0, Math.round((timelineCursor / 100) * Math.max(0, entries.length - 1)));
    return new Intl.DateTimeFormat('en', {
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
    }).format(new Date(entries[index].timestamp));
  }

  function timelineEvents() {
    const entries = temporalEntries(selectedLayer);
    if (entries.length === 0) return [];
    const index = Math.max(0, Math.round((timelineCursor / 100) * Math.max(0, entries.length - 1)));
    if (timelineMode === 'single') return entries.slice(Math.max(0, index - 2), index + 3);
    return entries.filter((entry) => entry.timestamp <= entries[index].timestamp).slice(-6);
  }

  const histogramBuckets = useMemo<HistogramBucket[]>(() => {
    if (!selectedLayer || !selectedNumericField) return [];
    const values = selectedLayer.features
      .map((feature) => feature.properties[selectedNumericField])
      .filter((value): value is number => typeof value === 'number' && Number.isFinite(value));
    if (values.length === 0) return [];

    const range = numericFieldRange(selectedLayer.features, selectedNumericField);
    if (range.min === range.max) return [{ label: String(range.min), count: values.length }];

    const bucketCount = 6;
    const step = (range.max - range.min) / bucketCount;
    const buckets: HistogramBucket[] = Array.from({ length: bucketCount }, (_, index) => ({
      label: `${(range.min + step * index).toFixed(0)}-${(range.min + step * (index + 1)).toFixed(0)}`,
      count: 0,
    }));
    for (const value of values) {
      const bucketIndex = Math.min(bucketCount - 1, Math.floor((value - range.min) / step));
      buckets[bucketIndex].count += 1;
    }
    return buckets;
  }, [selectedLayer, selectedNumericField]);

  // ── Action handlers ──

  async function runQuery() {
    if (!selectedLayerId) {
      notifications.warning('Select a layer before running a query');
      return;
    }
    setBusyAction('spatial-query');
    setUiError('');
    try {
      const point = currentPoint();
      const response = await runSpatialQuery({
        layer_id: selectedLayerId,
        operation: draft.operation,
        bounds:
          draft.operation === 'within' || draft.operation === 'intersects'
            ? pointToBounds(point, draft.radius_km)
            : undefined,
        point: draft.operation === 'nearest' || draft.operation === 'buffer' ? point : undefined,
        radius_km: draft.radius_km,
        limit: draft.limit,
      });
      setQueryResponse(response);
      notifications.success(`Spatial query returned ${response.summary.matched_count} matches`);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Unable to run spatial query';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function runClusters() {
    if (!selectedLayerId) {
      notifications.warning('Select a layer before clustering');
      return;
    }
    setBusyAction('cluster');
    setUiError('');
    try {
      const response = await clusterFeatures({
        layer_id: selectedLayerId,
        algorithm: draft.cluster_algorithm,
        cluster_count: draft.cluster_count,
        radius_km: draft.cluster_radius_km,
      });
      setClusterResponse(response);
      notifications.success(`Generated ${response.clusters.length} clusters`);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Unable to cluster layer';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function runRouting() {
    setBusyAction('route');
    setUiError('');
    try {
      const response = await routeFeatures({
        origin: { lat: Number(draft.origin_lat), lon: Number(draft.origin_lon) },
        destination: { lat: Number(draft.destination_lat), lon: Number(draft.destination_lon) },
        mode: draft.route_mode,
        max_minutes: draft.route_max_minutes,
      });
      setRouteResponse(response);
      notifications.success(`Route computed in ${response.duration_min} minutes`);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Unable to compute route';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function runGeocodeSearch() {
    setBusyAction('geocode');
    setUiError('');
    try {
      const response = await geocodeAddress({ address: draft.geocode_query });
      setGeocodeResponse(response);
      updateDraft({
        point_lat: response.coordinate.lat.toFixed(4),
        point_lon: response.coordinate.lon.toFixed(4),
        origin_lat: response.coordinate.lat.toFixed(4),
        origin_lon: response.coordinate.lon.toFixed(4),
      });
      notifications.success(`Geocoded ${response.address}`);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Unable to geocode address';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function runReverseGeocodeSearch() {
    setBusyAction('reverse-geocode');
    setUiError('');
    try {
      const response = await reverseGeocode({ coordinate: currentPoint() });
      setReverseGeocodeResponse(response);
      notifications.success(`Reverse geocoded ${response.address}`);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Unable to reverse geocode coordinate';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function saveLayerConfiguration() {
    if (!selectedLayer) {
      notifications.warning('Select a layer before saving settings');
      return;
    }
    setBusyAction('save-layer');
    setUiError('');
    try {
      const tags = tagText.split(',').map((value) => value.trim()).filter(Boolean);
      await updateLayer(selectedLayer.id, { style: styleDraft, indexed: persistIndexed, tags });
      notifications.success(`Saved settings for ${selectedLayer.name}`);
      await refreshAll(selectedLayer.id);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Unable to save layer settings';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function saveCurrentMapTemplate() {
    if (!visibleLayer || !selectedLayer) {
      notifications.warning('Select a layer before saving a template');
      return;
    }
    if (visibleLayer.features.length === 0) {
      notifications.warning('The current filtered view has no features to save');
      return;
    }
    setBusyAction('save-template');
    setUiError('');
    try {
      const tags = [...new Set([...selectedLayer.tags, 'template', 'saved-map'])];
      const created = await createLayer({
        name: templateName.trim() || `${selectedLayer.name} template`,
        description: templateDescription.trim() || `Saved map template derived from ${selectedLayer.name}`,
        source_kind: selectedLayer.source_kind,
        source_dataset: `${selectedLayer.source_dataset}.template`,
        geometry_type: selectedLayer.geometry_type,
        style: styleDraft,
        features: visibleLayer.features,
        tags,
        indexed: persistIndexed,
      });
      notifications.success(`Saved template ${created.name}`);
      await refreshAll(created.id);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Unable to save template';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  // ── Inspection panel summary ──
  const querySummaryCards = useMemo(() => {
    const response = queryResponse;
    if (!response) return [];
    return response.matched_features.slice(0, 6).map((feature) => ({
      id: feature.id,
      label: feature.label,
      detail: Object.entries(feature.properties)
        .slice(0, 3)
        .map(([key, value]) => `${key}: ${String(value)}`)
        .join(' • '),
    }));
  }, [queryResponse]);

  const numericFieldOptions = useMemo(() => {
    if (!selectedLayer) return [] as string[];
    return Object.keys(selectedLayer.features[0]?.properties ?? {}).filter((key) =>
      selectedLayer.features.some((feature) => typeof feature.properties[key] === 'number'),
    );
  }, [selectedLayer]);

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ maxWidth: 720 }}>
            <p className="of-eyebrow" style={{ color: '#0e7490' }}>
              Geospatial
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              Maps, layers, queries, clustering, and routing
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
              Live MapLibre canvas backed by the geospatial service: indexed layers, vector tile
              metadata, spatial queries, clustering, geocoding, routing, and saved map templates.
            </p>
          </div>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            {[
              { label: 'Layers', value: overview?.layer_count ?? 0 },
              { label: 'Indexed', value: overview?.indexed_layers ?? 0 },
              { label: 'Features', value: overview?.total_features ?? 0 },
              { label: 'Templates', value: templateLayers.length },
            ].map((stat) => (
              <div key={stat.label} className="of-panel-muted" style={{ padding: 16 }}>
                <p className="of-eyebrow">{stat.label}</p>
                <p style={{ marginTop: 8, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>
                  {stat.value}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>

      {uiError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {uiError}
        </div>
      )}

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.92fr) minmax(0, 1.08fr)' }}>
        <LayerPanel
          layers={layers}
          selectedLayerId={selectedLayerId}
          onSelectLayer={(id) => void selectLayerById(id)}
        />
        <MapView
          layer={visibleLayer}
          tile={tile}
          queryResponse={queryResponse}
          clusterResponse={clusterResponse}
          routeResponse={routeResponse}
          searchResults={searchResults}
        />
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.98fr) minmax(0, 1.02fr)' }}>
        <section className="of-panel" style={{ padding: 20 }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
            <div>
              <p className="of-eyebrow" style={{ color: '#0e7490' }}>
                Control panel
              </p>
              <h3 className="of-heading-md" style={{ marginTop: 4 }}>
                Layer management and styling
              </h3>
              <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                Adjust visual defaults, indexing posture, and saved tags for the active layer.
              </p>
            </div>
            <button
              type="button"
              className="of-btn of-btn-primary"
              onClick={() => void saveLayerConfiguration()}
              disabled={busy || !selectedLayer}
            >
              {busyAction === 'save-layer' ? 'Saving…' : 'Save settings'}
            </button>
          </div>

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 16 }}>
            <ColorField label="Layer color" value={styleDraft.color} onChange={(v) => updateStyleDraft({ color: v })} />
            <ColorField label="Cluster color" value={styleDraft.cluster_color} onChange={(v) => updateStyleDraft({ cluster_color: v })} />
            <RangeField
              label="Opacity"
              value={styleDraft.opacity}
              min={0.1}
              max={1}
              step={0.05}
              onChange={(v) => updateStyleDraft({ opacity: v })}
            />
            <RangeField
              label="Heatmap intensity"
              value={styleDraft.heatmap_intensity}
              min={0.1}
              max={2}
              step={0.1}
              onChange={(v) => updateStyleDraft({ heatmap_intensity: v })}
            />
            <NumberField
              label="Point radius"
              value={styleDraft.radius}
              onChange={(v) => updateStyleDraft({ radius: v })}
            />
            <NumberField
              label="Line width"
              value={styleDraft.line_width}
              onChange={(v) => updateStyleDraft({ line_width: v })}
            />
            <CheckboxField
              label="Show labels"
              checked={styleDraft.show_labels}
              onChange={(v) => updateStyleDraft({ show_labels: v })}
            />
            <CheckboxField
              label="Indexed layer"
              checked={persistIndexed}
              onChange={setPersistIndexed}
            />
          </div>

          <label style={{ display: 'block', marginTop: 16, fontSize: 13 }}>
            <div className="of-eyebrow" style={{ marginBottom: 6 }}>
              Tags
            </div>
            <input
              className="of-input"
              value={tagText}
              onChange={(e) => setTagText(e.target.value)}
              placeholder="operations, airports, saved-map"
            />
          </label>
        </section>

        <section className="of-panel" style={{ padding: 20 }}>
          <p className="of-eyebrow" style={{ color: '#0e7490' }}>
            Time
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 4 }}>
            Timeline, events, and temporal filtering
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Move through ordered events and limit the map to a single moment or an accumulated time
            window.
          </p>

          <div className="of-panel-muted" style={{ padding: 16, marginTop: 16 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
              <div>
                <p style={{ fontWeight: 600, color: 'var(--text-strong)', fontSize: 14 }}>
                  {timelineIndexLabel()}
                </p>
                <p className="of-eyebrow" style={{ marginTop: 4 }}>
                  {timelineMode === 'up_to' ? 'Timeline window' : 'Single event'}
                </p>
              </div>
              <div style={{ display: 'flex', gap: 6 }}>
                <button
                  type="button"
                  className={timelineMode === 'up_to' ? 'of-btn of-btn-primary' : 'of-btn'}
                  onClick={() => setTimelineMode('up_to')}
                  style={{ minHeight: 30, fontSize: 12 }}
                >
                  Timeline
                </button>
                <button
                  type="button"
                  className={timelineMode === 'single' ? 'of-btn of-btn-primary' : 'of-btn'}
                  onClick={() => setTimelineMode('single')}
                  style={{ minHeight: 30, fontSize: 12 }}
                >
                  Event
                </button>
              </div>
            </div>
            <input
              type="range"
              min={0}
              max={100}
              step={1}
              value={timelineCursor}
              onChange={(e) => setTimelineCursor(Number(e.target.value))}
              style={{ marginTop: 16, width: '100%' }}
            />
            <div style={{ display: 'grid', gap: 8, marginTop: 16 }}>
              {timelineEvents().map((event) => (
                <div key={`${event.feature.id}-${event.timestamp}`} className="of-panel" style={{ padding: 12 }}>
                  <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{event.label}</div>
                  <div className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
                    {new Date(event.timestamp).toLocaleString()}
                  </div>
                </div>
              ))}
              {timelineEvents().length === 0 && (
                <p className="of-text-muted" style={{ fontSize: 13 }}>
                  No temporal entries are available for the current layer.
                </p>
              )}
            </div>
          </div>
        </section>
      </div>

      <SpatialAnalysis
        selectedLayer={selectedLayer}
        draft={draft}
        busy={busy}
        queryResponse={queryResponse}
        clusterResponse={clusterResponse}
        geocodeResponse={geocodeResponse}
        reverseGeocodeResponse={reverseGeocodeResponse}
        onDraftChange={updateDraft}
        onRunQuery={() => void runQuery()}
        onRunCluster={() => void runClusters()}
        onRunRoute={() => void runRouting()}
        onGeocode={() => void runGeocodeSearch()}
        onReverseGeocode={() => void runReverseGeocodeSearch()}
      />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.92fr) minmax(0, 1.08fr)' }}>
        <section className="of-panel" style={{ padding: 20 }}>
          <p className="of-eyebrow" style={{ color: '#0e7490' }}>
            Selection and filtering
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 4 }}>
            Histogram and query-driven inspection
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Filter the active layer by a numeric property and inspect query matches as map selections.
          </p>

          <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '220px minmax(0, 1fr)', marginTop: 16 }}>
            <div style={{ display: 'grid', gap: 12 }}>
              <label style={{ display: 'block', fontSize: 13 }}>
                <div className="of-eyebrow" style={{ marginBottom: 6 }}>
                  Numeric field
                </div>
                <select
                  className="of-select"
                  value={selectedNumericField}
                  onChange={(e) => {
                    const field = e.target.value;
                    setSelectedNumericField(field);
                    if (selectedLayer && field) {
                      const range = numericFieldRange(selectedLayer.features, field);
                      setNumericMin(range.min);
                      setNumericMax(range.max);
                    }
                  }}
                >
                  <option value="">No numeric filter</option>
                  {numericFieldOptions.map((field) => (
                    <option key={field} value={field}>
                      {field}
                    </option>
                  ))}
                </select>
              </label>
              {selectedNumericField && (
                <>
                  <NumberField label="Min value" value={numericMin} onChange={setNumericMin} />
                  <NumberField label="Max value" value={numericMax} onChange={setNumericMax} />
                </>
              )}
            </div>

            <div className="of-panel-muted" style={{ padding: 16 }}>
              {histogramBuckets.length === 0 ? (
                <p className="of-text-muted" style={{ fontSize: 13 }}>
                  No numeric histogram is available for the active layer.
                </p>
              ) : (
                <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(6, 1fr)' }}>
                  {histogramBuckets.map((bucket) => {
                    const peak = Math.max(1, ...histogramBuckets.map((entry) => entry.count));
                    return (
                      <div key={bucket.label} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8 }}>
                        <div
                          style={{
                            display: 'flex',
                            height: 128,
                            width: '100%',
                            alignItems: 'flex-end',
                            background: '#fff',
                            borderRadius: 'var(--radius-md)',
                            padding: 8,
                            border: '1px solid var(--border-default)',
                          }}
                        >
                          <div
                            style={{
                              width: '100%',
                              borderRadius: 'var(--radius-sm)',
                              background: 'linear-gradient(180deg, #5eead4 0%, #0891b2 100%)',
                              height: `${Math.max(10, Math.round((bucket.count / peak) * 100))}%`,
                            }}
                          />
                        </div>
                        <p style={{ fontSize: 11, color: 'var(--text-muted)', textAlign: 'center' }}>{bucket.label}</p>
                        <p style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>{bucket.count}</p>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>

          <div style={{ display: 'grid', gap: 8, marginTop: 16 }}>
            {querySummaryCards.length === 0 ? (
              <p className="of-text-muted" style={{ fontSize: 13 }}>
                Run a spatial query to inspect selected map objects here.
              </p>
            ) : (
              querySummaryCards.map((card) => (
                <div key={card.id} className="of-panel-muted" style={{ padding: 12 }}>
                  <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{card.label}</p>
                  <p className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
                    {card.detail}
                  </p>
                </div>
              ))
            )}
          </div>
        </section>

        <section className="of-panel" style={{ padding: 20 }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
            <div>
              <p className="of-eyebrow" style={{ color: '#0e7490' }}>
                Templates and Workshop widget
              </p>
              <h3 className="of-heading-md" style={{ marginTop: 4 }}>
                Create and save maps
              </h3>
              <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                Persist the current filtered view as a reusable map template backed by a real
                geospatial layer.
              </p>
            </div>
            <button
              type="button"
              className="of-btn of-btn-primary"
              onClick={() => void saveCurrentMapTemplate()}
              disabled={busy || !selectedLayer}
            >
              {busyAction === 'save-template' ? 'Saving…' : 'Save template'}
            </button>
          </div>

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 16 }}>
            <label style={{ display: 'block', fontSize: 13 }}>
              <div className="of-eyebrow" style={{ marginBottom: 6 }}>
                Template name
              </div>
              <input
                className="of-input"
                value={templateName}
                onChange={(e) => setTemplateName(e.target.value)}
                placeholder="Regional operations view"
              />
            </label>
            <label style={{ display: 'block', fontSize: 13 }}>
              <div className="of-eyebrow" style={{ marginBottom: 6 }}>
                Description
              </div>
              <input
                className="of-input"
                value={templateDescription}
                onChange={(e) => setTemplateDescription(e.target.value)}
                placeholder="Saved view for operations review"
              />
            </label>
          </div>

          <div style={{ display: 'grid', gap: 8, marginTop: 16 }}>
            {templateLayers.length === 0 ? (
              <p className="of-text-muted" style={{ fontSize: 13 }}>
                No saved map templates yet.
              </p>
            ) : (
              templateLayers.map((layer) => (
                <button
                  key={layer.id}
                  type="button"
                  onClick={() => void selectLayerById(layer.id)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 16,
                    background: 'var(--bg-panel-muted)',
                    border: '1px solid var(--border-default)',
                    borderRadius: 'var(--radius-md)',
                    cursor: 'pointer',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                    <div>
                      <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{layer.name}</div>
                      <div className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
                        {layer.features.length} features • {layer.geometry_type} • {layer.source_kind}
                      </div>
                    </div>
                    <span
                      className="of-chip"
                      style={{ textTransform: 'uppercase', letterSpacing: '0.18em', fontSize: 11, fontWeight: 600 }}
                    >
                      Template
                    </span>
                  </div>
                  <p className="of-text-muted" style={{ fontSize: 13, marginTop: 12 }}>
                    {layer.description}
                  </p>
                </button>
              ))
            )}
          </div>
        </section>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.05fr) minmax(0, 0.95fr)' }}>
        <HeatmapLayer tile={tile} queryResponse={queryResponse} clusterResponse={clusterResponse} />
        <RouteLayer routeResponse={routeResponse} />
      </div>
    </section>
  );
}

// ── Local form-field helpers ──

function ColorField({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <label style={{ display: 'block', fontSize: 13 }}>
      <div className="of-eyebrow" style={{ marginBottom: 6 }}>
        {label}
      </div>
      <input
        type="color"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        style={{ height: 44, width: '100%', border: '1px solid var(--border-default)', borderRadius: 'var(--radius-sm)', padding: 4 }}
      />
    </label>
  );
}

function RangeField({
  label,
  value,
  min,
  max,
  step,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (v: number) => void;
}) {
  return (
    <label style={{ display: 'block', fontSize: 13 }}>
      <div className="of-eyebrow" style={{ marginBottom: 6 }}>
        {label}
      </div>
      <input
        type="range"
        value={value}
        min={min}
        max={max}
        step={step}
        onChange={(e) => onChange(Number(e.target.value))}
        style={{ width: '100%' }}
      />
    </label>
  );
}

function NumberField({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  return (
    <label style={{ display: 'block', fontSize: 13 }}>
      <div className="of-eyebrow" style={{ marginBottom: 6 }}>
        {label}
      </div>
      <input
        type="number"
        className="of-input"
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
      />
    </label>
  );
}

function CheckboxField({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        background: 'var(--bg-panel-muted)',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-md)',
        padding: '10px 14px',
        fontSize: 13,
      }}
    >
      <span style={{ fontWeight: 500 }}>{label}</span>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
    </label>
  );
}
