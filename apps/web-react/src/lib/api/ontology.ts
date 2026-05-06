import api from './client';

// ────────────────────────────────────────────────────────────────
// Ontology API — slices grow as routes are migrated. Today: object
// types + properties listing (used by /queries). Full ontology
// surface (links, indexing, design) gets added with /ontology* routes.
// ────────────────────────────────────────────────────────────────

export interface ObjectType {
  id: string;
  name: string;
  display_name: string;
  description: string;
  primary_key_property: string | null;
  icon: string | null;
  color: string | null;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export interface PropertyInlineEditConfig {
  enabled: boolean;
  permission_keys?: string[];
}

export interface Property {
  id: string;
  object_type_id: string;
  name: string;
  display_name: string;
  description: string;
  property_type: string;
  required: boolean;
  unique_constraint: boolean;
  time_dependent: boolean;
  default_value: unknown;
  validation_rules: unknown;
  inline_edit_config?: PropertyInlineEditConfig | null;
  created_at: string;
  updated_at: string;
}

export function listObjectTypes(params?: { page?: number; per_page?: number; search?: string }) {
  const qs = new URLSearchParams();
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  if (params?.search) qs.set('search', params.search);
  return api.get<{ data: ObjectType[]; total: number; page: number; per_page: number }>(
    `/ontology/types${qs.toString() ? `?${qs}` : ''}`,
  );
}

export function listProperties(typeId: string) {
  return api
    .get<{ data: Property[] }>(`/ontology/types/${typeId}/properties`)
    .then((response) => response.data);
}

// ────────────────────────────────────────────────────────────────
// Object instances + ontology graph + Quiver visual functions
// (used by /quiver). Other ontology surfaces ship with their routes.
// ────────────────────────────────────────────────────────────────

export interface ObjectInstance {
  id: string;
  object_type_id: string;
  properties: Record<string, unknown>;
  created_by: string;
  organization_id?: string | null;
  marking?: string;
  created_at: string;
  updated_at: string;
}

export interface GraphNode {
  id: string;
  kind: string;
  label: string;
  secondary_label: string | null;
  color: string | null;
  route: string | null;
  metadata: Record<string, unknown>;
}

export interface GraphEdge {
  id: string;
  kind: string;
  source: string;
  target: string;
  label: string;
  metadata: Record<string, unknown>;
}

export interface GraphSummary {
  scope: string;
  node_kinds: Record<string, number>;
  edge_kinds: Record<string, number>;
  object_types: Record<string, number>;
  markings: Record<string, number>;
  root_neighbor_count: number;
  max_hops_reached: number;
  boundary_crossings: number;
  sensitive_objects: number;
  sensitive_markings: string[];
}

export interface GraphResponse {
  mode: string;
  root_object_id: string | null;
  root_type_id: string | null;
  depth: number;
  total_nodes: number;
  total_edges: number;
  summary: GraphSummary;
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export type QuiverChartKind = 'line' | 'area' | 'bar' | 'point';

export interface QuiverVisualFunction {
  id: string;
  name: string;
  description: string;
  primary_type_id: string;
  secondary_type_id: string | null;
  join_field: string;
  secondary_join_field: string;
  date_field: string;
  metric_field: string;
  group_field: string;
  selected_group: string | null;
  chart_kind: QuiverChartKind;
  shared: boolean;
  vega_spec: Record<string, unknown>;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export function getObjectType(id: string) {
  return api.get<ObjectType>(`/ontology/types/${id}`);
}

export function executeInlineEdit(
  typeId: string,
  objectId: string,
  propertyId: string,
  body: { value: unknown; justification?: string },
) {
  return api.post<ExecuteActionResponse>(
    `/ontology/types/${typeId}/objects/${objectId}/inline-edit/${propertyId}`,
    body,
  );
}

export function queryObjects(typeId: string, body: { equals?: Record<string, unknown>; limit?: number }) {
  return api.post<{ data: ObjectInstance[]; total: number }>(
    `/ontology/types/${typeId}/objects/query`,
    body,
  );
}

export type SubmissionUserAttr = 'user_id' | 'email' | 'organization_id' | 'roles' | 'permissions' | 'auth_methods';

export type SubmissionOperator =
  | 'is' | 'is_not' | 'matches' | 'lt' | 'lte' | 'gt' | 'gte'
  | 'includes' | 'includes_any' | 'is_included_in'
  | 'each_is' | 'each_is_not'
  | 'is_empty' | 'is_not_empty';

export type SubmissionOperand =
  | { kind: 'static'; value: unknown }
  | { kind: 'param'; name: string }
  | { kind: 'user'; attr: SubmissionUserAttr };

export type SubmissionNode =
  | { type: 'leaf'; left: SubmissionOperand; op: SubmissionOperator; right?: SubmissionOperand }
  | { type: 'all'; children: SubmissionNode[] }
  | { type: 'any'; children: SubmissionNode[] };

export interface ObjectRevision {
  id: string;
  object_id: string;
  object_type_id: string;
  operation: 'insert' | 'update' | 'delete' | string;
  properties: Record<string, unknown>;
  marking: string;
  organization_id?: string | null;
  changed_by: string;
  revision_number: number;
  written_at: string;
}

export interface ListObjectRevisionsResponse {
  object_id: string;
  total: number;
  data: ObjectRevision[];
}

export interface RestoreObjectRevisionResponse {
  object: ObjectInstance;
  restored_from_revision_number: number;
  new_revision_number: number;
}

export function listObjectRevisions(typeId: string, objectId: string, params?: { limit?: number }) {
  const qs = new URLSearchParams();
  if (params?.limit !== undefined) qs.set('limit', String(params.limit));
  const tail = qs.toString() ? `?${qs}` : '';
  return api.get<ListObjectRevisionsResponse>(
    `/ontology/types/${typeId}/objects/${objectId}/revisions${tail}`,
  );
}

export function restoreObjectRevision(typeId: string, objectId: string, revisionNumber: number) {
  return api.post<RestoreObjectRevisionResponse>(
    `/ontology/types/${typeId}/objects/${objectId}/revisions/${revisionNumber}/restore`,
    {},
  );
}

export interface InlineEditBatchEntry {
  property_id: string;
  object_id: string;
  value: unknown;
  justification?: string;
}

export interface InlineEditBatchResponse {
  total: number;
  succeeded: number;
  failed: number;
  results: { property_id: string; object_id: string; status: 'success' | 'failure'; error?: string }[];
}

export function executeInlineEditBatch(typeId: string, edits: InlineEditBatchEntry[]) {
  return api.post<InlineEditBatchResponse>(
    `/ontology/types/${typeId}/inline-edit-batch`,
    { edits },
  );
}

export interface RevertActionExecutionResponse {
  execution_id: string;
  reverted: boolean;
  message: string | null;
}

export function revertActionExecution(executionId: string) {
  return api.post<RevertActionExecutionResponse>(
    `/ontology/actions/executions/${executionId}/revert`,
    {},
  );
}

export function createObject(typeId: string, body: { properties: Record<string, unknown> }) {
  return api.post<ObjectInstance>(`/ontology/types/${typeId}/objects`, body);
}

export function deleteObject(typeId: string, objectId: string) {
  return api.delete(`/ontology/types/${typeId}/objects/${objectId}`);
}

export function listObjects(typeId: string, params?: { page?: number; per_page?: number }) {
  const qs = new URLSearchParams();
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  return api.get<{ data: ObjectInstance[]; total: number; page?: number; per_page?: number }>(
    `/ontology/types/${typeId}/objects${qs.toString() ? `?${qs}` : ''}`,
  );
}

export function getOntologyGraph(params?: {
  root_object_id?: string;
  root_type_id?: string;
  depth?: number;
  limit?: number;
}) {
  const qs = new URLSearchParams();
  if (params?.root_object_id) qs.set('root_object_id', params.root_object_id);
  if (params?.root_type_id) qs.set('root_type_id', params.root_type_id);
  if (params?.depth) qs.set('depth', String(params.depth));
  if (params?.limit) qs.set('limit', String(params.limit));
  return api.get<GraphResponse>(`/ontology/graph${qs.toString() ? `?${qs}` : ''}`);
}

export function listQuiverVisualFunctions(params?: {
  page?: number;
  per_page?: number;
  search?: string;
  include_shared?: boolean;
}) {
  const qs = new URLSearchParams();
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  if (params?.search) qs.set('search', params.search);
  if (typeof params?.include_shared === 'boolean') {
    qs.set('include_shared', params.include_shared ? 'true' : 'false');
  }
  return api.get<{ data: QuiverVisualFunction[]; total: number; page: number; per_page: number }>(
    `/ontology/quiver/visual-functions${qs.toString() ? `?${qs}` : ''}`,
  );
}

export function createQuiverVisualFunction(body: {
  name: string;
  description?: string;
  primary_type_id: string;
  secondary_type_id?: string | null;
  join_field: string;
  secondary_join_field?: string;
  date_field: string;
  metric_field: string;
  group_field: string;
  selected_group?: string | null;
  chart_kind?: QuiverChartKind;
  shared?: boolean;
}) {
  return api.post<QuiverVisualFunction>('/ontology/quiver/visual-functions', body);
}

export function updateQuiverVisualFunction(
  id: string,
  body: Partial<{
    name: string;
    description: string;
    primary_type_id: string;
    secondary_type_id: string | null;
    join_field: string;
    secondary_join_field: string;
    date_field: string;
    metric_field: string;
    group_field: string;
    selected_group: string | null;
    chart_kind: QuiverChartKind;
    shared: boolean;
  }>,
) {
  return api.patch<QuiverVisualFunction>(`/ontology/quiver/visual-functions/${id}`, body);
}

export function deleteQuiverVisualFunction(id: string) {
  return api.delete(`/ontology/quiver/visual-functions/${id}`);
}

// ────────────────────────────────────────────────────────────────
// Vertex slice — search, neighbors, scenario simulation. Used by
// /vertex; future ontology routes can extend the same module.
// ────────────────────────────────────────────────────────────────

export interface NeighborLink {
  direction: 'inbound' | 'outbound';
  link_id: string;
  link_type_id: string;
  link_name: string;
  object: ObjectInstance;
}

export interface SearchResult {
  kind: string;
  id: string;
  object_type_id: string | null;
  title: string;
  subtitle: string | null;
  snippet: string;
  score: number;
  route: string;
  metadata: Record<string, unknown>;
}

export interface ScenarioSimulationOperation {
  label?: string | null;
  target_object_id?: string | null;
  action_id?: string | null;
  action_parameters?: Record<string, unknown>;
  properties_patch?: Record<string, unknown>;
}

export interface ScenarioSimulationCandidate {
  name: string;
  description?: string | null;
  operations: ScenarioSimulationOperation[];
}

export interface ScenarioSummary {
  impacted_object_count: number;
  changed_object_count: number;
  deleted_object_count: number;
  automatic_rule_matches: number;
  automatic_rule_applications: number;
  advisory_rule_matches: number;
  schedule_count: number;
  impacted_types: string[];
  changed_properties: string[];
  boundary_crossings: number;
  sensitive_objects: number;
  failed_constraints: number;
  achieved_goals: number;
  total_goals: number;
  goal_score: number;
}

export interface ScenarioSimulationResult {
  scenario_id: string;
  name: string;
  description: string | null;
  graph: GraphResponse;
  summary: ScenarioSummary;
}

export interface ObjectScenarioSimulationResponse {
  root_object_id: string;
  root_type_id: string;
  compared_at: string;
  baseline: ScenarioSimulationResult | null;
  scenarios: ScenarioSimulationResult[];
}

export function searchOntology(body: {
  query: string;
  kind?: string;
  object_type_id?: string;
  limit?: number;
  semantic?: boolean;
}) {
  return api.post<{ query: string; total: number; data: SearchResult[] }>('/ontology/search', body);
}

export function listNeighbors(typeId: string, objectId: string) {
  return api
    .get<{ data: NeighborLink[] }>(`/ontology/types/${typeId}/objects/${objectId}/neighbors`)
    .then((response) => response.data);
}

export function simulateObjectScenarios(
  typeId: string,
  objectId: string,
  body: {
    scenarios: ScenarioSimulationCandidate[];
    depth?: number;
    include_baseline?: boolean;
  },
) {
  return api.post<ObjectScenarioSimulationResponse>(
    `/ontology/types/${typeId}/objects/${objectId}/scenarios/simulate`,
    body,
  );
}

// ────────────────────────────────────────────────────────────────
// Storage insights — used by /object-databases. Reflects PostgreSQL
// table metrics, index definitions, search projections, and runtime
// activity timestamps from the ontology runtime.
// ────────────────────────────────────────────────────────────────

export interface OntologyStorageTableMetric {
  key: string;
  table_name: string;
  label: string;
  role: string;
  record_count: number;
}

export interface OntologyStorageIndexDefinition {
  table_name: string;
  index_name: string;
  index_definition: string;
}

export interface OntologyStorageDistributionMetric {
  id: string;
  label: string;
  count: number;
}

export interface OntologyStorageSearchKindMetric {
  kind: string;
  count: number;
}

export interface OntologyStorageInsights {
  database_backend: string;
  access_driver: string;
  graph_projection: string;
  search_projection: string;
  funnel_runtime: string;
  table_metrics: OntologyStorageTableMetric[];
  index_definitions: OntologyStorageIndexDefinition[];
  object_type_distribution: OntologyStorageDistributionMetric[];
  link_type_distribution: OntologyStorageDistributionMetric[];
  search_documents_total: number;
  search_documents_by_kind: OntologyStorageSearchKindMetric[];
  latest_object_write_at: string | null;
  latest_link_write_at: string | null;
  latest_funnel_run_at: string | null;
}

export function getOntologyStorageInsights() {
  return api.get<OntologyStorageInsights>('/ontology/storage/insights');
}

// ────────────────────────────────────────────────────────────────
// Link types, interfaces, action types, and projects — used by
// /ontology-design (and incoming routes that touch the broader
// ontology surface).
// ────────────────────────────────────────────────────────────────

export interface LinkType {
  id: string;
  name: string;
  display_name: string;
  description: string;
  source_type_id: string;
  target_type_id: string;
  cardinality: string;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export interface OntologyInterface {
  id: string;
  name: string;
  display_name: string;
  description: string;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export type OntologyProjectRole = 'viewer' | 'editor' | 'owner';

export interface OntologyProject {
  id: string;
  slug: string;
  display_name: string;
  description: string;
  workspace_slug: string | null;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export interface OntologyProjectMembership {
  project_id: string;
  user_id: string;
  role: OntologyProjectRole;
  created_at: string;
  updated_at: string;
}

export type ActionOperationKind =
  | 'update_object'
  | 'create_link'
  | 'delete_object'
  | 'invoke_function'
  | 'invoke_webhook'
  | 'create_interface'
  | 'modify_interface'
  | 'delete_interface'
  | 'create_interface_link'
  | 'delete_interface_link';

export interface ActionInputField {
  name: string;
  display_name?: string | null;
  description?: string | null;
  property_type: string;
  required: boolean;
  default_value?: unknown;
  struct_fields?: ActionInputField[];
}

export interface ActionFormCondition {
  left: string;
  operator: string;
  right?: unknown;
}

export interface ActionFormSectionOverride {
  conditions?: ActionFormCondition[];
  hidden?: boolean;
  columns?: number;
  title?: string | null;
  description?: string | null;
}

export interface ActionFormSection {
  id: string;
  title?: string | null;
  description?: string | null;
  columns?: number;
  collapsible?: boolean;
  visible?: boolean;
  parameter_names?: string[];
  overrides?: ActionFormSectionOverride[];
}

export interface ActionFormParameterOverride {
  parameter_name: string;
  conditions?: ActionFormCondition[];
  hidden?: boolean;
  required?: boolean;
  default_value?: unknown;
  display_name?: string | null;
  description?: string | null;
}

export interface ActionFormSchema {
  sections?: ActionFormSection[];
  parameter_overrides?: ActionFormParameterOverride[];
}

export interface ActionAuthorizationPolicy {
  required_permission_keys?: string[];
  any_role?: string[];
  all_roles?: string[];
  attribute_equals?: Record<string, unknown>;
  allowed_markings?: string[];
  minimum_clearance?: string | null;
  deny_guest_sessions?: boolean;
}

export interface ActionType {
  id: string;
  name: string;
  display_name: string;
  description: string;
  object_type_id: string;
  operation_kind: ActionOperationKind;
  input_schema: ActionInputField[];
  form_schema: ActionFormSchema;
  config: unknown;
  confirmation_required: boolean;
  permission_key: string | null;
  authorization_policy: ActionAuthorizationPolicy;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export function listLinkTypes(params?: { object_type_id?: string; page?: number; per_page?: number }) {
  const qs = new URLSearchParams();
  if (params?.object_type_id) qs.set('object_type_id', params.object_type_id);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  return api.get<{ data: LinkType[]; total: number }>(`/ontology/links?${qs}`);
}

export function listInterfaces(params?: { page?: number; per_page?: number; search?: string }) {
  const qs = new URLSearchParams();
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  if (params?.search) qs.set('search', params.search);
  return api.get<{ data: OntologyInterface[]; total: number; page: number; per_page: number }>(
    `/ontology/interfaces?${qs}`,
  );
}

export function listActionTypes(params?: {
  object_type_id?: string;
  page?: number;
  per_page?: number;
  search?: string;
}) {
  const qs = new URLSearchParams();
  if (params?.object_type_id) qs.set('object_type_id', params.object_type_id);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  if (params?.search) qs.set('search', params.search);
  return api.get<{ data: ActionType[]; total: number; page: number; per_page: number }>(
    `/ontology/actions?${qs}`,
  );
}

export interface CreateActionTypeBody {
  name: string;
  display_name?: string;
  description?: string;
  object_type_id: string;
  operation_kind: ActionOperationKind;
  input_schema?: ActionInputField[];
  form_schema?: ActionFormSchema;
  config?: unknown;
  confirmation_required?: boolean;
  permission_key?: string;
  authorization_policy?: ActionAuthorizationPolicy;
}

export interface UpdateActionTypeBody {
  display_name?: string;
  description?: string;
  operation_kind?: ActionOperationKind;
  input_schema?: ActionInputField[];
  form_schema?: ActionFormSchema;
  config?: unknown;
  confirmation_required?: boolean;
  permission_key?: string;
  authorization_policy?: ActionAuthorizationPolicy;
}

export interface ActionWhatIfBranch {
  id: string;
  action_id: string;
  target_object_id: string | null;
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  preview: Record<string, unknown>;
  before_object: Record<string, unknown> | null;
  after_object: Record<string, unknown> | null;
  deleted: boolean;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export interface ValidateActionResponse {
  valid: boolean;
  errors: string[];
  preview: unknown;
}

export interface ExecuteActionResponse {
  action: ActionType;
  target_object_id: string | null;
  deleted: boolean;
  preview: unknown;
  object: unknown | null;
  link: unknown | null;
  result: unknown | null;
}

export interface ExecuteBatchActionResponse {
  action: ActionType;
  total: number;
  succeeded: number;
  failed: number;
  results: Array<Record<string, unknown>>;
}

export interface ActionMetricsResponse {
  action_id: string;
  window: string;
  success_count: number;
  failure_count: number;
  p95_duration_ms: number | null;
  failure_categories: Record<string, number>;
}

export function createActionType(body: CreateActionTypeBody) {
  return api.post<ActionType>('/ontology/actions', body);
}

export function updateActionType(id: string, body: UpdateActionTypeBody) {
  return api.put<ActionType>(`/ontology/actions/${id}`, body);
}

export function deleteActionType(id: string) {
  return api.delete(`/ontology/actions/${id}`);
}

export function validateAction(id: string, body: { target_object_id?: string; parameters?: Record<string, unknown> }) {
  return api.post<ValidateActionResponse>(`/ontology/actions/${id}/validate`, body);
}

export function executeAction(id: string, body: {
  target_object_id?: string;
  parameters?: Record<string, unknown>;
  justification?: string;
}) {
  return api.post<ExecuteActionResponse>(`/ontology/actions/${id}/execute`, body);
}

export function executeActionBatch(id: string, body: {
  target_object_ids: string[];
  parameters?: Record<string, unknown>;
  justification?: string;
}) {
  return api.post<ExecuteBatchActionResponse>(`/ontology/actions/${id}/execute-batch`, body);
}

export function getActionMetrics(actionId: string, params?: { window?: string }) {
  const qs = new URLSearchParams();
  if (params?.window) qs.set('window', params.window);
  const tail = qs.toString();
  return api.get<ActionMetricsResponse>(`/ontology/actions/${actionId}/metrics${tail ? `?${tail}` : ''}`);
}

export function listActionWhatIfBranches(
  id: string,
  params?: { target_object_id?: string; page?: number; per_page?: number },
) {
  const qs = new URLSearchParams();
  if (params?.target_object_id) qs.set('target_object_id', params.target_object_id);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  return api.get<{ data: ActionWhatIfBranch[]; total: number; page: number; per_page: number }>(
    `/ontology/actions/${id}/what-if?${qs}`,
  );
}

export function createActionWhatIfBranch(
  id: string,
  body: { target_object_id?: string; parameters?: Record<string, unknown>; name?: string; description?: string },
) {
  return api.post<ActionWhatIfBranch>(`/ontology/actions/${id}/what-if`, body);
}

export function deleteActionWhatIfBranch(id: string, branchId: string) {
  return api.delete(`/ontology/actions/${id}/what-if/${branchId}`);
}

export function listProjects(params?: { page?: number; per_page?: number; search?: string }) {
  const qs = new URLSearchParams();
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  if (params?.search) qs.set('search', params.search);
  return api.get<{ data: OntologyProject[]; total: number; page: number; per_page: number }>(
    `/ontology/projects?${qs}`,
  );
}

export function listProjectMemberships(id: string) {
  return api
    .get<{ data: OntologyProjectMembership[] }>(`/ontology/projects/${id}/memberships`)
    .then((response) => response.data);
}

export function createProject(body: {
  slug: string;
  display_name?: string;
  description?: string;
  workspace_slug?: string;
  folders?: Array<{ name: string; description?: string; parent_folder_id?: string | null }>;
}) {
  return api.post<OntologyProject>('/ontology/projects', body);
}

export function getProject(id: string) {
  return api.get<OntologyProject>(`/ontology/projects/${id}`);
}

export function updateProject(id: string, body: {
  display_name?: string;
  description?: string;
  workspace_slug?: string | null;
}) {
  return api.patch<OntologyProject>(`/ontology/projects/${id}`, {
    ...body,
    workspace_slug: typeof body.workspace_slug === 'undefined' ? undefined : body.workspace_slug,
  });
}

export function deleteProject(id: string) {
  return api.delete(`/ontology/projects/${id}`);
}

export function listProjectFolders(id: string) {
  return api
    .get<{ data: OntologyProjectFolder[] }>(`/ontology/projects/${id}/folders`)
    .then((response) => response.data);
}

export function createProjectFolder(id: string, body: {
  name: string;
  description?: string;
  parent_folder_id?: string | null;
}) {
  return api.post<OntologyProjectFolder>(`/ontology/projects/${id}/folders`, body);
}

export function bindProjectResource(id: string, body: { resource_kind: string; resource_id: string }) {
  return api.post<OntologyProjectResourceBinding>(`/ontology/projects/${id}/resources`, body);
}

export function unbindProjectResource(id: string, resourceKind: string, resourceId: string) {
  return api.delete(`/ontology/projects/${id}/resources/${resourceKind}/${resourceId}`);
}

export function upsertProjectMembership(id: string, body: { user_id: string; role: OntologyProjectRole }) {
  return api.post<OntologyProjectMembership>(`/ontology/projects/${id}/memberships`, body);
}

export function deleteProjectMembership(id: string, userId: string) {
  return api.delete(`/ontology/projects/${id}/memberships/${userId}`);
}

export function replaceProjectWorkingState(id: string, changes: OntologyStagedChange[]) {
  return api.put<OntologyProjectWorkingState>(`/ontology/projects/${id}/working-state`, { changes });
}

export function getActionType(id: string) {
  return api.get<ActionType>(`/ontology/actions/${id}`);
}

export function getFunctionPackage(id: string) {
  return api.get<FunctionPackage>(`/ontology/functions/${id}`);
}

export function getFunctionPackageMetricsForId(id: string) {
  return api.get<FunctionPackageMetrics>(`/ontology/functions/${id}/metrics`);
}

export function getOntologyFunnelSource(id: string) {
  return api.get<OntologyFunnelSource>(`/ontology/funnel/sources/${id}`);
}

export function getObjectInstance(typeId: string, objectId: string) {
  return api.get<ObjectInstance>(`/ontology/types/${typeId}/objects/${objectId}`);
}

// ────────────────────────────────────────────────────────────────
// Object-type bindings (dataset → object type materialization)
// ────────────────────────────────────────────────────────────────

export type ObjectTypeBindingSyncMode = 'snapshot' | 'incremental' | 'view';

export interface ObjectTypeBindingPropertyMapping {
  source_column: string;
  target_property: string;
  transform?: string | null;
}

export interface ObjectTypeBinding {
  id: string;
  object_type_id: string;
  dataset_id: string;
  dataset_branch?: string | null;
  dataset_version?: number | null;
  primary_key_column: string;
  property_mapping: ObjectTypeBindingPropertyMapping[];
  sync_mode: ObjectTypeBindingSyncMode;
  default_marking: string;
  preview_limit: number;
  owner_id: string;
  last_materialized_at?: string | null;
  last_run_status?: string | null;
  last_run_summary?: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
}

export interface MaterializeBindingResponse {
  binding_id: string;
  status: string;
  rows_read: number;
  inserted: number;
  updated: number;
  skipped: number;
  errors: number;
  dry_run: boolean;
  error_details?: unknown[];
}

export function listObjectTypeBindings(typeId: string) {
  return api.get<{ data: ObjectTypeBinding[] }>(`/ontology/types/${typeId}/bindings`);
}

export function createObjectTypeBinding(
  typeId: string,
  body: {
    dataset_id: string;
    dataset_branch?: string;
    dataset_version?: number;
    primary_key_column: string;
    property_mapping: ObjectTypeBindingPropertyMapping[];
    sync_mode: ObjectTypeBindingSyncMode;
    default_marking?: string;
    preview_limit?: number;
  },
) {
  return api.post<ObjectTypeBinding>(`/ontology/types/${typeId}/bindings`, body);
}

export function updateObjectTypeBinding(
  typeId: string,
  bindingId: string,
  body: Partial<{
    dataset_branch: string;
    dataset_version: number;
    primary_key_column: string;
    property_mapping: ObjectTypeBindingPropertyMapping[];
    sync_mode: ObjectTypeBindingSyncMode;
    default_marking: string;
    preview_limit: number;
  }>,
) {
  return api.patch<ObjectTypeBinding>(`/ontology/types/${typeId}/bindings/${bindingId}`, body);
}

export function deleteObjectTypeBinding(typeId: string, bindingId: string) {
  return api.delete(`/ontology/types/${typeId}/bindings/${bindingId}`);
}

export function materializeObjectTypeBinding(
  typeId: string,
  bindingId: string,
  body?: { dataset_branch?: string; dataset_version?: number; limit?: number; dry_run?: boolean },
) {
  return api.post<MaterializeBindingResponse>(
    `/ontology/types/${typeId}/bindings/${bindingId}/materialize`,
    body ?? {},
  );
}

// ────────────────────────────────────────────────────────────────
// Rules + machinery — used by /dynamic-scheduling.
// ────────────────────────────────────────────────────────────────

export type RuleEvaluationMode = 'advisory' | 'automatic';

export interface RuleTriggerSpec {
  equals?: Record<string, unknown>;
  numeric_gte?: Record<string, number>;
  numeric_lte?: Record<string, number>;
  exists?: string[];
  changed_properties?: string[];
  markings?: string[];
}

export interface RuleAlertSpec {
  severity: 'low' | 'medium' | 'high' | 'critical';
  title: string;
  message?: string | null;
}

export interface RuleScheduleSpec {
  property_name: string;
  offset_hours: number;
  priority_score?: number;
  estimated_duration_minutes?: number;
  required_capability?: string | null;
  constraint_tags?: string[];
  hard_deadline_hours?: number | null;
}

export interface RuleEffectSpec {
  object_patch?: Record<string, unknown> | null;
  schedule?: RuleScheduleSpec | null;
  alert?: RuleAlertSpec | null;
}

export interface OntologyRule {
  id: string;
  name: string;
  display_name: string;
  description: string;
  object_type_id: string;
  evaluation_mode: RuleEvaluationMode;
  trigger_spec: RuleTriggerSpec;
  effect_spec: RuleEffectSpec;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export interface MachineryInsight {
  rule_id: string;
  name: string;
  display_name: string;
  evaluation_mode: RuleEvaluationMode;
  matched_runs: number;
  total_runs: number;
  pending_schedules: number;
  overdue_schedules: number;
  avg_schedule_lead_hours: number | null;
  dynamic_pressure: string;
  last_matched_at: string | null;
  last_object_id: string | null;
}

export interface MachineryQueueItem {
  id: string;
  rule_id: string;
  rule_run_id: string;
  object_id: string;
  rule_name: string;
  rule_display_name: string;
  object_type_id: string;
  status: string;
  scheduled_for: string;
  priority_score: number;
  estimated_duration_minutes: number;
  required_capability: string | null;
  constraint_snapshot: Record<string, unknown>;
  created_by: string;
  created_at: string;
  updated_at: string;
  started_at: string | null;
  completed_at: string | null;
}

export interface MachineryCapabilityLoad {
  capability: string;
  pending_count: number;
  total_estimated_minutes: number;
}

export interface MachineryQueueRecommendation {
  generated_at: string;
  strategy: string;
  queue_depth: number;
  overdue_count: number;
  total_estimated_minutes: number;
  next_due_at: string | null;
  recommended_order: string[];
  capability_load: MachineryCapabilityLoad[];
}

export interface MachineryQueueResponse {
  object_type_id: string | null;
  data: MachineryQueueItem[];
  recommendation: MachineryQueueRecommendation;
}

export function listRules(params?: {
  object_type_id?: string;
  search?: string;
  page?: number;
  per_page?: number;
}) {
  const qs = new URLSearchParams();
  if (params?.object_type_id) qs.set('object_type_id', params.object_type_id);
  if (params?.search) qs.set('search', params.search);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  return api.get<{ data: OntologyRule[]; total: number; page: number; per_page: number }>(
    `/ontology/rules?${qs}`,
  );
}

export function getMachineryInsights(params?: { object_type_id?: string }) {
  const qs = new URLSearchParams();
  if (params?.object_type_id) qs.set('object_type_id', params.object_type_id);
  return api.get<{ object_type_id: string | null; data: MachineryInsight[] }>(
    `/ontology/rules/insights?${qs}`,
  );
}

export function getMachineryQueue(params?: { object_type_id?: string }) {
  const qs = new URLSearchParams();
  if (params?.object_type_id) qs.set('object_type_id', params.object_type_id);
  return api.get<MachineryQueueResponse>(`/ontology/rules/machinery/queue?${qs}`);
}

export function updateMachineryQueueItem(id: string, body: { status: string }) {
  return api.patch<MachineryQueueItem>(`/ontology/rules/machinery/queue/${id}`, body);
}

// ────────────────────────────────────────────────────────────────
// Interface mutations + bindings — used by /interfaces.
// ────────────────────────────────────────────────────────────────

export interface InterfaceProperty {
  id: string;
  interface_id: string;
  name: string;
  display_name: string;
  description: string;
  property_type: string;
  required: boolean;
  unique_constraint: boolean;
  time_dependent: boolean;
  default_value: unknown;
  validation_rules: unknown;
  created_at: string;
  updated_at: string;
}

export interface ObjectTypeInterfaceBinding {
  object_type_id: string;
  interface_id: string;
  created_at: string;
}

export function createInterface(body: { name: string; display_name?: string; description?: string }) {
  return api.post<OntologyInterface>('/ontology/interfaces', body);
}

export function getInterface(id: string) {
  return api.get<OntologyInterface>(`/ontology/interfaces/${id}`);
}

export function updateInterface(id: string, body: { display_name?: string; description?: string }) {
  return api.patch<OntologyInterface>(`/ontology/interfaces/${id}`, body);
}

export function deleteInterface(id: string) {
  return api.delete(`/ontology/interfaces/${id}`);
}

export function listInterfaceProperties(id: string) {
  return api
    .get<{ data: InterfaceProperty[] }>(`/ontology/interfaces/${id}/properties`)
    .then((response) => response.data);
}

export function createInterfaceProperty(
  id: string,
  body: {
    name: string;
    display_name?: string;
    description?: string;
    property_type: string;
    required?: boolean;
    unique_constraint?: boolean;
    time_dependent?: boolean;
    default_value?: unknown;
    validation_rules?: unknown;
  },
) {
  return api.post<InterfaceProperty>(`/ontology/interfaces/${id}/properties`, body);
}

export function updateInterfaceProperty(
  id: string,
  propertyId: string,
  body: {
    display_name?: string;
    description?: string;
    required?: boolean;
    unique_constraint?: boolean;
    time_dependent?: boolean;
    default_value?: unknown;
    validation_rules?: unknown;
  },
) {
  return api.patch<InterfaceProperty>(`/ontology/interfaces/${id}/properties/${propertyId}`, body);
}

export function deleteInterfaceProperty(id: string, propertyId: string) {
  return api.delete(`/ontology/interfaces/${id}/properties/${propertyId}`);
}

export function listTypeInterfaces(typeId: string) {
  return api
    .get<{ data: OntologyInterface[] }>(`/ontology/types/${typeId}/interfaces`)
    .then((response) => response.data);
}

export function attachInterfaceToType(typeId: string, interfaceId: string) {
  return api.post<ObjectTypeInterfaceBinding>(`/ontology/types/${typeId}/interfaces/${interfaceId}`, {});
}

export function detachInterfaceFromType(typeId: string, interfaceId: string) {
  return api.delete(`/ontology/types/${typeId}/interfaces/${interfaceId}`);
}

export function updateObject(
  typeId: string,
  objectId: string,
  body: { properties: Record<string, unknown>; replace?: boolean; marking?: string },
) {
  return api.patch<ObjectInstance>(`/ontology/types/${typeId}/objects/${objectId}`, body);
}

// ────────────────────────────────────────────────────────────────
// Object views, rules, simulations — used by /object-views,
// /object-explorer, /foundry-rules, /machinery, etc.
// ────────────────────────────────────────────────────────────────

export interface RuleMatchResponse {
  rule_id: string;
  matched: boolean;
  trigger_payload: Record<string, unknown>;
  effect_preview: Record<string, unknown> | null;
}

export interface OntologyRuleRun {
  id: string;
  rule_id: string;
  object_id: string;
  matched: boolean;
  simulated: boolean;
  trigger_payload: Record<string, unknown>;
  effect_preview: Record<string, unknown> | null;
  created_by: string;
  created_at: string;
}

export interface ObjectViewResponse {
  object: ObjectInstance;
  summary: Record<string, unknown>;
  neighbors: NeighborLink[];
  graph: GraphResponse;
  applicable_actions: ActionType[];
  matching_rules: RuleMatchResponse[];
  recent_rule_runs: OntologyRuleRun[];
  timeline: Array<Record<string, unknown>>;
}

export function getObjectView(typeId: string, objectId: string) {
  return api.get<ObjectViewResponse>(`/ontology/types/${typeId}/objects/${objectId}/view`);
}

// ────────────────────────────────────────────────────────────────
// Object sets — used by /object-explorer.
// ────────────────────────────────────────────────────────────────

export interface ObjectSetPolicy {
  allowed_markings: string[];
  minimum_clearance: string | null;
  deny_guest_sessions: boolean;
  required_restricted_view_id: string | null;
}

export interface ObjectSetFilter {
  field: string;
  operator: string;
  value: unknown;
}

export interface ObjectSetTraversal {
  direction: 'outbound' | 'inbound' | 'both';
  link_type_id: string | null;
  target_object_type_id: string | null;
  max_hops: number;
}

export interface ObjectSetJoin {
  secondary_object_type_id: string;
  left_field: string;
  right_field: string;
  join_kind: 'inner' | 'left';
}

export interface ObjectSetDefinition {
  id: string;
  name: string;
  description: string;
  base_object_type_id: string;
  filters: ObjectSetFilter[];
  traversals: ObjectSetTraversal[];
  join: ObjectSetJoin | null;
  projections: string[];
  what_if_label: string | null;
  policy: ObjectSetPolicy;
  materialized_snapshot: unknown[] | null;
  materialized_at: string | null;
  materialized_row_count: number;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export interface ObjectSetEvaluationResponse {
  object_set: ObjectSetDefinition;
  total_base_matches: number;
  total_rows: number;
  traversal_neighbor_count: number;
  rows: Record<string, unknown>[];
  generated_at: string;
  materialized: boolean;
}

export function listObjectSets() {
  return api.get<{ data: ObjectSetDefinition[] }>('/ontology/object-sets');
}

export function createObjectSet(body: {
  name: string;
  description?: string;
  base_object_type_id: string;
  filters?: ObjectSetFilter[];
  traversals?: ObjectSetTraversal[];
  join?: ObjectSetJoin | null;
  projections?: string[];
  what_if_label?: string | null;
  policy?: Partial<ObjectSetPolicy>;
}) {
  return api.post<ObjectSetDefinition>('/ontology/object-sets', body);
}

export function updateObjectSet(
  id: string,
  body: Partial<{
    name: string;
    description: string;
    base_object_type_id: string;
    filters: ObjectSetFilter[];
    traversals: ObjectSetTraversal[];
    join: ObjectSetJoin | null;
    projections: string[];
    what_if_label: string | null;
    policy: ObjectSetPolicy;
  }>,
) {
  return api.patch<ObjectSetDefinition>(`/ontology/object-sets/${id}`, body);
}

export function deleteObjectSet(id: string) {
  return api.delete(`/ontology/object-sets/${id}`);
}

export function evaluateObjectSet(id: string, body?: { limit?: number }) {
  return api.post<ObjectSetEvaluationResponse>(`/ontology/object-sets/${id}/evaluate`, body ?? {});
}

export function materializeObjectSet(id: string, body?: { limit?: number }) {
  return api.post<ObjectSetEvaluationResponse>(`/ontology/object-sets/${id}/materialize`, body ?? {});
}

// ────────────────────────────────────────────────────────────────
// Ontology funnel sources / runs / health — used by /ontology-indexing.
// ────────────────────────────────────────────────────────────────

export interface OntologyFunnelPropertyMapping {
  source_field: string;
  target_property: string;
}

export interface OntologyFunnelSource {
  id: string;
  name: string;
  description: string;
  object_type_id: string;
  dataset_id: string;
  pipeline_id: string | null;
  dataset_branch: string | null;
  dataset_version: number | null;
  preview_limit: number;
  default_marking: string;
  status: string;
  property_mappings: OntologyFunnelPropertyMapping[];
  trigger_context: Record<string, unknown>;
  owner_id: string;
  last_run_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface OntologyFunnelRun {
  id: string;
  source_id: string;
  object_type_id: string;
  dataset_id: string;
  pipeline_id: string | null;
  pipeline_run_id: string | null;
  status: string;
  trigger_type: string;
  started_by: string | null;
  rows_read: number;
  inserted_count: number;
  updated_count: number;
  skipped_count: number;
  error_count: number;
  details: Record<string, unknown>;
  error_message: string | null;
  started_at: string;
  finished_at: string | null;
}

export interface OntologyFunnelSourceHealth {
  source: OntologyFunnelSource;
  health_status: string;
  health_reason: string;
  total_runs: number;
  successful_runs: number;
  failed_runs: number;
  warning_runs: number;
  success_rate: number;
  avg_duration_ms: number | null;
  p95_duration_ms: number | null;
  max_duration_ms: number | null;
  latest_run_status: string | null;
  last_run_at: string | null;
  last_success_at: string | null;
  last_failure_at: string | null;
  last_warning_at: string | null;
  rows_read: number;
  inserted_count: number;
  updated_count: number;
  skipped_count: number;
  error_count: number;
}

export interface OntologyFunnelHealthSummary {
  stale_after_hours: number;
  total_sources: number;
  active_sources: number;
  paused_sources: number;
  healthy_sources: number;
  degraded_sources: number;
  failing_sources: number;
  stale_sources: number;
  never_run_sources: number;
  total_runs: number;
  successful_runs: number;
  failed_runs: number;
  warning_runs: number;
  success_rate: number;
  rows_read: number;
  inserted_count: number;
  updated_count: number;
  skipped_count: number;
  error_count: number;
  last_run_at: string | null;
  sources: OntologyFunnelSourceHealth[];
}

export function getOntologyFunnelHealth(params?: {
  object_type_id?: string;
  stale_after_hours?: number;
}) {
  const qs = new URLSearchParams();
  if (params?.object_type_id) qs.set('object_type_id', params.object_type_id);
  if (params?.stale_after_hours) qs.set('stale_after_hours', String(params.stale_after_hours));
  return api.get<OntologyFunnelHealthSummary>(`/ontology/funnel/health?${qs}`);
}

export function listOntologyFunnelSources(params?: {
  object_type_id?: string;
  status?: string;
  page?: number;
  per_page?: number;
}) {
  const qs = new URLSearchParams();
  if (params?.object_type_id) qs.set('object_type_id', params.object_type_id);
  if (params?.status) qs.set('status', params.status);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  return api.get<{ data: OntologyFunnelSource[]; total: number; page: number; per_page: number }>(
    `/ontology/funnel/sources?${qs}`,
  );
}

export function createOntologyFunnelSource(body: {
  name: string;
  description?: string;
  object_type_id: string;
  dataset_id: string;
  pipeline_id?: string | null;
  dataset_branch?: string | null;
  dataset_version?: number | null;
  preview_limit?: number;
  default_marking?: string;
  status?: string;
  property_mappings?: OntologyFunnelPropertyMapping[];
  trigger_context?: Record<string, unknown>;
}) {
  return api.post<OntologyFunnelSource>('/ontology/funnel/sources', body);
}

export function updateOntologyFunnelSource(
  id: string,
  body: {
    name?: string;
    description?: string;
    pipeline_id?: string | null;
    dataset_branch?: string | null;
    dataset_version?: number | null;
    preview_limit?: number;
    default_marking?: string;
    status?: string;
    property_mappings?: OntologyFunnelPropertyMapping[];
    trigger_context?: Record<string, unknown>;
  },
) {
  return api.patch<OntologyFunnelSource>(`/ontology/funnel/sources/${id}`, body);
}

export function deleteOntologyFunnelSource(id: string) {
  return api.delete(`/ontology/funnel/sources/${id}`);
}

export function triggerOntologyFunnelRun(
  id: string,
  body?: {
    limit?: number;
    dataset_branch?: string;
    dataset_version?: number;
    skip_pipeline?: boolean;
    dry_run?: boolean;
    trigger_context?: Record<string, unknown>;
  },
) {
  return api.post<OntologyFunnelRun>(`/ontology/funnel/sources/${id}/run`, body ?? {});
}

export function listOntologyFunnelRuns(id: string, params?: { page?: number; per_page?: number }) {
  const qs = new URLSearchParams();
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  return api.get<{ data: OntologyFunnelRun[]; total: number; page: number; per_page: number }>(
    `/ontology/funnel/sources/${id}/runs?${qs}`,
  );
}

// ────────────────────────────────────────────────────────────────
// Project branches/proposals/migrations + shared property types —
// used by /ontologies, /ontology-manager, /ontology.
// ────────────────────────────────────────────────────────────────

export interface SharedPropertyType {
  id: string;
  name: string;
  display_name: string;
  description: string;
  property_type: string;
  required: boolean;
  unique_constraint: boolean;
  time_dependent: boolean;
  default_value: unknown;
  validation_rules: unknown;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export interface OntologyProjectResourceBinding {
  project_id: string;
  resource_kind: string;
  resource_id: string;
  bound_by: string;
  created_at: string;
}

export interface OntologyProjectMigrationResource {
  resource_kind: string;
  resource_id: string;
  label?: string;
}

export interface OntologyProjectMigration {
  id: string;
  project_id: string;
  source_project_id: string;
  target_project_id: string;
  resources: OntologyProjectMigrationResource[];
  submitted_at: string;
  status: string;
  note: string;
  submitted_by: string;
}

export interface OntologyStagedChange {
  id: string;
  kind: string;
  action: string;
  label: string;
  description: string;
  targetId?: string;
  parentRef?: {
    kind: string;
    id?: string;
    name?: string;
    slug?: string;
  };
  payload: Record<string, unknown>;
  warnings: string[];
  errors: string[];
  source: string;
  createdAt: string;
}

export interface OntologyProjectWorkingState {
  project_id: string;
  changes: OntologyStagedChange[];
  updated_by: string;
  updated_at: string;
}

export interface OntologyBranch {
  id: string;
  project_id: string;
  name: string;
  description: string;
  status: 'main' | 'draft' | 'in_review' | 'rebasing' | 'merged' | 'closed';
  proposal_id: string | null;
  changes: OntologyStagedChange[];
  conflict_resolutions: Record<string, 'main' | 'branch' | 'custom'>;
  enable_indexing: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
  latest_rebased_at: string;
}

export interface OntologyProposalTask {
  id: string;
  change_id: string;
  title: string;
  description: string;
  status: 'pending' | 'approved' | 'rejected';
  reviewer_id: string | null;
  comments: string[];
}

export interface OntologyProposalComment {
  id: string;
  author: string;
  body: string;
  created_at: string;
}

export interface OntologyProposal {
  id: string;
  project_id: string;
  branch_id: string;
  title: string;
  description: string;
  status: 'draft' | 'in_review' | 'approved' | 'merged' | 'closed';
  reviewer_ids: string[];
  tasks: OntologyProposalTask[];
  comments: OntologyProposalComment[];
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface OntologyProjectFolder {
  id: string;
  project_id: string;
  parent_folder_id: string | null;
  name: string;
  slug: string;
  description: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export function listSharedPropertyTypes(params?: { page?: number; per_page?: number; search?: string }) {
  const qs = new URLSearchParams();
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  if (params?.search) qs.set('search', params.search);
  return api.get<{ data: SharedPropertyType[]; total: number; page: number; per_page: number }>(
    `/ontology/shared-property-types?${qs}`,
  );
}

export function listProjectResources(id: string) {
  return api
    .get<{ data: OntologyProjectResourceBinding[] }>(`/ontology/projects/${id}/resources`)
    .then((r) => r.data);
}

export function getProjectWorkingState(id: string) {
  return api.get<OntologyProjectWorkingState>(`/ontology/projects/${id}/working-state`);
}

export function listProjectBranches(id: string) {
  return api.get<{ data: OntologyBranch[] }>(`/ontology/projects/${id}/branches`).then((r) => r.data);
}

export function createProjectBranch(
  id: string,
  body: { name: string; description?: string; changes: OntologyStagedChange[]; enable_indexing?: boolean },
) {
  return api.post<OntologyBranch>(`/ontology/projects/${id}/branches`, body);
}

export function updateProjectBranch(
  id: string,
  branchId: string,
  body: {
    description?: string;
    status?: OntologyBranch['status'];
    proposal_id?: string | null;
    changes?: OntologyStagedChange[];
    conflict_resolutions?: Record<string, 'main' | 'branch' | 'custom'>;
    enable_indexing?: boolean;
    latest_rebased_at?: string;
  },
) {
  return api.patch<OntologyBranch>(`/ontology/projects/${id}/branches/${branchId}`, body);
}

export function listProjectProposals(id: string) {
  return api.get<{ data: OntologyProposal[] }>(`/ontology/projects/${id}/proposals`).then((r) => r.data);
}

export function createProjectProposal(
  id: string,
  body: {
    branch_id: string;
    title: string;
    description?: string;
    reviewer_ids?: string[];
    tasks: OntologyProposalTask[];
    comments?: OntologyProposalComment[];
  },
) {
  return api.post<OntologyProposal>(`/ontology/projects/${id}/proposals`, body);
}

export function updateProjectProposal(
  id: string,
  proposalId: string,
  body: {
    title?: string;
    description?: string;
    status?: OntologyProposal['status'];
    reviewer_ids?: string[];
    tasks?: OntologyProposalTask[];
    comments?: OntologyProposalComment[];
  },
) {
  return api.patch<OntologyProposal>(`/ontology/projects/${id}/proposals/${proposalId}`, body);
}

export function listProjectMigrations(id: string) {
  return api.get<{ data: OntologyProjectMigration[] }>(`/ontology/projects/${id}/migrations`).then((r) => r.data);
}

export function createProjectMigration(
  id: string,
  body: {
    source_project_id: string;
    target_project_id: string;
    resources: OntologyProjectMigration['resources'];
    note?: string;
  },
) {
  return api.post<OntologyProjectMigration>(`/ontology/projects/${id}/migrations`, body);
}

// ────────────────────────────────────────────────────────────────
// Function packages — used by /functions, /object-monitors, /ml.
// ────────────────────────────────────────────────────────────────

export interface FunctionCapabilities {
  allow_ontology_read: boolean;
  allow_ontology_write: boolean;
  allow_ai: boolean;
  allow_network: boolean;
  timeout_seconds: number;
  max_source_bytes: number;
}

export interface FunctionPackage {
  id: string;
  name: string;
  version: string;
  display_name: string;
  description: string;
  runtime: string;
  source: string;
  entrypoint: string;
  capabilities: FunctionCapabilities;
  owner_id: string;
  created_at: string;
  updated_at: string;
}

export interface FunctionPackageRun {
  id: string;
  function_package_id: string;
  function_package_name: string;
  function_package_version: string;
  runtime: string;
  status: 'success' | 'failure' | string;
  invocation_kind: 'simulation' | 'action' | string;
  action_id: string | null;
  action_name: string | null;
  object_type_id: string | null;
  target_object_id: string | null;
  actor_id: string;
  duration_ms: number;
  error_message: string | null;
  started_at: string;
  completed_at: string;
}

export interface FunctionPackageMetrics {
  package: {
    id: string;
    name: string;
    version: string;
    display_name: string;
    runtime: string;
    entrypoint: string;
    capabilities: FunctionCapabilities;
  };
  total_runs: number;
  successful_runs: number;
  failed_runs: number;
  simulation_runs: number;
  action_runs: number;
  success_rate: number;
  avg_duration_ms: number | null;
  p95_duration_ms: number | null;
  max_duration_ms: number | null;
  last_run_at: string | null;
  last_success_at: string | null;
  last_failure_at: string | null;
}

export function listFunctionPackages(params?: { runtime?: string; search?: string; page?: number; per_page?: number }) {
  const qs = new URLSearchParams();
  if (params?.runtime) qs.set('runtime', params.runtime);
  if (params?.search) qs.set('search', params.search);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  return api.get<{ data: FunctionPackage[]; total: number; page: number; per_page: number }>(
    `/ontology/functions?${qs}`,
  );
}

export function createFunctionPackage(body: {
  name: string;
  version?: string;
  display_name?: string;
  description?: string;
  runtime: string;
  source: string;
  entrypoint?: string;
  capabilities?: Partial<FunctionCapabilities>;
}) {
  return api.post<FunctionPackage>('/ontology/functions', body);
}

export function updateFunctionPackage(
  id: string,
  body: {
    display_name?: string;
    description?: string;
    source?: string;
    entrypoint?: string;
    capabilities?: Partial<FunctionCapabilities>;
  },
) {
  return api.patch<FunctionPackage>(`/ontology/functions/${id}`, body);
}

export function deleteFunctionPackage(id: string) {
  return api.delete(`/ontology/functions/${id}`);
}

export function listFunctionPackageRuns(params?: {
  function_package_id?: string;
  invocation_kind?: string;
  status?: string;
  page?: number;
  per_page?: number;
}) {
  const qs = new URLSearchParams();
  if (params?.function_package_id) qs.set('function_package_id', params.function_package_id);
  if (params?.invocation_kind) qs.set('invocation_kind', params.invocation_kind);
  if (params?.status) qs.set('status', params.status);
  if (params?.page) qs.set('page', String(params.page));
  if (params?.per_page) qs.set('per_page', String(params.per_page));
  return api.get<{ data: FunctionPackageRun[]; total: number; page: number; per_page: number }>(
    `/ontology/functions/runs?${qs}`,
  );
}

export function listFunctionPackageMetrics() {
  return api.get<{ data: FunctionPackageMetrics[] }>('/ontology/functions/metrics');
}

export function executeFunctionPackage(id: string, body: { input: Record<string, unknown> }) {
  return api.post<{ result: unknown; logs: unknown[]; duration_ms: number }>(
    `/ontology/functions/${id}/execute`,
    body,
  );
}

// ────────────────────────────────────────────────────────────────
// Object/Link/Property/SharedProperty mutations — used by
// /object-link-types, /ontology-manager, /ontology, /action-types.
// ────────────────────────────────────────────────────────────────

export interface PropertyInlineEditConfig {
  action_type_id: string;
  input_name?: string | null;
}

export function createObjectType(body: {
  name: string;
  display_name?: string;
  description?: string;
  primary_key_property?: string;
  icon?: string;
  color?: string;
}) {
  return api.post<ObjectType>('/ontology/types', body);
}

export function updateObjectType(
  id: string,
  body: {
    display_name?: string;
    description?: string;
    primary_key_property?: string;
    icon?: string;
    color?: string;
  },
) {
  return api.put<ObjectType>(`/ontology/types/${id}`, body);
}

export function deleteObjectType(id: string) {
  return api.delete(`/ontology/types/${id}`);
}

export function createProperty(
  typeId: string,
  body: {
    name: string;
    display_name?: string;
    description?: string;
    property_type: string;
    required?: boolean;
    unique_constraint?: boolean;
    time_dependent?: boolean;
    default_value?: unknown;
    validation_rules?: unknown;
    inline_edit_config?: PropertyInlineEditConfig | null;
  },
) {
  return api.post<Property>(`/ontology/types/${typeId}/properties`, body);
}

export function updateProperty(
  typeId: string,
  propertyId: string,
  body: {
    display_name?: string;
    description?: string;
    required?: boolean;
    unique_constraint?: boolean;
    time_dependent?: boolean;
    default_value?: unknown;
    validation_rules?: unknown;
    inline_edit_config?: PropertyInlineEditConfig | null;
  },
) {
  return api.patch<Property>(`/ontology/types/${typeId}/properties/${propertyId}`, body);
}

export function deleteProperty(typeId: string, propertyId: string) {
  return api.delete(`/ontology/types/${typeId}/properties/${propertyId}`);
}

export function createSharedPropertyType(body: {
  name: string;
  display_name?: string;
  description?: string;
  property_type: string;
  required?: boolean;
  unique_constraint?: boolean;
  time_dependent?: boolean;
  default_value?: unknown;
  validation_rules?: unknown;
}) {
  return api.post<SharedPropertyType>('/ontology/shared-property-types', body);
}

export function updateSharedPropertyType(
  id: string,
  body: {
    display_name?: string;
    description?: string;
    required?: boolean;
    unique_constraint?: boolean;
    time_dependent?: boolean;
    default_value?: unknown;
    validation_rules?: unknown;
  },
) {
  return api.patch<SharedPropertyType>(`/ontology/shared-property-types/${id}`, body);
}

export function deleteSharedPropertyType(id: string) {
  return api.delete(`/ontology/shared-property-types/${id}`);
}

export function listTypeSharedPropertyTypes(typeId: string) {
  return api
    .get<{ data: SharedPropertyType[] }>(`/ontology/types/${typeId}/shared-property-types`)
    .then((response) => response.data);
}

export function attachSharedPropertyType(typeId: string, sharedPropertyTypeId: string) {
  return api.post<{ object_type_id: string; shared_property_type_id: string }>(
    `/ontology/types/${typeId}/shared-property-types/${sharedPropertyTypeId}`,
    {},
  );
}

export function detachSharedPropertyType(typeId: string, sharedPropertyTypeId: string) {
  return api.delete(`/ontology/types/${typeId}/shared-property-types/${sharedPropertyTypeId}`);
}

export function createLinkType(body: {
  name: string;
  display_name?: string;
  description?: string;
  source_type_id: string;
  target_type_id: string;
  cardinality?: string;
}) {
  return api.post<LinkType>('/ontology/links', body);
}

export function updateLinkType(id: string, body: { display_name?: string; description?: string; cardinality?: string }) {
  return api.patch<LinkType>(`/ontology/links/${id}`, body);
}

export function deleteLinkType(id: string) {
  return api.delete(`/ontology/links/${id}`);
}

// Rule mutations + simulation
export function createRule(body: {
  name: string;
  display_name?: string;
  description?: string;
  object_type_id: string;
  evaluation_mode?: RuleEvaluationMode;
  trigger_spec?: RuleTriggerSpec;
  effect_spec?: RuleEffectSpec;
}) {
  return api.post<OntologyRule>('/ontology/rules', body);
}

export function updateRule(
  id: string,
  body: {
    display_name?: string;
    description?: string;
    evaluation_mode?: RuleEvaluationMode;
    trigger_spec?: RuleTriggerSpec;
    effect_spec?: RuleEffectSpec;
  },
) {
  return api.patch<OntologyRule>(`/ontology/rules/${id}`, body);
}

export function simulateRule(id: string, body: { object_id: string; properties_patch?: Record<string, unknown> }) {
  return api.post<{
    rule: OntologyRule;
    matched: boolean;
    trigger_payload: Record<string, unknown>;
    effect_preview: Record<string, unknown> | null;
    object: ObjectInstance;
  }>(`/ontology/rules/${id}/simulate`, body);
}

export function applyRule(id: string, body: { object_id: string; properties_patch?: Record<string, unknown> }) {
  return api.post<{
    rule: OntologyRule;
    matched: boolean;
    trigger_payload: Record<string, unknown>;
    effect_preview: Record<string, unknown> | null;
    object: ObjectInstance;
  }>(`/ontology/rules/${id}/apply`, body);
}

export function deleteRule(id: string) {
  return api.delete(`/ontology/rules/${id}`);
}
