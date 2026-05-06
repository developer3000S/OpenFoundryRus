import { useEffect, useState } from 'react';

import { ClusterViewer } from '@/lib/components/fusion/ClusterViewer';
import { FusionSpreadsheet } from '@/lib/components/fusion/FusionSpreadsheet';
import { GoldenRecordView } from '@/lib/components/fusion/GoldenRecordView';
import { ManualReview, type ReviewDraft } from '@/lib/components/fusion/ManualReview';
import { MatchRuleBuilder, type MatchRuleDraft } from '@/lib/components/fusion/MatchRuleBuilder';
import { MergePreview, type MergeStrategyDraft } from '@/lib/components/fusion/MergePreview';
import { ResolutionResults, type JobDraft } from '@/lib/components/fusion/ResolutionResults';
import {
  createJob,
  createMergeStrategy,
  createRule,
  getCluster,
  getOverview,
  listClusters,
  listGoldenRecords,
  listJobs,
  listMergeStrategies,
  listReviewQueue,
  listRules,
  runJob,
  submitReview,
  updateMergeStrategy,
  updateRule,
  type ClusterDetail,
  type FusionJob,
  type FusionOverview,
  type GoldenRecord,
  type MatchRule,
  type MergeStrategy,
  type ResolvedCluster,
  type ReviewQueueItem,
  type RunResolutionJobResponse,
} from '@/lib/api/fusion';
import { notifications } from '@stores/notifications';

function formatJson(value: unknown) {
  return JSON.stringify(value, null, 2);
}

function parseCsv(value: string) {
  return value
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function parseJson<T>(value: string, fallback: T): T {
  if (!value.trim()) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    throw new Error('Invalid JSON payload');
  }
}

function emptyMatchRuleDraft(): MatchRuleDraft {
  return {
    name: 'Person Resolution Rule',
    description: '',
    status: 'active',
    entity_type: 'person',
    strategy_type: 'sorted-neighborhood',
    key_fields_text: 'email, phone, display_name',
    window_size: 4,
    bucket_count: 24,
    review_threshold: 0.76,
    auto_merge_threshold: 0.9,
    conditions_text: formatJson([
      { field: 'email', comparator: 'email_exact', weight: 0.35, threshold: 1.0, required: false },
      { field: 'phone', comparator: 'phone_exact', weight: 0.2, threshold: 1.0, required: false },
      { field: 'display_name', comparator: 'jaro_winkler', weight: 0.25, threshold: 0.86, required: true },
      { field: 'display_name', comparator: 'phonetic', weight: 0.1, threshold: 0.5, required: false },
      { field: 'company', comparator: 'fuzzy', weight: 0.1, threshold: 0.72, required: false },
    ]),
  };
}

function emptyMergeStrategyDraft(): MergeStrategyDraft {
  return {
    name: 'Person Survivorship',
    description: '',
    status: 'active',
    entity_type: 'person',
    default_strategy: 'longest_non_empty',
    rules_text: formatJson([
      { field: 'display_name', strategy: 'longest_non_empty', source_priority: ['crm', 'erp', 'support'], fallback: 'highest_confidence' },
      { field: 'email', strategy: 'source_priority', source_priority: ['crm', 'erp', 'support'], fallback: 'most_common' },
      { field: 'phone', strategy: 'most_common', source_priority: [], fallback: 'longest_non_empty' },
      { field: 'company', strategy: 'most_common', source_priority: [], fallback: 'longest_non_empty' },
    ]),
  };
}

function emptyJobDraft(): JobDraft {
  return {
    name: 'Customer 360 Batch',
    description: 'Resolve customer identities across CRM, ERP, and support exports.',
    status: 'draft',
    entity_type: 'person',
    match_rule_id: '',
    merge_strategy_id: '',
    source_labels_text: 'crm, erp, support',
    record_count: 12,
    review_sampling_rate: 0.25,
  };
}

function emptyReviewDraft(): ReviewDraft {
  return {
    decision: 'confirm_match',
    reviewed_by: 'reviewer@openfoundry.dev',
    notes: '',
  };
}

function ruleToDraft(rule: MatchRule): MatchRuleDraft {
  return {
    id: rule.id,
    name: rule.name,
    description: rule.description,
    status: rule.status,
    entity_type: rule.entity_type,
    strategy_type: rule.blocking_strategy.strategy_type,
    key_fields_text: rule.blocking_strategy.key_fields.join(', '),
    window_size: rule.blocking_strategy.window_size,
    bucket_count: rule.blocking_strategy.bucket_count,
    review_threshold: rule.review_threshold,
    auto_merge_threshold: rule.auto_merge_threshold,
    conditions_text: formatJson(rule.conditions),
  };
}

function strategyToDraft(strategy: MergeStrategy): MergeStrategyDraft {
  return {
    id: strategy.id,
    name: strategy.name,
    description: strategy.description,
    status: strategy.status,
    entity_type: strategy.entity_type,
    default_strategy: strategy.default_strategy,
    rules_text: formatJson(strategy.rules),
  };
}

export function FusionPage() {
  const [overview, setOverview] = useState<FusionOverview | null>(null);
  const [rules, setRules] = useState<MatchRule[]>([]);
  const [mergeStrategies, setMergeStrategies] = useState<MergeStrategy[]>([]);
  const [jobs, setJobs] = useState<FusionJob[]>([]);
  const [clusters, setClusters] = useState<ResolvedCluster[]>([]);
  const [reviewQueue, setReviewQueue] = useState<ReviewQueueItem[]>([]);
  const [goldenRecords, setGoldenRecords] = useState<GoldenRecord[]>([]);
  const [clusterDetail, setClusterDetail] = useState<ClusterDetail | null>(null);
  const [lastRun, setLastRun] = useState<RunResolutionJobResponse | null>(null);

  const [selectedJobId, setSelectedJobId] = useState('');
  const [selectedClusterId, setSelectedClusterId] = useState('');

  const [matchRuleDraft, setMatchRuleDraft] = useState<MatchRuleDraft>(emptyMatchRuleDraft);
  const [mergeStrategyDraft, setMergeStrategyDraft] = useState<MergeStrategyDraft>(emptyMergeStrategyDraft);
  const [jobDraft, setJobDraft] = useState<JobDraft>(emptyJobDraft);
  const [reviewDraft, setReviewDraft] = useState<ReviewDraft>(emptyReviewDraft);

  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [uiError, setUiError] = useState('');

  const busy = loading || busyAction.length > 0;

  async function refreshClusterDetail(clusterId: string) {
    setSelectedClusterId(clusterId);
    const detail = await getCluster(clusterId);
    setClusterDetail(detail);
  }

  async function refreshAll() {
    setLoading(true);
    setUiError('');
    try {
      const [overviewResponse, ruleResponse, mergeStrategyResponse, jobResponse, clusterResponse, reviewResponse, goldenResponse] =
        await Promise.all([
          getOverview(),
          listRules(),
          listMergeStrategies(),
          listJobs(),
          listClusters(),
          listReviewQueue(),
          listGoldenRecords(),
        ]);

      setOverview(overviewResponse);
      setRules(ruleResponse.data);
      setMergeStrategies(mergeStrategyResponse.data);
      setJobs(jobResponse.data);
      setClusters(clusterResponse.data);
      setReviewQueue(reviewResponse.data);
      setGoldenRecords(goldenResponse.data);

      setMatchRuleDraft((current) => (!current.id && ruleResponse.data[0] ? ruleToDraft(ruleResponse.data[0]) : current));
      setMergeStrategyDraft((current) =>
        !current.id && mergeStrategyResponse.data[0] ? strategyToDraft(mergeStrategyResponse.data[0]) : current,
      );
      setJobDraft((current) => ({
        ...current,
        match_rule_id: current.match_rule_id || ruleResponse.data[0]?.id || '',
        merge_strategy_id: current.merge_strategy_id || mergeStrategyResponse.data[0]?.id || '',
      }));

      const nextJobId = selectedJobId || jobResponse.data[0]?.id || '';
      setSelectedJobId(nextJobId);
      const nextClusterId = selectedClusterId || clusterResponse.data[0]?.id || '';
      if (nextClusterId) {
        await refreshClusterDetail(nextClusterId);
      } else {
        setClusterDetail(null);
      }
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Failed to load Fusion data';
      setUiError(message);
      notifications.error(message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refreshAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function runAction(label: string, action: () => Promise<void>) {
    setBusyAction(label);
    setUiError('');
    try {
      await action();
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Action failed';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function saveRule() {
    await runAction('save-rule', async () => {
      const payload = {
        name: matchRuleDraft.name.trim(),
        description: matchRuleDraft.description,
        status: matchRuleDraft.status,
        entity_type: matchRuleDraft.entity_type,
        blocking_strategy: {
          strategy_type: matchRuleDraft.strategy_type,
          key_fields: parseCsv(matchRuleDraft.key_fields_text),
          window_size: matchRuleDraft.window_size,
          bucket_count: matchRuleDraft.bucket_count,
        },
        conditions: parseJson(matchRuleDraft.conditions_text, []),
        review_threshold: matchRuleDraft.review_threshold,
        auto_merge_threshold: matchRuleDraft.auto_merge_threshold,
      };
      const saved = matchRuleDraft.id ? await updateRule(matchRuleDraft.id, payload) : await createRule(payload);
      setMatchRuleDraft(ruleToDraft(saved));
      await refreshAll();
      notifications.success('Match rule saved.');
    });
  }

  async function saveMergeStrategy() {
    await runAction('save-merge-strategy', async () => {
      const payload = {
        name: mergeStrategyDraft.name.trim(),
        description: mergeStrategyDraft.description,
        status: mergeStrategyDraft.status,
        entity_type: mergeStrategyDraft.entity_type,
        default_strategy: mergeStrategyDraft.default_strategy,
        rules: parseJson(mergeStrategyDraft.rules_text, []),
      };
      const saved = mergeStrategyDraft.id
        ? await updateMergeStrategy(mergeStrategyDraft.id, payload)
        : await createMergeStrategy(payload);
      setMergeStrategyDraft(strategyToDraft(saved));
      await refreshAll();
      notifications.success('Merge strategy saved.');
    });
  }

  async function saveJob() {
    await runAction('save-job', async () => {
      const saved = await createJob({
        name: jobDraft.name.trim(),
        description: jobDraft.description,
        status: jobDraft.status,
        entity_type: jobDraft.entity_type,
        match_rule_id: jobDraft.match_rule_id,
        merge_strategy_id: jobDraft.merge_strategy_id,
        config: {
          source_labels: parseCsv(jobDraft.source_labels_text),
          record_count: jobDraft.record_count,
          blocking_strategy_override: null,
          review_sampling_rate: jobDraft.review_sampling_rate,
        },
      });
      setSelectedJobId(saved.id);
      await refreshAll();
      notifications.success('Fusion job created.');
    });
  }

  async function runSelectedJob() {
    if (!selectedJobId) {
      notifications.warning('Select a job to run.');
      return;
    }
    await runAction('run-job', async () => {
      const result = await runJob(selectedJobId);
      setLastRun(result);
      await refreshAll();
      notifications.success('Fusion resolution run completed.');
    });
  }

  async function submitSelectedReview() {
    if (!selectedClusterId) {
      notifications.warning('Select a cluster from the review queue first.');
      return;
    }
    await runAction('submit-review', async () => {
      const detail = await submitReview(selectedClusterId, {
        decision: reviewDraft.decision,
        reviewed_by: reviewDraft.reviewed_by,
        notes: reviewDraft.notes,
      });
      setClusterDetail(detail);
      await refreshAll();
      notifications.success('Review decision recorded.');
    });
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <section
        style={{
          overflow: 'hidden',
          borderRadius: 36,
          border: '1px solid var(--border-default)',
          background:
            'radial-gradient(circle at top left, rgba(251,191,36,0.26), transparent 36%), linear-gradient(135deg, #111827 0%, #1f2937 34%, #f8fafc 100%)',
          padding: 24,
          color: '#f8fafc',
        }}
      >
        <div style={{ display: 'grid', gap: 24, gridTemplateColumns: 'minmax(0, 1.1fr) minmax(0, 0.9fr)' }}>
          <div>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.34em', color: '#fef3c7' }}>
              Identity Resolution
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 12, color: '#f8fafc' }}>
              Fusion: deterministic + ML matching with operator-grade governance
            </h1>
            <p style={{ marginTop: 12, fontSize: 13, lineHeight: 1.7, maxWidth: 720, color: 'rgba(248, 250, 252, 0.85)' }}>
              Build match rules and merge strategies, schedule resolution jobs, and inspect clusters,
              reviews, and golden records — all from one Fusion workspace.
            </p>
          </div>
          <div style={{ borderRadius: 28, border: '1px solid rgba(255,255,255,0.15)', background: 'rgba(255,255,255,0.1)', padding: 18 }}>
            <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.24em', color: '#fef3c7' }}>
              Operator loop
            </div>
            <div style={{ display: 'grid', gap: 8, marginTop: 12, fontSize: 13, color: 'rgba(248, 250, 252, 0.9)' }}>
              <div style={{ borderRadius: 16, border: '1px solid rgba(255,255,255,0.1)', background: 'rgba(255,255,255,0.05)', padding: '10px 14px' }}>
                Define rules and merge strategy
              </div>
              <div style={{ borderRadius: 16, border: '1px solid rgba(255,255,255,0.1)', background: 'rgba(255,255,255,0.05)', padding: '10px 14px' }}>
                Run resolution jobs and triage clusters
              </div>
              <div style={{ borderRadius: 16, border: '1px solid rgba(255,255,255,0.1)', background: 'rgba(255,255,255,0.05)', padding: '10px 14px' }}>
                Resolve manual reviews and ship golden records
              </div>
            </div>
          </div>
        </div>
      </section>

      {uiError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {uiError}
        </div>
      )}

      {loading ? (
        <div className="of-panel" style={{ padding: 56, textAlign: 'center', fontSize: 13, color: 'var(--text-muted)' }}>
          Loading Fusion workspace…
        </div>
      ) : (
        <>
          <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.08fr) minmax(0, 0.92fr)' }}>
            <MatchRuleBuilder
              rules={rules}
              draft={matchRuleDraft}
              busy={busy}
              onSelect={(ruleId) => {
                const rule = rules.find((item) => item.id === ruleId);
                if (rule) setMatchRuleDraft(ruleToDraft(rule));
              }}
              onDraftChange={(d) => setMatchRuleDraft(d)}
              onSave={() => void saveRule()}
              onReset={() => setMatchRuleDraft(emptyMatchRuleDraft())}
            />
            <MergePreview
              strategies={mergeStrategies}
              draft={mergeStrategyDraft}
              busy={busy}
              onSelect={(strategyId) => {
                const strategy = mergeStrategies.find((item) => item.id === strategyId);
                if (strategy) setMergeStrategyDraft(strategyToDraft(strategy));
              }}
              onDraftChange={(d) => setMergeStrategyDraft(d)}
              onSave={() => void saveMergeStrategy()}
              onReset={() => setMergeStrategyDraft(emptyMergeStrategyDraft())}
            />
          </div>

          <ResolutionResults
            overview={overview}
            jobs={jobs}
            rules={rules}
            mergeStrategies={mergeStrategies}
            draft={jobDraft}
            lastRun={lastRun}
            selectedJobId={selectedJobId}
            busy={busy}
            onSelectJob={(jobId) => setSelectedJobId(jobId)}
            onDraftChange={(d) => setJobDraft(d)}
            onSave={() => void saveJob()}
            onRun={() => void runSelectedJob()}
            onReset={() => setJobDraft(emptyJobDraft())}
          />

          <ClusterViewer
            clusters={clusters}
            selectedClusterId={selectedClusterId}
            clusterDetail={clusterDetail}
            busy={busy}
            onSelectCluster={(clusterId) => void refreshClusterDetail(clusterId)}
          />

          <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.95fr) minmax(0, 1.05fr)' }}>
            <ManualReview
              reviewQueue={reviewQueue}
              selectedClusterId={selectedClusterId}
              clusterDetail={clusterDetail}
              draft={reviewDraft}
              busy={busy}
              onSelectCluster={(clusterId) => void refreshClusterDetail(clusterId)}
              onDraftChange={(d) => setReviewDraft(d)}
              onSubmit={() => void submitSelectedReview()}
            />
            <GoldenRecordView goldenRecords={goldenRecords} clusterDetail={clusterDetail} />
          </div>

          <FusionSpreadsheet />
        </>
      )}
    </section>
  );
}
