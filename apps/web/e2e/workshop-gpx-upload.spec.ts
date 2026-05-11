import fs from 'node:fs';
import path from 'node:path';

import { expect, test, type Page } from '@playwright/test';

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
const baseTrails = readJson<DemoRow[]>('expected/trails.golden.json');
const baseEstimates = readJson<DemoRow[]>('expected/trail_effort_estimates.golden.json');
const coffeeShops = readJson<DemoRow[]>('fixtures/coffee_shops.json');
const recommendations = readJson<DemoRow[]>('expected/trail_coffee_recommendations.golden.json');
const links = readJson<DemoRow[]>('expected/trail_coffee_links.golden.json');
const weatherSnapshot = readJson<DemoRow>('expected/trail_weather_snapshot.golden.json');
const customTrail = readJson<DemoRow[]>('expected/custom_gpx_upload_trail.golden.json')[0];
const customEstimate = readJson<DemoRow[]>('expected/custom_gpx_upload_estimate.golden.json')[0];

const appResponse = {
  app,
  embed: { url: '/apps/runtime/run-fast', iframe_html: '' },
  published_version_number: 1,
  published_at: now,
};

test('custom GPX upload creates a Trail and effort estimate from Workshop runtime', async ({ page }) => {
  const calls = await mockTrailRunningRuntime(page);

  await page.goto('/apps/runtime/run-fast');
  await page.locator('nav[aria-label="App pages"] button', { hasText: 'Upload GPX' }).click();

  await expect(page.getByRole('heading', { name: 'Uploaded Trails' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Uploaded Effort Estimates' })).toBeVisible();
  const uploadedTrails = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Uploaded Trails' }) });
  const uploadedEstimates = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'Uploaded Effort Estimates' }) });
  await expect(uploadedTrails).not.toContainText('Custom Dawn Ridge');

  await page.locator('input[type="file"]').setInputFiles(path.join(demoRoot, 'fixtures/gpx/custom_dawn_ridge.gpx'));
  await expect(page.getByText('custom_dawn_ridge.gpx')).toBeVisible();
  await page.getByRole('button', { name: 'Submit' }).click();

  await expect(uploadedTrails).toContainText('Custom Dawn Ridge');
  await expect(uploadedEstimates).toContainText('Custom Dawn Ridge');
  await expect(uploadedEstimates).toContainText(String(customEstimate.estimated_pace_min_per_mile));
  await expect(uploadedEstimates).toContainText(String(customEstimate.estimated_max_heartrate));
  expect(calls.gpxParse).toBe(1);
  expect(calls.estimateFunction).toBe(1);
});

async function mockTrailRunningRuntime(page: Page) {
  const objectRows = createObjectRows();
  const calls = { gpxParse: 0, estimateFunction: 0 };

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
  await page.route('**/api/v1/pipelines/geospatial/gpx/parse', async (route) => {
    calls.gpxParse += 1;
    const payload = route.request().postDataBuffer();
    if (!payload || !payload.toString('utf8').includes('custom_dawn_ridge.gpx')) {
      throw new Error('GPX upload did not send the expected multipart file');
    }
    await route.fulfill({
      json: {
        row: backendGPXRow(customTrail),
        schema: [],
        trail: { trail_id: customTrail.trail_id, trail_name: customTrail.trail_name },
        meta: { runtime: 'lightweight_table', transform_type: 'gpx_parse', rows_affected: 1 },
      },
    });
  });
  await page.route('**/api/v1/ontology/functions/estimateTrailEffort/simulate', async (route) => {
    calls.estimateFunction += 1;
    const body = postDataJSON(route.request());
    if (body?.object_type_id !== 'TrailEffortEstimate') {
      throw new Error(`unexpected function target object type: ${String(body?.object_type_id)}`);
    }
    await route.fulfill({
      json: {
        package: {
          id: 'estimateTrailEffort',
          name: 'estimateTrailEffort',
          display_name: 'Estimate Trail Effort',
          version: '1.0.0',
          runtime: 'python',
          status: 'active',
        },
        preview: { rows: [customEstimate] },
        result: { estimate: customEstimate },
      },
    });
  });
  await page.route('**/api/v1/ontology/types', async (route) => {
    await route.fulfill({
      json: {
        data: objectTypeDefinitions(objectRows),
        total: Object.keys(objectRows).length,
        page: 1,
        per_page: 200,
      },
    });
  });
  await page.route(/\/api\/v1\/ontology\/types\/([^/]+)\/properties$/, async (route) => {
    const typeId = decodeURIComponent(new URL(route.request().url()).pathname.split('/').at(-2) ?? '');
    await route.fulfill({ json: { data: propertyDefinitions(typeId, objectRows) } });
  });
  await page.route(/\/api\/v1\/ontology\/types\/([^/]+)\/objects(\/query)?$/, async (route) => {
    const url = new URL(route.request().url());
    const segments = url.pathname.split('/');
    const typeId = decodeURIComponent(segments[5] ?? '');
    const body = route.request().method() === 'POST' ? postDataJSON(route.request()) : {};
    if (route.request().method() === 'POST' && url.pathname.endsWith('/objects')) {
      const object = createObject(typeId, body?.properties ?? {}, objectRows);
      await route.fulfill({ json: object });
      return;
    }
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

  return calls;
}

function createObjectRows(): Record<string, DemoObject[]> {
  return {
    Trail: toObjects('Trail', baseTrails, 'trail_id'),
    TrailEffortEstimate: toObjects('TrailEffortEstimate', baseEstimates, 'estimate_id'),
    CoffeeShop: toObjects('CoffeeShop', coffeeShops, 'coffee_shop_id'),
    TrailCoffeeRecommendation: toObjects('TrailCoffeeRecommendation', recommendations, 'recommendation_id'),
    TrailCoffeeLink: toObjects('TrailCoffeeLink', links, 'link_id'),
    WeatherSnapshot: toObjects('WeatherSnapshot', [weatherSnapshot], 'weather_snapshot_id'),
  };
}

function backendGPXRow(row: DemoRow) {
  const copy = { ...row };
  copy.trailhead_geo_point = copy.trailhead_geopoint;
  copy.route_geojson = JSON.stringify(copy.route_geojson);
  copy.route_bbox = JSON.stringify(copy.route_bbox);
  delete copy.trailhead_geopoint;
  return copy;
}

function postDataJSON(request: { postDataJSON: () => unknown }) {
  try {
    return request.postDataJSON() ?? {};
  } catch {
    return {};
  }
}

function createObject(typeId: string, properties: DemoRow, rows: Record<string, DemoObject[]>) {
  const primaryKey = primaryKeyFor(typeId);
  const object: DemoObject = {
    id: String(properties[primaryKey] ?? `${typeId}-${(rows[typeId] ?? []).length + 1}`),
    object_type_id: typeId,
    properties,
    created_by: 'e2e',
    created_at: now,
    updated_at: now,
  };
  rows[typeId] = [...(rows[typeId] ?? []), object];
  return object;
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

function objectTypeDefinitions(objectRows: Record<string, DemoObject[]>) {
  return [
    objectType('Trail', 'Trail', 'Trails', 'trail_id', 'trail_name', objectRows),
    objectType('TrailEffortEstimate', 'Trail Effort Estimate', 'Trail Effort Estimates', 'estimate_id', 'trail_name', objectRows),
    objectType('CoffeeShop', 'Coffee Shop', 'Coffee Shops', 'coffee_shop_id', 'name', objectRows),
    objectType('TrailCoffeeRecommendation', 'Trail Coffee Recommendation', 'Trail Coffee Recommendations', 'recommendation_id', 'coffee_shop_name', objectRows),
    objectType('TrailCoffeeLink', 'Trail Coffee Link', 'Trail Coffee Links', 'link_id', 'relationship_label', objectRows),
    objectType('WeatherSnapshot', 'Weather Snapshot', 'Weather Snapshots', 'weather_snapshot_id', 'trail_name', objectRows),
  ];
}

function objectType(id: string, displayName: string, plural: string, primaryKey: string, titleProperty: string, objectRows: Record<string, DemoObject[]>) {
  const properties = propertyDefinitions(id, objectRows);
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
    geopoint_property_names: properties.filter((property) => property.property_type === 'geopoint').map((property) => property.name),
    geoshape_property_names: properties.filter((property) => property.property_type === 'geoshape').map((property) => property.name),
    properties,
    property_count: properties.length,
    owner_id: '00000000-0000-0000-0000-000000000001',
    created_at: now,
    updated_at: now,
  };
}

function propertyDefinitions(typeId: string, objectRows: Record<string, DemoObject[]>) {
  const keys = Array.from(new Set((objectRows[typeId] ?? []).flatMap((object) => Object.keys(object.properties))));
  return keys.map((name) => ({
    id: `${typeId}.${name}`,
    object_type_id: typeId,
    name,
    display_name: displayName(name),
    description: '',
    property_type: propertyType(typeId, name, objectRows),
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
    const cmp = compareValues(left.properties[propertyName], right.properties[propertyName]);
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

function propertyType(typeId: string, name: string, objectRows: Record<string, DemoObject[]>) {
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
