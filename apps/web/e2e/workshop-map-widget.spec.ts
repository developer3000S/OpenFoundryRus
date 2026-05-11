import { expect, test } from '@playwright/test';

const appResponse = {
  app: {
    id: 'map-demo-app',
    name: 'Trail Running Map',
    slug: 'trail-running-map',
    description: 'Workshop map widget smoke.',
    status: 'published',
    pages: [
      {
        id: 'main',
        name: 'Main',
        path: '/',
        description: '',
        visible: true,
        layout: { kind: 'grid', columns: 12, gap: '16px', max_width: '1180px' },
        widgets: [
          {
            id: 'trail-map',
            widget_type: 'map',
            title: 'Trail Starts & Coffee Shops',
            description: 'MapLibre runtime with points and a trail polyline.',
            position: { x: 0, y: 0, width: 12, height: 5 },
            props: {
              base_layer_kind: 'blank',
              center_lat: 40.015,
              center_lon: -105.29,
              zoom: 11,
              output_variable_id: 'map-active-object',
              output_object_set_variable_id: 'map-selected-set',
              output_shape_variable_id: 'map-drawn-shape',
              shape_search_output_variable_id: 'map-shape-search',
              enable_shape_drawing: true,
              layers: [
                {
                  id: 'trail-route',
                  title: 'Trail route',
                  source: 'binding',
                  geometry_type: 'line',
                  geometry_field: 'geometry',
                  label_field: 'label',
                  filter_field: 'kind',
                  filter_value: 'trail_route',
                  color: '#c2410c',
                  line_width: 4,
                  visible: true,
                },
                {
                  id: 'trail-starts',
                  title: 'Trail starts',
                  source: 'object_set',
                  source_variable_id: 'trail-objects',
                  geometry_type: 'point',
                  latitude_field: 'lat',
                  longitude_field: 'lon',
                  label_field: 'label',
                  color: '#7c3aed',
                  radius: 7,
                  cluster_enabled: true,
                  cluster_radius: 96,
                  cluster_max_zoom: 9,
                  cluster_color: '#7c3aed',
                  visible: true,
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
                  radius: 6,
                  visible: true,
                },
                {
                  id: 'large-routes',
                  title: 'Viewport trail tiles',
                  source: 'geospatial_tile',
                  loading_mode: 'viewport_tiles',
                  tile_layer_id: '11111111-1111-4111-8111-111111111111',
                  tile_page_size: 2,
                  tile_simplify_tolerance: 0.0005,
                  geometry_type: 'line',
                  label_field: 'label',
                  color: '#2563eb',
                  line_width: 2,
                  locked: true,
                  visible: true,
                },
              ],
              overlay_layers: [
                {
                  id: 'open-space',
                  title: 'Open space boundary',
                  source: 'geojson_url',
                  url: '/fixtures/open-space.geojson',
                  geometry_type: 'polygon',
                  color: '#64748b',
                  opacity: 0.72,
                  fill_opacity: 0.18,
                  visible: true,
                },
              ],
            },
            binding: {
              source_type: 'query',
              query_text: 'select trail and coffee map fixture',
              fields: [],
              parameters: {},
              limit: 50,
            },
            events: [],
            children: [],
          },
          {
            id: 'selected-trail-details',
            widget_type: 'property_list',
            title: 'Selected Trail Details',
            description: 'Property list backed by the map selected object set.',
            position: { x: 0, y: 5, width: 4, height: 2 },
            props: {
              source_variable_id: 'map-selected-set',
              items: [{ id: 'details', property_names: ['label', 'difficulty'] }],
              number_of_columns: 1,
            },
            binding: null,
            events: [],
            children: [],
          },
          {
            id: 'selected-trail-table',
            widget_type: 'object_table',
            title: 'Selected Trail Table',
            description: 'Object table backed by the map selected object set.',
            position: { x: 4, y: 5, width: 4, height: 2 },
            props: {
              source_variable_id: 'map-selected-set',
              columns: ['label', 'difficulty'],
              default_sort_property: '',
              default_sort_direction: 'asc',
            },
            binding: null,
            events: [],
            children: [],
          },
          {
            id: 'selected-trail-actions',
            widget_type: 'button_group',
            title: 'Selected Trail Actions',
            description: 'Button defaults read from the map selected object set.',
            position: { x: 8, y: 5, width: 4, height: 2 },
            props: {
              buttons: [
                {
                  id: 'trail-action',
                  label: 'Plan selected trail',
                  on_click_kind: 'action',
                  action_type_id: 'plan-trail',
                  default_layout: 'form',
                  switch_layout: false,
                  conditional_visibility: false,
                  parameter_defaults: {
                    object: { kind: 'variable', variable_id: 'map-selected-set' },
                  },
                },
              ],
              orientation: 'horizontal',
              fill_horizontal: true,
            },
            binding: null,
            events: [],
            children: [],
          },
          {
            id: 'shape-search-results',
            widget_type: 'object_table',
            title: 'Shape Search Results',
            description: 'Object table backed by the drawn shape search output.',
            position: { x: 0, y: 7, width: 12, height: 2 },
            props: {
              source_variable_id: 'map-shape-search',
              columns: ['label', 'difficulty'],
              default_sort_property: '',
              default_sort_direction: 'asc',
            },
            binding: null,
            events: [],
            children: [],
          },
        ],
      },
    ],
    theme: {
      name: 'Trail',
      primary_color: '#0f766e',
      accent_color: '#c2410c',
      background_color: '#f8fafc',
      surface_color: '#ffffff',
      text_color: '#0f172a',
      heading_font: 'Inter',
      body_font: 'Inter',
      border_radius: 8,
      logo_url: null,
    },
    settings: {
      home_page_id: 'main',
      navigation_style: 'none',
      max_width: '1180px',
      show_branding: false,
      custom_css: null,
      builder_experience: 'workshop',
      ontology_source_type_id: null,
      object_set_variables: [],
      workshop_variables: [
        {
          id: 'trail-objects',
          kind: 'object_set',
          name: 'Trail objects',
          object_type_id: 'Trail',
        },
        {
          id: 'map-active-object',
          kind: 'object_set_active_object',
          name: 'Map active object',
          object_type_id: 'Trail',
          source_widget_id: 'trail-map',
        },
        {
          id: 'map-selected-set',
          kind: 'object_set_selection',
          name: 'Map selected object set',
          object_type_id: 'Trail',
          source_widget_id: 'trail-map',
        },
        {
          id: 'map-drawn-shape',
          kind: 'shape_output',
          name: 'Map drawn shape',
          object_type_id: '',
          source_widget_id: 'trail-map',
        },
        {
          id: 'map-shape-search',
          kind: 'object_set_selection',
          name: 'Map shape search',
          object_type_id: 'Trail',
          source_widget_id: 'trail-map',
        },
      ],
      consumer_mode: { enabled: false, allow_guest_access: false, portal_title: null, portal_subtitle: null, primary_cta_label: null, primary_cta_url: null },
      interactive_workshop: { enabled: false, title: null, subtitle: null, briefing_template: null, primary_scenario_widget_id: null, primary_agent_widget_id: null, suggested_questions: [], scenario_presets: [] },
      workshop_header: { title: null, icon: null, color: null },
      slate: {
        enabled: false,
        framework: 'react',
        package_name: '',
        entry_file: '',
        sdk_import: '',
        workspace: { enabled: false, repository_id: null, layout: '', runtime: '', dev_command: '', preview_command: '', files: [] },
        quiver_embed: { enabled: false, primary_type_id: null, secondary_type_id: null, join_field: null, secondary_join_field: null, date_field: null, metric_field: null, group_field: null, selected_group: null },
      },
    },
    template_key: null,
    created_by: 'e2e',
    published_version_id: 'version-1',
    created_at: '2026-05-11T00:00:00Z',
    updated_at: '2026-05-11T00:00:00Z',
  },
  embed: { url: '/apps/runtime/trail-running-map', iframe_html: '' },
  published_version_number: 1,
  published_at: '2026-05-11T00:00:00Z',
};

test('renders Workshop Map widget with MapLibre layers and nonblank canvas', async ({ page }) => {
  const tileFeatureRequests: string[] = [];
  await page.addInitScript(() => {
    window.localStorage.setItem('of_access_token', 'e2e-token');
  });
  await page.route('**/api/v1/auth/bootstrap-status', async (route) => {
    await route.fulfill({ json: { requires_initial_admin: false } });
  });
  await page.route('**/api/v1/users/me', async (route) => {
    await route.fulfill({
      json: {
        id: '00000000-0000-0000-0000-000000000001',
        email: 'runner@example.com',
        name: 'Trail Runner',
        is_active: true,
        roles: ['admin'],
        groups: [],
        permissions: ['*'],
        organization_id: null,
        attributes: {},
        mfa_enabled: false,
        mfa_enforced: false,
        auth_source: 'local',
        created_at: '2026-05-11T00:00:00Z',
      },
    });
  });
  await page.route('**/api/v1/apps/public/trail-running-map', async (route) => {
    await route.fulfill({ json: appResponse });
  });
  await page.route('**/api/v1/ontology/types/Trail/properties', async (route) => {
    await route.fulfill({
      json: {
        data: [
          { id: 'prop-label', object_type_id: 'Trail', name: 'label', display_name: 'Trail name', description: '', property_type: 'string', required: false, unique_constraint: false, time_dependent: false, default_value: null, validation_rules: null, created_at: '2026-05-11T00:00:00Z', updated_at: '2026-05-11T00:00:00Z' },
          { id: 'prop-difficulty', object_type_id: 'Trail', name: 'difficulty', display_name: 'Difficulty', description: '', property_type: 'string', required: false, unique_constraint: false, time_dependent: false, default_value: null, validation_rules: null, created_at: '2026-05-11T00:00:00Z', updated_at: '2026-05-11T00:00:00Z' },
          { id: 'prop-lat', object_type_id: 'Trail', name: 'lat', display_name: 'Latitude', description: '', property_type: 'double', required: false, unique_constraint: false, time_dependent: false, default_value: null, validation_rules: null, created_at: '2026-05-11T00:00:00Z', updated_at: '2026-05-11T00:00:00Z' },
          { id: 'prop-lon', object_type_id: 'Trail', name: 'lon', display_name: 'Longitude', description: '', property_type: 'double', required: false, unique_constraint: false, time_dependent: false, default_value: null, validation_rules: null, created_at: '2026-05-11T00:00:00Z', updated_at: '2026-05-11T00:00:00Z' },
        ],
      },
    });
  });
  await page.route('**/api/v1/ontology/types/Trail/objects**', async (route) => {
    await route.fulfill({
      json: {
        data: [
          {
            id: 'trail-1',
            object_type_id: 'Trail',
            properties: {
              label: 'Betasso trailhead',
              difficulty: 'Moderate',
              lat: 40.016353,
              lon: -105.34458,
            },
            created_by: 'e2e',
            created_at: '2026-05-11T00:00:00Z',
            updated_at: '2026-05-11T00:00:00Z',
          },
          {
            id: 'trail-2',
            object_type_id: 'Trail',
            properties: {
              label: 'Walker trailhead',
              difficulty: 'Hard',
              lat: 40.025,
              lon: -105.25,
            },
            created_by: 'e2e',
            created_at: '2026-05-11T00:00:00Z',
            updated_at: '2026-05-11T00:00:00Z',
          },
          {
            id: 'trail-3',
            object_type_id: 'Trail',
            properties: {
              label: 'Mesa trailhead',
              difficulty: 'Easy',
              lat: 39.9995,
              lon: -105.2888,
            },
            created_by: 'e2e',
            created_at: '2026-05-11T00:00:00Z',
            updated_at: '2026-05-11T00:00:00Z',
          },
          {
            id: 'trail-4',
            object_type_id: 'Trail',
            properties: {
              label: 'Sanitas trailhead',
              difficulty: 'Moderate',
              lat: 40.0208,
              lon: -105.3051,
            },
            created_by: 'e2e',
            created_at: '2026-05-11T00:00:00Z',
            updated_at: '2026-05-11T00:00:00Z',
          },
          {
            id: 'trail-5',
            object_type_id: 'Trail',
            properties: {
              label: 'Sunshine trailhead',
              difficulty: 'Hard',
              lat: 40.0369,
              lon: -105.3265,
            },
            created_by: 'e2e',
            created_at: '2026-05-11T00:00:00Z',
            updated_at: '2026-05-11T00:00:00Z',
          },
          {
            id: 'trail-6',
            object_type_id: 'Trail',
            properties: {
              label: 'Boulder Creek trailhead',
              difficulty: 'Easy',
              lat: 40.0152,
              lon: -105.292,
            },
            created_by: 'e2e',
            created_at: '2026-05-11T00:00:00Z',
            updated_at: '2026-05-11T00:00:00Z',
          },
        ],
        total: 6,
        page: 1,
        per_page: 5000,
      },
    });
  });
  await page.route('**/api/v1/ontology/types', async (route) => {
    await route.fulfill({ json: { data: [], total: 0, page: 1, per_page: 200 } });
  });
  await page.route('**/api/v1/ontology/actions/plan-trail', async (route) => {
    await route.fulfill({
      json: {
        id: 'plan-trail',
        name: 'plan_trail',
        display_name: 'Plan trail',
        description: '',
        object_type_id: 'Trail',
        input_schema: [
          { name: 'object', display_name: 'Trail', property_type: 'object_reference', required: true },
        ],
        config: { property_mappings: [] },
        created_at: '2026-05-11T00:00:00Z',
        updated_at: '2026-05-11T00:00:00Z',
      },
    });
  });
  await page.route('**/fixtures/open-space.geojson', async (route) => {
    await route.fulfill({
      json: {
        type: 'FeatureCollection',
        features: [
          {
            type: 'Feature',
            properties: { name: 'Boulder Open Space' },
            geometry: {
              type: 'Polygon',
              coordinates: [[
                [-105.36, 39.99],
                [-105.25, 39.99],
                [-105.25, 40.04],
                [-105.36, 40.04],
                [-105.36, 39.99],
              ]],
            },
          },
        ],
      },
    });
  });
  await page.route('**/api/v1/geospatial/tiles/*/features**', async (route) => {
    const url = new URL(route.request().url());
    tileFeatureRequests.push(url.search);
    await route.fulfill({
      json: {
        layer_id: '11111111-1111-4111-8111-111111111111',
        layer_name: 'Large trail routes',
        bounds: {
          min_lat: Number(url.searchParams.get('min_lat')),
          min_lon: Number(url.searchParams.get('min_lon')),
          max_lat: Number(url.searchParams.get('max_lat')),
          max_lon: Number(url.searchParams.get('max_lon')),
        },
        zoom: Number(url.searchParams.get('zoom')),
        simplify_tolerance: Number(url.searchParams.get('simplify_tolerance')),
        limit: Number(url.searchParams.get('limit')),
        offset: Number(url.searchParams.get('offset')),
        next_offset: undefined,
        total_matching_count: 1,
        returned_count: 1,
        features: [
          {
            id: 'tile-route-1',
            label: 'Viewport route segment',
            geometry: {
              type: 'line_string',
              coordinates: [
                { lat: 40.012, lon: -105.348 },
                { lat: 40.016353, lon: -105.34458 },
                { lat: 40.01851, lon: -105.28291 },
              ],
            },
            properties: { kind: 'viewport_route' },
          },
        ],
      },
    });
  });
  await page.route('**/api/v1/queries/execute', async (route) => {
    await route.fulfill({
      json: {
        columns: [
          { name: 'kind', data_type: 'text' },
          { name: 'label', data_type: 'text' },
          { name: 'lat', data_type: 'double' },
          { name: 'lon', data_type: 'double' },
          { name: 'geometry', data_type: 'json' },
        ],
        rows: [
          ['coffee', 'Alpine Modern Cafe', '40.01851', '-105.28291', ''],
          ['trail_route', 'Betasso route', '', '', '{"type":"LineString","coordinates":[[-105.34458,40.016353],[-105.328,40.017],[-105.301,40.019],[-105.28291,40.01851]]}'],
        ],
        total_rows: 2,
        execution_time_ms: 2,
      },
    });
  });

  await page.goto('/apps/runtime/trail-running-map');

  const map = page.getByTestId('workshop-map-widget');
  await expect(map).toBeVisible();
  await expect(page.getByText('Trail route')).toBeVisible();
  await expect(page.getByText('Viewport trail tiles')).toBeVisible();
  await expect(page.getByText('Open space boundary')).toBeVisible();
  await expect(page.getByTestId('workshop-map-feature-count')).toHaveText('9 features');
  await expect.poll(() => tileFeatureRequests.length).toBeGreaterThan(0);
  const tileRequest = new URLSearchParams(tileFeatureRequests[0].replace(/^\?/, ''));
  expect(tileRequest.get('limit')).toBe('2');
  expect(tileRequest.get('offset')).toBe('0');
  expect(tileRequest.has('min_lat')).toBe(true);
  expect(tileRequest.has('max_lon')).toBe(true);
  await expect(page.getByTestId('workshop-map-visible-overlay-count')).toHaveText('1 overlay');
  await page.getByTestId('workshop-map-overlay-toggle-open-space').locator('input').uncheck();
  await expect(page.getByTestId('workshop-map-visible-overlay-count')).toHaveText('0 overlays');
  await page.getByTestId('workshop-map-overlay-toggle-open-space').locator('input').check();
  await expect(page.getByTestId('workshop-map-visible-overlay-count')).toHaveText('1 overlay');

  await page.waitForFunction(() => {
    const maps = (window as any).__openFoundryMapLibreMaps as any[] | undefined;
    const map = maps?.[maps.length - 1];
    if (!map?.loaded?.()) return false;
    const point = map.project([-105.34458, 40.016353]);
    const features = map.queryRenderedFeatures(
      [[point.x - 14, point.y - 14], [point.x + 14, point.y + 14]],
      { layers: ['of-workshop-map-points', 'of-workshop-map-labels', 'of-workshop-map-cluster-points', 'of-workshop-map-cluster-labels'] },
    );
    return features.some((feature: any) => feature.properties?.__of_object_id === 'trail-1');
  });
  const clusterStats = await page.evaluate(async () => {
    const maps = (window as any).__openFoundryMapLibreMaps as any[];
    const map = maps[maps.length - 1];
    map.jumpTo({ center: [-105.3, 40.018], zoom: 6 });
    await new Promise<void>((resolve) => map.once('idle', () => resolve()));
    const canvas = map.getCanvas();
    const viewport = [[0, 0], [canvas.clientWidth, canvas.clientHeight]];
    const clustered = map.queryRenderedFeatures(viewport, { layers: ['of-workshop-map-clusters'] });
    const maxPointCount = Math.max(0, ...clustered.map((feature: any) => Number(feature.properties?.point_count ?? 0)));
    map.jumpTo({ center: [-105.34458, 40.016353], zoom: 13 });
    await new Promise<void>((resolve) => map.once('idle', () => resolve()));
    const unclustered = map.queryRenderedFeatures(viewport, { layers: ['of-workshop-map-cluster-points', 'of-workshop-map-cluster-labels'] });
    return {
      clusterCount: clustered.length,
      maxPointCount,
      unclusteredTrailIds: unclustered.map((feature: any) => feature.properties?.__of_object_id).filter(Boolean),
    };
  });
  expect(clusterStats.clusterCount).toBeGreaterThan(0);
  expect(clusterStats.maxPointCount).toBeGreaterThanOrEqual(2);
  expect(clusterStats.unclusteredTrailIds).toContain('trail-1');
  const drawBox = await page.evaluate(() => {
    const maps = (window as any).__openFoundryMapLibreMaps as any[];
    const map = maps[maps.length - 1];
    const point = map.project([-105.34458, 40.016353]);
    const rect = map.getCanvas().getBoundingClientRect();
    return {
      x1: rect.left + point.x - 42,
      y1: rect.top + point.y - 34,
      x2: rect.left + point.x + 42,
      y2: rect.top + point.y + 34,
    };
  });
  await page.getByTestId('workshop-map-draw-shape').click();
  await expect(page.getByTestId('workshop-map-draw-surface')).toBeVisible();
  await page.mouse.move(drawBox.x1, drawBox.y1);
  await page.mouse.down();
  await page.mouse.move(drawBox.x2, drawBox.y2, { steps: 6 });
  await page.mouse.up();
  await expect(page.getByTestId('workshop-map-shape-match-count')).toHaveText('1 match');
  await expect(page.getByText('Walker trailhead')).not.toBeVisible();
  await expect(page.getByText('Betasso trailhead')).toBeVisible();
  await page.getByTestId('workshop-map-clear-shape').click();
  await expect(page.getByTestId('workshop-map-shape-match-count')).toBeHidden();
  await expect(page.getByText('Betasso trailhead')).not.toBeVisible();

  const clickPoint = await page.evaluate(async () => {
    const maps = (window as any).__openFoundryMapLibreMaps as any[];
    const map = maps[maps.length - 1];
    map.jumpTo({ center: [-105.25, 40.025], zoom: 13 });
    await new Promise<void>((resolve) => map.once('idle', () => resolve()));
    const point = map.project([-105.25, 40.025]);
    const rect = map.getCanvas().getBoundingClientRect();
    return { x: rect.left + point.x, y: rect.top + point.y };
  });
  await page.mouse.click(clickPoint.x, clickPoint.y);

  await expect(page.getByRole('status')).toContainText('Selected Walker trailhead');
  await expect(page.getByRole('cell', { name: 'Hard' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Plan selected trail' })).toBeVisible();
  await page.getByRole('button', { name: 'Plan selected trail' }).click();
  await expect(page.getByRole('dialog')).toContainText('trail-2');

  const screenshot = await map.screenshot();
  expect(screenshot.length).toBeGreaterThan(8_000);

  const canvas = map.locator('canvas').first();
  await expect(canvas).toBeVisible();
  await page.waitForTimeout(750);
  const stats = await canvas.evaluate((node) => {
    const canvasNode = node as HTMLCanvasElement;
    const gl = canvasNode.getContext('webgl2') ?? canvasNode.getContext('webgl');
    if (!gl) return { supported: false, unique: 0, sampled: 0 };
    const width = canvasNode.width;
    const height = canvasNode.height;
    const pixels = new Uint8Array(width * height * 4);
    gl.readPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, pixels);
    const colors = new Set<string>();
    for (let i = 0; i < pixels.length; i += 64) {
      colors.add(`${pixels[i]},${pixels[i + 1]},${pixels[i + 2]},${pixels[i + 3]}`);
    }
    return { supported: true, unique: colors.size, sampled: pixels.length / 4 };
  });
  expect(stats.supported).toBe(true);
  expect(stats.sampled).toBeGreaterThan(1_000);
  expect(stats.unique).toBeGreaterThan(4);
});
