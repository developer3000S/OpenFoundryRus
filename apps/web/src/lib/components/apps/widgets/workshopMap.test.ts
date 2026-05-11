import { describe, expect, it } from 'vitest';

import type { ObjectInstance } from '@/lib/api/ontology';
import type { QueryResult } from '@/lib/api/queries';

import {
  buildFeaturesFromGeospatialLayer,
  buildFeaturesFromLinkedEdges,
  buildFeaturesFromConfiguredLayers,
  buildFeaturesFromObjects,
  buildFeaturesFromQueryResult,
  buildMapTemplateRenderRequest,
  collectFeatureBounds,
  createWorkshopMapStyle,
  isWorkshopMapLayerVisible,
  mergeMapTemplateWidgetProps,
  normalizeSavedOverlayConfig,
  readMapLayerConfigs,
  readMapOverlayConfigs,
} from './workshopMap';

describe('Workshop Map widget feature shaping', () => {
  it('builds point and line features from tabular rows with layer filters', () => {
    const result: QueryResult = {
      columns: [
        { name: 'kind', data_type: 'text' },
        { name: 'label', data_type: 'text' },
        { name: 'lat', data_type: 'double' },
        { name: 'lon', data_type: 'double' },
        { name: 'geometry', data_type: 'json' },
      ],
      rows: [
        ['trail_start', 'Mesa Trailhead', '39.999', '-105.289', ''],
        ['coffee', 'Boxcar Coffee', '40.018', '-105.278', ''],
        ['trail_route', 'Mesa route', '', '', '{"type":"LineString","coordinates":[[-105.289,39.999],[-105.278,40.018]]}'],
      ],
      total_rows: 3,
      execution_time_ms: 1,
    };

    const layers = readMapLayerConfigs({
      layers: [
        {
          id: 'trail-starts',
          title: 'Trail starts',
          source: 'binding',
          geometry_type: 'point',
          latitude_field: 'lat',
          longitude_field: 'lon',
          label_field: 'label',
          filter_field: 'kind',
          filter_value: 'trail_start',
          color: '#7c3aed',
          cluster_enabled: true,
          cluster_radius: 88,
          cluster_max_zoom: 9,
          cluster_color: '#6d28d9',
        },
        {
          id: 'coffee',
          title: 'Coffee shops',
          source: 'binding',
          geometry_type: 'point',
          latitude_field: 'lat',
          longitude_field: 'lon',
          label_field: 'label',
          filter_field: 'kind',
          filter_value: 'coffee',
          color: '#0f766e',
        },
        {
          id: 'routes',
          title: 'Routes',
          source: 'binding',
          geometry_type: 'line',
          geometry_field: 'geometry',
          label_field: 'label',
          filter_field: 'kind',
          filter_value: 'trail_route',
          color: '#c2410c',
        },
      ],
    });

    const collection = buildFeaturesFromQueryResult(result, layers);

    expect(collection.features).toHaveLength(3);
    expect(collection.features.map((feature) => feature.properties.__of_layer_id)).toEqual([
      'trail-starts',
      'coffee',
      'routes',
    ]);
    expect(collection.features[0].geometry).toEqual({ type: 'Point', coordinates: [-105.289, 39.999] });
    expect(collection.features[0].properties.__of_cluster_enabled).toBe(true);
    expect(collection.features[0].properties.__of_cluster_radius).toBe(88);
    expect(collection.features[0].properties.__of_cluster_max_zoom).toBe(9);
    expect(collection.features[0].properties.__of_cluster_color).toBe('#6d28d9');
    expect(collection.features[2].geometry.type).toBe('LineString');
    expect(collectFeatureBounds(collection)).toEqual([[-105.289, 39.999], [-105.278, 40.018]]);
  });

  it('builds selectable object features from ontology objects', () => {
    const layer = readMapLayerConfigs({
      layers: [
        {
          id: 'trail-polylines',
          title: 'Trail polylines',
          source: 'object_set',
          source_variable_id: 'trail-variable',
          geometry_type: 'line',
          geometry_field: 'route_geojson',
          label_field: 'trail_name',
          color: '#f97316',
        },
      ],
    })[0];
    const objects: ObjectInstance[] = [
      {
        id: 'trail-1',
        object_type_id: 'Trail',
        properties: {
          trail_name: 'Betasso Preserve',
          route_geojson: {
            type: 'LineString',
            coordinates: [
              [-105.34458, 40.016353],
              [-105.342, 40.018],
            ],
          },
        },
        created_by: 'test',
        created_at: '2026-05-11T00:00:00Z',
        updated_at: '2026-05-11T00:00:00Z',
      },
    ];

    const features = buildFeaturesFromObjects(objects, layer, {
      id: 'trail-variable',
      name: 'Trails',
      object_type_id: 'Trail',
    });

    expect(features).toHaveLength(1);
    expect(features[0].properties.__of_object_id).toBe('trail-1');
    expect(features[0].properties.__of_label).toBe('Betasso Preserve');
    expect(features[0].properties.__of_object_json).toContain('"id":"trail-1"');
  });

  it('builds visual link features from linked object edges', () => {
    const [trailLayer, coffeeLayer] = readMapLayerConfigs({
      layers: [
        {
          id: 'trails',
          title: 'Trails',
          source: 'object_set',
          source_variable_id: 'selected-trail',
          geometry_type: 'point',
          latitude_field: 'lat',
          longitude_field: 'lon',
          label_field: 'name',
          color: '#7c3aed',
        },
        {
          id: 'coffee',
          title: 'Coffee shops',
          source: 'object_set',
          source_variable_id: 'linked-coffee',
          geometry_type: 'point',
          latitude_field: 'lat',
          longitude_field: 'lon',
          label_field: 'name',
          color: '#0f766e',
        },
      ],
    });
    const features = [
      ...buildFeaturesFromObjects([
        {
          id: 'trail-1',
          object_type_id: 'Trail',
          properties: { name: 'Mesa Trail', lat: 40, lon: -105 },
          created_by: 'test',
          created_at: '2026-05-11T00:00:00Z',
          updated_at: '2026-05-11T00:00:00Z',
        },
      ], trailLayer),
      ...buildFeaturesFromObjects([
        {
          id: 'coffee-1',
          object_type_id: 'CoffeeShop',
          properties: { name: 'Boxcar Coffee', lat: 40.01, lon: -105.01 },
          created_by: 'test',
          created_at: '2026-05-11T00:00:00Z',
          updated_at: '2026-05-11T00:00:00Z',
        },
      ], coffeeLayer),
    ];

    const links = buildFeaturesFromLinkedEdges(features, [{
      link_id: 'trail_near_coffee:trail-1:coffee-1',
      link_type_id: 'trail_near_coffee',
      source_object_id: 'trail-1',
      target_object_id: 'coffee-1',
      direction: 'outgoing',
      depth: 1,
    }], { layerId: 'links-coffee', layerTitle: 'Coffee links', color: '#ea580c' });

    expect(links).toHaveLength(1);
    expect(links[0].geometry).toEqual({ type: 'LineString', coordinates: [[-105, 40], [-105.01, 40.01]] });
    expect(links[0].properties.__of_source).toBe('link');
    expect(links[0].properties.__of_locked).toBe(true);
  });

  it('adds GeoJSON and MVT overlay layers to the MapLibre style', () => {
    const overlays = readMapOverlayConfigs({
      overlay_layers: [
        {
          id: 'open-space',
          title: 'Open space boundary',
          source: 'geojson_url',
          url: '/fixtures/open-space.geojson',
          geometry_type: 'polygon',
          color: '#64748b',
          visible: true,
        },
        {
          id: 'contours',
          title: 'Contour MVT',
          source: 'mvt_url',
          url: 'https://tiles.example/contours/{z}/{x}/{y}.pbf',
          source_layer: 'contours',
          geometry_type: 'line',
          color: '#475569',
          visible: true,
        },
      ],
    });

    const style = createWorkshopMapStyle({ base_layer_kind: 'blank' }, overlays);

    expect(style.sources).toHaveProperty('of-overlay-source-open-space');
    expect(style.sources).toHaveProperty('of-overlay-source-contours');
    expect(style.layers.map((layer) => layer.id)).toEqual(
      expect.arrayContaining(['of-overlay-fill-open-space', 'of-overlay-vector-line-contours']),
    );
  });

  it('normalizes saved map layer resources into inline GeoJSON overlays', () => {
    const [overlay] = readMapOverlayConfigs({
      overlay_layers: [
        {
          id: 'saved-boundary',
          source: 'saved_map_layer',
          resource_id: 'layer-open-space',
          geometry_type: 'auto',
          color: '#64748b',
        },
      ],
    });

    const normalized = normalizeSavedOverlayConfig(overlay, {
      id: 'layer-open-space',
      name: 'Open space saved layer',
      geometry_type: 'polygon',
      style: { color: '#0f766e' },
      features: [
        {
          id: 'boundary-1',
          label: 'Boundary',
          geometry: {
            type: 'polygon',
            coordinates: [
              { lat: 40, lon: -105.3 },
              { lat: 40.02, lon: -105.3 },
              { lat: 40.02, lon: -105.25 },
              { lat: 40, lon: -105.3 },
            ],
          },
          properties: { kind: 'park' },
        },
      ],
    });

    expect(normalized.source).toBe('geojson_url');
    expect(normalized.data?.features).toHaveLength(1);
    expect(normalized.data?.features[0].geometry.type).toBe('Polygon');
  });

  it('builds viewport tile features from saved geospatial layer pages', () => {
    const [layer] = readMapLayerConfigs({
      layers: [
        {
          id: 'large-trails',
          title: 'Large trails',
          source: 'geospatial_tile',
          tile_layer_id: '11111111-1111-4111-8111-111111111111',
          tile_page_size: 250,
          tile_simplify_tolerance: 0.0005,
          geometry_type: 'line',
          color: '#2563eb',
          line_width: 2,
          locked: true,
        },
      ],
    });

    const features = buildFeaturesFromGeospatialLayer([
      {
        id: 'tile-route-1',
        label: 'Viewport route',
        geometry: {
          type: 'line_string',
          coordinates: [
            { lat: 40.016353, lon: -105.34458 },
            { lat: 40.01851, lon: -105.28291 },
          ],
        },
        properties: { kind: 'trail_route' },
      },
    ], layer);

    expect(layer.source).toBe('geospatial_tile');
    expect(layer.loading_mode).toBe('viewport_tiles');
    expect(features).toHaveLength(1);
    expect(features[0].geometry).toEqual({ type: 'LineString', coordinates: [[-105.34458, 40.016353], [-105.28291, 40.01851]] });
    expect(features[0].properties.__of_source).toBe('geospatial_tile');
    expect(features[0].properties.__of_tile_layer_id).toBe('11111111-1111-4111-8111-111111111111');
    expect(features[0].properties.__of_locked).toBe(true);
  });

  it('materializes saved map template props for Workshop runtime layers and overlays', () => {
    const rendered = {
      template_id: 'template-trails',
      template_name: 'Trail template',
      widget_props: {
        center_lat: 40.016,
        center_lon: -105.29,
        zoom: 11,
        layers: [
          {
            id: 'trail-style',
            title: 'Trail starts',
            source: 'object_set',
            source_variable_id: 'var-trails',
            geometry_type: 'point',
            latitude_field: 'start_lat',
            longitude_field: 'start_lon',
            color: '#16a34a',
          },
          {
            id: 'coffee-constant',
            title: 'Coffee',
            source: 'binding',
            geometry_type: 'point',
            color: '#92400e',
            features: [
              {
                id: 'coffee-1',
                label: 'Trailhead Coffee',
                geometry: { type: 'point', coordinates: { lat: 40.0161, lon: -105.3441 } },
                properties: { kind: 'coffee' },
              },
            ],
          },
        ],
        overlay_layers: [
          {
            id: 'open-space',
            title: 'Open space',
            source: 'geojson_url',
            url: '/fixtures/open-space.geojson',
            geometry_type: 'polygon',
          },
        ],
      },
    };

    const props = mergeMapTemplateWidgetProps({
      map_template_id: 'template-trails',
      output_object_set_variable_id: 'selectedTrail',
    }, rendered);
    const layers = readMapLayerConfigs(props);
    const overlays = readMapOverlayConfigs(props);
    const constantFeatures = buildFeaturesFromConfiguredLayers(layers);

    expect(props.output_object_set_variable_id).toBe('selectedTrail');
    expect(layers).toHaveLength(2);
    expect(layers[0].source).toBe('object_set');
    expect(layers[0].source_variable_id).toBe('var-trails');
    expect(overlays).toHaveLength(1);
    expect(constantFeatures).toHaveLength(1);
    expect(constantFeatures[0].properties.__of_source).toBe('map_template');
    expect(constantFeatures[0].geometry).toEqual({ type: 'Point', coordinates: [-105.3441, 40.0161] });
  });

  it('builds map template render requests from parameter mappings', () => {
    const request = buildMapTemplateRenderRequest({
      template_parameter_values: { radius: 3.5 },
      template_parameter_mappings: [
        { parameter_id: 'trail_set', variable_id: 'Trail selection' },
      ],
    }, [
      { id: 'var-trails', name: 'Trail selection', object_type_id: 'Trail' },
    ]);

    expect(request.parameter_values).toEqual({ radius: 3.5 });
    expect(request.variable_mappings).toEqual({ trail_set: 'var-trails' });
  });

  it('resolves layer visibility from boolean-like variables with static fallback', () => {
    const [layer] = readMapLayerConfigs({
      layers: [
        {
          id: 'coffee',
          title: 'Coffee shops',
          source: 'object_set',
          visibility_variable_id: 'show-coffee',
          visible: true,
        },
      ],
    });
    const engine = {
      getPrimitive: (variableId: string) => (variableId === 'show-coffee' ? 'false' : undefined),
    };

    expect(isWorkshopMapLayerVisible(layer, engine)).toBe(false);
    expect(isWorkshopMapLayerVisible({ ...layer, visibility_variable_id: '' }, engine)).toBe(true);
    expect(isWorkshopMapLayerVisible({ ...layer, visibility_variable_id: 'missing', visible: false }, engine)).toBe(false);
  });
});
