import { useEffect, useMemo, useState } from 'react';

import {
  getOntologyStorageInsights,
  type OntologyStorageInsights,
} from '@/lib/api/ontology';

interface LayerCard {
  title: string;
  tone: string;
  metric: string;
  detail: string;
  href: string;
  cta: string;
}

interface RuntimeMilestone {
  label: string;
  value: string;
  detail: string;
}

const numberFormatter = new Intl.NumberFormat('en-US');
const dateFormatter = new Intl.DateTimeFormat('en-GB', {
  dateStyle: 'medium',
  timeStyle: 'short',
});

function formatCount(value: number) {
  return numberFormatter.format(value);
}

function formatDate(value: string | null | undefined) {
  if (!value) return 'No activity recorded';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? 'No activity recorded' : dateFormatter.format(parsed);
}

export function ObjectDatabasesPage() {
  const [loading, setLoading] = useState(true);
  const [pageError, setPageError] = useState('');
  const [insights, setInsights] = useState<OntologyStorageInsights | null>(null);

  function load() {
    setLoading(true);
    setPageError('');
    let cancelled = false;
    getOntologyStorageInsights()
      .then((data) => {
        if (cancelled) return;
        setInsights(data);
      })
      .catch((error) => {
        if (cancelled) return;
        setPageError(error instanceof Error ? error.message : 'Failed to load object database insights');
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }

  useEffect(() => {
    const cancel = load();
    return cancel;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function getMetricCount(key: string) {
    return insights?.table_metrics.find((metric) => metric.key === key)?.record_count ?? 0;
  }

  const headlineCards = useMemo(
    () => [
      {
        label: 'Object rows',
        value: formatCount(getMetricCount('object_instances')),
        detail: 'Canonical ontology instances persisted in the transactional store.',
      },
      {
        label: 'Link rows',
        value: formatCount(getMetricCount('link_instances')),
        detail: 'Relationship edges materialized for graph traversal and object views.',
      },
      {
        label: 'Search documents',
        value: formatCount(insights?.search_documents_total ?? 0),
        detail: 'Projection documents synthesized by the ontology indexer for search surfaces.',
      },
      {
        label: 'Funnel sources',
        value: formatCount(getMetricCount('funnel_sources')),
        detail: 'Ingress definitions that hydrate object rows from datasets and pipelines.',
      },
    ],
    [insights],
  );

  const layerCards = useMemo<LayerCard[]>(
    () => [
      {
        title: 'Transactional object store',
        tone: 'background:#e7f3ff;color:#2458b8',
        metric: `${formatCount(getMetricCount('object_instances'))} rows`,
        detail:
          'Objects land in PostgreSQL-backed object_instances, with schema contracts held in object_types, properties, interfaces, and shared property bindings.',
        href: '/object-link-types',
        cta: 'Review schema',
      },
      {
        title: 'Graph relationship store',
        tone: 'background:#eaf7ef;color:#2f6d35',
        metric: `${formatCount(getMetricCount('link_instances'))} edges`,
        detail:
          'Links are first-class rows in link_instances, keyed by link_types, which gives OpenFoundry a concrete object-graph substrate rather than a docs-only abstraction.',
        href: '/vertex',
        cta: 'Open graph product',
      },
      {
        title: 'Search projection layer',
        tone: 'background:#f1ecff;color:#6f42c1',
        metric: `${formatCount(insights?.search_documents_total ?? 0)} docs`,
        detail:
          'The ontology indexer materializes searchable projection documents from types, interfaces, links, actions, and accessible objects for Object Explorer and semantic search.',
        href: '/object-explorer',
        cta: 'Explore search',
      },
      {
        title: 'Ingestion and hydration runtime',
        tone: 'background:#fff1df;color:#a35a11',
        metric: `${formatCount(getMetricCount('funnel_runs'))} runs`,
        detail:
          'Funnel sources and runs form the hydration control plane, bridging datasets and pipelines into persisted ontology rows with batch and streaming posture.',
        href: '/ontology-indexing',
        cta: 'Operate indexing',
      },
      {
        title: 'Governance and scoping',
        tone: 'background:#f3efe7;color:#6e5330',
        metric: `${formatCount(getMetricCount('projects'))} projects`,
        detail:
          'Ontology projects, resource bindings, and manager surfaces define how persisted objects and schema resources are segmented, reviewed, and shipped.',
        href: '/ontology-manager',
        cta: 'Open manager',
      },
    ],
    [insights],
  );

  const groupedTables = useMemo(() => {
    const groups = [
      { label: 'Schema tables', role: 'Schema' },
      { label: 'Runtime tables', role: 'Runtime' },
      { label: 'Ingestion tables', role: 'Ingestion' },
      { label: 'Governance tables', role: 'Governance' },
    ];

    return groups
      .map((group) => ({
        ...group,
        tables: (insights?.table_metrics ?? []).filter((metric) => metric.role === group.role),
      }))
      .filter((group) => group.tables.length > 0);
  }, [insights]);

  const runtimeMilestones = useMemo<RuntimeMilestone[]>(
    () => [
      {
        label: 'Latest object write',
        value: formatDate(insights?.latest_object_write_at),
        detail: 'Most recent updated_at observed in object_instances.',
      },
      {
        label: 'Latest link write',
        value: formatDate(insights?.latest_link_write_at),
        detail: 'Most recent materialized relationship row in link_instances.',
      },
      {
        label: 'Latest funnel run',
        value: formatDate(insights?.latest_funnel_run_at),
        detail: 'Most recent ingestion attempt across all ontology funnel sources.',
      },
    ],
    [insights],
  );

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ maxWidth: 720 }}>
            <p className="of-eyebrow" style={{ color: '#2458b8' }}>
              Object Databases
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              Inspect the real ontology storage topology behind OpenFoundry
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
              This product turns the architecture topic into a first-class operational surface:
              PostgreSQL-backed object rows, graph edges, search projections, Funnel hydration, and
              governance tables are all shown from the live ontology runtime.
            </p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, marginTop: 16 }}>
              <button
                type="button"
                className="of-button of-button--primary"
                onClick={() => load()}
                disabled={loading}
              >
                Refresh insights
              </button>
              <a href="/ontology" className="of-button">
                Back to ontology hub
              </a>
            </div>
          </div>
          <div className="of-panel-muted" style={{ padding: 16, minWidth: 280, background: '#0c0a09', color: '#f5f5f4' }}>
            <p className="of-eyebrow" style={{ color: '#94a3b8' }}>
              Storage runtime
            </p>
            <div style={{ display: 'grid', gap: 12, marginTop: 12, fontSize: 13 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                <span style={{ color: '#94a3b8' }}>Primary backend</span>
                <span style={{ fontWeight: 600 }}>{insights?.database_backend ?? 'Loading…'}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                <span style={{ color: '#94a3b8' }}>Access driver</span>
                <span style={{ fontWeight: 600 }}>{insights?.access_driver ?? 'Loading…'}</span>
              </div>
              <div style={{ height: 1, background: 'rgba(255,255,255,0.1)', margin: '4px 0' }} />
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                <span style={{ color: '#94a3b8' }}>Graph projection</span>
                <span style={{ fontWeight: 600, textAlign: 'right' }}>{insights?.graph_projection ?? 'Loading…'}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                <span style={{ color: '#94a3b8' }}>Search projection</span>
                <span style={{ fontWeight: 600, textAlign: 'right' }}>{insights?.search_projection ?? 'Loading…'}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                <span style={{ color: '#94a3b8' }}>Hydration runtime</span>
                <span style={{ fontWeight: 600, textAlign: 'right' }}>{insights?.funnel_runtime ?? 'Loading…'}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {pageError && (
        <div className="of-status-danger" style={{ padding: '12px 16px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {pageError}
        </div>
      )}

      {loading ? (
        <div className="of-panel" style={{ padding: 56, textAlign: 'center', fontSize: 13, color: 'var(--text-muted)' }}>
          Loading object database insights…
        </div>
      ) : insights ? (
        <>
          <section style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
            {headlineCards.map((card) => (
              <article key={card.label} className="of-panel" style={{ padding: 20 }}>
                <p className="of-eyebrow">{card.label}</p>
                <p style={{ marginTop: 8, fontSize: 28, fontWeight: 600, color: 'var(--text-strong)' }}>{card.value}</p>
                <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
                  {card.detail}
                </p>
              </article>
            ))}
          </section>

          <section className="of-panel" style={{ padding: 24 }}>
            <div style={{ marginBottom: 16 }}>
              <p className="of-eyebrow">Topology</p>
              <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                Storage layers mapped to real products
              </h2>
            </div>
            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))' }}>
              {layerCards.map((layer) => (
                <article
                  key={layer.title}
                  className="of-panel-muted"
                  style={{ padding: 16, display: 'flex', flexDirection: 'column' }}
                >
                  <span
                    style={{
                      ...Object.fromEntries(
                        layer.tone.split(';').map((entry) => {
                          const [k, v] = entry.split(':');
                          return [k.trim(), v.trim()];
                        }),
                      ),
                      display: 'inline-block',
                      width: 'fit-content',
                      padding: '4px 10px',
                      borderRadius: 999,
                      fontSize: 11,
                      fontWeight: 600,
                    }}
                  >
                    {layer.title}
                  </span>
                  <p style={{ marginTop: 12, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>{layer.metric}</p>
                  <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13, flex: 1 }}>
                    {layer.detail}
                  </p>
                  <a href={layer.href} style={{ marginTop: 12, fontSize: 13, fontWeight: 600 }}>
                    {layer.cta} →
                  </a>
                </article>
              ))}
            </div>
          </section>

          <section style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.05fr) minmax(0, 0.95fr)' }}>
            <article className="of-panel" style={{ padding: 24 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                <div>
                  <p className="of-eyebrow">Tables</p>
                  <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                    Persistent storage inventory
                  </h2>
                </div>
                <div className="of-chip">{formatCount(insights.table_metrics.length)} tracked tables</div>
              </div>
              <div style={{ display: 'grid', gap: 16, marginTop: 20 }}>
                {groupedTables.map((group) => (
                  <div key={group.label}>
                    <h3 style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-strong)' }}>{group.label}</h3>
                    <div className="of-panel-muted" style={{ marginTop: 8, padding: 0, overflow: 'hidden' }}>
                      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
                        <thead>
                          <tr style={{ background: 'var(--bg-subtle)', textAlign: 'left' }}>
                            <th style={{ padding: '10px 12px', fontWeight: 600 }}>Label</th>
                            <th style={{ padding: '10px 12px', fontWeight: 600 }}>Table</th>
                            <th style={{ padding: '10px 12px', fontWeight: 600, textAlign: 'right' }}>Rows</th>
                          </tr>
                        </thead>
                        <tbody>
                          {group.tables.map((metric) => (
                            <tr key={metric.key} style={{ borderTop: '1px solid var(--border-default)' }}>
                              <td style={{ padding: '10px 12px', fontWeight: 500, color: 'var(--text-strong)' }}>{metric.label}</td>
                              <td style={{ padding: '10px 12px', fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-muted)' }}>
                                {metric.table_name}
                              </td>
                              <td style={{ padding: '10px 12px', textAlign: 'right' }}>{formatCount(metric.record_count)}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                ))}
              </div>
            </article>

            <article className="of-panel" style={{ padding: 24 }}>
              <p className="of-eyebrow">Activity</p>
              <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                Runtime milestones
              </h2>
              <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
                {runtimeMilestones.map((milestone) => (
                  <div key={milestone.label} className="of-panel-muted" style={{ padding: 14 }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                      <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{milestone.label}</p>
                      <p className="of-text-muted" style={{ fontSize: 13 }}>{milestone.value}</p>
                    </div>
                    <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                      {milestone.detail}
                    </p>
                  </div>
                ))}
              </div>

              <div style={{ marginTop: 24 }}>
                <p className="of-eyebrow">Search kinds</p>
                <div style={{ display: 'grid', gap: 10, marginTop: 12 }}>
                  {insights.search_documents_by_kind.map((metric) => (
                    <div key={metric.kind}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, fontSize: 13, marginBottom: 4 }}>
                        <span style={{ fontWeight: 500, color: 'var(--text-strong)', textTransform: 'capitalize' }}>
                          {metric.kind.replaceAll('_', ' ')}
                        </span>
                        <span className="of-text-muted">{formatCount(metric.count)}</span>
                      </div>
                      <div style={{ height: 6, background: 'var(--bg-subtle)', borderRadius: 999, overflow: 'hidden' }}>
                        <div
                          style={{
                            height: '100%',
                            background: '#0f172a',
                            width: `${Math.max(8, insights.search_documents_total > 0 ? (metric.count / insights.search_documents_total) * 100 : 0)}%`,
                          }}
                        />
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </article>
          </section>

          <section style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fit, minmax(360px, 1fr))' }}>
            <article className="of-panel" style={{ padding: 24 }}>
              <p className="of-eyebrow">Distribution</p>
              <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                Object rows by type
              </h2>
              <div style={{ display: 'grid', gap: 10, marginTop: 16 }}>
                {insights.object_type_distribution.map((metric) => (
                  <div key={metric.id}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, fontSize: 13, marginBottom: 4 }}>
                      <span style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{metric.label}</span>
                      <span className="of-text-muted">{formatCount(metric.count)}</span>
                    </div>
                    <div style={{ height: 6, background: 'var(--bg-subtle)', borderRadius: 999, overflow: 'hidden' }}>
                      <div
                        style={{
                          height: '100%',
                          background: '#2458b8',
                          width: `${Math.max(8, getMetricCount('object_instances') > 0 ? (metric.count / getMetricCount('object_instances')) * 100 : 0)}%`,
                        }}
                      />
                    </div>
                  </div>
                ))}
              </div>
            </article>

            <article className="of-panel" style={{ padding: 24 }}>
              <p className="of-eyebrow">Distribution</p>
              <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                Link rows by type
              </h2>
              <div style={{ display: 'grid', gap: 10, marginTop: 16 }}>
                {insights.link_type_distribution.map((metric) => (
                  <div key={metric.id}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, fontSize: 13, marginBottom: 4 }}>
                      <span style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{metric.label}</span>
                      <span className="of-text-muted">{formatCount(metric.count)}</span>
                    </div>
                    <div style={{ height: 6, background: 'var(--bg-subtle)', borderRadius: 999, overflow: 'hidden' }}>
                      <div
                        style={{
                          height: '100%',
                          background: '#2f6d35',
                          width: `${Math.max(8, getMetricCount('link_instances') > 0 ? (metric.count / getMetricCount('link_instances')) * 100 : 0)}%`,
                        }}
                      />
                    </div>
                  </div>
                ))}
              </div>
            </article>
          </section>

          <section className="of-panel" style={{ padding: 24 }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
              <div style={{ maxWidth: 720 }}>
                <p className="of-eyebrow">Indexes</p>
                <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                  Database access paths
                </h2>
                <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13, lineHeight: 1.7 }}>
                  These definitions come directly from PostgreSQL metadata. They make the object
                  store, relationship traversal, Funnel hydration, and schema browsing surfaces
                  materially inspectable from the repo itself.
                </p>
              </div>
              <div className="of-chip">{formatCount(insights.index_definitions.length)} indexes</div>
            </div>
            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(360px, 1fr))', marginTop: 16 }}>
              {insights.index_definitions.map((indexDef) => (
                <article key={`${indexDef.table_name}-${indexDef.index_name}`} className="of-panel-muted" style={{ padding: 14 }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                    <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{indexDef.index_name}</p>
                    <span className="of-chip" style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>
                      {indexDef.table_name}
                    </span>
                  </div>
                  <pre
                    style={{
                      marginTop: 10,
                      overflowX: 'auto',
                      fontFamily: 'var(--font-mono)',
                      fontSize: 11,
                      color: 'var(--text-muted)',
                      whiteSpace: 'pre-wrap',
                      wordBreak: 'break-word',
                    }}
                  >
                    {indexDef.index_definition}
                  </pre>
                </article>
              ))}
            </div>
          </section>
        </>
      ) : null}
    </section>
  );
}
