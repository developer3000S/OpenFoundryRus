import type { StyleSpecification } from 'maplibre-gl';

import type { AppWidget } from '@/lib/api/apps';
import type { ObjectInstance } from '@/lib/api/ontology';
import type { QueryResult } from '@/lib/api/queries';

export type WorkshopMapGeometryKind = 'auto' | 'point' | 'line' | 'polygon';
export type WorkshopMapLayerSource = 'binding' | 'object_set' | 'object_type' | 'geospatial_tile';
export type WorkshopMapLoadingMode = 'eager' | 'viewport_tiles';

export interface WorkshopMapVariableLike {
  id: string;
  kind?: string;
  name: string;
  object_type_id?: string;
  static_filter?: { property_name: string; operator?: string; value?: unknown };
  static_filters?: Array<{ property_name: string; operator?: string; value?: unknown }>;
}

export interface WorkshopMapLayerConfig {
  id: string;
  title: string;
  source: WorkshopMapLayerSource;
  loading_mode: WorkshopMapLoadingMode;
  source_variable_id: string;
  object_type_id: string;
  tile_layer_id: string;
  tile_page_size: number;
  tile_simplify_tolerance: number;
  geometry_type: WorkshopMapGeometryKind;
  latitude_field: string;
  longitude_field: string;
  geometry_field: string;
  label_field: string;
  color: string;
  visible: boolean;
  locked: boolean;
  filter_field: string;
  filter_value: string;
  radius: number;
  line_width: number;
  fill_opacity: number;
  cluster_enabled: boolean;
  cluster_radius: number;
  cluster_max_zoom: number;
  cluster_color: string;
  features?: Array<{ id?: string; label?: string; geometry?: unknown; properties?: unknown }>;
}

export type WorkshopMapOverlaySource = 'geojson_url' | 'mvt_url' | 'saved_map_layer' | 'raster_url';

export interface WorkshopMapOverlayLayerConfig {
  id: string;
  title: string;
  source: WorkshopMapOverlaySource;
  url: string;
  resource_id: string;
  source_layer: string;
  geometry_type: WorkshopMapGeometryKind;
  color: string;
  visible: boolean;
  opacity: number;
  radius: number;
  line_width: number;
  fill_opacity: number;
  min_zoom: number;
  max_zoom: number;
  attribution: string;
  data?: WorkshopMapFeatureCollection | null;
}

export type WorkshopMapGeometry =
  | { type: 'Point'; coordinates: [number, number] }
  | { type: 'MultiPoint'; coordinates: [number, number][] }
  | { type: 'LineString'; coordinates: [number, number][] }
  | { type: 'MultiLineString'; coordinates: [number, number][][] }
  | { type: 'Polygon'; coordinates: [number, number][][] }
  | { type: 'MultiPolygon'; coordinates: [number, number][][][] };

export interface WorkshopMapFeature {
  type: 'Feature';
  properties: Record<string, unknown>;
  geometry: WorkshopMapGeometry;
}

export interface WorkshopMapFeatureCollection {
  type: 'FeatureCollection';
  features: WorkshopMapFeature[];
}

export const EMPTY_FEATURE_COLLECTION: WorkshopMapFeatureCollection = {
  type: 'FeatureCollection',
  features: [],
};

const DEFAULT_POINT_COLOR = '#2d72d2';
const DEFAULT_LINE_COLOR = '#c2410c';
const DEFAULT_POLYGON_COLOR = '#15803d';

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

function readString(value: unknown, fallback = '') {
  return typeof value === 'string' ? value : fallback;
}

function readNumber(value: unknown, fallback: number) {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

function readBoolean(value: unknown, fallback: boolean) {
  return typeof value === 'boolean' ? value : fallback;
}

function normalizeGeometryKind(value: unknown): WorkshopMapGeometryKind {
  if (value === 'point' || value === 'line' || value === 'polygon' || value === 'auto') return value;
  if (value === 'circle') return 'point';
  if (value === 'shape' || value === 'fill') return 'polygon';
  return 'auto';
}

function defaultColor(kind: WorkshopMapGeometryKind) {
  if (kind === 'line') return DEFAULT_LINE_COLOR;
  if (kind === 'polygon') return DEFAULT_POLYGON_COLOR;
  return DEFAULT_POINT_COLOR;
}

export function readMapLayerConfigs(props: Record<string, unknown> | null | undefined): WorkshopMapLayerConfig[] {
  const rawLayers = Array.isArray(props?.layers) ? props.layers : [];
  const layers = rawLayers
    .filter(isRecord)
    .map((entry, index) => normalizeLayerConfig(entry, index));
  if (layers.length > 0) return layers;

  const geometryType = normalizeGeometryKind(props?.geometry_type ?? props?.layer_type ?? 'point');
  return [
    {
      id: 'default',
      title: readString(props?.layer_title, 'Objects'),
      source: 'binding',
      loading_mode: 'eager',
      source_variable_id: '',
      object_type_id: '',
      tile_layer_id: '',
      tile_page_size: 500,
      tile_simplify_tolerance: 0,
      geometry_type: geometryType,
      latitude_field: readString(props?.latitude_field, 'lat'),
      longitude_field: readString(props?.longitude_field, 'lon'),
      geometry_field: readString(props?.geometry_field, ''),
      label_field: readString(props?.label_field, 'label'),
      color: readString(props?.color, defaultColor(geometryType)),
      visible: true,
      locked: readBoolean(props?.locked, false),
      filter_field: '',
      filter_value: '',
      radius: readNumber(props?.radius, 6),
      line_width: readNumber(props?.line_width, 3),
      fill_opacity: readNumber(props?.fill_opacity, 0.22),
      cluster_enabled: readBoolean(props?.cluster_enabled, false),
      cluster_radius: readNumber(props?.cluster_radius, 64),
      cluster_max_zoom: readNumber(props?.cluster_max_zoom, 10),
      cluster_color: readString(props?.cluster_color, readString(props?.color, defaultColor(geometryType))),
      features: [],
    },
  ];
}

function normalizeLayerConfig(entry: Record<string, unknown>, index: number): WorkshopMapLayerConfig {
  const geometryType = normalizeGeometryKind(entry.geometry_type ?? entry.layer_type ?? entry.kind);
  const source = normalizeLayerSource(entry.source ?? entry.source_kind ?? entry.loading_mode);
  const loadingMode = source === 'geospatial_tile'
    ? 'viewport_tiles'
    : normalizeLoadingMode(entry.loading_mode ?? entry.loadingMode);
  return {
    id: readString(entry.id, `layer-${index + 1}`),
    title: readString(entry.title, `Layer ${index + 1}`),
    source,
    loading_mode: loadingMode,
    source_variable_id: readString(entry.source_variable_id),
    object_type_id: readString(entry.object_type_id),
    tile_layer_id: readString(entry.tile_layer_id, readString(entry.layer_id, readString(entry.resource_id))),
    tile_page_size: readNumber(entry.tile_page_size, readNumber(entry.page_size, 500)),
    tile_simplify_tolerance: readNumber(entry.tile_simplify_tolerance, readNumber(entry.simplify_tolerance, 0)),
    geometry_type: geometryType,
    latitude_field: readString(entry.latitude_field, readString(entry.lat_field, 'lat')),
    longitude_field: readString(entry.longitude_field, readString(entry.lon_field, 'lon')),
    geometry_field: readString(entry.geometry_field, readString(entry.geojson_field)),
    label_field: readString(entry.label_field, readString(entry.name_field, 'label')),
    color: readString(entry.color, defaultColor(geometryType)),
    visible: readBoolean(entry.visible, true),
    locked: readBoolean(entry.locked, false),
    filter_field: readString(entry.filter_field),
    filter_value: readString(entry.filter_value),
    radius: readNumber(entry.radius, 6),
    line_width: readNumber(entry.line_width, 3),
    fill_opacity: readNumber(entry.fill_opacity, 0.22),
    cluster_enabled: readBoolean(entry.cluster_enabled, readBoolean(entry.enable_clusters, false)),
    cluster_radius: readNumber(entry.cluster_radius, 64),
    cluster_max_zoom: readNumber(entry.cluster_max_zoom, readNumber(entry.clusterMaxZoom, 10)),
    cluster_color: readString(entry.cluster_color, readString(entry.color, defaultColor(geometryType))),
    features: Array.isArray(entry.features) ? entry.features.filter(isRecord) : [],
  };
}

function normalizeLayerSource(value: unknown): WorkshopMapLayerSource {
  if (value === 'object_set') return 'object_set';
  if (value === 'object_type') return 'object_type';
  if (
    value === 'geospatial_tile' ||
    value === 'viewport_tile' ||
    value === 'viewport_tiles' ||
    value === 'vector_tile' ||
    value === 'saved_map_layer'
  ) {
    return 'geospatial_tile';
  }
  return 'binding';
}

function normalizeLoadingMode(value: unknown): WorkshopMapLoadingMode {
  return value === 'viewport_tiles' || value === 'tile' || value === 'tiles' ? 'viewport_tiles' : 'eager';
}

export function readMapOverlayConfigs(props: Record<string, unknown> | null | undefined): WorkshopMapOverlayLayerConfig[] {
  const rawOverlays = Array.isArray(props?.overlay_layers)
    ? props.overlay_layers
    : Array.isArray(props?.overlays)
      ? props.overlays
      : [];
  return rawOverlays.filter(isRecord).map((entry, index) => normalizeOverlayConfig(entry, index));
}

function normalizeOverlayConfig(entry: Record<string, unknown>, index: number): WorkshopMapOverlayLayerConfig {
  const source = normalizeOverlaySource(entry.source ?? entry.source_kind ?? entry.layer_type);
  const geometryType = normalizeGeometryKind(entry.geometry_type ?? entry.geometry_kind ?? entry.kind);
  return {
    id: readString(entry.id, `overlay-${index + 1}`),
    title: readString(entry.title, readString(entry.name, `Overlay ${index + 1}`)),
    source,
    url: readString(entry.url, readString(entry.geojson_url, readString(entry.mvt_url, readString(entry.tile_url, readString(entry.tile_url_template))))),
    resource_id: readString(entry.resource_id, readString(entry.layer_id, readString(entry.saved_layer_id))),
    source_layer: readString(entry.source_layer, readString(entry.sourceLayer)),
    geometry_type: geometryType,
    color: readString(entry.color, defaultColor(geometryType)),
    visible: readBoolean(entry.visible, true),
    opacity: readNumber(entry.opacity, 0.88),
    radius: readNumber(entry.radius, 5),
    line_width: readNumber(entry.line_width, 2),
    fill_opacity: readNumber(entry.fill_opacity, 0.16),
    min_zoom: readNumber(entry.min_zoom, readNumber(entry.minzoom, 0)),
    max_zoom: readNumber(entry.max_zoom, readNumber(entry.maxzoom, 22)),
    attribution: readString(entry.attribution),
    data: normalizeFeatureCollection(entry.data ?? entry.geojson),
  };
}

function normalizeOverlaySource(value: unknown): WorkshopMapOverlaySource {
  if (value === 'mvt_url' || value === 'mvt' || value === 'vector_tile' || value === 'vector_tiles') return 'mvt_url';
  if (value === 'saved_map_layer' || value === 'map_layer' || value === 'resource' || value === 'reference') return 'saved_map_layer';
  if (value === 'raster_url' || value === 'raster' || value === 'tile_url') return 'raster_url';
  return 'geojson_url';
}

export function normalizeSavedOverlayConfig(
  base: WorkshopMapOverlayLayerConfig,
  resource: Record<string, unknown>,
): WorkshopMapOverlayLayerConfig {
  const config = isRecord(resource.config) ? resource.config : {};
  const style = isRecord(resource.style) ? resource.style : {};
  const resourceSource = resource.source_kind ?? config.source_kind ?? resource.layer_type ?? config.layer_type;
  const url =
    base.url ||
    readString(resource.url) ||
    readString(config.url) ||
    readString(resource.geojson_url) ||
    readString(config.geojson_url) ||
    readString(resource.mvt_url) ||
    readString(config.mvt_url) ||
    readString(resource.tile_url) ||
    readString(config.tile_url) ||
    readString(resource.tile_url_template) ||
    readString(config.tile_url_template);
  const data =
    base.data ??
    normalizeFeatureCollection(resource.geojson ?? config.geojson ?? resource.data ?? config.data) ??
    featureCollectionFromSavedLayerResource(resource);
  const source = base.source === 'saved_map_layer' ? inferSavedOverlaySource(resourceSource, url, data) : base.source;
  return {
    ...base,
    title: base.title || readString(resource.name, readString(resource.title, base.id)),
    source,
    url,
    resource_id: base.resource_id || readString(resource.id),
    source_layer: base.source_layer || readString(resource.source_layer, readString(config.source_layer)),
    geometry_type: base.geometry_type === 'auto' ? normalizeGeometryKind(resource.geometry_type ?? config.geometry_type ?? 'auto') : base.geometry_type,
    color: base.color || readString(style.color, defaultColor(base.geometry_type)),
    opacity: base.opacity || readNumber(style.opacity, 0.88),
    radius: base.radius || readNumber(style.radius, 5),
    line_width: base.line_width || readNumber(style.line_width, 2),
    fill_opacity: base.fill_opacity || readNumber(style.fill_opacity, 0.16),
    attribution: base.attribution || readString(resource.attribution, readString(config.attribution)),
    data,
  };
}

function inferSavedOverlaySource(source: unknown, url: string, data: WorkshopMapFeatureCollection | null): WorkshopMapOverlaySource {
  const normalized = normalizeOverlaySource(source);
  if (normalized !== 'geojson_url') return normalized;
  if (data) return 'geojson_url';
  if (/\.(mvt|pbf)(\?|$)/i.test(url) || /\{z\}\/\{x\}\/\{y\}/i.test(url)) return 'mvt_url';
  if (/\.(png|jpg|jpeg|webp)(\?|$)/i.test(url)) return 'raster_url';
  return 'geojson_url';
}

export function buildFeaturesFromQueryResult(result: QueryResult | null, layers: WorkshopMapLayerConfig[]): WorkshopMapFeatureCollection {
  if (!result) return EMPTY_FEATURE_COLLECTION;
  const rows = Array.isArray(result.rows) ? result.rows : [];
  const columns = Array.isArray(result.columns) ? result.columns.map((column) => column.name) : [];
  const features: WorkshopMapFeature[] = [];
  for (const layer of layers.filter((entry) => entry.source === 'binding')) {
    for (const row of rows) {
      const record = rowToRecord(columns, row as unknown[]);
      if (!recordMatchesLayerFilter(record, layer)) continue;
      const feature = featureFromRecord(record, layer);
      if (feature) features.push(feature);
    }
  }
  return { type: 'FeatureCollection', features };
}

export function buildFeaturesFromObjects(
  objects: ObjectInstance[],
  layer: WorkshopMapLayerConfig,
  variable: WorkshopMapVariableLike | null = null,
): WorkshopMapFeature[] {
  const features: WorkshopMapFeature[] = [];
  for (const object of objects) {
    const record = { id: object.id, object_id: object.id, object_type_id: object.object_type_id, ...(object.properties ?? {}) };
    if (!recordMatchesLayerFilter(record, layer)) continue;
    const feature = featureFromRecord(record, layer);
    if (!feature) continue;
    features.push({
      ...feature,
      properties: {
        ...feature.properties,
        __of_source: 'object',
        __of_object_id: object.id,
        __of_object_type_id: object.object_type_id,
        __of_object_json: JSON.stringify(object),
        __of_variable_id: variable?.id ?? '',
      },
    });
  }
  return features;
}

export function buildFeaturesFromGeospatialLayer(
  features: Array<{ id?: string; label?: string; geometry?: unknown; properties?: unknown }>,
  layer: WorkshopMapLayerConfig,
): WorkshopMapFeature[] {
  return features
    .map((feature): WorkshopMapFeature | null => {
      const geometry = savedLayerGeometryToGeoJSON(feature.geometry);
      if (!geometry) return null;
      const properties = isRecord(feature.properties) ? feature.properties : {};
      const label = stringifyLabel(feature.label) || stringifyLabel(properties.label) || stringifyLabel(feature.id) || layer.title;
      return {
        type: 'Feature',
        properties: {
          ...properties,
          id: feature.id,
          label,
          __of_source: 'geospatial_tile',
          __of_object_id: feature.id ?? '',
          __of_label: label,
          __of_layer_id: layer.id,
          __of_layer_title: layer.title,
          __of_tile_layer_id: layer.tile_layer_id,
          __of_color: layer.color,
          __of_radius: layer.radius,
          __of_line_width: layer.line_width,
          __of_fill_opacity: layer.fill_opacity,
          __of_locked: layer.locked,
          __of_cluster_enabled: layer.cluster_enabled,
          __of_cluster_radius: layer.cluster_radius,
          __of_cluster_max_zoom: layer.cluster_max_zoom,
          __of_cluster_color: layer.cluster_color,
        },
        geometry,
      };
    })
    .filter((feature): feature is WorkshopMapFeature => Boolean(feature));
}

export function buildFeaturesFromConfiguredLayers(layers: WorkshopMapLayerConfig[]): WorkshopMapFeature[] {
  return layers.flatMap((layer) => (layer.features ?? []).map((feature): WorkshopMapFeature | null => {
    const geometry = savedLayerGeometryToGeoJSON(feature.geometry);
    if (!geometry) return null;
    const properties = isRecord(feature.properties) ? feature.properties : {};
    const label = stringifyLabel(feature.label) || stringifyLabel(properties.label) || stringifyLabel(feature.id) || layer.title;
    return {
      type: 'Feature',
      properties: {
        ...properties,
        id: feature.id,
        label,
        __of_source: 'map_template',
        __of_object_id: feature.id ?? '',
        __of_label: label,
        __of_layer_id: layer.id,
        __of_layer_title: layer.title,
        __of_color: layer.color,
        __of_radius: layer.radius,
        __of_line_width: layer.line_width,
        __of_fill_opacity: layer.fill_opacity,
        __of_locked: layer.locked,
        __of_cluster_enabled: layer.cluster_enabled,
        __of_cluster_radius: layer.cluster_radius,
        __of_cluster_max_zoom: layer.cluster_max_zoom,
        __of_cluster_color: layer.cluster_color,
      },
      geometry,
    };
  }).filter((feature): feature is WorkshopMapFeature => Boolean(feature)));
}

function rowToRecord(columns: string[], row: unknown[]) {
  const record: Record<string, unknown> = {};
  columns.forEach((name, index) => {
    record[name] = row[index];
  });
  return record;
}

function recordMatchesLayerFilter(record: Record<string, unknown>, layer: WorkshopMapLayerConfig) {
  if (!layer.filter_field) return true;
  const value = record[layer.filter_field];
  if (!layer.filter_value) return value !== null && value !== undefined && String(value).trim() !== '';
  return String(value) === layer.filter_value;
}

function featureFromRecord(record: Record<string, unknown>, layer: WorkshopMapLayerConfig): WorkshopMapFeature | null {
  const geometry = geometryFromRecord(record, layer);
  if (!geometry) return null;
  const label = stringifyLabel(record[layer.label_field]) || stringifyLabel(record.name) || stringifyLabel(record.id) || layer.title;
  return {
    type: 'Feature',
    properties: {
      ...record,
      __of_label: label,
      __of_layer_id: layer.id,
      __of_layer_title: layer.title,
      __of_color: layer.color,
      __of_radius: layer.radius,
      __of_line_width: layer.line_width,
      __of_fill_opacity: layer.fill_opacity,
      __of_locked: layer.locked,
      __of_cluster_enabled: layer.cluster_enabled,
      __of_cluster_radius: layer.cluster_radius,
      __of_cluster_max_zoom: layer.cluster_max_zoom,
      __of_cluster_color: layer.cluster_color,
    },
    geometry,
  };
}

function stringifyLabel(value: unknown) {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return '';
}

function geometryFromRecord(record: Record<string, unknown>, layer: WorkshopMapLayerConfig): WorkshopMapGeometry | null {
  const rawGeometry = layer.geometry_field ? record[layer.geometry_field] : null;
  const parsedGeometry = parseGeometry(rawGeometry);
  const geometryType = layer.geometry_type;
  if (parsedGeometry) {
    if (geometryType === 'auto') return parsedGeometry;
    if (geometryType === 'point' && isPointGeometry(parsedGeometry)) return parsedGeometry;
    if (geometryType === 'line' && isLineGeometry(parsedGeometry)) return parsedGeometry;
    if (geometryType === 'polygon' && isPolygonGeometry(parsedGeometry)) return parsedGeometry;
  }
  if (geometryType === 'line' || geometryType === 'polygon') return null;
  const point = parsePoint(record[layer.latitude_field], record[layer.longitude_field]);
  return point ? { type: 'Point', coordinates: point } : null;
}

function parseGeometry(value: unknown): WorkshopMapGeometry | null {
  const parsed = parseJSONLike(value);
  if (!isRecord(parsed)) {
    const point = parseGeoPointLike(value);
    return point ? { type: 'Point', coordinates: point } : null;
  }
  if (parsed.type === 'Feature' && isRecord(parsed.geometry)) return normalizeGeometry(parsed.geometry);
  if (parsed.type === 'FeatureCollection' && Array.isArray(parsed.features)) {
    const first = parsed.features.find((feature) => isRecord(feature) && isRecord(feature.geometry));
    return isRecord(first) && isRecord(first.geometry) ? normalizeGeometry(first.geometry) : null;
  }
  return normalizeGeometry(parsed);
}

function parseJSONLike(value: unknown): unknown {
  if (typeof value !== 'string') return value;
  const trimmed = value.trim();
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return value;
  try {
    return JSON.parse(trimmed);
  } catch {
    return value;
  }
}

function normalizeGeometry(value: Record<string, unknown>): WorkshopMapGeometry | null {
  const type = value.type;
  if (type === 'Point' && isCoordinate(value.coordinates)) return { type, coordinates: value.coordinates };
  if (type === 'MultiPoint' && isCoordinateArray(value.coordinates)) return { type, coordinates: value.coordinates };
  if (type === 'LineString' && isCoordinateArray(value.coordinates)) return { type, coordinates: value.coordinates };
  if (type === 'MultiLineString' && isCoordinateArrayArray(value.coordinates)) return { type, coordinates: value.coordinates };
  if (type === 'Polygon' && isCoordinateArrayArray(value.coordinates)) return { type, coordinates: value.coordinates };
  if (type === 'MultiPolygon' && isCoordinateArrayArrayArray(value.coordinates)) return { type, coordinates: value.coordinates };
  return null;
}

function parsePoint(latValue: unknown, lonValue: unknown): [number, number] | null {
  const lat = readCoordinateNumber(latValue);
  const lon = readCoordinateNumber(lonValue);
  if (lat === null || lon === null) return null;
  if (lat < -90 || lat > 90 || lon < -180 || lon > 180) return null;
  return [lon, lat];
}

function parseGeoPointLike(value: unknown): [number, number] | null {
  const parsed = parseJSONLike(value);
  if (Array.isArray(parsed) && parsed.length >= 2) {
    const first = readCoordinateNumber(parsed[0]);
    const second = readCoordinateNumber(parsed[1]);
    if (first === null || second === null) return null;
    if (Math.abs(first) <= 90 && Math.abs(second) <= 180) return [second, first];
    if (Math.abs(first) <= 180 && Math.abs(second) <= 90) return [first, second];
  }
  if (isRecord(parsed)) {
    const lat = parsed.lat ?? parsed.latitude;
    const lon = parsed.lon ?? parsed.lng ?? parsed.longitude;
    return parsePoint(lat, lon);
  }
  if (typeof value === 'string') {
    const parts = value.split(',').map((part) => part.trim());
    if (parts.length === 2) return parsePoint(parts[0], parts[1]);
  }
  return null;
}

function readCoordinateNumber(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value.trim());
    if (Number.isFinite(parsed)) return parsed;
  }
  return null;
}

function isCoordinate(value: unknown): value is [number, number] {
  return Array.isArray(value) && value.length >= 2 && readCoordinateNumber(value[0]) !== null && readCoordinateNumber(value[1]) !== null;
}

function normalizeCoordinate(value: unknown): [number, number] | null {
  if (!isCoordinate(value)) return null;
  const lon = readCoordinateNumber(value[0]);
  const lat = readCoordinateNumber(value[1]);
  if (lon === null || lat === null || lat < -90 || lat > 90 || lon < -180 || lon > 180) return null;
  return [lon, lat];
}

function isCoordinateArray(value: unknown): value is [number, number][] {
  return Array.isArray(value) && value.map(normalizeCoordinate).every(Boolean);
}

function isCoordinateArrayArray(value: unknown): value is [number, number][][] {
  return Array.isArray(value) && value.every(isCoordinateArray);
}

function isCoordinateArrayArrayArray(value: unknown): value is [number, number][][][] {
  return Array.isArray(value) && value.every(isCoordinateArrayArray);
}

function isPointGeometry(geometry: WorkshopMapGeometry) {
  return geometry.type === 'Point' || geometry.type === 'MultiPoint';
}

function isLineGeometry(geometry: WorkshopMapGeometry) {
  return geometry.type === 'LineString' || geometry.type === 'MultiLineString';
}

function isPolygonGeometry(geometry: WorkshopMapGeometry) {
  return geometry.type === 'Polygon' || geometry.type === 'MultiPolygon';
}

function normalizeFeatureCollection(value: unknown): WorkshopMapFeatureCollection | null {
  const parsed = parseJSONLike(value);
  if (!isRecord(parsed) || parsed.type !== 'FeatureCollection' || !Array.isArray(parsed.features)) return null;
  const features = parsed.features
    .filter(isRecord)
    .map((feature) => {
      const geometry = isRecord(feature.geometry) ? normalizeGeometry(feature.geometry) : null;
      if (!geometry) return null;
      return {
        type: 'Feature' as const,
        properties: isRecord(feature.properties) ? feature.properties : {},
        geometry,
      };
    })
    .filter((feature): feature is WorkshopMapFeature => Boolean(feature));
  return { type: 'FeatureCollection', features };
}

function featureCollectionFromSavedLayerResource(resource: Record<string, unknown>): WorkshopMapFeatureCollection | null {
  const rawFeatures = Array.isArray(resource.features) ? resource.features : [];
  if (rawFeatures.length === 0) return null;
  const features = rawFeatures
    .filter(isRecord)
    .map((feature): WorkshopMapFeature | null => {
      const geometry = savedLayerGeometryToGeoJSON(feature.geometry);
      if (!geometry) return null;
      return {
        type: 'Feature' as const,
        properties: {
          ...(isRecord(feature.properties) ? feature.properties : {}),
          id: feature.id,
          label: feature.label,
        },
        geometry,
      };
    })
    .filter((feature): feature is WorkshopMapFeature => Boolean(feature));
  return features.length > 0 ? { type: 'FeatureCollection', features } : null;
}

function savedLayerGeometryToGeoJSON(value: unknown): WorkshopMapGeometry | null {
  if (!isRecord(value)) return null;
  if (typeof value.type === 'string') {
    const type = value.type.toLowerCase();
    if (type === 'point') {
      const point = pointFromCoordinateRecord(value.coordinates);
      return point ? { type: 'Point', coordinates: point } : null;
    }
    if ((type === 'line_string' || type === 'linestring') && Array.isArray(value.coordinates)) {
      const points = value.coordinates.map(pointFromCoordinateRecord).filter((point): point is [number, number] => Boolean(point));
      return points.length > 1 ? { type: 'LineString', coordinates: points } : null;
    }
    if (type === 'polygon' && Array.isArray(value.coordinates)) {
      const ring = value.coordinates.map(pointFromCoordinateRecord).filter((point): point is [number, number] => Boolean(point));
      return ring.length > 2 ? { type: 'Polygon', coordinates: [ring] } : null;
    }
  }
  return normalizeGeometry(value);
}

function pointFromCoordinateRecord(value: unknown): [number, number] | null {
  if (!isRecord(value)) return normalizeCoordinate(value);
  return parsePoint(value.lat ?? value.latitude, value.lon ?? value.lng ?? value.longitude);
}

export function collectFeatureBounds(collection: WorkshopMapFeatureCollection): [[number, number], [number, number]] | null {
  const points = collection.features.flatMap((feature) => flattenGeometryCoordinates(feature.geometry));
  if (points.length === 0) return null;
  let minLon = Number.POSITIVE_INFINITY;
  let minLat = Number.POSITIVE_INFINITY;
  let maxLon = Number.NEGATIVE_INFINITY;
  let maxLat = Number.NEGATIVE_INFINITY;
  for (const [lon, lat] of points) {
    minLon = Math.min(minLon, lon);
    minLat = Math.min(minLat, lat);
    maxLon = Math.max(maxLon, lon);
    maxLat = Math.max(maxLat, lat);
  }
  if (!Number.isFinite(minLon) || !Number.isFinite(minLat) || !Number.isFinite(maxLon) || !Number.isFinite(maxLat)) return null;
  if (minLon === maxLon && minLat === maxLat) {
    return [[minLon - 0.02, minLat - 0.02], [maxLon + 0.02, maxLat + 0.02]];
  }
  return [[minLon, minLat], [maxLon, maxLat]];
}

function flattenGeometryCoordinates(geometry: WorkshopMapGeometry): [number, number][] {
  switch (geometry.type) {
    case 'Point':
      return [geometry.coordinates];
    case 'MultiPoint':
    case 'LineString':
      return geometry.coordinates;
    case 'MultiLineString':
    case 'Polygon':
      return geometry.coordinates.flat();
    case 'MultiPolygon':
      return geometry.coordinates.flat(2);
  }
}

export function createWorkshopMapStyle(
  props: Record<string, unknown> | null | undefined,
  overlays: WorkshopMapOverlayLayerConfig[] = [],
): StyleSpecification {
  const baseLayerKind = readString(props?.base_layer_kind, readString(props?.base_layer, 'blank'));
  const baseTileURL = readString(props?.base_tile_url, 'https://tile.openstreetmap.org/{z}/{x}/{y}.png');
  const useRasterBase = baseLayerKind !== 'blank' && baseLayerKind !== 'none' && baseTileURL.trim().length > 0;
  const sources: Record<string, unknown> = useRasterBase
    ? {
        'of-base-raster': {
          type: 'raster',
          tiles: [baseTileURL],
          tileSize: 256,
          attribution: 'OpenStreetMap',
        },
      }
    : {};
  const layers: unknown[] = [
    {
      id: 'of-base-background',
      type: 'background',
      paint: { 'background-color': readString(props?.base_color, '#edf2f7') },
    },
    ...(useRasterBase
      ? [
          {
            id: 'of-base-raster',
            type: 'raster' as const,
            source: 'of-base-raster',
            paint: { 'raster-opacity': readNumber(props?.base_tile_opacity, 0.82) },
          },
        ]
      : []),
  ];
  for (const overlay of overlays.filter((entry) => entry.visible)) {
    appendOverlayToStyle(sources, layers, overlay);
  }
  return {
    version: 8,
    sources: sources as never,
    layers: layers as never,
  };
}

export interface WorkshopMapTemplateRenderLike {
  template_id?: string;
  template_name?: string;
  widget_props?: Record<string, unknown>;
}

export function mapTemplateIDFromProps(props: Record<string, unknown> | null | undefined) {
  return readString(props?.map_template_id, readString(props?.template_id)).trim();
}

export function buildMapTemplateRenderRequest(
  props: Record<string, unknown> | null | undefined,
  variables: WorkshopMapVariableLike[] = [],
) {
  const parameterValues = readTemplateParameterValues(props);
  const variableMappings = readTemplateVariableMappings(props);
  const variableByName = new Map(variables.map((variable) => [variable.name, variable.id]));
  for (const [parameter, variableId] of Object.entries(variableMappings)) {
    variableMappings[parameter] = variableByName.get(variableId) ?? variableId;
  }
  return {
    parameter_values: parameterValues,
    variable_mappings: variableMappings,
  };
}

export function mergeMapTemplateWidgetProps(
  props: Record<string, unknown> | null | undefined,
  render: WorkshopMapTemplateRenderLike | null,
) {
  const base = isRecord(props) ? props : {};
  const rendered = isRecord(render?.widget_props) ? render.widget_props : {};
  const merged: Record<string, unknown> = { ...rendered, ...base };
  if (!Array.isArray(base.layers) && Array.isArray(rendered.layers)) merged.layers = rendered.layers;
  if (!Array.isArray(base.overlay_layers) && Array.isArray(rendered.overlay_layers)) merged.overlay_layers = rendered.overlay_layers;
  if (!Array.isArray(base.overlays) && Array.isArray(rendered.overlay_layers)) merged.overlays = rendered.overlay_layers;
  return merged;
}

function readTemplateParameterValues(props: Record<string, unknown> | null | undefined) {
  const source = props?.template_parameter_values ?? props?.parameter_values;
  if (!isRecord(source)) return {};
  return { ...source };
}

function readTemplateVariableMappings(props: Record<string, unknown> | null | undefined) {
  const source = props?.template_parameter_mappings ?? props?.parameter_mappings ?? props?.variable_mappings;
  if (isRecord(source)) {
    return Object.fromEntries(Object.entries(source).map(([key, value]) => [key, readString(value)]).filter(([, value]) => value.length > 0)) as Record<string, string>;
  }
  if (!Array.isArray(source)) return {};
  const out: Record<string, string> = {};
  for (const entry of source.filter(isRecord)) {
    const parameter = readString(entry.parameter_id, readString(entry.parameter_name, readString(entry.id, readString(entry.name))));
    const variable = readString(entry.variable_id, readString(entry.source_variable_id, readString(entry.value)));
    if (parameter && variable) out[parameter] = variable;
  }
  return out;
}

function appendOverlayToStyle(sources: Record<string, unknown>, layers: unknown[], overlay: WorkshopMapOverlayLayerConfig) {
  const sourceId = `of-overlay-source-${safeMapId(overlay.id)}`;
  if (overlay.source === 'raster_url') {
    if (!overlay.url) return;
    sources[sourceId] = {
      type: 'raster',
      tiles: [overlay.url],
      tileSize: 256,
      ...(overlay.attribution ? { attribution: overlay.attribution } : {}),
    };
    layers.push({
      id: `of-overlay-raster-${safeMapId(overlay.id)}`,
      type: 'raster',
      source: sourceId,
      minzoom: overlay.min_zoom,
      maxzoom: overlay.max_zoom,
      paint: { 'raster-opacity': overlay.opacity },
    });
    return;
  }
  if (overlay.source === 'mvt_url') {
    if (!overlay.url) return;
    sources[sourceId] = {
      type: 'vector',
      tiles: [overlay.url],
      minzoom: overlay.min_zoom,
      maxzoom: overlay.max_zoom,
      ...(overlay.attribution ? { attribution: overlay.attribution } : {}),
    };
    appendVectorOverlayLayers(layers, overlay, sourceId);
    return;
  }
  const data = overlay.data ?? overlay.url;
  if (!data) return;
  sources[sourceId] = { type: 'geojson', data };
  appendGeoJSONOverlayLayers(layers, overlay, sourceId);
}

function appendGeoJSONOverlayLayers(layers: unknown[], overlay: WorkshopMapOverlayLayerConfig, sourceId: string) {
  const id = safeMapId(overlay.id);
  if (overlay.geometry_type === 'auto' || overlay.geometry_type === 'polygon') {
    layers.push({
      id: `of-overlay-fill-${id}`,
      type: 'fill',
      source: sourceId,
      filter: ['any', ['==', ['geometry-type'], 'Polygon'], ['==', ['geometry-type'], 'MultiPolygon']],
      paint: {
        'fill-color': overlay.color,
        'fill-opacity': overlay.fill_opacity,
        'fill-outline-color': overlay.color,
      },
    });
  }
  if (overlay.geometry_type === 'auto' || overlay.geometry_type === 'line') {
    layers.push({
      id: `of-overlay-line-${id}`,
      type: 'line',
      source: sourceId,
      filter: ['any', ['==', ['geometry-type'], 'LineString'], ['==', ['geometry-type'], 'MultiLineString']],
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: {
        'line-color': overlay.color,
        'line-width': overlay.line_width,
        'line-opacity': overlay.opacity,
      },
    });
  }
  if (overlay.geometry_type === 'auto' || overlay.geometry_type === 'point') {
    layers.push({
      id: `of-overlay-circle-${id}`,
      type: 'circle',
      source: sourceId,
      filter: ['any', ['==', ['geometry-type'], 'Point'], ['==', ['geometry-type'], 'MultiPoint']],
      paint: {
        'circle-radius': overlay.radius,
        'circle-color': overlay.color,
        'circle-opacity': overlay.opacity,
        'circle-stroke-color': '#ffffff',
        'circle-stroke-width': 1,
      },
    });
  }
}

function appendVectorOverlayLayers(layers: unknown[], overlay: WorkshopMapOverlayLayerConfig, sourceId: string) {
  const id = safeMapId(overlay.id);
  const sourceLayer = overlay.source_layer || overlay.id || 'default';
  if (overlay.geometry_type === 'polygon') {
    layers.push({
      id: `of-overlay-vector-fill-${id}`,
      type: 'fill',
      source: sourceId,
      'source-layer': sourceLayer,
      minzoom: overlay.min_zoom,
      maxzoom: overlay.max_zoom,
      paint: {
        'fill-color': overlay.color,
        'fill-opacity': overlay.fill_opacity,
        'fill-outline-color': overlay.color,
      },
    });
    return;
  }
  if (overlay.geometry_type === 'line') {
    layers.push({
      id: `of-overlay-vector-line-${id}`,
      type: 'line',
      source: sourceId,
      'source-layer': sourceLayer,
      minzoom: overlay.min_zoom,
      maxzoom: overlay.max_zoom,
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: {
        'line-color': overlay.color,
        'line-width': overlay.line_width,
        'line-opacity': overlay.opacity,
      },
    });
    return;
  }
  layers.push({
    id: `of-overlay-vector-circle-${id}`,
    type: 'circle',
    source: sourceId,
    'source-layer': sourceLayer,
    minzoom: overlay.min_zoom,
    maxzoom: overlay.max_zoom,
    paint: {
      'circle-radius': overlay.radius,
      'circle-color': overlay.color,
      'circle-opacity': overlay.opacity,
      'circle-stroke-color': '#ffffff',
      'circle-stroke-width': 1,
    },
  });
}

function safeMapId(value: string) {
  return value.replace(/[^a-zA-Z0-9_-]/g, '-');
}

export function makeDefaultMapWidget(): AppWidget {
  return {
    id: `map_${Date.now().toString(36)}`,
    widget_type: 'map',
    title: 'Map 1',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 3 },
    props: {
      latitude_field: 'lat',
      longitude_field: 'lon',
      label_field: 'label',
      zoom: 11,
      base_layer_kind: 'blank',
      layers: [
        {
          id: 'points',
          title: 'Objects',
          source: 'binding',
          loading_mode: 'eager',
          tile_layer_id: '',
          tile_page_size: 500,
          tile_simplify_tolerance: 0,
          geometry_type: 'point',
          latitude_field: 'lat',
          longitude_field: 'lon',
          label_field: 'label',
          color: DEFAULT_POINT_COLOR,
          visible: true,
        },
      ],
    },
    binding: null,
    events: [],
    children: [],
  };
}
