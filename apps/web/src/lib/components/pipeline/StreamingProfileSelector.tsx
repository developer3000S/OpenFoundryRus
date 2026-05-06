import { useEffect, useMemo, useState } from 'react';

import {
  attachProfileToPipeline,
  detachProfileFromPipeline,
  getPipelineEffectiveFlinkConfig,
  listPipelineStreamingProfiles,
  listStreamingProfileProjectRefs,
  listStreamingProfiles,
  type EffectiveFlinkConfig,
  type StreamingProfile,
} from '@/lib/api/streaming';

interface StreamingProfileSelectorProps {
  pipelineRid: string;
  projectRid: string;
  readOnly?: boolean;
}

export function StreamingProfileSelector({ pipelineRid, projectRid, readOnly = false }: StreamingProfileSelectorProps) {
  const [allProfiles, setAllProfiles] = useState<StreamingProfile[]>([]);
  const [attached, setAttached] = useState<StreamingProfile[]>([]);
  const [availableInProject, setAvailableInProject] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [effective, setEffective] = useState<EffectiveFlinkConfig | null>(null);
  const [effectiveError, setEffectiveError] = useState('');

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const [profilesRes, attachedRes] = await Promise.all([
        listStreamingProfiles({}),
        listPipelineStreamingProfiles(pipelineRid),
      ]);
      setAllProfiles(profilesRes.data);
      setAttached(attachedRes.data);
      const inProject = new Set<string>();
      for (const p of profilesRes.data) {
        try {
          const refs = await listStreamingProfileProjectRefs(p.id);
          if (refs.data.some((r) => r.project_rid === projectRid)) inProject.add(p.id);
        } catch { /* ignore single-profile failure */ }
      }
      setAvailableInProject(inProject);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (pipelineRid && projectRid) void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pipelineRid, projectRid]);

  const attachedIds = useMemo(() => new Set(attached.map((p) => p.id)), [attached]);
  const availableForPicker = useMemo(
    () => allProfiles.filter((p) => availableInProject.has(p.id) && !attachedIds.has(p.id)),
    [allProfiles, availableInProject, attachedIds],
  );

  async function attach(profile: StreamingProfile) {
    try {
      await attachProfileToPipeline(pipelineRid, { project_rid: projectRid, profile_id: profile.id });
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  async function detach(profileId: string) {
    try {
      await detachProfileFromPipeline(pipelineRid, profileId);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  async function toggleAdvanced() {
    if (advancedOpen) {
      setAdvancedOpen(false);
      return;
    }
    setEffectiveError('');
    try {
      setEffective(await getPipelineEffectiveFlinkConfig(pipelineRid));
      setAdvancedOpen(true);
    } catch (cause) {
      setEffectiveError(cause instanceof Error ? cause.message : String(cause));
      setAdvancedOpen(true);
    }
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 8, padding: 12, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 8, color: '#e2e8f0' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h3 style={{ margin: 0, fontSize: 14 }}>Streaming profiles</h3>
        <button type="button" onClick={() => void refresh()} disabled={loading} className="of-button" style={{ fontSize: 11 }}>
          Refresh
        </button>
      </header>

      {error && <p style={{ color: '#fca5a5', fontSize: 12, margin: 0 }}>{error}</p>}

      <div>
        <p className="of-eyebrow" style={{ fontSize: 10 }}>Attached ({attached.length})</p>
        <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, marginTop: 4 }}>
          {attached.map((p) => (
            <li key={p.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '6px 8px', background: '#1e293b', borderRadius: 4, fontSize: 12 }}>
              <span><strong>{p.name}</strong> · {p.category} · {p.size_class}</span>
              {!readOnly && (
                <button type="button" onClick={() => void detach(p.id)} className="of-button" style={{ fontSize: 11, color: '#fca5a5', borderColor: '#7f1d1d' }}>Detach</button>
              )}
            </li>
          ))}
          {attached.length === 0 && <li style={{ color: '#94a3b8', fontStyle: 'italic', fontSize: 11 }}>None.</li>}
        </ul>
      </div>

      {!readOnly && availableForPicker.length > 0 && (
        <div>
          <p className="of-eyebrow" style={{ fontSize: 10 }}>Available in project ({availableForPicker.length})</p>
          <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, marginTop: 4 }}>
            {availableForPicker.map((p) => (
              <li key={p.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '6px 8px', background: '#0f172a', borderRadius: 4, fontSize: 12 }}>
                <span><strong>{p.name}</strong> · {p.category} · {p.size_class}</span>
                <button type="button" onClick={() => void attach(p)} className="of-button" style={{ fontSize: 11 }}>Attach</button>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div>
        <button type="button" onClick={() => void toggleAdvanced()} className="of-button" style={{ fontSize: 11 }}>
          {advancedOpen ? '▾ Hide advanced' : '▸ Show effective Flink config'}
        </button>
        {advancedOpen && (
          <div style={{ marginTop: 8 }}>
            {effectiveError && <p style={{ color: '#fca5a5', fontSize: 11 }}>{effectiveError}</p>}
            {effective && (
              <pre style={{ background: '#020617', color: '#cbd5e1', padding: 8, borderRadius: 4, fontSize: 10, overflow: 'auto', maxHeight: 240 }}>
                {JSON.stringify(effective, null, 2)}
              </pre>
            )}
          </div>
        )}
      </div>
    </section>
  );
}
