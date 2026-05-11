import {
  evaluateObjectSet,
  listObjects,
  queryObjects,
  type ObjectInstance,
  type ObjectQueryFilter,
  type ObjectQuerySort,
  type ObjectSearchAroundQuery,
  type LinkedObjectEdge,
  type ObjectKnnQuery,
  type ObjectKnnResult,
  type ObjectSetAggregationResult,
  type ObjectSetAggregationSpec,
  type ObjectSetEvaluationResponse,
} from '@/lib/api/ontology';

import {
  variableFiltersForObjectSet,
  type WorkshopVariableEngineResult,
  type WorkshopVariableFilter,
  type WorkshopVariableLike,
} from './workshopVariables';

export interface WorkshopObjectSetExecutionOptions {
  objectTypeId?: string;
  limit?: number;
  sort?: ObjectQuerySort[];
  aggregations?: ObjectSetAggregationSpec[];
  includeCount?: boolean;
}

export interface WorkshopObjectSetExecutionRequest extends WorkshopObjectSetExecutionOptions {
  variableId?: string;
  variable?: WorkshopVariableLike | null;
  variables?: WorkshopVariableLike[];
  engine?: WorkshopVariableEngineResult | null;
}

export interface WorkshopObjectSetExecutionResult {
  data: ObjectInstance[];
  total: number;
  count: number;
  objectTypeId: string;
  source: 'object_type' | 'object_set' | 'selected_objects' | 'saved_object_set' | 'search_around' | 'knn';
  filters: WorkshopVariableFilter[];
  sort: ObjectQuerySort[];
  aggregations: ObjectSetAggregationResult[];
  linkedEdges: LinkedObjectEdge[];
  knnResults: ObjectKnnResult[];
  contract: WorkshopObjectSetQueryContract;
}

export interface WorkshopObjectSetQueryContract {
  object_type_id: string;
  filters: ObjectQueryFilter[];
  sort: ObjectQuerySort[];
  limit: number;
  include_count: boolean;
  aggregations: ObjectSetAggregationSpec[];
  selected_object_ids?: string[];
  search_around?: ObjectSearchAroundQuery;
  knn?: ObjectKnnQuery;
}

export interface WorkshopObjectSetExecutorDependencies {
  listObjects: typeof listObjects;
  queryObjects: typeof queryObjects;
  evaluateObjectSet: typeof evaluateObjectSet;
}

const DEFAULT_LIMIT = 5000;

const DEFAULT_DEPS: WorkshopObjectSetExecutorDependencies = {
  listObjects,
  queryObjects,
  evaluateObjectSet,
};

export async function executeWorkshopObjectSet(
  request: WorkshopObjectSetExecutionRequest,
  deps: WorkshopObjectSetExecutorDependencies = DEFAULT_DEPS,
): Promise<WorkshopObjectSetExecutionResult> {
  return executeObjectSetInternal(request, deps, new Set());
}

export function objectSetQueryFilters(filters: WorkshopVariableFilter[]): ObjectQueryFilter[] {
  return expandFilters(filters).map((filter) => ({
    property_name: filter.property_name,
    operator: filter.operator ?? 'equals',
    value: filter.value,
  }));
}

export function applyObjectSetFilters(objects: ObjectInstance[], filters: WorkshopVariableFilter[]) {
  const expanded = expandFilters(filters);
  if (expanded.length === 0) return objects;
  return objects.filter((object) => expanded.every((filter) => matchesObjectFilter(object, filter)));
}

export function applyObjectSetSort(objects: ObjectInstance[], sort: ObjectQuerySort[]) {
  const normalized = compactSorts(sort);
  if (normalized.length === 0) return objects;
  return [...objects].sort((left, right) => {
    for (const item of normalized) {
      const comparison = compareValues(left.properties?.[item.property_name], right.properties?.[item.property_name]);
      if (comparison === 0) continue;
      return isDescending(item.direction) ? -comparison : comparison;
    }
    return left.id.localeCompare(right.id, undefined, { numeric: true, sensitivity: 'base' });
  });
}

export function computeObjectSetAggregations(
  objects: ObjectInstance[],
  aggregations: ObjectSetAggregationSpec[],
): ObjectSetAggregationResult[] {
  return compactAggregations(aggregations).map((aggregation) => {
    const metric = normalizeAggregationFunction(aggregation.function);
    const propertyName = aggregation.property_name?.trim();
    const id = aggregation.id?.trim() || aggregation.alias?.trim() || [metric, propertyName].filter(Boolean).join(':') || metric;
    if (metric === 'count') {
      const count = propertyName
        ? objects.filter((object) => !isEmptyValue(object.properties?.[propertyName])).length
        : objects.length;
      return { id, alias: aggregation.alias, function: metric, property_name: propertyName, value: count, count };
    }
    if (metric === 'distinct_count' || metric === 'approx_distinct') {
      const seen = new Set<string>();
      for (const object of objects) {
        const value = propertyName ? object.properties?.[propertyName] : object.id;
        if (!isEmptyValue(value)) seen.add(stringValue(value));
      }
      return { id, alias: aggregation.alias, function: metric, property_name: propertyName, value: seen.size, count: seen.size };
    }
    const numbers = propertyName
      ? objects.map((object) => numberValue(object.properties?.[propertyName])).filter((entry): entry is number => entry !== null)
      : [];
    if (numbers.length === 0) {
      return { id, alias: aggregation.alias, function: metric, property_name: propertyName, value: null, count: 0 };
    }
    const total = numbers.reduce((sum, entry) => sum + entry, 0);
    const value = metric === 'sum'
      ? total
      : metric === 'avg' || metric === 'average'
        ? total / numbers.length
        : metric === 'min'
          ? Math.min(...numbers)
          : metric === 'max'
            ? Math.max(...numbers)
            : null;
    return { id, alias: aggregation.alias, function: metric, property_name: propertyName, value, count: numbers.length };
  });
}

async function executeObjectSetInternal(
  request: WorkshopObjectSetExecutionRequest,
  deps: WorkshopObjectSetExecutorDependencies,
  seen: Set<string>,
): Promise<WorkshopObjectSetExecutionResult> {
  const variables = request.variables ?? [];
  const variable = request.variable ?? (request.variableId ? variables.find((entry) => entry.id === request.variableId) ?? null : null);
  const limit = clampLimit(request.limit);
  const engineObjectSet = variable ? request.engine?.getObjectSet(variable.id) : undefined;
  const objectTypeId = request.objectTypeId || variable?.object_type_id || engineObjectSet?.objectTypeId || '';
  const allFilters = variableFiltersForObjectSet(variable, request.engine);
  const ownFilters = variableOwnFilters(variable, request.engine);
  const sort = objectSetQuerySorts(variable, request.sort ?? engineObjectSet?.sort);
  const aggregations = objectSetQueryAggregations(variable, request.aggregations ?? engineObjectSet?.aggregations);
  const includeCount = request.includeCount ?? true;

  if (variable?.kind === 'object_set_selection' || (engineObjectSet?.objects && !variable?.source_variable_id)) {
    const objects = engineObjectSet?.objects ?? (variable ? request.engine?.getSelectedObjectSet(variable.id) : undefined) ?? [];
    const selectedFilters = ownFilters.length > 0 ? ownFilters : allFilters;
    const filtered = applyObjectSetFilters(objects, selectedFilters);
    const ordered = applyObjectSetSort(filtered, sort);
    const limited = ordered.slice(0, limit);
    return objectSetResult({
      data: limited,
      total: filtered.length,
      objectTypeId: objectTypeId || filtered[0]?.object_type_id || '',
      source: 'selected_objects',
      filters: selectedFilters,
      sort,
      limit,
      includeCount,
      aggregations,
      aggregationRows: filtered,
      selectedObjectIds: objects.map((object) => object.id),
    });
  }

  const searchAround = readSearchAroundConfig(variable);
  if (searchAround) {
    const anchors = await executeAnchorObjectSet(searchAround, variables, request.engine, limit, deps, seen);
    if (searchAround.kind === 'linked') {
      return executeLinkedSearchAroundObjectSet(objectTypeId, allFilters, limit, sort, aggregations, includeCount, anchors, searchAround, deps);
    }
    const base = await executeObjectTypeObjectSet(objectTypeId, allFilters, limit, sort, aggregations, includeCount, deps);
    const filtered = applySearchAround(base.data, anchors, searchAround);
    const ordered = applyObjectSetSort(filtered, sort);
    return objectSetResult({
      data: ordered.slice(0, limit),
      total: filtered.length,
      objectTypeId: base.objectTypeId,
      source: 'search_around',
      filters: allFilters,
      sort,
      limit,
      includeCount,
      aggregations,
      aggregationRows: filtered,
    });
  }

  const knn = readKNNConfig(variable, limit);
  if (knn) {
    return executeKNNObjectSet(objectTypeId, allFilters, limit, sort, aggregations, includeCount, knn, variables, request.engine, deps, seen);
  }

  const savedObjectSetId = savedObjectSetIdForVariable(variable);
  if (savedObjectSetId) {
    const response = await deps.evaluateObjectSet(savedObjectSetId, { limit });
    const objects = objectSetEvaluationToObjects(response, objectTypeId);
    const savedFilters = ownFilters.length > 0 ? ownFilters : allFilters;
    const filtered = applyObjectSetFilters(objects, savedFilters);
    const ordered = applyObjectSetSort(filtered, sort);
    return objectSetResult({
      data: ordered.slice(0, limit),
      total: filtered.length,
      objectTypeId: response.object_set.base_object_type_id || objectTypeId,
      source: 'saved_object_set',
      filters: savedFilters,
      sort,
      limit,
      includeCount,
      aggregations,
      aggregationRows: filtered,
    });
  }

  const sourceVariableId = variable?.source_variable_id || stringFromRecord(variable?.metadata, 'source_variable_id');
  if (sourceVariableId) {
    if (seen.has(sourceVariableId)) {
      return objectSetResult({
        data: [],
        total: 0,
        objectTypeId,
        source: 'object_set',
        filters: ownFilters,
        sort,
        limit,
        includeCount,
        aggregations,
      });
    }
    seen.add(sourceVariableId);
    const sourceVariable = variables.find((entry) => entry.id === sourceVariableId) ?? null;
    const source = await executeObjectSetInternal({
      variable: sourceVariable,
      variables,
      engine: request.engine,
      objectTypeId: sourceVariable?.object_type_id || objectTypeId,
      limit,
    }, deps, seen);
    const filtered = applyObjectSetFilters(source.data, ownFilters);
    const ordered = applyObjectSetSort(filtered, sort);
    return objectSetResult({
      data: ordered.slice(0, limit),
      total: filtered.length,
      objectTypeId: objectTypeId || source.objectTypeId,
      source: source.source === 'selected_objects' ? 'selected_objects' : 'object_set',
      filters: [...source.filters, ...ownFilters],
      sort,
      limit,
      includeCount,
      aggregations,
      aggregationRows: filtered,
      selectedObjectIds: source.contract.selected_object_ids,
    });
  }

  return executeObjectTypeObjectSet(objectTypeId, allFilters, limit, sort, aggregations, includeCount, deps);
}

async function executeKNNObjectSet(
  objectTypeId: string,
  filters: WorkshopVariableFilter[],
  limit: number,
  sort: ObjectQuerySort[],
  aggregations: ObjectSetAggregationSpec[],
  includeCount: boolean,
  knn: KNNConfig,
  variables: WorkshopVariableLike[],
  engine: WorkshopVariableEngineResult | null | undefined,
  deps: WorkshopObjectSetExecutorDependencies,
  seen: Set<string>,
): Promise<WorkshopObjectSetExecutionResult> {
  const vector = await resolveKNNVector(knn, variables, engine, deps, seen, limit);
  const knnQuery: ObjectKnnQuery = {
    property_name: knn.propertyName,
    vector,
    k: knn.k,
    metric: knn.metric,
  };
  if (!objectTypeId || vector.length === 0) {
    return objectSetResult({
      data: [],
      total: 0,
      objectTypeId,
      source: 'knn',
      filters,
      sort,
      limit,
      includeCount,
      aggregations,
      knn: knnQuery,
    });
  }
  const response = await deps.queryObjects(objectTypeId, {
    filters: objectSetQueryFilters(filters),
    sort,
    per_page: limit,
    limit,
    include_count: includeCount,
    aggregations,
    knn: knnQuery,
  });
  return objectSetResult({
    data: response.data,
    total: response.count ?? response.total,
    objectTypeId,
    source: 'knn',
    filters,
    sort,
    limit,
    includeCount,
    aggregations,
    responseAggregations: response.aggregations,
    knnResults: response.knn_results ?? [],
    knn: response.object_set?.knn ?? knnQuery,
  });
}

async function executeObjectTypeObjectSet(
  objectTypeId: string,
  filters: WorkshopVariableFilter[],
  limit: number,
  sort: ObjectQuerySort[],
  aggregations: ObjectSetAggregationSpec[],
  includeCount: boolean,
  deps: WorkshopObjectSetExecutorDependencies,
): Promise<WorkshopObjectSetExecutionResult> {
  if (!objectTypeId) {
    return objectSetResult({
      data: [],
      total: 0,
      objectTypeId: '',
      source: 'object_type',
      filters,
      sort,
      limit,
      includeCount,
      aggregations,
    });
  }
  const response = await deps.queryObjects(objectTypeId, {
    filters: objectSetQueryFilters(filters),
    sort,
    per_page: limit,
    limit,
    include_count: includeCount,
    aggregations,
  });
  return objectSetResult({
    data: response.data,
    total: response.count ?? response.total,
    objectTypeId,
    source: 'object_type',
    filters,
    sort,
    limit,
    includeCount,
    aggregations,
    responseAggregations: response.aggregations,
  });
}

async function executeLinkedSearchAroundObjectSet(
  objectTypeId: string,
  filters: WorkshopVariableFilter[],
  limit: number,
  sort: ObjectQuerySort[],
  aggregations: ObjectSetAggregationSpec[],
  includeCount: boolean,
  anchors: ObjectInstance[],
  searchAround: LinkedSearchAroundConfig,
  deps: WorkshopObjectSetExecutorDependencies,
): Promise<WorkshopObjectSetExecutionResult> {
  const sourceObjectIds = uniqueNonEmpty(anchors.map((object) => object.id));
  const searchAroundQuery: ObjectSearchAroundQuery = {
    source_object_ids: sourceObjectIds,
    link_type_ids: searchAround.linkTypeIds,
    direction: searchAround.direction,
    depth: searchAround.depth,
    target_object_type_id: objectTypeId,
  };
  if (!objectTypeId || sourceObjectIds.length === 0 || searchAround.linkTypeIds.length === 0) {
    return objectSetResult({
      data: [],
      total: 0,
      objectTypeId,
      source: 'search_around',
      filters,
      sort,
      limit,
      includeCount,
      aggregations,
      searchAround: searchAroundQuery,
    });
  }
  const response = await deps.queryObjects(objectTypeId, {
    filters: objectSetQueryFilters(filters),
    sort,
    per_page: limit,
    limit,
    include_count: includeCount,
    aggregations,
    search_around: searchAroundQuery,
  });
  return objectSetResult({
    data: response.data,
    total: response.count ?? response.total,
    objectTypeId,
    source: 'search_around',
    filters,
    sort,
    limit,
    includeCount,
    aggregations,
    responseAggregations: response.aggregations,
    linkedEdges: response.linked_edges ?? [],
    searchAround: response.object_set?.search_around ?? searchAroundQuery,
  });
}

function objectSetResult({
  data,
  total,
  objectTypeId,
  source,
  filters,
  sort,
  limit,
  includeCount,
  aggregations,
  aggregationRows,
  responseAggregations,
  selectedObjectIds,
  linkedEdges,
  searchAround,
  knnResults,
  knn,
}: {
  data: ObjectInstance[];
  total: number;
  objectTypeId: string;
  source: WorkshopObjectSetExecutionResult['source'];
  filters: WorkshopVariableFilter[];
  sort: ObjectQuerySort[];
  limit: number;
  includeCount: boolean;
  aggregations: ObjectSetAggregationSpec[];
  aggregationRows?: ObjectInstance[];
  responseAggregations?: ObjectSetAggregationResult[];
  selectedObjectIds?: string[];
  linkedEdges?: LinkedObjectEdge[];
  searchAround?: ObjectSearchAroundQuery;
  knnResults?: ObjectKnnResult[];
  knn?: ObjectKnnQuery;
}): WorkshopObjectSetExecutionResult {
  const normalizedFilters = objectSetQueryFilters(filters);
  const normalizedSort = compactSorts(sort);
  const normalizedAggregations = compactAggregations(aggregations);
  const aggregationResults = responseAggregations ?? computeObjectSetAggregations(aggregationRows ?? data, normalizedAggregations);
  return {
    data,
    total,
    count: total,
    objectTypeId,
    source,
    filters,
    sort: normalizedSort,
    aggregations: aggregationResults,
    linkedEdges: linkedEdges ?? [],
    knnResults: knnResults ?? [],
    contract: {
      object_type_id: objectTypeId,
      filters: normalizedFilters,
      sort: normalizedSort,
      limit,
      include_count: includeCount,
      aggregations: normalizedAggregations,
      ...(selectedObjectIds && selectedObjectIds.length > 0 ? { selected_object_ids: selectedObjectIds } : {}),
      ...(searchAround ? { search_around: searchAround } : {}),
      ...(knn ? { knn } : {}),
    },
  };
}

async function executeAnchorObjectSet(
  searchAround: SearchAroundConfig,
  variables: WorkshopVariableLike[],
  engine: WorkshopVariableEngineResult | null | undefined,
  limit: number,
  deps: WorkshopObjectSetExecutorDependencies,
  seen: Set<string>,
) {
  const anchorVariableId = searchAround.sourceVariableId;
  const anchorVariable = variables.find((entry) => entry.id === anchorVariableId) ?? null;
  if (!anchorVariable) return [];
  const anchors = await executeObjectSetInternal({
    variable: anchorVariable,
    variables,
    engine,
    objectTypeId: anchorVariable.object_type_id,
    limit,
  }, deps, seen);
  return anchors.data;
}

function variableOwnFilters(
  variable: WorkshopVariableLike | null | undefined,
  engine: WorkshopVariableEngineResult | null | undefined,
): WorkshopVariableFilter[] {
  if (!variable) return [];
  const filters = [
    ...(variable.static_filter ? [variable.static_filter] : []),
    ...(Array.isArray(variable.static_filters) ? variable.static_filters : []),
  ];
  for (const filterVariableId of filterVariableIdsForObjectSet(variable)) {
    const filterOutput = engine?.getObjectSetFilter(filterVariableId);
    if (filterOutput) filters.push(...filterOutput.filters);
  }
  return compactFilters(filters);
}

function filterVariableIdsForObjectSet(variable: WorkshopVariableLike): string[] {
  const ids = new Set<string>();
  addString(ids, variable.filter_variable_id);
  addString(ids, stringFromRecord(variable.metadata, 'filter_variable_id'));
  addStringArray(ids, variable.metadata?.filter_variable_ids);
  ids.delete(variable.id);
  return [...ids];
}

function addString(ids: Set<string>, value: unknown) {
  if (typeof value !== 'string') return;
  const trimmed = value.trim();
  if (trimmed) ids.add(trimmed);
}

function addStringArray(ids: Set<string>, value: unknown) {
  if (!Array.isArray(value)) return;
  for (const entry of value) addString(ids, entry);
}

function appendString(values: string[], value: unknown) {
  if (typeof value !== 'string') return;
  const trimmed = value.trim();
  if (trimmed) values.push(trimmed);
}

function objectSetQuerySorts(variable: WorkshopVariableLike | null | undefined, explicit: unknown): ObjectQuerySort[] {
  return compactSorts([
    ...readSorts(explicit),
    ...readSorts(variable?.metadata?.sort),
    ...readSorts(variable?.metadata?.sorts),
    ...readSorts(variable?.metadata?.sort_by),
    ...readSorts(variable?.metadata?.order_by),
    ...(isRecord(variable?.default_value) ? readSorts(variable.default_value.sort) : []),
  ]);
}

function objectSetQueryAggregations(variable: WorkshopVariableLike | null | undefined, explicit: unknown): ObjectSetAggregationSpec[] {
  return compactAggregations([
    ...readAggregations(explicit),
    ...readAggregations(variable?.metadata?.aggregations),
    ...readAggregations(variable?.metadata?.aggregation),
    ...(isRecord(variable?.default_value) ? readAggregations(variable.default_value.aggregations) : []),
  ]);
}

function compactFilters(filters: unknown[]): WorkshopVariableFilter[] {
  return filters
    .filter((entry): entry is WorkshopVariableFilter => isRecord(entry) && typeof entry.property_name === 'string' && entry.property_name.trim().length > 0)
    .map((entry) => ({
      ...entry,
      property_name: entry.property_name.trim(),
      operator: typeof entry.operator === 'string' && entry.operator.trim() ? entry.operator.trim() : 'equals',
    }));
}

function compactSorts(sort: unknown[]): ObjectQuerySort[] {
  const out: ObjectQuerySort[] = [];
  for (const entry of sort) {
    if (!isRecord(entry)) continue;
    const propertyName = stringFromRecord(entry, 'property_name') || stringFromRecord(entry, 'property') || stringFromRecord(entry, 'field') || stringFromRecord(entry, 'name');
    if (!propertyName) continue;
    const direction = stringFromRecord(entry, 'direction') || stringFromRecord(entry, 'order') || 'asc';
    out.push({ property_name: propertyName, direction: isDescending(direction) ? 'desc' : 'asc' });
  }
  return out;
}

function compactAggregations(aggregations: unknown[]): ObjectSetAggregationSpec[] {
  const out: ObjectSetAggregationSpec[] = [];
  for (const entry of aggregations) {
    if (!isRecord(entry)) continue;
    const fn = normalizeAggregationFunction(stringFromRecord(entry, 'function') || stringFromRecord(entry, 'metric') || stringFromRecord(entry, 'aggregation'));
    if (!fn) continue;
    const propertyName = stringFromRecord(entry, 'property_name') || stringFromRecord(entry, 'property') || stringFromRecord(entry, 'field');
    out.push({
      id: stringFromRecord(entry, 'id') || undefined,
      alias: stringFromRecord(entry, 'alias') || stringFromRecord(entry, 'name') || undefined,
      function: fn,
      property_name: propertyName || undefined,
    });
  }
  return out;
}

function readSorts(value: unknown): ObjectQuerySort[] {
  if (!value) return [];
  if (Array.isArray(value)) return compactSorts(value);
  if (isRecord(value)) return compactSorts([value]);
  if (typeof value !== 'string') return [];
  const [propertyName, direction = 'asc'] = value.split(':').map((part) => part.trim());
  return propertyName ? [{ property_name: propertyName, direction: isDescending(direction) ? 'desc' : 'asc' }] : [];
}

function readAggregations(value: unknown): ObjectSetAggregationSpec[] {
  if (!value) return [];
  if (Array.isArray(value)) return compactAggregations(value);
  if (isRecord(value)) return compactAggregations([value]);
  if (typeof value !== 'string') return [];
  const [fn, propertyName] = value.split(':').map((part) => part.trim());
  const normalized = normalizeAggregationFunction(fn);
  return normalized ? [{ function: normalized, property_name: propertyName || undefined }] : [];
}

function normalizeAggregationFunction(value: unknown) {
  const normalized = typeof value === 'string' ? value.trim().toLowerCase() : '';
  if (normalized === 'average') return 'avg';
  if (normalized === 'distinct' || normalized === 'count_distinct' || normalized === 'unique') return 'distinct_count';
  if (['count', 'sum', 'avg', 'min', 'max', 'distinct_count', 'approx_distinct'].includes(normalized)) return normalized;
  return normalized;
}

function expandFilters(filters: WorkshopVariableFilter[]): WorkshopVariableFilter[] {
  return compactFilters(filters.flatMap((filter) => {
    if (filter.min !== undefined || filter.max !== undefined) {
      return [
        filter.min !== undefined ? { ...filter, operator: 'gte', value: filter.min } : null,
        filter.max !== undefined ? { ...filter, operator: 'lte', value: filter.max } : null,
      ].filter(Boolean);
    }
    return [filter];
  }));
}

function isDescending(direction: unknown) {
  return ['desc', 'descending', '-1'].includes(String(direction ?? '').toLowerCase().trim());
}

function isEmptyValue(value: unknown) {
  return value === null || value === undefined || stringValue(value) === '';
}

function matchesObjectFilter(object: ObjectInstance, filter: WorkshopVariableFilter) {
  const props = object.properties ?? {};
  const actual = props[filter.property_name];
  const expected = filter.value;
  const operator = String(filter.operator ?? 'equals').toLowerCase().trim();
  switch (operator) {
    case 'contains':
      return stringValue(actual).includes(stringValue(expected));
    case 'not_equals':
    case 'neq':
    case '!=':
      return stringValue(actual) !== stringValue(expected);
    case 'gte':
    case '>=':
      return compareValues(actual, expected) >= 0;
    case 'lte':
    case '<=':
      return compareValues(actual, expected) <= 0;
    case 'gt':
    case '>':
      return compareValues(actual, expected) > 0;
    case 'lt':
    case '<':
      return compareValues(actual, expected) < 0;
    case 'in':
      return Array.isArray(expected)
        ? expected.map(stringValue).includes(stringValue(actual))
        : stringValue(actual) === stringValue(expected);
    case 'is_empty':
      return actual === null || actual === undefined || stringValue(actual) === '';
    case 'is_not_empty':
      return !(actual === null || actual === undefined || stringValue(actual) === '');
    default:
      return stringValue(actual) === stringValue(expected);
  }
}

function compareValues(actual: unknown, expected: unknown) {
  const actualNumber = numberValue(actual);
  const expectedNumber = numberValue(expected);
  if (actualNumber !== null && expectedNumber !== null) {
    if (actualNumber < expectedNumber) return -1;
    if (actualNumber > expectedNumber) return 1;
    return 0;
  }
  return stringValue(actual).localeCompare(stringValue(expected));
}

function numberValue(value: unknown): number | null {
  const number = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(number) ? number : null;
}

function stringValue(value: unknown) {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return value.toLowerCase().trim();
  if (typeof value === 'number' || typeof value === 'boolean') return String(value).toLowerCase();
  return JSON.stringify(value).toLowerCase();
}

function objectSetEvaluationToObjects(response: ObjectSetEvaluationResponse, fallbackObjectTypeId: string): ObjectInstance[] {
  const objectTypeId = response.object_set.base_object_type_id || fallbackObjectTypeId;
  return response.rows.map((row, index) => {
    const propertiesSource = isRecord(row.properties) ? row.properties : row;
    const properties = Object.fromEntries(
      Object.entries(propertiesSource).filter(([key]) => !['id', 'object_id', 'object_type_id', 'type_id', 'properties'].includes(key)),
    );
    return {
      id: String(row.id ?? row.object_id ?? row.primary_key ?? `${response.object_set.id}:${index}`),
      object_type_id: String(row.object_type_id ?? row.type_id ?? objectTypeId),
      properties,
      created_by: 'object-set-evaluation',
      created_at: response.generated_at,
      updated_at: response.generated_at,
    };
  });
}

type SearchAroundConfig = GeoSearchAroundConfig | LinkedSearchAroundConfig;

interface BaseSearchAroundConfig {
  sourceVariableId: string;
}

interface GeoSearchAroundConfig extends BaseSearchAroundConfig {
  kind: 'geospatial';
  radius: number;
  units: 'meters' | 'km' | 'miles';
  latitudeProperty: string;
  longitudeProperty: string;
  sourceLatitudeProperty: string;
  sourceLongitudeProperty: string;
}

interface LinkedSearchAroundConfig extends BaseSearchAroundConfig {
  kind: 'linked';
  linkTypeIds: string[];
  direction: 'outgoing' | 'incoming' | 'both';
  depth: number;
}

interface KNNConfig {
  propertyName: string;
  vector: number[];
  sourceVariableId: string;
  sourcePropertyName: string;
  k: number;
  metric: 'cosine' | 'euclidean' | 'dot';
}

function readKNNConfig(variable: WorkshopVariableLike | null | undefined, fallbackK: number): KNNConfig | null {
  const raw = variable?.metadata?.knn ?? variable?.metadata?.nearest_neighbors ?? variable?.metadata?.nearestNeighbors;
  if (!isRecord(raw)) return null;
  const propertyName = stringFromRecord(raw, 'property_name') ||
    stringFromRecord(raw, 'property') ||
    stringFromRecord(raw, 'vector_property') ||
    stringFromRecord(raw, 'embedding_property');
  if (!propertyName) return null;
  const vector = readNumericVector(raw.vector ?? raw.query_vector ?? raw.queryVector);
  const sourceVariableId = stringFromRecord(raw, 'source_variable_id') || stringFromRecord(raw, 'sourceVariableId');
  if (vector.length === 0 && !sourceVariableId) return null;
  const sourcePropertyName = stringFromRecord(raw, 'source_property_name') ||
    stringFromRecord(raw, 'source_vector_property') ||
    stringFromRecord(raw, 'source_embedding_property') ||
    propertyName;
  return {
    propertyName,
    vector,
    sourceVariableId,
    sourcePropertyName,
    k: clampKNNK(numberFromRecord(raw, 'k') ?? numberFromRecord(raw, 'k_value') ?? numberFromRecord(raw, 'kValue'), fallbackK),
    metric: normalizeKNNMetric(stringFromRecord(raw, 'metric')),
  };
}

async function resolveKNNVector(
  config: KNNConfig,
  variables: WorkshopVariableLike[],
  engine: WorkshopVariableEngineResult | null | undefined,
  deps: WorkshopObjectSetExecutorDependencies,
  seen: Set<string>,
  limit: number,
): Promise<number[]> {
  if (config.vector.length > 0) return config.vector;
  if (!config.sourceVariableId || seen.has(config.sourceVariableId)) return [];
  seen.add(config.sourceVariableId);
  const sourceVariable = variables.find((entry) => entry.id === config.sourceVariableId) ?? null;
  if (!sourceVariable) return [];
  const source = await executeObjectSetInternal({
    variable: sourceVariable,
    variables,
    engine,
    objectTypeId: sourceVariable.object_type_id,
    limit,
  }, deps, seen);
  return readNumericVector(source.data[0]?.properties?.[config.sourcePropertyName]);
}

function readSearchAroundConfig(variable: WorkshopVariableLike | null | undefined): SearchAroundConfig | null {
  const raw = variable?.metadata?.search_around;
  if (!isRecord(raw)) return null;
  const sourceVariableId = stringFromRecord(raw, 'source_variable_id') || stringFromRecord(raw, 'sourceVariableId');
  const linkTypeIds = readStringList(raw.link_type_ids ?? raw.linkTypeIds ?? raw.link_types ?? raw.linkTypes);
  appendString(linkTypeIds, raw.link_type_id);
  appendString(linkTypeIds, raw.linkTypeId);
  if (sourceVariableId && linkTypeIds.length > 0) {
    return {
      kind: 'linked',
      sourceVariableId,
      linkTypeIds: uniqueNonEmpty(linkTypeIds),
      direction: normalizeSearchAroundDirection(stringFromRecord(raw, 'direction')),
      depth: clampSearchAroundDepth(numberFromRecord(raw, 'depth') ?? 1),
    };
  }
  const radius = numberFromRecord(raw, 'radius') ?? numberFromRecord(raw, 'radius_miles') ?? numberFromRecord(raw, 'radius_km');
  if (!sourceVariableId || radius === null || radius <= 0) return null;
  const explicitUnits = stringFromRecord(raw, 'units').toLowerCase();
  const units = explicitUnits === 'km' || explicitUnits === 'kilometers'
    ? 'km'
    : explicitUnits === 'meters' || explicitUnits === 'm'
      ? 'meters'
      : numberFromRecord(raw, 'radius_km') !== null
        ? 'km'
        : 'miles';
  return {
    kind: 'geospatial',
    sourceVariableId,
    radius,
    units,
    latitudeProperty: stringFromRecord(raw, 'latitude_property') || stringFromRecord(raw, 'lat_property') || 'lat',
    longitudeProperty: stringFromRecord(raw, 'longitude_property') || stringFromRecord(raw, 'lon_property') || 'lon',
    sourceLatitudeProperty: stringFromRecord(raw, 'source_latitude_property') || stringFromRecord(raw, 'source_lat_property') || stringFromRecord(raw, 'latitude_property') || 'lat',
    sourceLongitudeProperty: stringFromRecord(raw, 'source_longitude_property') || stringFromRecord(raw, 'source_lon_property') || stringFromRecord(raw, 'longitude_property') || 'lon',
  };
}

function applySearchAround(objects: ObjectInstance[], anchors: ObjectInstance[], config: SearchAroundConfig) {
  if (config.kind !== 'geospatial') return [];
  if (anchors.length === 0) return [];
  const radiusMeters = config.units === 'meters' ? config.radius : config.units === 'km' ? config.radius * 1000 : config.radius * 1609.344;
  const anchorPoints = anchors
    .map((object) => ({
      lat: Number(object.properties?.[config.sourceLatitudeProperty]),
      lon: Number(object.properties?.[config.sourceLongitudeProperty]),
    }))
    .filter((point) => Number.isFinite(point.lat) && Number.isFinite(point.lon));
  if (anchorPoints.length === 0) return [];
  return objects.filter((object) => {
    const lat = Number(object.properties?.[config.latitudeProperty]);
    const lon = Number(object.properties?.[config.longitudeProperty]);
    if (!Number.isFinite(lat) || !Number.isFinite(lon)) return false;
    return anchorPoints.some((anchor) => haversineMeters(anchor.lat, anchor.lon, lat, lon) <= radiusMeters);
  });
}

function normalizeSearchAroundDirection(value: string): 'outgoing' | 'incoming' | 'both' {
  const normalized = value.toLowerCase().trim();
  if (normalized === 'incoming' || normalized === 'inbound') return 'incoming';
  if (normalized === 'both' || normalized === 'any' || normalized === 'all') return 'both';
  return 'outgoing';
}

function clampSearchAroundDepth(value: number) {
  if (!Number.isFinite(value)) return 1;
  return Math.max(1, Math.min(5, Math.trunc(value)));
}

function haversineMeters(lat1: number, lon1: number, lat2: number, lon2: number) {
  const toRad = (degrees: number) => (degrees * Math.PI) / 180;
  const earthRadiusMeters = 6371008.8;
  const dLat = toRad(lat2 - lat1);
  const dLon = toRad(lon2 - lon1);
  const a =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos(toRad(lat1)) * Math.cos(toRad(lat2)) *
    Math.sin(dLon / 2) * Math.sin(dLon / 2);
  return 2 * earthRadiusMeters * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}

function savedObjectSetIdForVariable(variable: WorkshopVariableLike | null | undefined) {
  if (!variable) return '';
  const record = variable as unknown as Record<string, unknown>;
  return (
    stringFromRecord(record, 'object_set_id') ||
    stringFromRecord(record, 'saved_object_set_id') ||
    stringFromRecord(variable.metadata, 'object_set_id') ||
    stringFromRecord(variable.metadata, 'saved_object_set_id') ||
    (isRecord(variable.default_value)
      ? stringFromRecord(variable.default_value, 'object_set_id') || stringFromRecord(variable.default_value, 'saved_object_set_id')
      : '')
  );
}

function clampLimit(limit: number | undefined) {
  if (!Number.isFinite(limit ?? NaN)) return DEFAULT_LIMIT;
  return Math.max(1, Math.min(DEFAULT_LIMIT, Math.trunc(limit ?? DEFAULT_LIMIT)));
}

function stringFromRecord(record: Record<string, unknown> | undefined, key: string): string {
  const value = record?.[key];
  return typeof value === 'string' ? value.trim() : '';
}

function readStringList(value: unknown): string[] {
  if (Array.isArray(value)) return value.filter((entry): entry is string => typeof entry === 'string').map((entry) => entry.trim()).filter(Boolean);
  if (typeof value === 'string') return value.split(',').map((entry) => entry.trim()).filter(Boolean);
  return [];
}

function readNumericVector(value: unknown): number[] {
  if (Array.isArray(value)) {
    const out = value.map((entry) => Number(entry));
    return out.every((entry) => Number.isFinite(entry)) ? out : [];
  }
  if (typeof value === 'string') {
    try {
      return readNumericVector(JSON.parse(value));
    } catch {
      return [];
    }
  }
  return [];
}

function clampKNNK(value: number | null, fallback: number) {
  const raw = value ?? fallback;
  if (!Number.isFinite(raw) || raw <= 0) return Math.min(10, DEFAULT_LIMIT);
  return Math.max(1, Math.min(100, Math.trunc(raw)));
}

function normalizeKNNMetric(value: string): 'cosine' | 'euclidean' | 'dot' {
  const normalized = value.toLowerCase().trim();
  if (normalized === 'euclidean' || normalized === 'l2' || normalized === 'distance') return 'euclidean';
  if (normalized === 'dot' || normalized === 'dot_product' || normalized === 'inner_product') return 'dot';
  return 'cosine';
}

function uniqueNonEmpty(values: string[]) {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed || seen.has(trimmed)) continue;
    seen.add(trimmed);
    out.push(trimmed);
  }
  return out;
}

function numberFromRecord(record: Record<string, unknown>, key: string): number | null {
  const value = record[key];
  const number = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(number) ? number : null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}
