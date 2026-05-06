import api from './client';

const SCHEDULES_BASE = '/data-integration/v1/schedules';

export type CronFlavor = 'UNIX_5' | 'QUARTZ_6';

export type RunOutcome = 'SUCCEEDED' | 'IGNORED' | 'FAILED';

export interface TimeTrigger {
  cron: string;
  time_zone: string;
  flavor: CronFlavor;
}

export interface EventTrigger {
  type: 'NEW_LOGIC' | 'DATA_UPDATED' | 'JOB_SUCCEEDED' | 'SCHEDULE_RAN_SUCCESSFULLY';
  target_rid: string;
  branch_filter?: string[];
}

export interface CompoundTrigger {
  op: 'AND' | 'OR';
  components: Trigger[];
}

export interface Trigger {
  kind:
    | { time: TimeTrigger }
    | { event: EventTrigger }
    | { compound: CompoundTrigger };
}

export interface ScheduleTarget {
  kind: Record<string, unknown>;
}

export type ScheduleScopeKind = 'USER' | 'PROJECT_SCOPED';

export interface Schedule {
  id: string;
  rid: string;
  project_rid: string;
  name: string;
  description: string;
  trigger: Trigger;
  target: ScheduleTarget;
  paused: boolean;
  paused_reason: string | null;
  paused_at: string | null;
  auto_pause_exempt: boolean;
  pending_re_run: boolean;
  active_run_id: string | null;
  version: number;
  created_by: string;
  created_at: string;
  updated_at: string;
  last_run_at: string | null;
  scope_kind: ScheduleScopeKind;
  project_scope_rids: string[];
  run_as_user_id: string | null;
  service_principal_id: string | null;
}

export interface ScheduleRun {
  id: string;
  rid: string;
  schedule_id: string;
  outcome: RunOutcome;
  build_rid: string | null;
  failure_reason: string | null;
  triggered_at: string;
  finished_at: string | null;
  trigger_snapshot: Record<string, string>;
  schedule_version: number;
}

export interface ScheduleVersion {
  id: string;
  schedule_id: string;
  version: number;
  name: string;
  description: string;
  trigger_json: unknown;
  target_json: unknown;
  edited_by: string;
  edited_at: string;
  comment: string;
}

export interface FieldChange<T> {
  before: T;
  after: T;
}

export interface JsonDiffEntry {
  path: string;
  before: unknown;
  after: unknown;
}

export interface VersionDiff {
  schedule_id: string;
  from_version: number;
  to_version: number;
  name_diff: FieldChange<string> | null;
  description_diff: FieldChange<string> | null;
  trigger_diff: JsonDiffEntry[];
  target_diff: JsonDiffEntry[];
}

export const AUTO_PAUSED_REASON = 'AUTO_PAUSED_AFTER_FAILURES';

export interface ListSchedulesQuery {
  project?: string;
  paused?: boolean;
  owner?: string;
  q?: string;
  files?: string[];
  users?: string[];
  projects?: string[];
  sort?: 'name' | 'created_at' | 'last_run_at' | 'updated_at';
  limit?: number;
  offset?: number;
}

export function listSchedules(params: ListSchedulesQuery = {}) {
  const qs = new URLSearchParams();
  if (params.project) qs.set('project', params.project);
  if (params.paused !== undefined) qs.set('paused', String(params.paused));
  if (params.owner) qs.set('owner', params.owner);
  if (params.q) qs.set('q', params.q);
  for (const f of params.files ?? []) qs.append('files', f);
  for (const u of params.users ?? []) qs.append('users', u);
  for (const p of params.projects ?? []) qs.append('projects', p);
  if (params.sort) qs.set('sort', params.sort);
  if (params.limit) qs.set('limit', String(params.limit));
  if (params.offset) qs.set('offset', String(params.offset));
  const tail = qs.toString();
  return api.get<{ data: Schedule[]; total: number }>(
    `${SCHEDULES_BASE}${tail ? `?${tail}` : ''}`,
  );
}

export function getSchedule(rid: string) {
  return api.get<Schedule>(`${SCHEDULES_BASE}/${encodeURIComponent(rid)}`);
}

export function patchSchedule(
  rid: string,
  patch: Partial<{
    name: string;
    description: string;
    trigger: Trigger;
    target: ScheduleTarget;
    paused: boolean;
    change_comment: string;
  }>,
) {
  return api.patch<Schedule>(`${SCHEDULES_BASE}/${encodeURIComponent(rid)}`, patch);
}

export function pauseSchedule(rid: string, reason?: string) {
  return api.post<Schedule>(`${SCHEDULES_BASE}/${encodeURIComponent(rid)}:pause`, { reason });
}

export function resumeSchedule(rid: string) {
  return api.post<Schedule>(`${SCHEDULES_BASE}/${encodeURIComponent(rid)}:resume`, {});
}

export function setAutoPauseExempt(rid: string, exempt: boolean) {
  return api.post<Schedule>(`${SCHEDULES_BASE}/${encodeURIComponent(rid)}:exempt-from-auto-pause`, { exempt });
}

export function runScheduleNow(rid: string) {
  return api.post<{ run_id: string; schedule_rid: string; requested_by: string }>(
    `${SCHEDULES_BASE}/${encodeURIComponent(rid)}:run-now`,
    {},
  );
}

export function listScheduleRuns(
  rid: string,
  params: { limit?: number; offset?: number; outcome?: RunOutcome } = {},
) {
  const qs = new URLSearchParams();
  if (params.limit) qs.set('limit', String(params.limit));
  if (params.offset) qs.set('offset', String(params.offset));
  if (params.outcome) qs.set('outcome', params.outcome);
  const tail = qs.toString();
  return api.get<{ schedule_rid: string; data: ScheduleRun[]; total: number }>(
    `${SCHEDULES_BASE}/${encodeURIComponent(rid)}/runs${tail ? `?${tail}` : ''}`,
  );
}

export function listScheduleVersions(
  rid: string,
  params: { limit?: number; offset?: number } = {},
) {
  const qs = new URLSearchParams();
  if (params.limit) qs.set('limit', String(params.limit));
  if (params.offset) qs.set('offset', String(params.offset));
  const tail = qs.toString();
  return api.get<{ schedule_rid: string; current_version: number; data: ScheduleVersion[] }>(
    `${SCHEDULES_BASE}/${encodeURIComponent(rid)}/versions${tail ? `?${tail}` : ''}`,
  );
}

export function getScheduleVersion(rid: string, n: number) {
  return api.get<ScheduleVersion>(`${SCHEDULES_BASE}/${encodeURIComponent(rid)}/versions/${n}`);
}

export function getScheduleVersionDiff(rid: string, from: number, to: number) {
  return api.get<VersionDiff>(
    `${SCHEDULES_BASE}/${encodeURIComponent(rid)}/versions/diff?from=${from}&to=${to}`,
  );
}

export function convertToProjectScope(
  rid: string,
  body: { project_scope_rids: string[]; clearances?: string[] },
) {
  return api.post<{ schedule: Schedule; service_principal: Record<string, unknown> }>(
    `${SCHEDULES_BASE}/${encodeURIComponent(rid)}:convert-to-project-scope`,
    body,
  );
}

const LINTER_BASE = '/data-integration/v1/scheduling-linter';

export type LinterRuleId =
  | 'Sch001InactiveLastNinety'
  | 'Sch002PausedLongerThanThirty'
  | 'Sch003HighFailureRate'
  | 'Sch004OwnerInactive'
  | 'Sch005UserScopeOwnerStale'
  | 'Sch006HighFrequencyCron'
  | 'Sch007EventTriggerWithoutBranchFilter';

export interface LinterFinding {
  id: string;
  rule_id: LinterRuleId;
  severity: 'Info' | 'Warning' | 'Error';
  schedule_rid: string;
  project_rid: string;
  message: string;
  recommended_action: 'Notify' | 'Pause' | 'Delete' | 'Archive';
}

export interface SweepReport {
  findings: LinterFinding[];
}

export function runSweep(params: { project?: string; production?: boolean } = {}) {
  const qs = new URLSearchParams();
  if (params.project) qs.set('project', params.project);
  if (params.production !== undefined) qs.set('production', String(params.production));
  const tail = qs.toString();
  return api.post<{ dry_run: boolean; findings: LinterFinding[]; by_rule: string[] }>(
    `${LINTER_BASE}/sweep${tail ? `?${tail}` : ''}`,
    {},
  );
}

export function applySweep(body: {
  rule_ids?: LinterRuleId[];
  finding_ids?: string[];
  report: SweepReport;
}) {
  return api.post<{ applied: Array<Record<string, unknown>> }>(`${LINTER_BASE}/sweep:apply`, body);
}
