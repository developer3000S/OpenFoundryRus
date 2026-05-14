import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  getJobOutputsV1,
  type BuildEnvelope,
  type Job,
  type JobOutputRow,
  type JobOutputsResponse,
} from '@/lib/api/buildsV1';

import { StateBadge } from './StateBadge';

interface ArtifactsPanelProps {
  build: BuildEnvelope;
  selectedJobRid?: string;
  onSelectJob?: (rid: string) => void;
}

interface ArtifactRow {
  key: string;
  job: Job;
  output?: JobOutputRow;
  transactionRid: string;
}

function shortRid(value: string | null | undefined, chars = 12) {
  return value ? value.slice(0, chars) : '—';
}

function DatasetRidLink({ rid }: { rid: string | null | undefined }) {
  if (!rid) return <>—</>;
  return (
    <Link to={`/datasets/${encodeURIComponent(rid)}`} className="of-link" style={{ fontFamily: 'var(--font-mono)' }}>
      {shortRid(rid, 18)}
    </Link>
  );
}

function artifactRows(build: BuildEnvelope, outputsByJob: Record<string, JobOutputsResponse | null>): ArtifactRow[] {
  return (build.jobs ?? []).flatMap((job) => {
    const response = outputsByJob[job.rid];
    if (response?.outputs.length) {
      return response.outputs.map((output, index) => ({
        key: `${job.rid}:${output.transaction_rid}:${index}`,
        job,
        output,
        transactionRid: output.transaction_rid,
      }));
    }
    return (job.output_transaction_rids ?? []).map((transactionRid, index) => ({
      key: `${job.rid}:${transactionRid}:${index}`,
      job,
      transactionRid,
    }));
  });
}

export function ArtifactsPanel({ build, selectedJobRid, onSelectJob }: ArtifactsPanelProps) {
  const [outputsByJob, setOutputsByJob] = useState<Record<string, JobOutputsResponse | null>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    const jobs = build.jobs ?? [];
    if (jobs.length === 0) {
      setOutputsByJob({});
      return;
    }

    async function loadOutputs() {
      setLoading(true);
      setError('');
      try {
        const pairs = await Promise.all(
          jobs.map(async (job) => {
            try {
              return [job.rid, await getJobOutputsV1(job.rid)] as const;
            } catch {
              return [job.rid, null] as const;
            }
          }),
        );
        if (cancelled) return;
        setOutputsByJob(Object.fromEntries(pairs));
        const failed = pairs.filter(([, value]) => value === null).length;
        setError(failed > 0 ? `${failed} job output request${failed === 1 ? '' : 's'} could not be loaded.` : '');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void loadOutputs();
    return () => {
      cancelled = true;
    };
  }, [build.jobs]);

  const rows = useMemo(() => artifactRows(build, outputsByJob), [build, outputsByJob]);
  const committed = rows.filter((row) => row.output?.committed).length;
  const aborted = rows.filter((row) => row.output?.aborted).length;

  return (
    <section className="of-panel" style={{ display: 'grid', gap: 12, padding: 16 }}>
      <header style={{ display: 'flex', alignItems: 'start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <p className="of-eyebrow" style={{ margin: 0 }}>Artifacts</p>
          <h2 className="of-heading-md" style={{ margin: '4px 0 0' }}>Output transactions</h2>
          <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>
            {rows.length} outputs · {committed} committed · {aborted} aborted
          </p>
        </div>
        {loading && <span className="of-text-muted" style={{ fontSize: 12 }}>Loading outputs…</span>}
      </header>

      {error && (
        <div className="of-status-warning" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          {error}
        </div>
      )}

      {rows.length === 0 ? (
        <div style={{ padding: 24, border: '1px dashed var(--border-default)', borderRadius: 'var(--radius-md)', textAlign: 'center' }}>
          <p className="of-text-muted" style={{ margin: 0 }}>No output artifacts recorded for this build.</p>
        </div>
      ) : (
        <div style={{ overflow: 'auto', border: '1px solid var(--border-default)', borderRadius: 'var(--radius-md)' }}>
          <table className="of-table" style={{ minWidth: 860 }}>
            <thead>
              <tr>
                <th>Job</th>
                <th>Job state</th>
                <th>Dataset</th>
                <th>Transaction</th>
                <th>Commit</th>
                <th>Content hash</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const selected = selectedJobRid === row.job.rid;
                return (
                  <tr key={row.key} style={selected ? { background: 'var(--status-info-bg)' } : undefined}>
                    <td style={{ fontFamily: 'var(--font-mono)' }}>
                      {onSelectJob ? (
                        <button
                          type="button"
                          onClick={() => onSelectJob(row.job.rid)}
                          style={{ padding: 0, border: 0, background: 'transparent', color: 'var(--text-link)', fontFamily: 'var(--font-mono)' }}
                        >
                          {shortRid(row.job.rid)}
                        </button>
                      ) : (
                        shortRid(row.job.rid)
                      )}
                    </td>
                    <td><StateBadge kind="job" state={row.job.state} size="sm" /></td>
                    <td><DatasetRidLink rid={row.output?.output_dataset_rid} /></td>
                    <td style={{ fontFamily: 'var(--font-mono)' }}>{shortRid(row.transactionRid, 24)}</td>
                    <td>
                      {row.output ? (
                        <span className={row.output.aborted ? 'of-status-danger' : row.output.committed ? 'of-status-success' : 'of-status-info'} style={{ borderRadius: 999, padding: '2px 7px', fontSize: 11, fontWeight: 700 }}>
                          {row.output.aborted ? 'aborted' : row.output.committed ? 'committed' : 'open'}
                        </span>
                      ) : (
                        <span className="of-text-muted">pending</span>
                      )}
                    </td>
                    <td style={{ fontFamily: 'var(--font-mono)' }}>{shortRid(row.job.output_content_hash, 18)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
