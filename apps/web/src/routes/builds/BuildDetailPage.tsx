import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import {
  abortBuildV1,
  getBuildV1,
  getJobInputResolutionsV1,
  getJobOutputsV1,
  type BuildEnvelope,
  type JobInputResolutionsResponse,
  type JobOutputsResponse,
} from '@/lib/api/buildsV1';

export function BuildDetailPage() {
  const { rid: ridParam = '' } = useParams();
  const rid = decodeURIComponent(ridParam);

  const [build, setBuild] = useState<BuildEnvelope | null>(null);
  const [selectedJob, setSelectedJob] = useState<string>('');
  const [outputs, setOutputs] = useState<JobOutputsResponse | null>(null);
  const [resolutions, setResolutions] = useState<JobInputResolutionsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      setBuild(await getBuildV1(rid));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load build');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rid]);

  useEffect(() => {
    if (!selectedJob) return;
    let cancelled = false;
    async function load() {
      try {
        const [oRes, rRes] = await Promise.all([getJobOutputsV1(selectedJob), getJobInputResolutionsV1(selectedJob)]);
        if (cancelled) return;
        setOutputs(oRes);
        setResolutions(rRes);
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load job');
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [selectedJob]);

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/builds" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
        ← Builds
      </Link>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {loading ? (
        <p className="of-text-muted">Loading…</p>
      ) : build ? (
        <>
          <header>
            <h1 className="of-heading-xl">Build {build.rid.slice(0, 12)}</h1>
            <p className="of-text-muted" style={{ fontSize: 13 }}>
              {build.state} · {build.build_branch} · pipeline {build.pipeline_rid?.slice(0, 12)}
            </p>
            {(build.state === 'BUILD_QUEUED' || build.state === 'BUILD_RUNNING' || build.state === 'BUILD_RESOLUTION') && (
              <button
                type="button"
                onClick={async () => {
                  if (!window.confirm('Abort?')) return;
                  await abortBuildV1(rid);
                  await refresh();
                }}
                className="of-button"
                style={{ marginTop: 8, color: '#b91c1c', borderColor: '#fecaca' }}
              >
                Abort
              </button>
            )}
          </header>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Build envelope</p>
            <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 280 }}>
              {JSON.stringify(build, null, 2)}
            </pre>
          </section>

          <section className="of-panel" style={{ padding: 16 }}>
            <p className="of-eyebrow">Jobs ({build.jobs?.length ?? 0})</p>
            <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none' }}>
              {(build.jobs ?? []).map((j) => (
                <li key={j.rid} style={{ padding: 6, borderBottom: '1px solid var(--border-default)' }}>
                  <button
                    type="button"
                    onClick={() => setSelectedJob(j.rid)}
                    style={{ background: 'none', border: 'none', padding: 0, textAlign: 'left', cursor: 'pointer' }}
                  >
                    <strong>{j.rid.slice(0, 12)}</strong> · {j.state} · attempt {j.attempt}
                  </button>
                </li>
              ))}
            </ul>
          </section>

          {selectedJob && (
            <>
              <section className="of-panel" style={{ padding: 16 }}>
                <p className="of-eyebrow">Job outputs</p>
                <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
                  {JSON.stringify(outputs, null, 2)}
                </pre>
              </section>
              <section className="of-panel" style={{ padding: 16 }}>
                <p className="of-eyebrow">Job input resolutions</p>
                <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
                  {JSON.stringify(resolutions, null, 2)}
                </pre>
              </section>
            </>
          )}
        </>
      ) : null}
    </section>
  );
}
