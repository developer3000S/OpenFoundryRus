import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';

import { listProjects, type OntologyProject } from '@/lib/api/ontology';
import {
  createPipeline,
  type DistributedConfig,
  type ExternalConfig,
  type IncrementalConfig,
  type PipelineNode,
  type PipelineRetryPolicy,
  type PipelineScheduleConfig,
  type PipelineType,
  type StreamingConfig,
} from '@/lib/api/pipelines';
import { ChoosePipelineLocationDialog } from '@/lib/components/pipeline/ChoosePipelineLocationDialog';
import { Glyph } from '@/lib/components/ui/Glyph';

type PrimaryFamily = 'BATCH' | 'STREAMING';
type SubmitState = 'idle' | 'loading' | 'success' | 'error';

interface PrimaryCard {
  id: PrimaryFamily;
  title: string;
  summary: string;
}

interface AdvancedOption {
  id: PipelineType;
  family: PrimaryFamily;
  label: string;
  helper: string;
  badge?: string;
}

const PRIMARY_CARDS: PrimaryCard[] = [
  {
    id: 'BATCH',
    title: 'Batch pipeline',
    summary:
      'Builds and transforms entire datasets on each deploy. For data that is ingested periodically.',
  },
  {
    id: 'STREAMING',
    title: 'Streaming pipeline',
    summary:
      'Transforms data continuously as new data is made available. For high frequency ingestion.',
  },
];

const ADVANCED_OPTIONS: AdvancedOption[] = [
  { id: 'BATCH', family: 'BATCH', label: 'Standard', helper: 'Build your pipelines using advanced expressions. Backed by Spark.', badge: 'Default' },
  { id: 'FASTER', family: 'BATCH', label: 'Lightweight / Faster', helper: 'Fast interactive builds and previews on OpenFoundry local table execution.', badge: 'New' },
  { id: 'DISTRIBUTED', family: 'BATCH', label: 'Spark / Flink distributed', helper: 'Run the same graph through a distributed compute adapter.', badge: 'Cluster' },
  { id: 'EXTERNAL', family: 'BATCH', label: 'External', helper: 'Build your pipelines using external compute platforms.', badge: 'Beta' },
  { id: 'INCREMENTAL', family: 'BATCH', label: 'Incremental', helper: 'Only process changed rows.' },
  { id: 'STREAMING', family: 'STREAMING', label: 'Streaming', helper: 'Continuous topology over a stream.' },
];

const DEFAULT_SCHEDULE: PipelineScheduleConfig = { enabled: false, cron: null };
const DEFAULT_RETRY_POLICY: PipelineRetryPolicy = {
  max_attempts: 1,
  retry_on_failure: false,
  allow_partial_reexecution: true,
};

function makeId(prefix = 'node') {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `${prefix}_${Date.now()}_${Math.floor(Math.random() * 10_000)}`;
}

function defaultNodes(pipelineType: PipelineType, virtualTableRid: string | null): PipelineNode[] {
  if (virtualTableRid) {
    const sourceId = makeId('source');
    const transformId = makeId('transform');
    return [
      {
        id: sourceId,
        label: 'Read virtual table',
        transform_type: 'external',
        config: {
          source_kind: 'virtual_table',
          virtual_table_rid: virtualTableRid,
          mode: 'incremental',
        },
        depends_on: [],
        input_dataset_ids: [],
        output_dataset_id: null,
      },
      {
        id: transformId,
        label: 'Incremental transform',
        transform_type: 'sql',
        config: { sql: 'SELECT * FROM source_rows' },
        depends_on: [sourceId],
        input_dataset_ids: [],
        output_dataset_id: null,
      },
    ];
  }
  if (pipelineType === 'FASTER') {
    const sourceId = makeId('source');
    const filterId = makeId('filter');
    return [
      {
        id: sourceId,
        label: 'Lightweight sample rows',
        transform_type: 'dataset_input',
        config: { rows: [{ value: 1, status: 'ready' }] },
        depends_on: [],
        input_dataset_ids: [],
        output_dataset_id: null,
      },
      {
        id: filterId,
        label: 'Local table filter',
        transform_type: 'filter',
        config: { predicate: 'value > 0' },
        depends_on: [sourceId],
        input_dataset_ids: [],
        output_dataset_id: null,
      },
    ];
  }
  return [
    {
      id: makeId(),
      label: pipelineType === 'STREAMING' ? 'Streaming transform' : 'Sql transform',
      transform_type: 'sql',
      config: { sql: 'SELECT 1 AS value' },
      depends_on: [],
      input_dataset_ids: [],
      output_dataset_id: null,
    },
  ];
}

function defaultIncremental(virtualTableRid: string | null): IncrementalConfig {
  return {
    replay_on_deploy: false,
    watermark_columns: [],
    allowed_transaction_types: virtualTableRid ? 'INSERT,UPDATE' : 'INSERT',
  };
}

function defaultStreaming(): StreamingConfig {
  return {
    input_stream_id: null,
    output_stream_id: null,
    streaming_profile_id: null,
    parallelism: 1,
  };
}

function defaultExternal(virtualTableRid: string | null): ExternalConfig {
  return {
    source_system: virtualTableRid ? 'virtual_table' : 'external',
    source_id: virtualTableRid,
    compute_profile_id: null,
  };
}

function defaultDistributed(): DistributedConfig {
  return {
    engine: 'spark',
    compute_profile_id: null,
    runner_image: null,
  };
}

function formatTimestamp(date: Date) {
  const weekday = date.toLocaleString('en-US', { weekday: 'short' });
  const month = date.toLocaleString('en-US', { month: 'short' });
  const day = date.getDate();
  const year = date.getFullYear();
  let hours = date.getHours();
  const minutes = String(date.getMinutes()).padStart(2, '0');
  const seconds = String(date.getSeconds()).padStart(2, '0');
  const meridiem = hours >= 12 ? 'PM' : 'AM';
  hours = hours % 12 || 12;
  return `${weekday}, ${month} ${day}, ${year}, ${hours}:${minutes}:${seconds} ${meridiem}`;
}

function PipelineFileGlyph({ size = 18 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <rect x="3" y="9" width="6" height="6" rx="1.5" stroke="#15803d" strokeWidth="1.6" />
      <rect x="15" y="9" width="6" height="6" rx="1.5" stroke="#15803d" strokeWidth="1.6" />
      <path d="M9 12h6" stroke="#15803d" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}

export function PipelineNewPage() {
  const [searchParams] = useSearchParams();
  const virtualTableRid = searchParams.get('virtual_table');
  const navigate = useNavigate();

  const initialFamily: PrimaryFamily = 'BATCH';
  const initialPipelineType: PipelineType = virtualTableRid ? 'INCREMENTAL' : 'BATCH';

  const [createdAt] = useState(() => new Date());
  const defaultName = useMemo(() => {
    if (virtualTableRid) {
      const tail = virtualTableRid.split('/').pop() || virtualTableRid;
      return `Incremental pipeline for ${tail}`;
    }
    return `New pipeline (${formatTimestamp(createdAt)})`;
  }, [createdAt, virtualTableRid]);

  const [name, setName] = useState(defaultName);
  const [primary, setPrimary] = useState<PrimaryFamily>(initialFamily);
  const [pipelineType, setPipelineType] = useState<PipelineType>(initialPipelineType);

  const [projectId, setProjectId] = useState(searchParams.get('project_id') ?? '');
  const [projectLabel, setProjectLabel] = useState('');
  const [projectError, setProjectError] = useState('');
  const [_projects, setProjects] = useState<OntologyProject[]>([]);

  const [showLocation, setShowLocation] = useState(false);
  const [submitState, setSubmitState] = useState<SubmitState>('idle');
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    listProjects({ per_page: 100 })
      .then((res) => {
        if (!cancelled) {
          setProjects(res.data);
          if (projectId && !projectLabel) {
            const match = res.data.find((entry) => entry.id === projectId);
            if (match) setProjectLabel(match.display_name || match.slug);
          }
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled) setProjectError(cause instanceof Error ? cause.message : String(cause));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function handlePrimaryChange(next: PrimaryFamily) {
    setPrimary(next);
    if (next === 'STREAMING') {
      setPipelineType('STREAMING');
    } else if (pipelineType === 'STREAMING') {
      setPipelineType(virtualTableRid ? 'INCREMENTAL' : 'BATCH');
    }
  }

  const advancedForFamily = useMemo(
    () => ADVANCED_OPTIONS.filter((option) => option.family === primary),
    [primary],
  );

  const canCreate = Boolean(name.trim()) && Boolean(projectId);
  const busy = submitState === 'loading';

  async function handleCreate() {
    if (!canCreate) return;
    setSubmitState('loading');
    setError('');

    try {
      const body: Parameters<typeof createPipeline>[0] = {
        name: name.trim(),
        status: 'draft',
        pipeline_type: pipelineType,
        nodes: defaultNodes(pipelineType, virtualTableRid),
        schedule_config: DEFAULT_SCHEDULE,
        retry_policy: DEFAULT_RETRY_POLICY,
        project_id: projectId,
      };
      if (pipelineType === 'INCREMENTAL') body.incremental = defaultIncremental(virtualTableRid);
      if (pipelineType === 'STREAMING') body.streaming = defaultStreaming();
      if (pipelineType === 'EXTERNAL') body.external = defaultExternal(virtualTableRid);
      if (pipelineType === 'DISTRIBUTED') body.distributed = defaultDistributed();

      const created = await createPipeline(body);
      setSubmitState('success');
      navigate(`/pipelines/${created.id}/edit`);
    } catch (cause) {
      setSubmitState('error');
      setError(cause instanceof Error ? cause.message : 'Create failed');
    }
  }

  return (
    <section className="of-pipe-create-page">
      <div className="of-pipe-create-breadcrumb">
        <Link to="/pipelines">Pipelines</Link>
        <Glyph name="chevron-right" size={12} />
        <span>New</span>
      </div>

      <div className="of-pipe-create-card">
        <header className="of-pipe-create-card__header">
          <h1>Configuration</h1>
        </header>

        <div className="of-pipe-create-card__body">
          {virtualTableRid && (
            <div className="of-pipe-create-info">
              Source virtual table: <code>{virtualTableRid}</code>
            </div>
          )}

          <section className="of-pipe-create-section">
            <h2 className="of-pipe-create-section__title">Pipeline name and location</h2>
            <div className="of-pipe-create-name-row">
              <div className="of-pipe-create-name-info">
                <span className="of-pipe-create-name-icon" aria-hidden="true">
                  <PipelineFileGlyph size={20} />
                </span>
                <div className="of-pipe-create-name-text">
                  <input
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    className="of-pipe-create-name-input"
                    aria-label="Pipeline name"
                    placeholder="Pipeline name"
                  />
                  <p className={`of-pipe-create-location${projectId ? '' : ' of-pipe-create-location--empty'}`}>
                    {projectId ? projectLabel : 'No location selected'}
                  </p>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setShowLocation(true)}
                className="of-button of-button--primary of-pipe-create-select-location"
              >
                <Glyph name="folder" size={14} />
                Select location
              </button>
            </div>
            {projectError && (
              <p className="of-pipe-create-warning">{projectError}</p>
            )}
          </section>

          <section className="of-pipe-create-section">
            <h2 className="of-pipe-create-section__title">Pipeline type</h2>
            <div className="of-pipe-create-types">
              {PRIMARY_CARDS.map((card) => {
                const active = primary === card.id;
                return (
                  <button
                    key={card.id}
                    type="button"
                    aria-pressed={active}
                    onClick={() => handlePrimaryChange(card.id)}
                    className={`of-pipe-create-type-card${active ? ' of-pipe-create-type-card--active' : ''}`}
                  >
                    <span
                      className={`of-pipe-create-radio${active ? ' of-pipe-create-radio--active' : ''}`}
                      aria-hidden="true"
                    >
                      {active && <span className="of-pipe-create-radio__dot" />}
                    </span>
                    <div className="of-pipe-create-type-card__text">
                      <strong>{card.title}</strong>
                      <p>{card.summary}</p>
                    </div>
                  </button>
                );
              })}
            </div>

            {primary === 'BATCH' && (
              <div style={{ marginTop: 18 }}>
                <h3 style={{ margin: '0 0 8px', fontSize: 12, fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
                  Select batch compute
                </h3>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10 }}>
                  {advancedForFamily
                    .filter((option) => option.id !== 'INCREMENTAL')
                    .map((option) => {
                      const active = pipelineType === option.id;
                      return (
                        <button
                          key={option.id}
                          type="button"
                          aria-pressed={active}
                          onClick={() => setPipelineType(option.id)}
                          style={{
                            display: 'grid',
                            gap: 6,
                            padding: '12px 14px',
                            borderRadius: 6,
                            border: active ? '2px solid var(--status-info)' : '1px solid var(--border-default)',
                            background: active ? 'rgba(45, 114, 210, 0.04)' : '#fff',
                            cursor: 'pointer',
                            textAlign: 'left',
                          }}
                        >
                          <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <strong style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-strong)' }}>{option.label}</strong>
                            {option.badge ? (
                              <span
                                style={{
                                  fontSize: 10,
                                  fontWeight: 600,
                                  padding: '2px 8px',
                                  borderRadius: 999,
                                  background: option.badge === 'Default' ? 'rgba(45, 114, 210, 0.12)' : option.badge === 'New' ? 'rgba(34, 197, 94, 0.16)' : 'rgba(108, 117, 125, 0.16)',
                                  color: option.badge === 'Default' ? 'var(--status-info)' : option.badge === 'New' ? '#15803d' : '#475569',
                                }}
                              >
                                {option.badge}
                              </span>
                            ) : null}
                          </span>
                          <span style={{ fontSize: 12, color: 'var(--text-muted)', lineHeight: 1.4 }}>{option.helper}</span>
                        </button>
                      );
                    })}
                </div>
              </div>
            )}
          </section>

          {error && <div className="of-pipe-create-error">{error}</div>}
        </div>

        <footer className="of-pipe-create-card__footer">
          <button
            type="button"
            onClick={() => navigate(-1)}
            className="of-link"
            style={{ background: 'none', border: 0, padding: '6px 8px', cursor: 'pointer', fontSize: 13, color: 'var(--status-info)' }}
          >
            ← Back
          </button>
          <button
            type="button"
            onClick={() => void handleCreate()}
            disabled={!canCreate || busy}
            className={`of-pipe-create-submit${canCreate && !busy ? ' of-pipe-create-submit--ready' : ''}`}
          >
            {busy ? 'Creating...' : 'Create pipeline'}
          </button>
        </footer>
      </div>

      <ChoosePipelineLocationDialog
        open={showLocation}
        initialFileName={name}
        initialProjectId={projectId}
        onCancel={() => setShowLocation(false)}
        onSave={({ fileName, projectId: nextProject, projectLabel: nextLabel }) => {
          setName(fileName);
          setProjectId(nextProject);
          setProjectLabel(nextLabel);
          setShowLocation(false);
        }}
      />
    </section>
  );
}
