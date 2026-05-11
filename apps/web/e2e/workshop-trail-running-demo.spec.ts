import fs from 'node:fs';
import path from 'node:path';

import { expect, test, type Page, type TestInfo } from '@playwright/test';

const now = '2026-05-11T00:00:00Z';
const demoRoot = path.resolve(process.cwd(), '../../tools/demo/trail-running');

function readJson<T>(relativePath: string): T {
  return JSON.parse(fs.readFileSync(path.join(demoRoot, relativePath), 'utf8')) as T;
}

type DemoRow = Record<string, unknown>;

type DemoObject = {
  id: string;
  object_type_id: string;
  properties: DemoRow;
  created_by: string;
  created_at: string;
  updated_at: string;
};

const app = readJson<DemoRow>('workshop/run_fast.workshop_app.json');
const trails = readJson<DemoRow[]>('expected/trails.golden.json');
const estimates = readJson<DemoRow[]>('expected/trail_effort_estimates.golden.json');
const coffeeShops = readJson<DemoRow[]>('fixtures/coffee_shops.json');
const recommendations = readJson<DemoRow[]>('expected/trail_coffee_recommendations.golden.json');
const links = readJson<DemoRow[]>('expected/trail_coffee_links.golden.json');
const weatherSnapshot = readJson<DemoRow>('expected/trail_weather_snapshot.golden.json');

test.use({ trace: 'on' });

const objectRows: Record<string, DemoObject[]> = {
  Trail: toObjects('Trail', trails, 'trail_id'),
  TrailEffortEstimate: toObjects('TrailEffortEstimate', estimates, 'estimate_id'),
  CoffeeShop: toObjects('CoffeeShop', coffeeShops, 'coffee_shop_id'),
  TrailCoffeeRecommendation: toObjects('TrailCoffeeRecommendation', recommendations, 'recommendation_id'),
  TrailCoffeeLink: toObjects('TrailCoffeeLink', links, 'link_id'),
  WeatherSnapshot: toObjects('WeatherSnapshot', [weatherSnapshot], 'weather_snapshot_id'),
};

const appResponse = {
  app,
  embed: { url: '/apps/runtime/run-fast', iframe_html: '' },
  published_version_number: 1,
  published_at: now,
};

type TrailRunningRuntimeMocks = {
  weatherValidateRequests: unknown[];
  weatherExecuteRequests: unknown[];
};

test('Trail Running Workshop demo publishes list, filters, actions, charts, map, and detail pages', async ({ page }, testInfo) => {
  const mocks: TrailRunningRuntimeMocks = {
    weatherValidateRequests: [],
    weatherExecuteRequests: [],
  };
  await mockTrailRunningRuntime(page, mocks);

  await page.goto('/apps/runtime/run-fast');

  const trailTable = page.getByRole('table').first();
  await expect(page.getByRole('button', { name: 'Trail Overview' })).toHaveClass(/is-active/);
  await expect(page.getByRole('heading', { name: 'Trail List' })).toBeVisible();
  await expect(trailTable).toContainText('Boulder Creek Path East');
  await expect(trailTable).toContainText('Green Mountain Ascent');

  await page.getByLabel('Trail Name values').fill('Green Mountain Ascent');
  await expect(trailTable).toContainText('Green Mountain Ascent');
  await expect(trailTable).not.toContainText('Boulder Creek Path East');
  await page.getByLabel('Trail Name values').fill('');
  await expect(trailTable).toContainText('Boulder Creek Path East');

  await page.getByLabel('Select Green Mountain Ascent').check();
  await expect(page.getByLabel('Select Green Mountain Ascent')).toBeChecked();
  await expect(page.getByRole('heading', { name: 'Trail Distances' })).toBeVisible();
  await expect(page.locator('canvas').first()).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Weather Conditions' })).toBeVisible();
  await expect(page.getByText('68.4 Fahrenheit')).toBeVisible();
  await expect(page.getByText('6.2 mph')).toBeVisible();
  await page.getByRole('button', { name: 'Get Weather' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByPlaceholder('Trail ID')).toHaveValue('boulder-creek-path');
  await expect(page.getByPlaceholder('Trail Name')).toHaveValue('Boulder Creek Path East');
  await expect(page.getByPlaceholder('Latitude')).toHaveValue('40.0153');
  await expect(page.getByPlaceholder('Longitude')).toHaveValue('-105.289');
  await expect(page.getByPlaceholder('Trailhead GeoPoint')).toHaveValue('40.0153,-105.289');
  await page.getByRole('button', { name: 'Submit' }).click();
  await expect(page.getByRole('status')).toContainText('Object saved successfully.');
  await expect.poll(() => mocks.weatherValidateRequests.length).toBe(1);
  await expect.poll(() => mocks.weatherExecuteRequests.length).toBe(1);
  const executeBody = mocks.weatherExecuteRequests[0] as { parameters?: Record<string, unknown> };
  expect(executeBody.parameters?.trail_id).toBe('boulder-creek-path');
  expect(executeBody.parameters?.trail_name).toBe('Boulder Creek Path East');
  expect(Number(executeBody.parameters?.latitude)).toBeCloseTo(40.0153, 4);
  expect(Number(executeBody.parameters?.longitude)).toBeCloseTo(-105.289, 4);
  expect(executeBody.parameters?.trailhead_geopoint).toBe('40.0153,-105.289');
  await attachScreenshot(testInfo, page, 'trail-running-overview');
  await page.getByLabel('Dismiss').click();

  await page.getByRole('button', { name: 'Trail Map' }).click();
  await expect(page.getByRole('button', { name: 'Trail Map' })).toHaveClass(/is-active/);
  await expect(page.getByRole('heading', { name: 'Trail Starts & Coffee Shops' })).toBeVisible();
  await expect(page.getByTestId('workshop-map-widget')).toBeVisible();
  await expect(page.getByTestId('workshop-map-layer-toggle-trail-starts')).toContainText('Trail Starts');
  await expect(page.getByTestId('workshop-map-layer-toggle-coffee-shops')).toContainText('Coffee Shops');
  await expect(page.getByRole('heading', { name: 'Nearest Coffee Shops' })).toBeVisible();
  await expect(page.getByRole('table').first()).toContainText('Pine Ridge Coffee');
  await attachScreenshot(testInfo, page, 'trail-running-map');

  await page.getByRole('button', { name: 'Trail Detail' }).click();
  await expect(page.getByRole('button', { name: 'Trail Detail' })).toHaveClass(/is-active/);
  await expect(page.getByRole('heading', { name: 'Selected Trail Details' })).toBeVisible();
  await expect(page.locator('section[aria-label="Selected Trail Details"]')).toContainText('Boulder Creek Path East');
  await expect(page.getByRole('heading', { name: 'Effort Estimate' })).toBeVisible();
  await expect(page.getByText('9.14 min/mi')).toBeVisible();
  await expect(page.getByText('165 bpm')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Coffee Recommendations' })).toBeVisible();
  await expect(page.getByRole('table').first()).toContainText('Trailhead Espresso');
  await attachScreenshot(testInfo, page, 'trail-running-detail');
});

async function attachScreenshot(testInfo: TestInfo, page: Page, name: string) {
  await testInfo.attach(name, {
    body: await page.screenshot({ fullPage: true }),
    contentType: 'image/png',
  });
}

async function mockTrailRunningRuntime(page: Page, mocks?: TrailRunningRuntimeMocks) {
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
        created_at: now,
      },
    });
  });
  await page.route('**/api/v1/apps/public/run-fast', async (route) => {
    await route.fulfill({ json: appResponse });
  });
  await page.route('**/api/v1/ontology/types', async (route) => {
    await route.fulfill({
      json: {
        data: objectTypeDefinitions(),
        total: Object.keys(objectRows).length,
        page: 1,
        per_page: 200,
      },
    });
  });
  await page.route(/\/api\/v1\/ontology\/types\/([^/]+)\/properties$/, async (route) => {
    const typeId = decodeURIComponent(new URL(route.request().url()).pathname.split('/').at(-2) ?? '');
    await route.fulfill({ json: { data: propertyDefinitions(typeId) } });
  });
  await page.route(/\/api\/v1\/ontology\/types\/([^/]+)\/objects(\/query)?/, async (route) => {
    const segments = new URL(route.request().url()).pathname.split('/');
    const typeId = decodeURIComponent(segments[5] ?? '');
    const body = route.request().method() === 'POST' ? postDataJSON(route.request()) : {};
    const rows = applyObjectQuery(objectRows[typeId] ?? [], body);
    await route.fulfill({
      json: {
        data: rows,
        total: rows.length,
        count: rows.length,
        page: 1,
        per_page: body?.per_page ?? body?.limit ?? 5000,
        aggregations: [],
      },
    });
  });
  await page.route('**/api/v1/ontology/actions/FetchTrailWeather', async (route) => {
    await route.fulfill({
      json: {
        id: 'FetchTrailWeather',
        name: 'FetchTrailWeather',
        display_name: 'Fetch Trail Weather',
        description: 'Fetch current weather for a trailhead and write WeatherSnapshot.',
        object_type_id: 'WeatherSnapshot',
        operation_kind: 'create_or_modify_object',
        input_schema: [
          { name: 'trail_id', display_name: 'Trail ID', property_type: 'string', required: true, default_value: null },
          { name: 'trail_name', display_name: 'Trail Name', property_type: 'string', required: true, default_value: null },
          { name: 'latitude', display_name: 'Latitude', property_type: 'double', required: true, default_value: null },
          { name: 'longitude', display_name: 'Longitude', property_type: 'double', required: true, default_value: null },
          { name: 'trailhead_geopoint', display_name: 'Trailhead GeoPoint', property_type: 'geopoint', required: true, default_value: null },
        ],
        form_schema: { sections: [], parameter_overrides: [] },
        config: { webhook_id: 'open_meteo_current_weather' },
        confirmation_required: false,
        created_at: now,
        updated_at: now,
      },
    });
  });
  await page.route('**/api/v1/ontology/actions/FetchTrailWeather/validate', async (route) => {
    const body = postDataJSON(route.request());
    mocks?.weatherValidateRequests.push(body);
    await route.fulfill({
      json: {
        valid: true,
        errors: [],
        preview: { kind: 'create_or_modify_object', target_object_id: objectRows.WeatherSnapshot[0]?.id ?? null },
      },
    });
  });
  await page.route('**/api/v1/ontology/actions/FetchTrailWeather/execute', async (route) => {
    const body = postDataJSON(route.request());
    mocks?.weatherExecuteRequests.push(body);
    await route.fulfill({
      json: {
        action: {
          id: 'FetchTrailWeather',
          name: 'FetchTrailWeather',
          display_name: 'Fetch Trail Weather',
          object_type_id: 'WeatherSnapshot',
          operation_kind: 'create_or_modify_object',
        },
        target_object_id: objectRows.WeatherSnapshot[0]?.id ?? null,
        deleted: false,
        preview: { kind: 'create_or_modify_object', target_object_id: objectRows.WeatherSnapshot[0]?.id ?? null },
        object: objectRows.WeatherSnapshot[0] ?? null,
        link: null,
        result: { webhook_id: 'open_meteo_current_weather', status: 'succeeded' },
      },
    });
  });
}

function postDataJSON(request: { postDataJSON: () => unknown }) {
  try {
    return request.postDataJSON() ?? {};
  } catch {
    return {};
  }
}

function toObjects(objectTypeId: string, rows: DemoRow[], primaryKey: string): DemoObject[] {
  return rows.map((row, index) => ({
    id: String(row[primaryKey] ?? `${objectTypeId}-${index + 1}`),
    object_type_id: objectTypeId,
    properties: row,
    created_by: 'e2e',
    created_at: now,
    updated_at: now,
  }));
}

function objectTypeDefinitions() {
  return [
    objectType('Trail', 'Trail', 'Trails', 'trail_id', 'trail_name'),
    objectType('TrailEffortEstimate', 'Trail Effort Estimate', 'Trail Effort Estimates', 'estimate_id', 'trail_name'),
    objectType('CoffeeShop', 'Coffee Shop', 'Coffee Shops', 'coffee_shop_id', 'name'),
    objectType('TrailCoffeeRecommendation', 'Trail Coffee Recommendation', 'Trail Coffee Recommendations', 'recommendation_id', 'coffee_shop_name'),
    objectType('TrailCoffeeLink', 'Trail Coffee Link', 'Trail Coffee Links', 'link_id', 'relationship_label'),
    objectType('WeatherSnapshot', 'Weather Snapshot', 'Weather Snapshots', 'weather_snapshot_id', 'trail_name'),
  ];
}

function objectType(id: string, displayName: string, plural: string, primaryKey: string, titleProperty: string) {
  return {
    id,
    api_name: id,
    name: id,
    display_name: displayName,
    plural_display_name: plural,
    description: '',
    primary_key: primaryKey,
    primary_key_property: primaryKey,
    title_property: titleProperty,
    icon: null,
    color: null,
    visibility: 'normal',
    status: 'active',
    editable: true,
    searchable_property_names: [titleProperty, primaryKey],
    geopoint_property_names: propertyDefinitions(id).filter((property) => property.property_type === 'geopoint').map((property) => property.name),
    geoshape_property_names: propertyDefinitions(id).filter((property) => property.property_type === 'geoshape').map((property) => property.name),
    properties: propertyDefinitions(id),
    property_count: propertyDefinitions(id).length,
    owner_id: '00000000-0000-0000-0000-000000000001',
    created_at: now,
    updated_at: now,
  };
}

function propertyDefinitions(typeId: string) {
  const keys = Array.from(new Set((objectRows[typeId] ?? []).flatMap((object) => Object.keys(object.properties))));
  return keys.map((name) => ({
    id: `${typeId}.${name}`,
    object_type_id: typeId,
    name,
    display_name: displayName(name),
    description: '',
    property_type: propertyType(typeId, name),
    required: name.endsWith('_id') || name === 'trail_id',
    unique_constraint: primaryKeyFor(typeId) === name,
    time_dependent: false,
    default_value: null,
    validation_rules: null,
    created_at: now,
    updated_at: now,
  }));
}

function applyObjectQuery(rows: DemoObject[], body: any) {
  const filters = Array.isArray(body?.filters) ? body.filters : [];
  const sort = Array.isArray(body?.sort) ? body.sort : [];
  const limit = Number(body?.limit ?? body?.per_page ?? 5000);
  const filtered = rows.filter((object) => filters.every((filter: any) => matchesFilter(object, filter)));
  const sorted = [...filtered].sort((left, right) => compareObjects(left, right, sort));
  return sorted.slice(0, Number.isFinite(limit) && limit > 0 ? limit : 5000);
}

function matchesFilter(object: DemoObject, filter: any) {
  const propertyName = String(filter?.property_name ?? '');
  if (!propertyName) return true;
  const actual = object.properties[propertyName];
  const operator = String(filter?.operator ?? 'equals');
  const expected = filter?.value;
  if (operator === 'contains') return String(actual ?? '').toLowerCase().includes(String(expected ?? '').toLowerCase());
  if (operator === 'in' && Array.isArray(expected)) return expected.map(String).includes(String(actual));
  if (operator === 'gte') return Number(actual) >= Number(expected);
  if (operator === 'lte') return Number(actual) <= Number(expected);
  return String(actual ?? '') === String(expected ?? '');
}

function compareObjects(left: DemoObject, right: DemoObject, sort: any[]) {
  for (const item of sort) {
    const propertyName = String(item?.property_name ?? '');
    if (!propertyName) continue;
    const direction = String(item?.direction ?? 'asc').toLowerCase() === 'desc' ? -1 : 1;
    const a = left.properties[propertyName];
    const b = right.properties[propertyName];
    const cmp = compareValues(a, b);
    if (cmp !== 0) return cmp * direction;
  }
  return left.id.localeCompare(right.id, undefined, { numeric: true, sensitivity: 'base' });
}

function compareValues(a: unknown, b: unknown) {
  const na = typeof a === 'number' ? a : Number(a);
  const nb = typeof b === 'number' ? b : Number(b);
  if (Number.isFinite(na) && Number.isFinite(nb)) return na - nb;
  return String(a ?? '').localeCompare(String(b ?? ''), undefined, { numeric: true, sensitivity: 'base' });
}

function propertyType(typeId: string, name: string) {
  const sample = (objectRows[typeId] ?? []).map((object) => object.properties[name]).find((value) => value !== null && value !== undefined);
  if (name.endsWith('_geojson') || name === 'route_geojson' || name === 'line_geojson') return 'geoshape';
  if (name.endsWith('_geopoint') || name === 'trailhead_geopoint' || name === 'coffee_geopoint') return 'geopoint';
  if (name.endsWith('_ids') || Array.isArray(sample) || (sample && typeof sample === 'object')) return 'json';
  if (typeof sample === 'boolean') return 'boolean';
  if (typeof sample === 'number') return Number.isInteger(sample) && /(^rank$|count$|point_count|direction)/.test(name) ? 'integer' : 'double';
  if (name.endsWith('_time') || name.endsWith('_at')) return 'timestamp';
  return 'string';
}

function primaryKeyFor(typeId: string) {
  if (typeId === 'Trail') return 'trail_id';
  if (typeId === 'TrailEffortEstimate') return 'estimate_id';
  if (typeId === 'CoffeeShop') return 'coffee_shop_id';
  if (typeId === 'TrailCoffeeRecommendation') return 'recommendation_id';
  if (typeId === 'TrailCoffeeLink') return 'link_id';
  if (typeId === 'WeatherSnapshot') return 'weather_snapshot_id';
  return 'id';
}

function displayName(name: string) {
  return name
    .split('_')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}
