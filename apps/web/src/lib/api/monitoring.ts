import { api } from './client';

export type ResourceType =
  | 'STREAMING_DATASET'
  | 'STREAMING_PIPELINE'
  | 'TIME_SERIES_SYNC'
  | 'GEOTEMPORAL_OBSERVATIONS';

export type MonitorKind =
  | 'INGEST_RECORDS'
  | 'OUTPUT_RECORDS'
  | 'CHECKPOINT_LIVENESS'
  | 'LAST_CHECKPOINT_DURATION'
  | 'CHECKPOINT_TRIGGER_FAILURES'
  | 'CONSECUTIVE_CHECKPOINT_FAILURES'
  | 'TOTAL_LAG'
  | 'TOTAL_THROUGHPUT'
  | 'UTILIZATION'
  | 'POINTS_WRITTEN_TO_TS'
  | 'GEOTEMPORAL_OBS_SENT';

export type Comparator = 'LT' | 'LTE' | 'GT' | 'GTE' | 'EQ';
export type Severity = 'INFO' | 'WARN' | 'CRITICAL';

export interface MonitoringView {
  id: string;
  name: string;
  description: string;
  project_rid: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface MonitorRule {
  id: string;
  view_id: string;
  name: string;
  resource_type: ResourceType;
  resource_rid: string;
  monitor_kind: MonitorKind;
  window_seconds: number;
  comparator: Comparator;
  threshold: number;
  severity: Severity;
  enabled: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface MonitorEvaluation {
  id: string;
  rule_id: string;
  evaluated_at: string;
  observed_value: number;
  fired: boolean;
  alert_id: string | null;
}

interface ListResponse<T> {
  data: T[];
}

export interface CreateMonitoringViewRequest {
  name: string;
  description?: string;
  project_rid: string;
}

export interface CreateMonitorRuleRequest {
  view_id: string;
  name?: string;
  resource_type: ResourceType;
  resource_rid: string;
  monitor_kind: MonitorKind;
  window_seconds: number;
  comparator: Comparator;
  threshold: number;
  severity?: Severity;
}

export interface PatchMonitorRuleRequest {
  name?: string;
  window_seconds?: number;
  comparator?: Comparator;
  threshold?: number;
  severity?: Severity;
  enabled?: boolean;
}

export function listMonitoringViews() {
  return api.get<ListResponse<MonitoringView>>('/monitoring/monitoring-views');
}

export function createMonitoringView(body: CreateMonitoringViewRequest) {
  return api.post<MonitoringView>('/monitoring/monitoring-views', body);
}

export function getMonitoringView(id: string) {
  return api.get<MonitoringView>(`/monitoring/monitoring-views/${id}`);
}

export function listRulesForView(viewId: string) {
  return api.get<ListResponse<MonitorRule>>(`/monitoring/monitoring-views/${viewId}/rules`);
}

export function listMonitorRules(filters: {
  resource_type?: ResourceType;
  resource_rid?: string;
  monitor_kind?: MonitorKind;
} = {}) {
  const params = new URLSearchParams();
  if (filters.resource_type) params.set('resource_type', filters.resource_type);
  if (filters.resource_rid) params.set('resource_rid', filters.resource_rid);
  if (filters.monitor_kind) params.set('monitor_kind', filters.monitor_kind);
  const qs = params.toString();
  const url = qs ? `/monitoring/monitor-rules?${qs}` : '/monitoring/monitor-rules';
  return api.get<ListResponse<MonitorRule>>(url);
}

export function createMonitorRule(body: CreateMonitorRuleRequest) {
  return api.post<MonitorRule>('/monitoring/monitor-rules', body);
}

export function patchMonitorRule(id: string, body: PatchMonitorRuleRequest) {
  return api.patch<MonitorRule>(`/monitoring/monitor-rules/${id}`, body);
}

export function deleteMonitorRule(id: string) {
  return api.delete<void>(`/monitoring/monitor-rules/${id}`);
}

export function listEvaluationsForRule(ruleId: string, limit = 50) {
  return api.get<ListResponse<MonitorEvaluation>>(`/monitoring/monitor-rules/${ruleId}/evaluations?limit=${limit}`);
}

export const MONITOR_KIND_CATALOGUE: Array<{
  group: 'INGEST' | 'OUTPUT' | 'CHECKPOINT' | 'PERFORMANCE';
  kind: MonitorKind;
  label: string;
  resourceTypes: ResourceType[];
  hint: string;
}> = [
  { group: 'INGEST', kind: 'INGEST_RECORDS', label: 'Records ingested', resourceTypes: ['STREAMING_DATASET'], hint: 'Alert when fewer than threshold records are written to the live view.' },
  { group: 'OUTPUT', kind: 'OUTPUT_RECORDS', label: 'Output records', resourceTypes: ['STREAMING_PIPELINE'], hint: 'Alert on processed records emitted by the streaming pipeline.' },
  { group: 'OUTPUT', kind: 'POINTS_WRITTEN_TO_TS', label: 'Points written to Time Series DB', resourceTypes: ['TIME_SERIES_SYNC'], hint: 'Beta: alerts on points written by a time-series sync.' },
  { group: 'OUTPUT', kind: 'GEOTEMPORAL_OBS_SENT', label: 'Geotemporal observations sent', resourceTypes: ['GEOTEMPORAL_OBSERVATIONS'], hint: 'Beta: alerts on observations forwarded to the geotime ingest.' },
  { group: 'CHECKPOINT', kind: 'CHECKPOINT_LIVENESS', label: 'Checkpoint liveness', resourceTypes: ['STREAMING_PIPELINE'], hint: 'Highly recommended for production. Alerts on stalled checkpoints.' },
  { group: 'CHECKPOINT', kind: 'LAST_CHECKPOINT_DURATION', label: 'Last checkpoint duration', resourceTypes: ['STREAMING_PIPELINE'], hint: 'Alert when the last checkpoint took longer than the threshold (ms).' },
  { group: 'CHECKPOINT', kind: 'CHECKPOINT_TRIGGER_FAILURES', label: 'Checkpoint trigger failures', resourceTypes: ['STREAMING_PIPELINE'], hint: 'Alert on consecutive trigger failures.' },
  { group: 'CHECKPOINT', kind: 'CONSECUTIVE_CHECKPOINT_FAILURES', label: 'Consecutive checkpoint failures', resourceTypes: ['STREAMING_PIPELINE'], hint: 'Alert on consecutive checkpoint failures.' },
  { group: 'PERFORMANCE', kind: 'TOTAL_LAG', label: 'Total lag', resourceTypes: ['STREAMING_PIPELINE', 'STREAMING_DATASET'], hint: 'Alert when input lag exceeds the threshold (ms).' },
  { group: 'PERFORMANCE', kind: 'TOTAL_THROUGHPUT', label: 'Total throughput', resourceTypes: ['STREAMING_PIPELINE', 'STREAMING_DATASET'], hint: 'Alert on records/sec falling outside the configured range.' },
  { group: 'PERFORMANCE', kind: 'UTILIZATION', label: 'Utilization', resourceTypes: ['STREAMING_PIPELINE'], hint: 'Alert when utilization (0..1) crosses the threshold.' },
];
