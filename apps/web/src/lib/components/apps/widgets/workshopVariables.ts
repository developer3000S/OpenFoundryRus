import type { ObjectInstance } from '@/lib/api/ontology';
import type { WorkshopMapFeatureCollection } from './workshopMap';

export type WorkshopVariablePrimitive =
  | null
  | string
  | number
  | boolean
  | Date
  | WorkshopVariablePrimitive[]
  | { [key: string]: WorkshopVariablePrimitive };

export interface WorkshopVariableFilter {
  property_name: string;
  operator?: 'equals' | 'contains' | 'gte' | 'lte' | string;
  value?: unknown;
  min?: unknown;
  max?: unknown;
}

export interface WorkshopVariableLike {
  id: string;
  kind: string;
  name: string;
  object_type_id?: string;
  source_widget_id?: string;
  source_variable_id?: string;
  filter_variable_id?: string;
  static_filter?: WorkshopVariableFilter;
  static_filters?: WorkshopVariableFilter[];
  default_value?: unknown;
  metadata?: Record<string, unknown>;
}

export interface WorkshopRuntimeFilterValue {
  values?: string[];
  search?: string;
  range_min?: string;
  range_max?: string;
}

export interface WorkshopRuntimeFilterMetadata {
  outputVariableId?: string;
  sourceWidgetId?: string;
  propertyName?: string;
  component?: string;
}

export interface WorkshopVariableRuntimeState {
  activeObjects?: Record<string, ObjectInstance | null>;
  selectedObjectSets?: Record<string, ObjectInstance[]>;
  shapeOutputs?: Record<string, WorkshopMapFeatureCollection | null>;
  filterValues?: Record<string, WorkshopRuntimeFilterValue>;
  filterMetadata?: Record<string, WorkshopRuntimeFilterMetadata>;
  primitiveValues?: Record<string, unknown>;
  runtimeParameters?: Record<string, string>;
}

export interface WorkshopVariableGraphNode {
  id: string;
  kind: string;
  dependencies: string[];
  dependents: string[];
}

export type WorkshopVariableDiagnosticCode =
  | 'duplicate_variable'
  | 'missing_dependency'
  | 'cycle'
  | 'unsupported_aggregation';

export interface WorkshopVariableDiagnostic {
  code: WorkshopVariableDiagnosticCode;
  variable_id?: string;
  message: string;
}

export interface PrimitiveVariableValue {
  kind: 'primitive';
  variableId: string;
  value: unknown;
  source: 'runtime_parameter' | 'state' | 'static' | 'empty';
}

export interface ObjectSetVariableValue {
  kind: 'object_set';
  variableId: string;
  objectTypeId: string;
  sourceVariableId?: string;
  filters: WorkshopVariableFilter[];
  objects?: ObjectInstance[];
  objectIds?: string[];
}

export interface ObjectSetFilterVariableValue {
  kind: 'object_set_filter';
  variableId: string;
  filters: WorkshopVariableFilter[];
}

export interface SelectedObjectVariableValue {
  kind: 'selected_object';
  variableId: string;
  object: ObjectInstance | null;
}

export interface ShapeVariableValue {
  kind: 'shape';
  variableId: string;
  shape: WorkshopMapFeatureCollection | null;
}

export interface AggregationVariableValue {
  kind: 'aggregation';
  variableId: string;
  metric: string;
  value: number | null;
  sourceVariableId: string;
}

export type WorkshopResolvedVariableValue =
  | PrimitiveVariableValue
  | ObjectSetVariableValue
  | ObjectSetFilterVariableValue
  | SelectedObjectVariableValue
  | ShapeVariableValue
  | AggregationVariableValue;

export interface WorkshopVariableEngineResult {
  values: Record<string, WorkshopResolvedVariableValue>;
  graph: Record<string, WorkshopVariableGraphNode>;
  evaluationOrder: string[];
  dirtyVariableIds: string[];
  diagnostics: WorkshopVariableDiagnostic[];
  getValue: (variableId: string) => WorkshopResolvedVariableValue | undefined;
  getObjectSet: (variableId: string) => ObjectSetVariableValue | undefined;
  getObjectSetFilter: (variableId: string) => ObjectSetFilterVariableValue | undefined;
  getActiveObject: (variableId: string) => ObjectInstance | null;
  getSelectedObjectSet: (variableId: string) => ObjectInstance[];
  getPrimitive: (variableId: string) => unknown;
}

export const EMPTY_WORKSHOP_VARIABLE_ENGINE: WorkshopVariableEngineResult = {
  values: {},
  graph: {},
  evaluationOrder: [],
  dirtyVariableIds: [],
  diagnostics: [],
  getValue: () => undefined,
  getObjectSet: () => undefined,
  getObjectSetFilter: () => undefined,
  getActiveObject: () => null,
  getSelectedObjectSet: () => [],
  getPrimitive: () => undefined,
};

const PRIMITIVE_KINDS = new Set([
  'primitive',
  'string',
  'numeric',
  'number',
  'boolean',
  'date',
  'timestamp',
  'array',
  'struct',
  'geopoint',
  'geoshape',
  'url_parameter',
  'runtime_parameter',
]);

const OBJECT_SET_KINDS = new Set(['object_set', 'object_set_definition']);
const FILTER_KINDS = new Set(['object_set_filter', 'filter_output']);

export function createWorkshopVariableEngine(
  variables: WorkshopVariableLike[],
  state: WorkshopVariableRuntimeState = {},
  previous?: Pick<WorkshopVariableEngineResult, 'values'> | null,
  changedVariableIds: string[] = [],
): WorkshopVariableEngineResult {
  const normalized = variables.filter((variable) => variable.id);
  const diagnostics: WorkshopVariableDiagnostic[] = [];
  const byId = new Map<string, WorkshopVariableLike>();
  for (const variable of normalized) {
    if (byId.has(variable.id)) {
      diagnostics.push({
        code: 'duplicate_variable',
        variable_id: variable.id,
        message: `Duplicate Workshop variable id "${variable.id}".`,
      });
      continue;
    }
    byId.set(variable.id, variable);
  }

  const graph = buildVariableGraph([...byId.values()], byId, diagnostics);
  const evaluationOrder = topologicalOrder(graph, diagnostics);
  const values: Record<string, WorkshopResolvedVariableValue> = {};
  for (const variableId of evaluationOrder) {
    const variable = byId.get(variableId);
    if (!variable) continue;
    values[variableId] = resolveVariable(variable, values, state, diagnostics);
  }

  const dirtyVariableIds = computeDirtyVariables(values, graph, previous?.values ?? {}, changedVariableIds);
  return {
    values,
    graph,
    evaluationOrder,
    dirtyVariableIds,
    diagnostics,
    getValue: (variableId) => values[variableId],
    getObjectSet: (variableId) => {
      const value = values[variableId];
      return value?.kind === 'object_set' ? value : undefined;
    },
    getObjectSetFilter: (variableId) => {
      const value = values[variableId];
      return value?.kind === 'object_set_filter' ? value : undefined;
    },
    getActiveObject: (variableId) => {
      const value = values[variableId];
      if (value?.kind === 'selected_object') return value.object;
      const objectSet = value?.kind === 'object_set' ? value.objects?.[0] : undefined;
      return objectSet ?? null;
    },
    getSelectedObjectSet: (variableId) => {
      const value = values[variableId];
      if (value?.kind === 'object_set') return value.objects ?? [];
      if (value?.kind === 'selected_object') return value.object ? [value.object] : [];
      return [];
    },
    getPrimitive: (variableId) => {
      const value = values[variableId];
      return value?.kind === 'primitive' || value?.kind === 'aggregation' ? value.value : undefined;
    },
  };
}

export function variableFiltersForObjectSet(
  variable: WorkshopVariableLike | null | undefined,
  engine: Pick<WorkshopVariableEngineResult, 'getObjectSet'> | null | undefined,
): WorkshopVariableFilter[] {
  if (!variable) return [];
  const objectSet = engine?.getObjectSet(variable.id);
  if (objectSet) return objectSet.filters;
  return compactFilters([
    ...(variable.static_filter ? [variable.static_filter] : []),
    ...(Array.isArray(variable.static_filters) ? variable.static_filters : []),
  ]);
}

function buildVariableGraph(
  variables: WorkshopVariableLike[],
  byId: Map<string, WorkshopVariableLike>,
  diagnostics: WorkshopVariableDiagnostic[],
): Record<string, WorkshopVariableGraphNode> {
  const graph: Record<string, WorkshopVariableGraphNode> = {};
  for (const variable of variables) {
    const dependencies = dependenciesForVariable(variable);
    graph[variable.id] = {
      id: variable.id,
      kind: variable.kind,
      dependencies,
      dependents: [],
    };
    for (const dependency of dependencies) {
      if (!byId.has(dependency)) {
        diagnostics.push({
          code: 'missing_dependency',
          variable_id: variable.id,
          message: `Variable "${variable.id}" depends on missing variable "${dependency}".`,
        });
      }
    }
  }
  for (const node of Object.values(graph)) {
    for (const dependency of node.dependencies) {
      if (graph[dependency]) graph[dependency].dependents.push(node.id);
    }
  }
  return graph;
}

function dependenciesForVariable(variable: WorkshopVariableLike): string[] {
  const deps = new Set<string>();
  addDependency(deps, variable.source_variable_id);
  addDependency(deps, variable.filter_variable_id);
  addDependency(deps, stringFromMetadata(variable.metadata, 'source_variable_id'));
  addDependency(deps, stringFromMetadata(variable.metadata, 'filter_variable_id'));
  const dependsOn = variable.metadata?.depends_on;
  if (Array.isArray(dependsOn)) {
    for (const dependency of dependsOn) addDependency(deps, typeof dependency === 'string' ? dependency : '');
  }
  return [...deps].filter((dependency) => dependency !== variable.id);
}

function addDependency(deps: Set<string>, value: string | undefined) {
  const trimmed = value?.trim();
  if (trimmed) deps.add(trimmed);
}

function topologicalOrder(
  graph: Record<string, WorkshopVariableGraphNode>,
  diagnostics: WorkshopVariableDiagnostic[],
): string[] {
  const order: string[] = [];
  const visiting = new Set<string>();
  const visited = new Set<string>();

  function visit(id: string, path: string[]) {
    if (visited.has(id)) return;
    if (visiting.has(id)) {
      diagnostics.push({
        code: 'cycle',
        variable_id: id,
        message: `Workshop variable cycle detected: ${[...path, id].join(' -> ')}.`,
      });
      return;
    }
    const node = graph[id];
    if (!node) return;
    visiting.add(id);
    for (const dependency of node.dependencies) visit(dependency, [...path, id]);
    visiting.delete(id);
    visited.add(id);
    order.push(id);
  }

  for (const id of Object.keys(graph)) visit(id, []);
  return order;
}

function resolveVariable(
  variable: WorkshopVariableLike,
  values: Record<string, WorkshopResolvedVariableValue>,
  state: WorkshopVariableRuntimeState,
  diagnostics: WorkshopVariableDiagnostic[],
): WorkshopResolvedVariableValue {
  if (FILTER_KINDS.has(variable.kind)) return resolveFilterVariable(variable, state);
  if (OBJECT_SET_KINDS.has(variable.kind)) return resolveObjectSetVariable(variable, values);
  if (variable.kind === 'object_set_active_object') {
    return { kind: 'selected_object', variableId: variable.id, object: state.activeObjects?.[variable.id] ?? null };
  }
  if (variable.kind === 'object_set_selection') {
    const objects = state.selectedObjectSets?.[variable.id] ?? [];
    return {
      kind: 'object_set',
      variableId: variable.id,
      objectTypeId: variable.object_type_id ?? objects[0]?.object_type_id ?? '',
      objects,
      objectIds: objects.map((object) => object.id),
      filters: [],
    };
  }
  if (variable.kind === 'shape_output') {
    return { kind: 'shape', variableId: variable.id, shape: state.shapeOutputs?.[variable.id] ?? null };
  }
  if (variable.kind === 'aggregation') return resolveAggregationVariable(variable, values, diagnostics);
  if (PRIMITIVE_KINDS.has(variable.kind) || variable.kind.length > 0) return resolvePrimitiveVariable(variable, state);
  return { kind: 'primitive', variableId: variable.id, value: null, source: 'empty' };
}

function resolvePrimitiveVariable(
  variable: WorkshopVariableLike,
  state: WorkshopVariableRuntimeState,
): PrimitiveVariableValue {
  if (Object.prototype.hasOwnProperty.call(state.primitiveValues ?? {}, variable.id)) {
    return { kind: 'primitive', variableId: variable.id, value: state.primitiveValues?.[variable.id], source: 'state' };
  }
  const parameterName = runtimeParameterName(variable);
  if (parameterName && Object.prototype.hasOwnProperty.call(state.runtimeParameters ?? {}, parameterName)) {
    return {
      kind: 'primitive',
      variableId: variable.id,
      value: state.runtimeParameters?.[parameterName],
      source: 'runtime_parameter',
    };
  }
  if (variable.default_value !== undefined) {
    return { kind: 'primitive', variableId: variable.id, value: variable.default_value, source: 'static' };
  }
  return { kind: 'primitive', variableId: variable.id, value: null, source: 'empty' };
}

function resolveObjectSetVariable(
  variable: WorkshopVariableLike,
  values: Record<string, WorkshopResolvedVariableValue>,
): ObjectSetVariableValue {
  const sourceVariableId = variable.source_variable_id || stringFromMetadata(variable.metadata, 'source_variable_id');
  const sourceValue = sourceVariableId ? values[sourceVariableId] : undefined;
  const filterVariableId = variable.filter_variable_id || stringFromMetadata(variable.metadata, 'filter_variable_id');
  const filterValue = filterVariableId ? values[filterVariableId] : undefined;
  const sourceObjectSet = sourceValue?.kind === 'object_set' ? sourceValue : undefined;
  const filterOutput = filterValue?.kind === 'object_set_filter' ? filterValue : undefined;
  return {
    kind: 'object_set',
    variableId: variable.id,
    objectTypeId: variable.object_type_id || sourceObjectSet?.objectTypeId || '',
    sourceVariableId,
    filters: compactFilters([
      ...(sourceObjectSet?.filters ?? []),
      ...(variable.static_filter ? [variable.static_filter] : []),
      ...(Array.isArray(variable.static_filters) ? variable.static_filters : []),
      ...(filterOutput?.filters ?? []),
    ]),
    objects: sourceObjectSet?.objects,
    objectIds: sourceObjectSet?.objectIds,
  };
}

function resolveFilterVariable(
  variable: WorkshopVariableLike,
  state: WorkshopVariableRuntimeState,
): ObjectSetFilterVariableValue {
  const filters: WorkshopVariableFilter[] = [];
  filters.push(...defaultFilters(variable));
  const values = state.filterValues ?? {};
  const metadata = state.filterMetadata ?? {};
  for (const [filterId, value] of Object.entries(values)) {
    const meta = metadata[filterId] ?? {};
    const matchesOutput = meta.outputVariableId && meta.outputVariableId === variable.id;
    const matchesWidget = meta.sourceWidgetId && variable.source_widget_id && meta.sourceWidgetId === variable.source_widget_id;
    if (!matchesOutput && !matchesWidget) continue;
    filters.push(...runtimeFilterValueToFilters(value, meta));
  }
  return { kind: 'object_set_filter', variableId: variable.id, filters: compactFilters(filters) };
}

function resolveAggregationVariable(
  variable: WorkshopVariableLike,
  values: Record<string, WorkshopResolvedVariableValue>,
  diagnostics: WorkshopVariableDiagnostic[],
): AggregationVariableValue {
  const sourceVariableId = variable.source_variable_id || stringFromMetadata(variable.metadata, 'source_variable_id');
  const metric = stringFromMetadata(variable.metadata, 'metric') || 'count';
  const property = stringFromMetadata(variable.metadata, 'property_name');
  const source = sourceVariableId ? values[sourceVariableId] : undefined;
  const objects = source?.kind === 'object_set' ? source.objects ?? [] : [];
  let value: number | null = null;
  if (metric === 'count') {
    value = objects.length > 0 ? objects.length : source?.kind === 'object_set' ? source.objectIds?.length ?? 0 : 0;
  } else {
    const numbers = objects
      .map((object) => Number(property ? object.properties?.[property] : undefined))
      .filter((entry) => Number.isFinite(entry));
    if (numbers.length === 0) {
      value = null;
    } else if (metric === 'sum') {
      value = numbers.reduce((sum, entry) => sum + entry, 0);
    } else if (metric === 'avg' || metric === 'average') {
      value = numbers.reduce((sum, entry) => sum + entry, 0) / numbers.length;
    } else if (metric === 'min') {
      value = Math.min(...numbers);
    } else if (metric === 'max') {
      value = Math.max(...numbers);
    } else {
      diagnostics.push({
        code: 'unsupported_aggregation',
        variable_id: variable.id,
        message: `Unsupported aggregation metric "${metric}".`,
      });
    }
  }
  return { kind: 'aggregation', variableId: variable.id, metric, value, sourceVariableId };
}

function runtimeFilterValueToFilters(
  value: WorkshopRuntimeFilterValue,
  metadata: WorkshopRuntimeFilterMetadata,
): WorkshopVariableFilter[] {
  const propertyName = metadata.propertyName?.trim();
  if (!propertyName) return [];
  const filters: WorkshopVariableFilter[] = [];
  const search = value.search?.trim();
  if (search) filters.push({ property_name: propertyName, operator: 'contains', value: search });
  for (const entry of value.values ?? []) {
    if (entry.trim()) filters.push({ property_name: propertyName, operator: 'equals', value: entry });
  }
  if (value.range_min?.trim()) filters.push({ property_name: propertyName, operator: 'gte', value: value.range_min.trim() });
  if (value.range_max?.trim()) filters.push({ property_name: propertyName, operator: 'lte', value: value.range_max.trim() });
  return filters;
}

function defaultFilters(variable: WorkshopVariableLike): WorkshopVariableFilter[] {
  const defaults = [
    ...(variable.static_filter ? [variable.static_filter] : []),
    ...(Array.isArray(variable.static_filters) ? variable.static_filters : []),
  ];
  if (Array.isArray(variable.default_value)) defaults.push(...variable.default_value);
  if (isRecord(variable.default_value) && Array.isArray(variable.default_value.filters)) {
    defaults.push(...variable.default_value.filters);
  }
  return compactFilters(defaults);
}

function compactFilters(filters: unknown[]): WorkshopVariableFilter[] {
  return filters
    .filter((entry): entry is WorkshopVariableFilter => isRecord(entry) && typeof entry.property_name === 'string' && entry.property_name.trim().length > 0)
    .map((entry) => ({
      ...entry,
      property_name: entry.property_name.trim(),
      operator: typeof entry.operator === 'string' ? entry.operator : 'equals',
    }));
}

function runtimeParameterName(variable: WorkshopVariableLike): string {
  return (
    stringFromMetadata(variable.metadata, 'parameter_name') ||
    stringFromMetadata(variable.metadata, 'runtime_parameter') ||
    stringFromMetadata(variable.metadata, 'url_parameter') ||
    variable.name ||
    variable.id
  );
}

function stringFromMetadata(metadata: Record<string, unknown> | undefined, key: string): string {
  const value = metadata?.[key];
  return typeof value === 'string' ? value.trim() : '';
}

function computeDirtyVariables(
  values: Record<string, WorkshopResolvedVariableValue>,
  graph: Record<string, WorkshopVariableGraphNode>,
  previousValues: Record<string, WorkshopResolvedVariableValue>,
  changedVariableIds: string[],
): string[] {
  const dirty = new Set(changedVariableIds.filter((id) => graph[id]));
  for (const id of Object.keys(values)) {
    if (!sameVariableValue(values[id], previousValues[id])) dirty.add(id);
  }
  const queue = [...dirty];
  while (queue.length > 0) {
    const id = queue.shift();
    if (!id) continue;
    for (const dependent of graph[id]?.dependents ?? []) {
      if (!dirty.has(dependent)) {
        dirty.add(dependent);
        queue.push(dependent);
      }
    }
  }
  return [...dirty];
}

function sameVariableValue(left: WorkshopResolvedVariableValue | undefined, right: WorkshopResolvedVariableValue | undefined) {
  if (left === right) return true;
  if (!left || !right) return false;
  return stableStringify(left) === stableStringify(right);
}

function stableStringify(value: unknown) {
  return JSON.stringify(value, (_key, entry) => {
    if (!isRecord(entry) || Array.isArray(entry)) return entry;
    return Object.keys(entry).sort().reduce<Record<string, unknown>>((out, key) => {
      out[key] = entry[key];
      return out;
    }, {});
  });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}
