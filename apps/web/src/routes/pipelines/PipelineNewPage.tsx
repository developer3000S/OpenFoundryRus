import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { JsonEditor } from '@/lib/components/JsonEditor';
import { createPipeline, type PipelineNode } from '@/lib/api/pipelines';

function makeId() {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `node_${Date.now()}_${Math.floor(Math.random() * 10_000)}`;
}

function defaultNodes(): PipelineNode[] {
  return [
    {
      id: makeId(),
      label: 'Sql transform',
      transform_type: 'sql',
      config: { sql: 'SELECT 1 AS value' },
      depends_on: [],
      input_dataset_ids: [],
      output_dataset_id: null,
    },
  ];
}

export function PipelineNewPage() {
  const [name, setName] = useState('New pipeline');
  const [description, setDescription] = useState('');
  const [pipelineType, setPipelineType] = useState<'BATCH' | 'INCREMENTAL' | 'STREAMING' | 'EXTERNAL' | 'FASTER'>('BATCH');
  const [nodesJson, setNodesJson] = useState(JSON.stringify(defaultNodes(), null, 2));
  const [scheduleJson, setScheduleJson] = useState(
    JSON.stringify({ enabled: false, cron: '0 */15 * * * *' }, null, 2),
  );
  const [retryJson, setRetryJson] = useState(
    JSON.stringify({ max_attempts: 1, retry_on_failure: false, allow_partial_reexecution: true }, null, 2),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  async function handleCreate() {
    setBusy(true);
    setError('');
    try {
      const created = await createPipeline({
        name,
        description: description || undefined,
        pipeline_type: pipelineType,
        nodes: JSON.parse(nodesJson),
        schedule_config: JSON.parse(scheduleJson),
        retry_policy: JSON.parse(retryJson),
      });
      navigate(`/pipelines/${created.id}/edit`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/pipelines" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Pipelines</Link>
      <header>
        <h1 className="of-heading-xl">New pipeline</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Create a pipeline with JSON-driven DAG, schedule, and retry config.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
        <label style={{ fontSize: 13 }}>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 13 }}>
          Description
          <input value={description} onChange={(e) => setDescription(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 13 }}>
          Pipeline type
          <select
            value={pipelineType}
            onChange={(e) => setPipelineType(e.target.value as typeof pipelineType)}
            className="of-input"
            style={{ marginTop: 4 }}
          >
            <option value="BATCH">BATCH</option>
            <option value="INCREMENTAL">INCREMENTAL</option>
            <option value="STREAMING">STREAMING</option>
            <option value="EXTERNAL">EXTERNAL</option>
            <option value="FASTER">FASTER</option>
          </select>
        </label>
        <JsonEditor label="Nodes JSON (DAG)" value={nodesJson} onChange={setNodesJson} minHeight={240} />
        <JsonEditor label="Schedule config JSON" value={scheduleJson} onChange={setScheduleJson} minHeight={80} />
        <JsonEditor label="Retry policy JSON" value={retryJson} onChange={setRetryJson} minHeight={80} />
        <div>
          <button type="button" onClick={() => void handleCreate()} disabled={busy} className="of-button of-button--primary">
            Create
          </button>
        </div>
      </section>
    </section>
  );
}
