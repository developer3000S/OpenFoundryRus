import { useEffect, useState } from 'react';

import {
  createBatchPrediction,
  createDeployment,
  createExperiment,
  createFeature,
  createModel,
  createModelVersion,
  createRun,
  createTrainingJob,
  generateDriftReport,
  getOverview,
  listBatchPredictions,
  listDeployments,
  listExperiments,
  listFeatures,
  listModelVersions,
  listModels,
  listRuns,
  listTrainingJobs,
  realtimePredict,
  transitionModelVersion,
  type BatchPredictionJob,
  type Experiment,
  type ExperimentRun,
  type FeatureDefinition,
  type MlStudioOverview,
  type ModelDeployment,
  type ModelVersion,
  type RegisteredModel,
  type TrainingJob,
} from '@/lib/api/ml';
import { JsonEditor } from '@/lib/components/JsonEditor';
import { Tabs } from '@/lib/components/Tabs';

type Tab = 'overview' | 'experiments' | 'models' | 'features' | 'training' | 'deployments' | 'batch';

export function MlPage() {
  const [tab, setTab] = useState<Tab>('overview');
  const [overview, setOverview] = useState<MlStudioOverview | null>(null);
  const [experiments, setExperiments] = useState<Experiment[]>([]);
  const [runsByExp, setRunsByExp] = useState<Record<string, ExperimentRun[]>>({});
  const [models, setModels] = useState<RegisteredModel[]>([]);
  const [versionsByModel, setVersionsByModel] = useState<Record<string, ModelVersion[]>>({});
  const [features, setFeatures] = useState<FeatureDefinition[]>([]);
  const [trainingJobs, setTrainingJobs] = useState<TrainingJob[]>([]);
  const [deployments, setDeployments] = useState<ModelDeployment[]>([]);
  const [batchJobs, setBatchJobs] = useState<BatchPredictionJob[]>([]);

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const [expDraftJson, setExpDraftJson] = useState(
    JSON.stringify({ name: 'My experiment', description: '', task_type: 'classification', primary_metric: 'accuracy', tags: [] }, null, 2),
  );
  const [runDraftExpId, setRunDraftExpId] = useState('');
  const [runDraftJson, setRunDraftJson] = useState(
    JSON.stringify({ name: 'baseline', status: 'completed', metrics: [{ name: 'accuracy', value: 0.92 }], notes: '' }, null, 2),
  );
  const [modelDraftJson, setModelDraftJson] = useState(
    JSON.stringify({ name: 'My model', description: '', problem_type: 'classification', status: 'active', tags: [] }, null, 2),
  );
  const [versionDraftModelId, setVersionDraftModelId] = useState('');
  const [versionDraftJson, setVersionDraftJson] = useState(
    JSON.stringify({ version_label: 'v1', stage: 'Staging', metrics: [], hyperparameters: {} }, null, 2),
  );
  const [featureDraftJson, setFeatureDraftJson] = useState(
    JSON.stringify({ name: 'feat_1', entity_name: 'user', data_type: 'float', online_enabled: true, samples: [] }, null, 2),
  );
  const [trainingDraftJson, setTrainingDraftJson] = useState(
    JSON.stringify({ name: 'Training run', dataset_ids: [], training_config: {}, objective_metric_name: 'accuracy' }, null, 2),
  );
  const [deployDraftJson, setDeployDraftJson] = useState(
    JSON.stringify({ model_id: '', name: 'Deployment', endpoint_path: '/api/v1/ml/runtime/my-endpoint', strategy_type: 'shadow', traffic_split: [] }, null, 2),
  );
  const [predictDeployId, setPredictDeployId] = useState('');
  const [predictInputJson, setPredictInputJson] = useState(JSON.stringify({ inputs: [{ x: 1 }], explain: false }, null, 2));
  const [predictResult, setPredictResult] = useState<unknown>(null);
  const [batchDraftJson, setBatchDraftJson] = useState(
    JSON.stringify({ deployment_id: '', records: [], output_destination: null }, null, 2),
  );
  const [driftDeployId, setDriftDeployId] = useState('');

  async function refresh() {
    setError('');
    try {
      const [ov, exp, mod, feat, tj, dep, bp] = await Promise.all([
        getOverview().catch(() => null),
        listExperiments().catch(() => ({ data: [] as Experiment[] })),
        listModels().catch(() => ({ data: [] as RegisteredModel[] })),
        listFeatures().catch(() => ({ data: [] as FeatureDefinition[] })),
        listTrainingJobs().catch(() => ({ data: [] as TrainingJob[] })),
        listDeployments().catch(() => ({ data: [] as ModelDeployment[] })),
        listBatchPredictions().catch(() => ({ data: [] as BatchPredictionJob[] })),
      ]);
      setOverview(ov);
      setExperiments(exp.data);
      setModels(mod.data);
      setFeatures(feat.data);
      setTrainingJobs(tj.data);
      setDeployments(dep.data);
      setBatchJobs(bp.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load ML studio');
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function loadRuns(expId: string) {
    try {
      const res = await listRuns(expId);
      setRunsByExp((prev) => ({ ...prev, [expId]: res.data }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load runs');
    }
  }

  async function loadVersions(modelId: string) {
    try {
      const res = await listModelVersions(modelId);
      setVersionsByModel((prev) => ({ ...prev, [modelId]: res.data }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load versions');
    }
  }

  async function withBusy(fn: () => Promise<unknown>, label: string) {
    setBusy(true);
    setError('');
    try {
      await fn();
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : `${label} failed`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">ML Studio</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Experiments + runs + models + versions + features + training jobs + deployments + drift + batch predictions.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <Tabs
        tabs={['overview', 'experiments', 'models', 'features', 'training', 'deployments', 'batch'] as const}
        active={tab}
        onChange={setTab}
      />

      {tab === 'overview' && overview && (
        <section className="of-panel" style={{ padding: 16 }}>
          <pre style={{ padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
            {JSON.stringify(overview, null, 2)}
          </pre>
        </section>
      )}

      {tab === 'experiments' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Create experiment</p>
            <JsonEditor value={expDraftJson} onChange={setExpDraftJson} minHeight={140} />
            <button
              type="button"
              onClick={() => void withBusy(() => createExperiment(JSON.parse(expDraftJson)), 'Create experiment')}
              disabled={busy}
              className="of-button of-button--primary"
              style={{ marginTop: 6 }}
            >
              Create
            </button>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Experiments ({experiments.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {experiments.map((exp) => (
                <li key={exp.id} style={{ padding: 10, borderBottom: '1px solid var(--border-default)' }}>
                  <strong>{exp.name}</strong> · {exp.task_type} · {exp.run_count} runs · {exp.status}
                  <button type="button" onClick={() => void loadRuns(exp.id)} className="of-button" style={{ fontSize: 11, marginLeft: 8 }}>
                    Runs
                  </button>
                  <button type="button" onClick={() => setRunDraftExpId(exp.id)} className="of-button" style={{ fontSize: 11, marginLeft: 4 }}>
                    Add run
                  </button>
                  {runsByExp[exp.id] && (
                    <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 11 }}>
                      {runsByExp[exp.id].map((r) => (
                        <li key={r.id}>{r.name} · {r.status}</li>
                      ))}
                    </ul>
                  )}
                </li>
              ))}
            </ul>
          </section>

          {runDraftExpId && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Add run to {runDraftExpId}</p>
              <JsonEditor value={runDraftJson} onChange={setRunDraftJson} minHeight={140} />
              <div style={{ display: 'flex', gap: 6, marginTop: 6 }}>
                <button
                  type="button"
                  onClick={() => void withBusy(async () => {
                    await createRun(runDraftExpId, JSON.parse(runDraftJson));
                    await loadRuns(runDraftExpId);
                  }, 'Create run')}
                  disabled={busy}
                  className="of-button of-button--primary"
                >
                  Create run
                </button>
                <button type="button" onClick={() => setRunDraftExpId('')} className="of-button">Cancel</button>
              </div>
            </section>
          )}
        </>
      )}

      {tab === 'models' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Create model</p>
            <JsonEditor value={modelDraftJson} onChange={setModelDraftJson} minHeight={120} />
            <button
              type="button"
              onClick={() => void withBusy(() => createModel(JSON.parse(modelDraftJson)), 'Create model')}
              disabled={busy}
              className="of-button of-button--primary"
              style={{ marginTop: 6 }}
            >
              Create
            </button>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Models ({models.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {models.map((m) => (
                <li key={m.id} style={{ padding: 10, borderBottom: '1px solid var(--border-default)' }}>
                  <strong>{m.name}</strong> · {m.problem_type} · {m.status}
                  <button type="button" onClick={() => void loadVersions(m.id)} className="of-button" style={{ fontSize: 11, marginLeft: 8 }}>
                    Versions
                  </button>
                  <button type="button" onClick={() => setVersionDraftModelId(m.id)} className="of-button" style={{ fontSize: 11, marginLeft: 4 }}>
                    Add version
                  </button>
                  {versionsByModel[m.id] && (
                    <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 11 }}>
                      {versionsByModel[m.id].map((v) => (
                        <li key={v.id}>
                          {v.version_label} · {v.stage}
                          <button
                            type="button"
                            onClick={() => void withBusy(async () => {
                              await transitionModelVersion(v.id, v.stage === 'Production' ? 'Staging' : 'Production');
                              await loadVersions(m.id);
                            }, 'Transition')}
                            className="of-button"
                            style={{ fontSize: 10, marginLeft: 6 }}
                          >
                            ⇄ stage
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </li>
              ))}
            </ul>
          </section>

          {versionDraftModelId && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Add version to {versionDraftModelId}</p>
              <JsonEditor value={versionDraftJson} onChange={setVersionDraftJson} minHeight={120} />
              <div style={{ display: 'flex', gap: 6, marginTop: 6 }}>
                <button
                  type="button"
                  onClick={() => void withBusy(async () => {
                    await createModelVersion(versionDraftModelId, JSON.parse(versionDraftJson));
                    await loadVersions(versionDraftModelId);
                  }, 'Create version')}
                  disabled={busy}
                  className="of-button of-button--primary"
                >
                  Create
                </button>
                <button type="button" onClick={() => setVersionDraftModelId('')} className="of-button">Cancel</button>
              </div>
            </section>
          )}
        </>
      )}

      {tab === 'features' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Create feature</p>
            <JsonEditor value={featureDraftJson} onChange={setFeatureDraftJson} minHeight={140} />
            <button
              type="button"
              onClick={() => void withBusy(() => createFeature(JSON.parse(featureDraftJson)), 'Create feature')}
              disabled={busy}
              className="of-button of-button--primary"
              style={{ marginTop: 6 }}
            >
              Create
            </button>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Features ({features.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
              {features.map((f) => (
                <li key={f.id}>
                  <strong>{f.name}</strong> · {f.entity_name}/{f.data_type} · online: {f.online_enabled ? 'yes' : 'no'} · {f.status}
                </li>
              ))}
            </ul>
          </section>
        </>
      )}

      {tab === 'training' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Submit training job</p>
            <JsonEditor value={trainingDraftJson} onChange={setTrainingDraftJson} minHeight={140} />
            <button
              type="button"
              onClick={() => void withBusy(() => createTrainingJob(JSON.parse(trainingDraftJson)), 'Create training job')}
              disabled={busy}
              className="of-button of-button--primary"
              style={{ marginTop: 6 }}
            >
              Submit
            </button>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Training jobs ({trainingJobs.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
              {trainingJobs.map((j) => (
                <li key={j.id}>
                  <strong>{j.name}</strong> · {j.status} · {j.trials.length} trials
                </li>
              ))}
            </ul>
          </section>
        </>
      )}

      {tab === 'deployments' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Create deployment</p>
            <JsonEditor value={deployDraftJson} onChange={setDeployDraftJson} minHeight={140} />
            <button
              type="button"
              onClick={() => void withBusy(() => createDeployment(JSON.parse(deployDraftJson)), 'Create deployment')}
              disabled={busy}
              className="of-button of-button--primary"
              style={{ marginTop: 6 }}
            >
              Deploy
            </button>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Deployments ({deployments.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {deployments.map((d) => (
                <li key={d.id} style={{ padding: 10, borderBottom: '1px solid var(--border-default)' }}>
                  <strong>{d.name}</strong> · {d.status} · {d.endpoint_path} · {d.strategy_type}
                  <div style={{ display: 'flex', gap: 6, marginTop: 6 }}>
                    <button
                      type="button"
                      onClick={() => setPredictDeployId(d.id)}
                      className="of-button"
                      style={{ fontSize: 11 }}
                    >
                      Predict
                    </button>
                    <button
                      type="button"
                      onClick={() => setDriftDeployId(d.id)}
                      className="of-button"
                      style={{ fontSize: 11 }}
                    >
                      Drift
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          </section>

          {predictDeployId && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Predict ({predictDeployId})</p>
              <JsonEditor value={predictInputJson} onChange={setPredictInputJson} minHeight={80} />
              <div style={{ display: 'flex', gap: 6, marginTop: 6 }}>
                <button
                  type="button"
                  onClick={() => void (async () => {
                    setBusy(true);
                    try {
                      const body = JSON.parse(predictInputJson);
                      setPredictResult(await realtimePredict(predictDeployId, body));
                    } catch (cause) {
                      setError(cause instanceof Error ? cause.message : 'Predict failed');
                    } finally {
                      setBusy(false);
                    }
                  })()}
                  disabled={busy}
                  className="of-button of-button--primary"
                >
                  Predict
                </button>
                <button type="button" onClick={() => setPredictDeployId('')} className="of-button">Close</button>
              </div>
              {!!predictResult && (
                <pre style={{ marginTop: 8, padding: 10, background: '#0c0a09', color: '#a5f3fc', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
                  {JSON.stringify(predictResult, null, 2)}
                </pre>
              )}
            </section>
          )}

          {driftDeployId && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Drift ({driftDeployId})</p>
              <button
                type="button"
                onClick={() => void withBusy(() => generateDriftReport(driftDeployId, { auto_retrain: false }), 'Drift report')}
                disabled={busy}
                className="of-button of-button--primary"
              >
                Generate drift report
              </button>
              <button type="button" onClick={() => setDriftDeployId('')} className="of-button" style={{ marginLeft: 6 }}>Close</button>
            </section>
          )}
        </>
      )}

      {tab === 'batch' && (
        <>
          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Submit batch prediction</p>
            <JsonEditor value={batchDraftJson} onChange={setBatchDraftJson} minHeight={140} />
            <button
              type="button"
              onClick={() => void withBusy(() => createBatchPrediction(JSON.parse(batchDraftJson)), 'Create batch prediction')}
              disabled={busy}
              className="of-button of-button--primary"
              style={{ marginTop: 6 }}
            >
              Submit
            </button>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Batch jobs ({batchJobs.length})</p>
            <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
              {batchJobs.map((b) => (
                <li key={b.id}>
                  {b.id} · {b.status} · {b.deployment_id} · {b.record_count} records
                </li>
              ))}
            </ul>
          </section>
        </>
      )}
    </section>
  );
}
