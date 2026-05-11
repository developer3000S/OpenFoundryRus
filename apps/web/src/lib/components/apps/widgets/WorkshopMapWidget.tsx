import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react';
import type { Map as MapLibreMap, MapGeoJSONFeature, MapLayerMouseEvent, MapMouseEvent } from 'maplibre-gl';

import type { AppWidget } from '@/lib/api/apps';
import { getLayer, getViewportTileFeatures, renderMapTemplate, type RenderMapTemplateResponse } from '@/lib/api/geospatial';
import { type ObjectInstance } from '@/lib/api/ontology';
import type { QueryResult } from '@/lib/api/queries';
import { MapLibreCanvas } from '@/lib/components/MapLibreCanvas';

import {
  type WorkshopVariableEngineResult,
  type WorkshopVariableLike,
} from './workshopVariables';
import { executeWorkshopObjectSet, type WorkshopObjectSetExecutionResult } from './workshopObjectSets';
import {
  buildFeaturesFromObjects,
  buildFeaturesFromGeospatialLayer,
  buildFeaturesFromConfiguredLayers,
  buildFeaturesFromQueryResult,
  buildFeaturesFromLinkedEdges,
  buildMapTemplateRenderRequest,
  collectFeatureBounds,
  createWorkshopMapStyle,
  isWorkshopMapLayerVisible,
  mapTemplateIDFromProps,
  mergeMapTemplateWidgetProps,
  normalizeSavedOverlayConfig,
  readMapLayerConfigs,
  readMapOverlayConfigs,
  type WorkshopMapFeature,
  type WorkshopMapFeatureCollection,
  type WorkshopMapGeometry,
  type WorkshopMapLayerConfig,
  type WorkshopMapOverlayLayerConfig,
} from './workshopMap';

interface WorkshopMapWidgetProps {
  widget: AppWidget;
  result?: QueryResult | null;
  variables?: WorkshopVariableLike[];
  variableEngine?: WorkshopVariableEngineResult | null;
  onSelectObject?: (variableId: string, object: ObjectInstance | null) => void;
  onSelectObjectSet?: (variableId: string, objects: ObjectInstance[]) => void;
  onShapeChange?: (variableId: string, shape: WorkshopMapFeatureCollection | null) => void;
  onSelectRecord?: (payload: Record<string, unknown>) => void | Promise<void>;
}

interface TooltipState {
  x: number;
  y: number;
  label: string;
  layer: string;
}

interface ClusterOptions {
  enabled: boolean;
  radius: number;
  maxZoom: number;
  color: string;
}

const SOURCE_ID = 'of-workshop-map-data';
const FILL_LAYER_ID = 'of-workshop-map-fills';
const LINE_LAYER_ID = 'of-workshop-map-lines';
const POINT_LAYER_ID = 'of-workshop-map-points';
const LABEL_LAYER_ID = 'of-workshop-map-labels';
const CLUSTER_SOURCE_ID = 'of-workshop-map-cluster-data';
const CLUSTER_CIRCLE_LAYER_ID = 'of-workshop-map-clusters';
const CLUSTER_COUNT_LAYER_ID = 'of-workshop-map-cluster-count';
const CLUSTER_POINT_LAYER_ID = 'of-workshop-map-cluster-points';
const CLUSTER_LABEL_LAYER_ID = 'of-workshop-map-cluster-labels';
const SHAPE_SOURCE_ID = 'of-workshop-map-drawn-shape';
const SHAPE_FILL_LAYER_ID = 'of-workshop-map-drawn-shape-fill';
const SHAPE_LINE_LAYER_ID = 'of-workshop-map-drawn-shape-line';

const MAP_LAYER_IDS = [FILL_LAYER_ID, LINE_LAYER_ID, POINT_LAYER_ID, LABEL_LAYER_ID];
const CLUSTER_LAYER_IDS = [CLUSTER_CIRCLE_LAYER_ID, CLUSTER_COUNT_LAYER_ID, CLUSTER_POINT_LAYER_ID, CLUSTER_LABEL_LAYER_ID];
const INTERACTIVE_LAYER_IDS = [...MAP_LAYER_IDS, ...CLUSTER_LAYER_IDS];
const EMPTY_DRAWN_SHAPE_COLLECTION: WorkshopMapFeatureCollection = { type: 'FeatureCollection', features: [] };

export function WorkshopMapWidget({
  widget,
  result = null,
  variables = [],
  variableEngine = null,
  onSelectObject,
  onSelectObjectSet,
  onShapeChange,
  onSelectRecord,
}: WorkshopMapWidgetProps) {
  const mapRef = useRef<MapLibreMap | null>(null);
  const drawStartRef = useRef<[number, number] | null>(null);
  const fittedRef = useRef(false);
  const tileRequestSeqRef = useRef(0);
  const [objectFeatures, setObjectFeatures] = useState<WorkshopMapFeature[]>([]);
  const [tileFeatures, setTileFeatures] = useState<WorkshopMapFeature[]>([]);
  const [resolvedOverlays, setResolvedOverlays] = useState<WorkshopMapOverlayLayerConfig[]>([]);
  const [templateRender, setTemplateRender] = useState<RenderMapTemplateResponse | null>(null);
  const [drawMode, setDrawMode] = useState(false);
  const [draftShape, setDraftShape] = useState<WorkshopMapFeature | null>(null);
  const [drawnShape, setDrawnShape] = useState<WorkshopMapFeature | null>(null);
  const [tooltip, setTooltip] = useState<TooltipState | null>(null);
  const [selectedLabel, setSelectedLabel] = useState('');
  const [loadError, setLoadError] = useState('');

  const rawProps = widget.props ?? {};
  const mapTemplateId = useMemo(() => mapTemplateIDFromProps(rawProps), [rawProps]);
  const templateRequest = useMemo(() => buildMapTemplateRenderRequest(rawProps, variables), [rawProps, variables]);
  const props = useMemo(() => mergeMapTemplateWidgetProps(rawProps, templateRender), [rawProps, templateRender]);
  const layers = useMemo(() => readMapLayerConfigs(props), [props]);
  const overlays = useMemo(() => readMapOverlayConfigs(props), [props]);
  const [hiddenLayerIds, setHiddenLayerIds] = useState<Set<string>>(() => defaultHiddenLayerIds(layers, overlays));
  const visibilitySignature = useMemo(
    () => [...layers.map((layer) => `object:${layer.id}:${layer.visible}`), ...overlays.map((overlay) => `overlay:${overlay.id}:${overlay.visible}`)].join('|'),
    [layers, overlays],
  );

  useEffect(() => {
    setHiddenLayerIds(defaultHiddenLayerIds(layers, overlays));
  }, [visibilitySignature]);

  const visibleLayers = useMemo(
    () => layers.filter((layer) => isWorkshopMapLayerVisible(layer, variableEngine) && !hiddenLayerIds.has(objectLayerKey(layer.id))),
    [hiddenLayerIds, layers, variableEngine],
  );
  const visibleOverlays = useMemo(
    () => overlays.filter((overlay) => isWorkshopMapLayerVisible(overlay, variableEngine) && !hiddenLayerIds.has(overlayLayerKey(overlay.id))),
    [hiddenLayerIds, overlays, variableEngine],
  );
  const viewportTileLayers = useMemo(
    () => visibleLayers.filter((layer) => layer.source === 'geospatial_tile' || layer.loading_mode === 'viewport_tiles'),
    [visibleLayers],
  );
  const bindingCollection = useMemo(() => buildFeaturesFromQueryResult(result ?? null, visibleLayers), [result, visibleLayers]);
  const configuredFeatures = useMemo(() => buildFeaturesFromConfiguredLayers(visibleLayers), [visibleLayers]);
  const featureCollection = useMemo<WorkshopMapFeatureCollection>(() => ({
    type: 'FeatureCollection',
    features: [...bindingCollection.features, ...configuredFeatures, ...objectFeatures, ...tileFeatures],
  }), [bindingCollection.features, configuredFeatures, objectFeatures, tileFeatures]);
  const clusterOptions = useMemo(() => readClusterOptions(visibleLayers), [visibleLayers]);
  const splitCollections = useMemo(() => splitClusterableFeatures(featureCollection), [featureCollection]);

  const style = useMemo(() => createWorkshopMapStyle(props, resolvedOverlays), [props, resolvedOverlays]);
  const mapKey = useMemo(() => JSON.stringify({ style, center_lat: props.center_lat, center_lon: props.center_lon, zoom: props.zoom, cluster: clusterOptions }), [clusterOptions, props.center_lat, props.center_lon, props.zoom, style]);
  const center = useMemo<[number, number]>(() => {
    const lon = numberProp(props.center_lon, null);
    const lat = numberProp(props.center_lat, null);
    if (lon !== null && lat !== null) return [lon, lat];
    return [-105.2705, 40.015];
  }, [props]);
  const zoom = numberProp(props.zoom, 10) ?? 10;
  const showLegend = booleanProp(props.show_legend, true);
  const outputVariableId = stringProp(props.output_variable_id);
  const outputObjectSetVariableId = stringProp(props.output_object_set_variable_id);
  const outputShapeVariableId = stringProp(props.output_shape_variable_id);
  const shapeSearchOutputVariableId = stringProp(props.shape_search_output_variable_id);
  const enableShapeDrawing = booleanProp(props.enable_shape_drawing, Boolean(outputShapeVariableId || shapeSearchOutputVariableId));
  const mapHeight = numberProp(props.height, 360) ?? 360;
  const activeShape = draftShape ?? drawnShape;
  const shapeCollection = useMemo<WorkshopMapFeatureCollection>(() => (
    activeShape ? { type: 'FeatureCollection', features: [activeShape] } : EMPTY_DRAWN_SHAPE_COLLECTION
  ), [activeShape]);
  const shapeMatchedObjects = useMemo(() => objectsIntersectingShape(drawnShape, featureCollection), [drawnShape, featureCollection]);

  const toggleLayer = useCallback((key: string) => {
    setHiddenLayerIds((previous) => {
      const next = new Set(previous);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function loadTemplate() {
      if (!mapTemplateId) {
        setTemplateRender(null);
        return;
      }
      try {
        const rendered = await renderMapTemplate(mapTemplateId, templateRequest);
        if (!cancelled) {
          setTemplateRender(rendered);
          setLoadError('');
        }
      } catch (cause) {
        if (!cancelled) {
          setTemplateRender(null);
          setLoadError(cause instanceof Error ? cause.message : 'Map template render failed');
        }
      }
    }
    void loadTemplate();
    return () => {
      cancelled = true;
    };
  }, [mapTemplateId, templateRequest]);

  const clearShape = useCallback(() => {
    drawStartRef.current = null;
    setDraftShape(null);
    setDrawnShape(null);
    setDrawMode(false);
  }, []);

  const handleDrawPointerDown = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (!drawMode || !mapRef.current) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    const start = lngLatFromPointer(mapRef.current, event);
    drawStartRef.current = start;
    setDraftShape(rectangleShapeFeature(start, start));
  }, [drawMode]);

  const handleDrawPointerMove = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (!drawMode || !mapRef.current || !drawStartRef.current) return;
    event.preventDefault();
    const end = lngLatFromPointer(mapRef.current, event);
    setDraftShape(rectangleShapeFeature(drawStartRef.current, end));
  }, [drawMode]);

  const handleDrawPointerUp = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (!drawMode || !mapRef.current || !drawStartRef.current) return;
    event.preventDefault();
    try {
      event.currentTarget.releasePointerCapture(event.pointerId);
    } catch {
      // The browser may have already released capture after pointerup.
    }
    const end = lngLatFromPointer(mapRef.current, event);
    const nextShape = rectangleShapeFeature(drawStartRef.current, end);
    drawStartRef.current = null;
    setDraftShape(null);
    setDrawnShape(nextShape);
    setDrawMode(false);
  }, [drawMode]);

  useEffect(() => {
    let cancelled = false;
    async function loadObjectLayers() {
      setLoadError('');
      const objectLayerFeatures: WorkshopMapFeature[] = [];
      const linkedEdgeGroups: Array<{ layer: WorkshopMapLayerConfig; edges: WorkshopObjectSetExecutionResult['linkedEdges'] }> = [];
      try {
        for (const layer of visibleLayers) {
          if (layer.source === 'binding' || layer.source === 'geospatial_tile' || layer.loading_mode === 'viewport_tiles') continue;
          const variable = layer.source_variable_id ? variables.find((entry) => entry.id === layer.source_variable_id) ?? null : null;
          const objectTypeId = variable?.object_type_id || layer.object_type_id;
          if (!objectTypeId) continue;
          const result = await loadObjectSetForLayer(objectTypeId, variable, variables, layer, variableEngine);
          if (cancelled) return;
          objectLayerFeatures.push(...buildFeaturesFromObjects(result.data, layer, variable));
          if (result.linkedEdges.length > 0) {
            linkedEdgeGroups.push({ layer, edges: result.linkedEdges });
          }
        }
        const linkedLayerFeatures = linkedEdgeGroups.flatMap(({ layer, edges }) => buildFeaturesFromLinkedEdges(objectLayerFeatures, edges, {
          layerId: `links-${layer.id}`,
          layerTitle: `${layer.title} links`,
          color: layer.color,
          lineWidth: Math.max(1, layer.line_width - 1),
        }));
        if (!cancelled) setObjectFeatures([...linkedLayerFeatures, ...objectLayerFeatures]);
      } catch (cause) {
        if (!cancelled) {
          setObjectFeatures([]);
          setLoadError(cause instanceof Error ? cause.message : 'Map object layer load failed');
        }
      }
    }
    void loadObjectLayers();
    return () => {
      cancelled = true;
    };
  }, [variableEngine, variables, visibleLayers]);

  const loadViewportTileLayers = useCallback(async (map: MapLibreMap) => {
    const requestSeq = ++tileRequestSeqRef.current;
    const activeLayers = viewportTileLayers.filter((layer) => layer.tile_layer_id.trim().length > 0);
    if (activeLayers.length === 0) {
      setTileFeatures([]);
      return;
    }
    try {
      const bounds = map.getBounds();
      const zoomLevel = map.getZoom();
      const pages = await Promise.all(activeLayers.map(async (layer) => {
        const page = await getViewportTileFeatures(layer.tile_layer_id, {
          min_lat: bounds.getSouth(),
          min_lon: bounds.getWest(),
          max_lat: bounds.getNorth(),
          max_lon: bounds.getEast(),
          zoom: zoomLevel,
          limit: clampNumber(layer.tile_page_size, 1, 5000, 500),
          offset: 0,
          simplify_tolerance: Math.max(0, layer.tile_simplify_tolerance),
        });
        return buildFeaturesFromGeospatialLayer(page.features, layer);
      }));
      if (requestSeq === tileRequestSeqRef.current) {
        setTileFeatures(pages.flat());
      }
    } catch (cause) {
      if (requestSeq === tileRequestSeqRef.current) {
        setTileFeatures([]);
        setLoadError(cause instanceof Error ? cause.message : 'Map tile layer load failed');
      }
    }
  }, [viewportTileLayers]);

  useEffect(() => {
    if (viewportTileLayers.length === 0) {
      setTileFeatures([]);
      return;
    }
    if (!mapRef.current) return;
    void loadViewportTileLayers(mapRef.current);
  }, [loadViewportTileLayers, viewportTileLayers]);

  useEffect(() => {
    let cancelled = false;
    async function resolveOverlays() {
      const next: WorkshopMapOverlayLayerConfig[] = [];
      try {
        for (const overlay of visibleOverlays) {
          if (overlay.source !== 'saved_map_layer') {
            next.push(overlay);
            continue;
          }
          if (!overlay.resource_id) {
            next.push(overlay);
            continue;
          }
          const resource = await getLayer(overlay.resource_id);
          if (cancelled) return;
          next.push(normalizeSavedOverlayConfig(overlay, resource as unknown as Record<string, unknown>));
        }
        if (!cancelled) setResolvedOverlays(next);
      } catch (cause) {
        if (!cancelled) {
          setResolvedOverlays([]);
          setLoadError(cause instanceof Error ? cause.message : 'Map overlay layer load failed');
        }
      }
    }
    void resolveOverlays();
    return () => {
      cancelled = true;
    };
  }, [visibleOverlays]);

  useEffect(() => {
    if (outputShapeVariableId) onShapeChange?.(outputShapeVariableId, drawnShape ? { type: 'FeatureCollection', features: [drawnShape] } : null);
  }, [drawnShape, onShapeChange, outputShapeVariableId]);

  useEffect(() => {
    if (shapeSearchOutputVariableId) onSelectObjectSet?.(shapeSearchOutputVariableId, shapeMatchedObjects);
  }, [onSelectObjectSet, shapeMatchedObjects, shapeSearchOutputVariableId]);

  const syncMapData = useCallback((map: MapLibreMap, next: WorkshopMapFeatureCollection, fitCollection: WorkshopMapFeatureCollection) => {
    let source: { setData?: (data: WorkshopMapFeatureCollection) => void } | undefined;
    try {
      source = map.getSource(SOURCE_ID) as { setData?: (data: WorkshopMapFeatureCollection) => void } | undefined;
    } catch {
      return;
    }
    source?.setData?.(next);
    const bounds = collectFeatureBounds(fitCollection);
    if (bounds && !fittedRef.current) {
      try {
        map.fitBounds(bounds, { padding: 42, maxZoom: 13, duration: 0 });
        fittedRef.current = true;
      } catch {
        // MapLibre may briefly expose a removed map instance while React swaps
        // styles for overlay visibility changes. The next loaded instance will
        // receive the same data and fit normally.
      }
    }
  }, []);

  const syncClusterData = useCallback((map: MapLibreMap, next: WorkshopMapFeatureCollection) => {
    try {
      const source = map.getSource(CLUSTER_SOURCE_ID) as { setData?: (data: WorkshopMapFeatureCollection) => void } | undefined;
      source?.setData?.(next);
    } catch {
      // React can briefly hold a removed MapLibre instance while styles swap.
    }
  }, []);

  const syncShapeData = useCallback((map: MapLibreMap, next: WorkshopMapFeatureCollection) => {
    try {
      const source = map.getSource(SHAPE_SOURCE_ID) as { setData?: (data: WorkshopMapFeatureCollection) => void } | undefined;
      source?.setData?.(next);
    } catch {
      // React can briefly hold a removed MapLibre instance while styles swap.
    }
  }, []);

  useEffect(() => {
    if (!mapRef.current) return;
    syncMapData(mapRef.current, splitCollections.base, featureCollection);
    syncClusterData(mapRef.current, splitCollections.clustered);
  }, [featureCollection, splitCollections, syncClusterData, syncMapData]);

  useEffect(() => {
    if (!mapRef.current) return;
    syncShapeData(mapRef.current, shapeCollection);
  }, [shapeCollection, syncShapeData]);

  const handleMapLoad = useCallback((map: MapLibreMap) => {
    mapRef.current = map;
    fittedRef.current = false;
    installWorkshopMapLayers(map, splitCollections.base);
    installWorkshopClusterLayers(map, splitCollections.clustered, clusterOptions);
    installWorkshopShapeLayers(map, shapeCollection);
    syncMapData(map, splitCollections.base, featureCollection);
    syncClusterData(map, splitCollections.clustered);
    syncShapeData(map, shapeCollection);
    void loadViewportTileLayers(map);

    const onMove = (event: MapLayerMouseEvent) => {
      const feature = event.features?.[0];
      if (!feature) return;
      const props = (feature.properties ?? {}) as Record<string, unknown>;
      map.getCanvas().style.cursor = props.__of_locked === true ? '' : 'pointer';
      const clusterLabel = clusterFeatureLabel(props);
      setTooltip({
        x: event.point.x,
        y: event.point.y,
        label: clusterLabel || stringify(props.__of_label) || stringify(props.__of_object_id) || 'Object',
        layer: stringify(props.__of_layer_title),
      });
    };
    const onLeave = () => {
      map.getCanvas().style.cursor = '';
      setTooltip(null);
    };
    const onClick = (event: MapMouseEvent) => {
      const features = map.queryRenderedFeatures(event.point, { layers: existingLayerIds(map, INTERACTIVE_LAYER_IDS) });
      const clusterFeature = pickClusterFeature(features);
      if (clusterFeature) {
        zoomToCluster(map, clusterFeature);
        return;
      }
      const feature = pickSelectableFeature(features);
      if (!feature) return;
      void handleFeatureSelection(feature, outputVariableId, outputObjectSetVariableId, onSelectObject, onSelectObjectSet, onSelectRecord, setSelectedLabel);
    };
    const onMoveEnd = () => {
      void loadViewportTileLayers(map);
    };

    for (const layerId of existingLayerIds(map, INTERACTIVE_LAYER_IDS)) {
      map.on('mousemove', layerId, onMove);
      map.on('mouseleave', layerId, onLeave);
    }
    map.on('click', onClick);
    map.on('moveend', onMoveEnd);
  }, [clusterOptions, featureCollection, loadViewportTileLayers, onSelectObject, onSelectObjectSet, onSelectRecord, outputObjectSetVariableId, outputVariableId, shapeCollection, splitCollections, syncClusterData, syncMapData, syncShapeData]);

  return (
    <div
      data-testid="workshop-map-widget"
      style={{
        position: 'relative',
        minHeight: Math.max(260, mapHeight),
        height: '100%',
        overflow: 'hidden',
        borderRadius: 8,
        border: '1px solid #d6dde3',
        background: '#edf2f7',
      }}
    >
      <MapLibreCanvas
        key={mapKey}
        className="of-workshop-map-canvas"
        style={style}
        center={center}
        zoom={zoom}
        height="100%"
        preserveDrawingBuffer
        onMapLoad={handleMapLoad}
      />

      {enableShapeDrawing ? (
        <div style={{ position: 'absolute', top: 12, left: 12, display: 'inline-flex', gap: 6, zIndex: 3 }}>
          <button
            type="button"
            data-testid="workshop-map-draw-shape"
            onClick={() => setDrawMode((value) => !value)}
            style={{ padding: '7px 10px', borderRadius: 6, border: drawMode ? '1px solid #1d4ed8' : '1px solid rgba(148, 163, 184, 0.55)', background: drawMode ? '#dbeafe' : 'rgba(255, 255, 255, 0.94)', color: '#0f172a', fontSize: 12, fontWeight: 600, cursor: 'pointer', boxShadow: '0 8px 20px rgba(15, 23, 42, 0.10)' }}
          >
            Draw area
          </button>
          {drawnShape ? (
            <button
              type="button"
              data-testid="workshop-map-clear-shape"
              onClick={clearShape}
              style={{ padding: '7px 10px', borderRadius: 6, border: '1px solid rgba(148, 163, 184, 0.55)', background: 'rgba(255, 255, 255, 0.94)', color: '#be123c', fontSize: 12, fontWeight: 600, cursor: 'pointer', boxShadow: '0 8px 20px rgba(15, 23, 42, 0.10)' }}
            >
              Clear
            </button>
          ) : null}
          {drawnShape || draftShape ? (
            <span data-testid="workshop-map-shape-match-count" style={{ alignSelf: 'center', padding: '7px 10px', borderRadius: 6, background: 'rgba(15, 23, 42, 0.84)', color: '#fff', fontSize: 12, fontWeight: 600 }}>
              {shapeMatchedObjects.length} {shapeMatchedObjects.length === 1 ? 'match' : 'matches'}
            </span>
          ) : null}
        </div>
      ) : null}

      {drawMode ? (
        <div
          data-testid="workshop-map-draw-surface"
          onPointerDown={handleDrawPointerDown}
          onPointerMove={handleDrawPointerMove}
          onPointerUp={handleDrawPointerUp}
          onPointerCancel={clearShape}
          style={{ position: 'absolute', inset: 0, zIndex: 2, cursor: 'crosshair', touchAction: 'none', background: 'rgba(59, 130, 246, 0.04)' }}
        />
      ) : null}

      {showLegend ? (
        <div
          style={{
            position: 'absolute',
            left: 12,
            bottom: 12,
            display: 'grid',
            gap: 6,
            maxWidth: 240,
            padding: '10px 12px',
            borderRadius: 6,
            border: '1px solid rgba(148, 163, 184, 0.45)',
            background: 'rgba(255, 255, 255, 0.92)',
            boxShadow: '0 8px 20px rgba(15, 23, 42, 0.12)',
          }}
        >
          {layers.map((layer) => (
            <label key={layer.id} data-testid={`workshop-map-layer-toggle-${layer.id}`} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: '#334155', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={isWorkshopMapLayerVisible(layer, variableEngine) && !hiddenLayerIds.has(objectLayerKey(layer.id))}
                onChange={() => toggleLayer(objectLayerKey(layer.id))}
                style={{ margin: 0 }}
              />
              <span style={{ width: 16, height: layer.geometry_type === 'line' ? 3 : 10, borderRadius: 999, background: layer.color, border: '1px solid rgba(15, 23, 42, 0.12)' }} />
              <span>{layer.title}</span>
            </label>
          ))}
          {overlays.map((overlay) => (
            <label key={overlay.id} data-testid={`workshop-map-overlay-toggle-${overlay.id}`} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: '#334155', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={isWorkshopMapLayerVisible(overlay, variableEngine) && !hiddenLayerIds.has(overlayLayerKey(overlay.id))}
                onChange={() => toggleLayer(overlayLayerKey(overlay.id))}
                style={{ margin: 0 }}
              />
              <span style={{ width: 16, height: overlay.geometry_type === 'line' ? 3 : 10, borderRadius: overlay.geometry_type === 'polygon' ? 2 : 999, background: overlay.color, opacity: overlay.opacity, border: '1px solid rgba(15, 23, 42, 0.12)' }} />
              <span>{overlay.title}</span>
            </label>
          ))}
          <span data-testid="workshop-map-feature-count" style={{ fontSize: 11, color: '#64748b' }}>
            {featureCollection.features.length} {featureCollection.features.length === 1 ? 'feature' : 'features'}
          </span>
          <span data-testid="workshop-map-visible-overlay-count" style={{ fontSize: 11, color: '#64748b' }}>
            {visibleOverlays.length} {visibleOverlays.length === 1 ? 'overlay' : 'overlays'}
          </span>
        </div>
      ) : null}

      {tooltip ? (
        <div
          role="tooltip"
          style={{
            position: 'absolute',
            left: Math.min(Math.max(tooltip.x + 12, 8), 520),
            top: Math.max(tooltip.y - 12, 8),
            pointerEvents: 'none',
            padding: '7px 9px',
            borderRadius: 6,
            background: '#0f172a',
            color: '#fff',
            fontSize: 12,
            boxShadow: '0 8px 20px rgba(15, 23, 42, 0.25)',
          }}
        >
          <strong style={{ display: 'block', fontSize: 12 }}>{tooltip.label}</strong>
          {tooltip.layer ? <span style={{ color: '#cbd5e1', fontSize: 11 }}>{tooltip.layer}</span> : null}
        </div>
      ) : null}

      {selectedLabel ? (
        <div
          role="status"
          style={{
            position: 'absolute',
            right: 12,
            top: 12,
            padding: '7px 10px',
            borderRadius: 6,
            background: 'rgba(15, 23, 42, 0.84)',
            color: '#fff',
            fontSize: 12,
          }}
        >
          Selected {selectedLabel}
        </div>
      ) : null}

      {loadError ? (
        <div style={{ position: 'absolute', right: 12, bottom: 12, maxWidth: 320, borderRadius: 6, border: '1px solid #fecaca', background: '#fff1f2', padding: '8px 10px', color: '#be123c', fontSize: 12 }}>
          {loadError}
        </div>
      ) : null}

      {featureCollection.features.length === 0 && !loadError ? (
        <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', pointerEvents: 'none', color: '#64748b', fontSize: 13 }}>
          Add point columns or a GeoJSON geometry field to this map.
        </div>
      ) : null}
    </div>
  );
}

function defaultHiddenLayerIds(layers: WorkshopMapLayerConfig[], overlays: WorkshopMapOverlayLayerConfig[]) {
  const hidden = new Set<string>();
  for (const layer of layers) {
    if (!layer.visible) hidden.add(objectLayerKey(layer.id));
  }
  for (const overlay of overlays) {
    if (!overlay.visible) hidden.add(overlayLayerKey(overlay.id));
  }
  return hidden;
}

function objectLayerKey(id: string) {
  return `object:${id}`;
}

function overlayLayerKey(id: string) {
  return `overlay:${id}`;
}

function readClusterOptions(layers: WorkshopMapLayerConfig[]): ClusterOptions {
  const clusterLayers = layers.filter((layer) => layer.cluster_enabled);
  if (clusterLayers.length === 0) return { enabled: false, radius: 64, maxZoom: 10, color: '#2563eb' };
  return {
    enabled: true,
    radius: clampNumber(Math.max(...clusterLayers.map((layer) => layer.cluster_radius)), 24, 160, 64),
    maxZoom: clampNumber(Math.min(...clusterLayers.map((layer) => layer.cluster_max_zoom)), 1, 18, 10),
    color: clusterLayers[0]?.cluster_color || clusterLayers[0]?.color || '#2563eb',
  };
}

function splitClusterableFeatures(collection: WorkshopMapFeatureCollection) {
  const base: WorkshopMapFeature[] = [];
  const clustered: WorkshopMapFeature[] = [];
  for (const feature of collection.features) {
    if (feature.geometry.type === 'Point' && feature.properties.__of_cluster_enabled === true) {
      clustered.push(feature);
    } else {
      base.push(feature);
    }
  }
  return {
    base: { type: 'FeatureCollection' as const, features: base },
    clustered: { type: 'FeatureCollection' as const, features: clustered },
  };
}

function clampNumber(value: number, min: number, max: number, fallback: number) {
  if (!Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, value));
}

async function loadObjectSetForLayer(
  objectTypeId: string,
  variable: WorkshopVariableLike | null,
  variables: WorkshopVariableLike[],
  layer: WorkshopMapLayerConfig,
  variableEngine: WorkshopVariableEngineResult | null,
) {
  if (layer.filter_field && layer.filter_value) {
    const nextVariable: WorkshopVariableLike = {
      id: `map-layer-${layer.id}`,
      kind: 'object_set_definition',
      name: layer.title,
      object_type_id: objectTypeId,
      source_variable_id: variable?.id,
      static_filters: [{ property_name: layer.filter_field, operator: 'equals', value: layer.filter_value }],
    };
    const response = await executeWorkshopObjectSet({
      variable: nextVariable,
      variables: variable ? [...variables, nextVariable] : [nextVariable],
      engine: variableEngine,
      objectTypeId,
      limit: 5000,
    });
    return response;
  }
  return executeWorkshopObjectSet({
    variable,
    variables,
    engine: variableEngine,
    objectTypeId,
    limit: 5000,
  });
}

function installWorkshopMapLayers(map: MapLibreMap, data: WorkshopMapFeatureCollection) {
  if (!map.getSource(SOURCE_ID)) {
    map.addSource(SOURCE_ID, { type: 'geojson', data: data as never });
  }
  if (!map.getLayer(FILL_LAYER_ID)) {
    map.addLayer({
      id: FILL_LAYER_ID,
      type: 'fill',
      source: SOURCE_ID,
      filter: ['any', ['==', ['geometry-type'], 'Polygon'], ['==', ['geometry-type'], 'MultiPolygon']],
      paint: {
        'fill-color': ['coalesce', ['get', '__of_color'], '#15803d'],
        'fill-opacity': ['coalesce', ['get', '__of_fill_opacity'], 0.22],
        'fill-outline-color': ['coalesce', ['get', '__of_color'], '#15803d'],
      },
    } as never);
  }
  if (!map.getLayer(LINE_LAYER_ID)) {
    map.addLayer({
      id: LINE_LAYER_ID,
      type: 'line',
      source: SOURCE_ID,
      filter: ['any', ['==', ['geometry-type'], 'LineString'], ['==', ['geometry-type'], 'MultiLineString']],
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: {
        'line-color': ['coalesce', ['get', '__of_color'], '#c2410c'],
        'line-width': ['coalesce', ['get', '__of_line_width'], 3],
        'line-opacity': 0.88,
      },
    } as never);
  }
  if (!map.getLayer(POINT_LAYER_ID)) {
    map.addLayer({
      id: POINT_LAYER_ID,
      type: 'circle',
      source: SOURCE_ID,
      filter: ['any', ['==', ['geometry-type'], 'Point'], ['==', ['geometry-type'], 'MultiPoint']],
      paint: {
        'circle-radius': ['coalesce', ['get', '__of_radius'], 6],
        'circle-color': ['coalesce', ['get', '__of_color'], '#2d72d2'],
        'circle-stroke-color': '#ffffff',
        'circle-stroke-width': 2,
      },
    } as never);
  }
  if (!map.getLayer(LABEL_LAYER_ID)) {
    map.addLayer({
      id: LABEL_LAYER_ID,
      type: 'symbol',
      source: SOURCE_ID,
      filter: ['any', ['==', ['geometry-type'], 'Point'], ['==', ['geometry-type'], 'MultiPoint']],
      layout: {
        'text-field': ['get', '__of_label'],
        'text-size': 12,
        'text-offset': [0, 1.25],
        'text-anchor': 'top',
        'text-allow-overlap': false,
      },
      paint: {
        'text-color': '#1e293b',
        'text-halo-color': '#ffffff',
        'text-halo-width': 1.25,
      },
    } as never);
  }
}

function installWorkshopClusterLayers(map: MapLibreMap, data: WorkshopMapFeatureCollection, options: ClusterOptions) {
  if (!options.enabled) return;
  if (!map.getSource(CLUSTER_SOURCE_ID)) {
    map.addSource(CLUSTER_SOURCE_ID, {
      type: 'geojson',
      data: data as never,
      cluster: true,
      clusterRadius: options.radius,
      clusterMaxZoom: options.maxZoom,
    } as never);
  }
  if (!map.getLayer(CLUSTER_CIRCLE_LAYER_ID)) {
    map.addLayer({
      id: CLUSTER_CIRCLE_LAYER_ID,
      type: 'circle',
      source: CLUSTER_SOURCE_ID,
      filter: ['has', 'point_count'],
      paint: {
        'circle-color': [
          'step',
          ['get', 'point_count'],
          tintClusterColor(options.color, 0.42),
          10,
          options.color,
          25,
          shadeClusterColor(options.color, 0.72),
        ],
        'circle-radius': ['step', ['get', 'point_count'], 18, 10, 24, 25, 32],
        'circle-stroke-color': '#ffffff',
        'circle-stroke-width': 2,
      },
    } as never);
  }
  if (!map.getLayer(CLUSTER_COUNT_LAYER_ID)) {
    map.addLayer({
      id: CLUSTER_COUNT_LAYER_ID,
      type: 'symbol',
      source: CLUSTER_SOURCE_ID,
      filter: ['has', 'point_count'],
      layout: {
        'text-field': ['get', 'point_count_abbreviated'],
        'text-size': 12,
        'text-font': ['Open Sans Bold', 'Arial Unicode MS Bold'],
        'text-allow-overlap': true,
      },
      paint: {
        'text-color': '#ffffff',
        'text-halo-color': 'rgba(15, 23, 42, 0.20)',
        'text-halo-width': 0.75,
      },
    } as never);
  }
  if (!map.getLayer(CLUSTER_POINT_LAYER_ID)) {
    map.addLayer({
      id: CLUSTER_POINT_LAYER_ID,
      type: 'circle',
      source: CLUSTER_SOURCE_ID,
      filter: ['!', ['has', 'point_count']],
      paint: {
        'circle-radius': ['coalesce', ['get', '__of_radius'], 6],
        'circle-color': ['coalesce', ['get', '__of_color'], '#2d72d2'],
        'circle-stroke-color': '#ffffff',
        'circle-stroke-width': 2,
      },
    } as never);
  }
  if (!map.getLayer(CLUSTER_LABEL_LAYER_ID)) {
    map.addLayer({
      id: CLUSTER_LABEL_LAYER_ID,
      type: 'symbol',
      source: CLUSTER_SOURCE_ID,
      filter: ['!', ['has', 'point_count']],
      layout: {
        'text-field': ['get', '__of_label'],
        'text-size': 12,
        'text-offset': [0, 1.25],
        'text-anchor': 'top',
        'text-allow-overlap': false,
      },
      paint: {
        'text-color': '#1e293b',
        'text-halo-color': '#ffffff',
        'text-halo-width': 1.25,
      },
    } as never);
  }
}

function installWorkshopShapeLayers(map: MapLibreMap, data: WorkshopMapFeatureCollection) {
  if (!map.getSource(SHAPE_SOURCE_ID)) {
    map.addSource(SHAPE_SOURCE_ID, { type: 'geojson', data: data as never });
  }
  const beforePointLayer = map.getLayer(POINT_LAYER_ID) ? POINT_LAYER_ID : undefined;
  if (!map.getLayer(SHAPE_FILL_LAYER_ID)) {
    map.addLayer({
      id: SHAPE_FILL_LAYER_ID,
      type: 'fill',
      source: SHAPE_SOURCE_ID,
      paint: {
        'fill-color': '#2563eb',
        'fill-opacity': 0.14,
        'fill-outline-color': '#1d4ed8',
      },
    } as never, beforePointLayer);
  }
  if (!map.getLayer(SHAPE_LINE_LAYER_ID)) {
    map.addLayer({
      id: SHAPE_LINE_LAYER_ID,
      type: 'line',
      source: SHAPE_SOURCE_ID,
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: {
        'line-color': '#1d4ed8',
        'line-width': 2,
        'line-dasharray': [2, 1],
      },
    } as never, beforePointLayer);
  }
}

function pickSelectableFeature(features: MapGeoJSONFeature[]) {
  return features.find((feature) => parseObject((feature.properties ?? {}).__of_object_json))
    ?? features.find((feature) => !isClusterFeature(feature) && (feature.properties ?? {}).__of_locked !== true)
    ?? null;
}

function pickClusterFeature(features: MapGeoJSONFeature[]) {
  return features.find(isClusterFeature) ?? null;
}

function isClusterFeature(feature: MapGeoJSONFeature) {
  const props = (feature.properties ?? {}) as Record<string, unknown>;
  return props.cluster === true || props.cluster === 1 || props.cluster === 'true' || props.point_count !== undefined;
}

function clusterFeatureLabel(props: Record<string, unknown>) {
  const count = Number(props.point_count);
  if (!Number.isFinite(count) || count <= 0) return '';
  return `${count} ${count === 1 ? 'object' : 'objects'}`;
}

function zoomToCluster(map: MapLibreMap, feature: MapGeoJSONFeature) {
  const props = (feature.properties ?? {}) as Record<string, unknown>;
  const clusterId = props.cluster_id;
  const geometry = feature.geometry;
  if (geometry.type !== 'Point') return;
  const center = geometry.coordinates as [number, number];
  const source = map.getSource(CLUSTER_SOURCE_ID) as {
    getClusterExpansionZoom?: (clusterId: number, callback: (error?: Error | null, zoom?: number) => void) => void;
  } | undefined;
  const numericClusterId = typeof clusterId === 'number' ? clusterId : Number(clusterId);
  if (!source?.getClusterExpansionZoom || !Number.isFinite(numericClusterId)) {
    map.easeTo({ center, zoom: Math.min(map.getZoom() + 2, 18) });
    return;
  }
  source.getClusterExpansionZoom(numericClusterId, (error, zoom) => {
    if (error || typeof zoom !== 'number') {
      map.easeTo({ center, zoom: Math.min(map.getZoom() + 2, 18) });
      return;
    }
    map.easeTo({ center, zoom: Math.min(zoom + 0.35, 18) });
  });
}

function existingLayerIds(map: MapLibreMap, ids: string[]) {
  return ids.filter((id) => Boolean(map.getLayer(id)));
}

async function handleFeatureSelection(
  feature: MapGeoJSONFeature,
  outputVariableId: string,
  outputObjectSetVariableId: string,
  onSelectObject: WorkshopMapWidgetProps['onSelectObject'],
  onSelectObjectSet: WorkshopMapWidgetProps['onSelectObjectSet'],
  onSelectRecord: WorkshopMapWidgetProps['onSelectRecord'],
  setSelectedLabel: (value: string) => void,
) {
  const props = (feature.properties ?? {}) as Record<string, unknown>;
  if (props.__of_locked === true) return;
  const label = stringify(props.__of_label) || stringify(props.__of_object_id) || 'object';
  setSelectedLabel(label);
  const object = parseObject(props.__of_object_json);
  if (object && outputVariableId) onSelectObject?.(outputVariableId, object);
  if (outputObjectSetVariableId) onSelectObjectSet?.(outputObjectSetVariableId, object ? [object] : []);
  await onSelectRecord?.({
    ...props,
    label,
    layer_id: props.__of_layer_id,
    object_id: props.__of_object_id,
  });
}

function parseObject(value: unknown): ObjectInstance | null {
  if (typeof value !== 'string' || !value.trim()) return null;
  try {
    const parsed = JSON.parse(value) as ObjectInstance;
    return parsed && typeof parsed.id === 'string' ? parsed : null;
  } catch {
    return null;
  }
}

function lngLatFromPointer(map: MapLibreMap, event: ReactPointerEvent<HTMLDivElement>): [number, number] {
  const rect = map.getCanvas().getBoundingClientRect();
  const point = map.unproject([event.clientX - rect.left, event.clientY - rect.top]);
  return [point.lng, point.lat];
}

function rectangleShapeFeature(start: [number, number], end: [number, number]): WorkshopMapFeature {
  const minLon = Math.min(start[0], end[0]);
  const maxLon = Math.max(start[0], end[0]);
  const minLat = Math.min(start[1], end[1]);
  const maxLat = Math.max(start[1], end[1]);
  return {
    type: 'Feature',
    properties: {
      __of_shape_kind: 'drawn_rectangle',
      __of_label: 'Drawn area',
    },
    geometry: {
      type: 'Polygon',
      coordinates: [[
        [minLon, minLat],
        [maxLon, minLat],
        [maxLon, maxLat],
        [minLon, maxLat],
        [minLon, minLat],
      ]],
    },
  };
}

function objectsIntersectingShape(shape: WorkshopMapFeature | null, collection: WorkshopMapFeatureCollection) {
  if (!shape || shape.geometry.type !== 'Polygon') return [];
  const matches: ObjectInstance[] = [];
  const seen = new Set<string>();
  for (const feature of collection.features) {
    const object = parseObject(feature.properties.__of_object_json);
    if (!object || seen.has(object.id)) continue;
    if (!geometryIntersectsPolygon(feature.geometry, shape.geometry.coordinates[0])) continue;
    seen.add(object.id);
    matches.push(object);
  }
  return matches;
}

function geometryIntersectsPolygon(geometry: WorkshopMapGeometry, polygon: [number, number][]) {
  const points = flattenGeometryPoints(geometry);
  if (points.some((point) => pointInPolygon(point, polygon))) return true;
  const geometryBounds = boundsFromPoints(points);
  const polygonBounds = boundsFromPoints(polygon);
  return Boolean(geometryBounds && polygonBounds && boundsOverlap(geometryBounds, polygonBounds));
}

function flattenGeometryPoints(geometry: WorkshopMapGeometry): [number, number][] {
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

function pointInPolygon(point: [number, number], polygon: [number, number][]) {
  const [x, y] = point;
  let inside = false;
  for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i++) {
    const [xi, yi] = polygon[i];
    const [xj, yj] = polygon[j];
    const intersects = yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / ((yj - yi) || Number.EPSILON) + xi;
    if (intersects) inside = !inside;
  }
  return inside;
}

function boundsFromPoints(points: [number, number][]) {
  if (points.length === 0) return null;
  return points.reduce(
    (bounds, [lon, lat]) => ({
      minLon: Math.min(bounds.minLon, lon),
      minLat: Math.min(bounds.minLat, lat),
      maxLon: Math.max(bounds.maxLon, lon),
      maxLat: Math.max(bounds.maxLat, lat),
    }),
    { minLon: Number.POSITIVE_INFINITY, minLat: Number.POSITIVE_INFINITY, maxLon: Number.NEGATIVE_INFINITY, maxLat: Number.NEGATIVE_INFINITY },
  );
}

function boundsOverlap(
  left: { minLon: number; minLat: number; maxLon: number; maxLat: number },
  right: { minLon: number; minLat: number; maxLon: number; maxLat: number },
) {
  return left.minLon <= right.maxLon && left.maxLon >= right.minLon && left.minLat <= right.maxLat && left.maxLat >= right.minLat;
}

function tintClusterColor(hex: string, amount: number) {
  return mixHex(hex, '#ffffff', amount);
}

function shadeClusterColor(hex: string, amount: number) {
  return mixHex(hex, '#0f172a', 1 - amount);
}

function mixHex(left: string, right: string, amount: number) {
  const l = parseHex(left) ?? parseHex('#2563eb');
  const r = parseHex(right) ?? parseHex('#ffffff');
  if (!l || !r) return left;
  const mix = (a: number, b: number) => Math.round(a * (1 - amount) + b * amount);
  return `#${[mix(l[0], r[0]), mix(l[1], r[1]), mix(l[2], r[2])].map((value) => value.toString(16).padStart(2, '0')).join('')}`;
}

function parseHex(value: string) {
  const match = /^#?([0-9a-f]{6})$/i.exec(value.trim());
  if (!match) return null;
  const raw = match[1];
  return [Number.parseInt(raw.slice(0, 2), 16), Number.parseInt(raw.slice(2, 4), 16), Number.parseInt(raw.slice(4, 6), 16)] as const;
}

function stringProp(value: unknown) {
  return typeof value === 'string' ? value : '';
}

function numberProp(value: unknown, fallback: number | null) {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

function booleanProp(value: unknown, fallback: boolean) {
  return typeof value === 'boolean' ? value : fallback;
}

function stringify(value: unknown) {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return '';
}
