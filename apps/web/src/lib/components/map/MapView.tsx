import { useCallback, useEffect, useRef } from 'react';
import type { GeoJSONSource, Map as MapLibreMap } from 'maplibre-gl';

import { MapLibreCanvas } from '@components/MapLibreCanvas';
import type {
  ClusterResponse,
  Coordinate,
  GeocodeResponse,
  Geometry,
  LayerDefinition,
  MapFeature,
  RouteResponse,
  SpatialQueryResponse,
  VectorTileResponse,
} from '@/lib/api/geospatial';

type GeoJsonGeometry =
  | { type: 'Point'; coordinates: [number, number] }
  | { type: 'LineString'; coordinates: [number, number][] }
  | { type: 'Polygon'; coordinates: [number, number][][] };

type GeoJsonFeature = {
  type: 'Feature';
  properties: Record<string, unknown>;
  geometry: GeoJsonGeometry;
};

type GeoJsonCollection = {
  type: 'FeatureCollection';
  features: GeoJsonFeature[];
};

const EMPTY_COLLECTION: GeoJsonCollection = { type: 'FeatureCollection', features: [] };

function toLngLat(point: Coordinate): [number, number] {
  return [point.lon, point.lat];
}

function toGeoJsonGeometry(geometry: Geometry): GeoJsonGeometry {
  if (geometry.type === 'point') {
    return { type: 'Point', coordinates: toLngLat(geometry.coordinates) };
  }
  if (geometry.type === 'line_string') {
    return { type: 'LineString', coordinates: geometry.coordinates.map(toLngLat) };
  }
  return { type: 'Polygon', coordinates: [geometry.coordinates.map(toLngLat)] };
}

function toFeatureCollection(
  features: MapFeature[],
  style?: LayerDefinition['style'],
): GeoJsonCollection {
  return {
    type: 'FeatureCollection',
    features: features.map((feature) => ({
      type: 'Feature',
      properties: {
        label: feature.label,
        ...(feature.properties ?? {}),
        color: style?.color,
        radius: style?.radius,
        lineWidth: style?.line_width,
      },
      geometry: toGeoJsonGeometry(feature.geometry),
    })),
  };
}

function toClusterCollection(response: ClusterResponse | null): GeoJsonCollection {
  return {
    type: 'FeatureCollection',
    features:
      response?.clusters.map((cluster) => ({
        type: 'Feature' as const,
        properties: { cluster_id: cluster.cluster_id, member_count: cluster.member_count },
        geometry: { type: 'Point' as const, coordinates: toLngLat(cluster.centroid) },
      })) ?? [],
  };
}

function toRouteCollection(response: RouteResponse | null): GeoJsonCollection {
  if (!response) return EMPTY_COLLECTION;
  return {
    type: 'FeatureCollection',
    features: [
      {
        type: 'Feature',
        properties: { mode: response.mode },
        geometry: {
          type: 'LineString',
          coordinates: response.polyline.map(toLngLat),
        },
      },
    ],
  };
}

function toSearchCollection(results: GeocodeResponse[]): GeoJsonCollection {
  return {
    type: 'FeatureCollection',
    features: results.map((result) => ({
      type: 'Feature',
      properties: { label: result.address, source: result.source },
      geometry: { type: 'Point', coordinates: toLngLat(result.coordinate) },
    })),
  };
}

function collectCoordinates(features: MapFeature[]): Coordinate[] {
  return features.flatMap((feature) => {
    if (feature.geometry.type === 'point') return [feature.geometry.coordinates];
    return feature.geometry.coordinates;
  });
}

function setupSourcesAndLayers(map: MapLibreMap) {
  const sources = ['base-source', 'query-source', 'cluster-source', 'route-source', 'search-source'];
  for (const id of sources) {
    if (!map.getSource(id)) {
      map.addSource(id, { type: 'geojson', data: EMPTY_COLLECTION });
    }
  }

  map.addLayer({
    id: 'heatmap-layer',
    type: 'heatmap',
    source: 'base-source',
    filter: ['==', ['geometry-type'], 'Point'],
    paint: {
      'heatmap-intensity': 0.9,
      'heatmap-radius': 28,
      'heatmap-opacity': 0.72,
      'heatmap-color': [
        'interpolate',
        ['linear'],
        ['heatmap-density'],
        0,
        'rgba(34, 211, 238, 0)',
        0.3,
        'rgba(14, 165, 233, 0.35)',
        0.6,
        'rgba(8, 145, 178, 0.58)',
        1,
        'rgba(6, 95, 70, 0.82)',
      ],
    },
  });

  map.addLayer({
    id: 'polygon-layer',
    type: 'fill',
    source: 'base-source',
    filter: ['==', ['geometry-type'], 'Polygon'],
    paint: {
      'fill-color': ['coalesce', ['get', 'color'], '#1d4ed8'],
      'fill-opacity': 0.26,
    },
  });

  map.addLayer({
    id: 'line-layer',
    type: 'line',
    source: 'base-source',
    filter: ['==', ['geometry-type'], 'LineString'],
    paint: {
      'line-color': ['coalesce', ['get', 'color'], '#0f766e'],
      'line-width': ['coalesce', ['get', 'lineWidth'], 3],
      'line-opacity': 0.86,
    },
  });

  map.addLayer({
    id: 'point-layer',
    type: 'circle',
    source: 'base-source',
    filter: ['==', ['geometry-type'], 'Point'],
    paint: {
      'circle-color': ['coalesce', ['get', 'color'], '#d97706'],
      'circle-radius': ['coalesce', ['get', 'radius'], 8],
      'circle-stroke-width': 2,
      'circle-stroke-color': '#f8fafc',
      'circle-opacity': 0.9,
    },
  });

  map.addLayer({
    id: 'query-layer',
    type: 'circle',
    source: 'query-source',
    paint: {
      'circle-color': '#06b6d4',
      'circle-radius': 9,
      'circle-stroke-width': 2,
      'circle-stroke-color': '#083344',
    },
  });

  map.addLayer({
    id: 'cluster-layer',
    type: 'circle',
    source: 'cluster-source',
    paint: {
      'circle-color': '#0f766e',
      'circle-radius': ['+', 10, ['coalesce', ['get', 'member_count'], 1]],
      'circle-opacity': 0.82,
    },
  });

  map.addLayer({
    id: 'route-layer',
    type: 'line',
    source: 'route-source',
    paint: {
      'line-color': '#1e293b',
      'line-width': 4,
      'line-dasharray': [2, 1],
    },
  });

  map.addLayer({
    id: 'search-layer',
    type: 'circle',
    source: 'search-source',
    paint: {
      'circle-color': '#ec4899',
      'circle-radius': 7,
      'circle-stroke-color': '#831843',
      'circle-stroke-width': 2,
    },
  });
}

function setSourceData(map: MapLibreMap, id: string, data: GeoJsonCollection) {
  const source = map.getSource(id) as GeoJSONSource | undefined;
  source?.setData(data);
}

interface MapViewProps {
  layer: LayerDefinition | null;
  tile?: VectorTileResponse | null;
  queryResponse?: SpatialQueryResponse | null;
  clusterResponse?: ClusterResponse | null;
  routeResponse?: RouteResponse | null;
  searchResults?: GeocodeResponse[];
}

export function MapView({
  layer,
  tile,
  queryResponse = null,
  clusterResponse = null,
  routeResponse = null,
  searchResults = [],
}: MapViewProps) {
  const mapRef = useRef<MapLibreMap | null>(null);
  const styleReadyRef = useRef(false);
  const lastFittedLayerIdRef = useRef<string>('');

  const handleMapLoad = useCallback((map: MapLibreMap) => {
    mapRef.current = map;
    setupSourcesAndLayers(map);
    styleReadyRef.current = true;
    syncFromProps();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function syncFromProps() {
    const map = mapRef.current;
    if (!map || !styleReadyRef.current) return;

    setSourceData(map, 'base-source', toFeatureCollection(layer?.features ?? [], layer?.style));
    setSourceData(map, 'query-source', toFeatureCollection(queryResponse?.matched_features ?? []));
    setSourceData(map, 'cluster-source', toClusterCollection(clusterResponse));
    setSourceData(map, 'route-source', toRouteCollection(routeResponse));
    setSourceData(map, 'search-source', toSearchCollection(searchResults));

    // Fit only on the first render of a new layer (matches the Svelte fittedToLayer flag).
    const layerId = layer?.id ?? '';
    if (layer && layerId && lastFittedLayerIdRef.current !== layerId && layer.features.length > 0) {
      const points = collectCoordinates(layer.features);
      if (points.length > 0) {
        if (points.length === 1) {
          map.flyTo({ center: [points[0].lon, points[0].lat], zoom: 6 });
        } else {
          const lats = points.map((p) => p.lat);
          const lons = points.map((p) => p.lon);
          map.fitBounds(
            [
              [Math.min(...lons), Math.min(...lats)],
              [Math.max(...lons), Math.max(...lats)],
            ],
            { padding: 40, duration: 0 },
          );
        }
        lastFittedLayerIdRef.current = layerId;
      }
    }
  }

  useEffect(() => {
    syncFromProps();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layer, queryResponse, clusterResponse, routeResponse, searchResults]);

  return (
    <section className="of-panel" style={{ padding: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 12, padding: '0 4px' }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#0e7490' }}>
            MapLibre GL
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 4 }}>
            Live layer canvas
          </h3>
        </div>
        {tile && (
          <span
            className="of-chip"
            style={{ textTransform: 'uppercase', letterSpacing: '0.18em', fontSize: 11, fontWeight: 600 }}
          >
            {tile.format} • {tile.feature_count} features
          </span>
        )}
      </div>
      <MapLibreCanvas height="28rem" onMapLoad={handleMapLoad} className="lineage-canvas" />
    </section>
  );
}
