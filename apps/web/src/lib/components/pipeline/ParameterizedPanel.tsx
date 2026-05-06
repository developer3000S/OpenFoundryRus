import { useEffect, useState } from 'react';

import {
  createDeployment,
  deleteDeployment,
  enableParameterized,
  listDeployments,
  runDeployment,
  type ParameterizedPipeline,
  type PipelineDeployment,
} from '@/lib/api/parameterized';
import { JsonEditor } from '@/lib/components/JsonEditor';

interface ParameterizedPanelProps {
  pipelineRid: string;
  existing?: ParameterizedPipeline | null;
}

export function ParameterizedPanel({ pipelineRid, existing = null }: ParameterizedPanelProps) {
  const [parameterized, setParameterized] = useState<ParameterizedPipeline | null>(existing);
  const [deployments, setDeployments] = useState<PipelineDeployment[]>([]);
  const [error, setError] = useState<string | null>(null);

  const [deploymentKeyParam, setDeploymentKeyParam] = useState('');
  const [outputRids, setOutputRids] = useState('');
  const [unionViewRid, setUnionViewRid] = useState('');
  const [newDeploymentKey, setNewDeploymentKey] = useState('');
  const [newDeploymentValues, setNewDeploymentValues] = useState('{}');

  async function refresh() {
    if (!parameterized) return;
    try {
      const res = await listDeployments(parameterized.id);
      setDeployments(res.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  useEffect(() => {
    if (parameterized) void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [parameterized?.id]);

  async function onEnable() {
    try {
      setError(null);
      const rids = outputRids.split(',').map((s) => s.trim()).filter(Boolean);
      const next = await enableParameterized(pipelineRid, {
        deployment_key_param: deploymentKeyParam.trim(),
        output_dataset_rids: rids,
        union_view_dataset_rid: unionViewRid.trim(),
      });
      setParameterized(next);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  async function onCreate() {
    if (!parameterized) return;
    try {
      setError(null);
      const values = JSON.parse(newDeploymentValues || '{}');
      await createDeployment(parameterized.id, { deployment_key: newDeploymentKey.trim(), parameter_values: values });
      setNewDeploymentKey('');
      setNewDeploymentValues('{}');
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  async function onRun(d: PipelineDeployment) {
    if (!parameterized) return;
    try {
      await runDeployment(parameterized.id, d.id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  async function onDelete(d: PipelineDeployment) {
    if (!parameterized) return;
    if (typeof window !== 'undefined' && !window.confirm(`Delete deployment ${d.deployment_key}?`)) return;
    try {
      await deleteDeployment(parameterized.id, d.id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0' }}>
      <h3 style={{ margin: 0, fontSize: 14 }}>Parameterized pipeline (Beta)</h3>
      {error && <p style={{ color: '#fca5a5', fontSize: 12, margin: 0 }}>{error}</p>}
      {!parameterized ? (
        <div style={{ display: 'grid', gap: 6 }}>
          <p className="of-text-muted" style={{ fontSize: 12, margin: 0 }}>
            Same logic, different parameter values, one deployment per value-set, manual-only dispatch.
          </p>
          <label style={{ fontSize: 12 }}>
            Deployment key parameter
            <input value={deploymentKeyParam} onChange={(e) => setDeploymentKeyParam(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 12 }}>
            Output dataset RIDs (comma-separated)
            <input value={outputRids} onChange={(e) => setOutputRids(e.target.value)} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
          </label>
          <label style={{ fontSize: 12 }}>
            Union view dataset RID
            <input value={unionViewRid} onChange={(e) => setUnionViewRid(e.target.value)} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }} />
          </label>
          <button type="button" onClick={() => void onEnable()} className="of-button of-button--primary" style={{ fontSize: 12 }}>Enable parameterization</button>
        </div>
      ) : (
        <>
          <div className="of-text-muted" style={{ fontSize: 11 }}>
            Enabled · key param: <code>{parameterized.deployment_key_param}</code>
          </div>
          <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4 }}>
            {deployments.map((d) => (
              <li key={d.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '6px 8px', background: '#1e293b', borderRadius: 4, fontSize: 12 }}>
                <span><strong>{d.deployment_key}</strong> · {Object.keys(d.parameter_values).length} params</span>
                <span style={{ display: 'flex', gap: 4 }}>
                  <button type="button" onClick={() => void onRun(d)} className="of-button" style={{ fontSize: 11 }}>Run</button>
                  <button type="button" onClick={() => void onDelete(d)} className="of-button" style={{ fontSize: 11, color: '#fca5a5', borderColor: '#7f1d1d' }}>Delete</button>
                </span>
              </li>
            ))}
            {deployments.length === 0 && <li style={{ color: '#94a3b8', fontStyle: 'italic', fontSize: 11 }}>No deployments yet.</li>}
          </ul>
          <div style={{ display: 'grid', gap: 6, padding: 8, background: '#1e293b', borderRadius: 4 }}>
            <p className="of-eyebrow" style={{ fontSize: 10 }}>Add deployment</p>
            <input value={newDeploymentKey} onChange={(e) => setNewDeploymentKey(e.target.value)} placeholder="deployment_key" className="of-input" style={{ fontSize: 12 }} />
            <JsonEditor value={newDeploymentValues} onChange={setNewDeploymentValues} minHeight={100} />
            <button type="button" onClick={() => void onCreate()} className="of-button of-button--primary" style={{ fontSize: 11 }}>Create</button>
          </div>
        </>
      )}
    </section>
  );
}
