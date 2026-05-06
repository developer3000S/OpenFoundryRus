import { useMemo, useState } from 'react';

import {
  virtualTables,
  type EnableAutoRegistrationRequest,
  type FolderMirrorKind,
  type VirtualTableProvider,
  type VirtualTableSourceLink,
} from '@/lib/api/virtual-tables';

interface Props {
  open: boolean;
  sourceRid: string;
  provider: VirtualTableProvider;
  onClose: () => void;
  onEnabled?: (link: VirtualTableSourceLink) => void;
}

type StepId = 'project' | 'layout' | 'tags' | 'interval' | 'review';

const POLL_OPTIONS = [
  { label: '15 minutes', value: 900 },
  { label: '1 hour', value: 3600 },
  { label: '6 hours', value: 21600 },
  { label: '24 hours', value: 86400 },
];

export function CreateAutoRegistrationModal({ open, sourceRid, provider, onClose, onEnabled }: Props) {
  const [stepIndex, setStepIndex] = useState(0);
  const [projectName, setProjectName] = useState('');
  const [layout, setLayout] = useState<FolderMirrorKind>('NESTED');
  const [tagFilters, setTagFilters] = useState<string[]>([]);
  const [tagDraft, setTagDraft] = useState('');
  const [pollIntervalSeconds, setPollIntervalSeconds] = useState(3600);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const steps: StepId[] = useMemo(
    () => provider === 'DATABRICKS' ? ['project', 'layout', 'tags', 'interval', 'review'] : ['project', 'layout', 'interval', 'review'],
    [provider],
  );

  if (!open) return null;

  const current: StepId = steps[Math.min(stepIndex, steps.length - 1)] ?? 'review';

  function isValid(): boolean {
    switch (current) {
      case 'project': return projectName.trim().length > 0;
      case 'layout': return layout === 'FLAT' || layout === 'NESTED';
      case 'tags': return true;
      case 'interval': return pollIntervalSeconds >= 60;
      case 'review': return true;
    }
  }

  function addTag() {
    const t = tagDraft.trim();
    if (!t) return;
    setTagFilters((prev) => prev.includes(t) ? prev : [...prev, t]);
    setTagDraft('');
  }

  async function submit() {
    setBusy(true);
    setError('');
    const body: EnableAutoRegistrationRequest = {
      project_name: projectName.trim(),
      folder_mirror_kind: layout,
      table_tag_filters: provider === 'DATABRICKS' ? tagFilters : [],
      poll_interval_seconds: pollIntervalSeconds,
    };
    try {
      const link = await virtualTables.enableAutoRegistration(sourceRid, body);
      onEnabled?.(link);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to enable auto-registration');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div role="dialog" aria-modal="true" style={backdropStyle}>
      <div style={modalStyle}>
        <header>
          <h3 style={{ margin: '0 0 4px' }}>Enable auto-registration</h3>
          <p style={{ ...mutedStyle, fontSize: 12, margin: 0 }}>
            Step {stepIndex + 1} of {steps.length} · <span style={{ textTransform: 'capitalize' }}>{current.replace('-', ' ')}</span>
          </p>
          <ol style={stepperStyle}>
            {steps.map((id, i) => (
              <li key={id} style={i === stepIndex ? stepActiveStyle : i < stepIndex ? stepDoneStyle : stepIdleStyle}>{id}</li>
            ))}
          </ol>
        </header>

        {current === 'project' && (
          <section>
            <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <span>Foundry-managed project name</span>
              <input type="text" value={projectName} onChange={(e) => setProjectName(e.target.value)} placeholder="warehouse mirror" style={inputStyle} />
            </label>
            <p style={hintStyle}>A new project will be created and managed by <code>virtual-table-service</code>. Users cannot create or modify resources in it directly.</p>
          </section>
        )}

        {current === 'layout' && (
          <section style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <label style={radioCardStyle}>
              <input type="radio" name="layout" checked={layout === 'NESTED'} onChange={() => setLayout('NESTED')} style={{ marginTop: 4 }} />
              <div>
                <strong>Nested</strong>
                <p style={hintStyle}><code>{'<project>/<database>/<schema>/<table>'}</code></p>
                <pre style={previewStyle}>{`main\n└── public\n    └── orders`}</pre>
              </div>
            </label>
            <label style={radioCardStyle}>
              <input type="radio" name="layout" checked={layout === 'FLAT'} onChange={() => setLayout('FLAT')} style={{ marginTop: 4 }} />
              <div>
                <strong>Flat</strong>
                <p style={hintStyle}><code>{'<project>/<database>__<schema>__<table>'}</code></p>
                <pre style={previewStyle}>main__public__orders</pre>
              </div>
            </label>
          </section>
        )}

        {current === 'tags' && (
          <section>
            <p style={hintStyle}>Only Databricks tables tagged with at least one of these tags will be auto-registered. Reads <code>INFORMATION_SCHEMA.TABLE_TAGS</code> on the workspace metastore.</p>
            <div style={{ display: 'flex', gap: 8 }}>
              <input type="text" placeholder="gold, pii, certified…" value={tagDraft} onChange={(e) => setTagDraft(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addTag(); } }} style={{ ...inputStyle, flex: 1 }} />
              <button type="button" onClick={addTag} style={btnStyle}>Add</button>
            </div>
            <ul style={{ display: 'flex', flexWrap: 'wrap', gap: 4, listStyle: 'none', padding: 0, margin: '8px 0 0' }}>
              {tagFilters.map((tag) => (
                <li key={tag} style={chipStyle}>
                  {tag}
                  <button type="button" onClick={() => setTagFilters((prev) => prev.filter((t) => t !== tag))} aria-label={`Remove ${tag}`} style={{ border: 'none', background: 'transparent', cursor: 'pointer', color: '#b91c1c', fontSize: 12 }}>✕</button>
                </li>
              ))}
            </ul>
          </section>
        )}

        {current === 'interval' && (
          <section>
            <p style={hintStyle}>How often the scanner re-runs against the source. Per Foundry doc the recommendation is 1 hour — too short costs compute, too long delays new-table discovery.</p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              {POLL_OPTIONS.map((opt) => (
                <label key={opt.value} style={radioCardStyle}>
                  <input type="radio" name="poll" checked={pollIntervalSeconds === opt.value} onChange={() => setPollIntervalSeconds(opt.value)} />
                  {opt.label}
                </label>
              ))}
              <label style={{ ...radioCardStyle, flexDirection: 'row', alignItems: 'center' }}>
                <input type="number" min={60} value={pollIntervalSeconds} onChange={(e) => setPollIntervalSeconds(Number(e.target.value))} style={{ width: 128, padding: '4px 8px', border: '1px solid #d1d5db', borderRadius: 4 }} />
                <span>seconds</span>
              </label>
            </div>
          </section>
        )}

        {current === 'review' && (
          <section>
            <dl style={{ display: 'grid', gridTemplateColumns: 'max-content 1fr', gap: '4px 12px', margin: 0, fontSize: 14 }}>
              <dt style={dtStyle}>Project</dt><dd style={{ margin: 0 }}>{projectName}</dd>
              <dt style={dtStyle}>Layout</dt><dd style={{ margin: 0 }}>{layout}</dd>
              {provider === 'DATABRICKS' && (
                <>
                  <dt style={dtStyle}>Tag filters</dt>
                  <dd style={{ margin: 0 }}>{tagFilters.length === 0 ? <span style={mutedStyle}>none — every accessible table is mirrored</span> : tagFilters.join(', ')}</dd>
                </>
              )}
              <dt style={dtStyle}>Poll interval</dt><dd style={{ margin: 0 }}>{pollIntervalSeconds} seconds</dd>
            </dl>
          </section>
        )}

        {error && <div role="alert" style={errorStyle}>{error}</div>}

        <footer style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button type="button" onClick={onClose} disabled={busy} style={btnStyle}>Cancel</button>
          {stepIndex > 0 && <button type="button" onClick={() => setStepIndex((i) => i - 1)} disabled={busy} style={btnStyle}>Back</button>}
          {stepIndex < steps.length - 1 ? (
            <button type="button" onClick={() => setStepIndex((i) => i + 1)} disabled={!isValid() || busy} style={primaryBtnStyle}>Next</button>
          ) : (
            <button type="button" onClick={() => void submit()} disabled={busy} style={primaryBtnStyle}>{busy ? 'Enabling…' : 'Enable'}</button>
          )}
        </footer>
      </div>
    </div>
  );
}

const backdropStyle: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100 };
const modalStyle: React.CSSProperties = { background: '#fff', borderRadius: 8, padding: 20, width: 'min(620px, 92vw)', maxHeight: '92vh', overflow: 'auto', display: 'flex', flexDirection: 'column', gap: 12 };
const stepperStyle: React.CSSProperties = { display: 'flex', gap: 8, listStyle: 'none', padding: 0, margin: '8px 0 0', fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.05em' };
const stepIdleStyle: React.CSSProperties = { padding: '2px 8px', borderRadius: 4, background: '#f3f4f6', color: '#6b7280' };
const stepActiveStyle: React.CSSProperties = { padding: '2px 8px', borderRadius: 4, background: '#1d4ed8', color: '#fff' };
const stepDoneStyle: React.CSSProperties = { padding: '2px 8px', borderRadius: 4, background: '#d1fae5', color: '#065f46' };
const hintStyle: React.CSSProperties = { margin: '4px 0 0', color: '#4b5563', fontSize: 12 };
const radioCardStyle: React.CSSProperties = { display: 'flex', gap: 8, alignItems: 'flex-start', padding: 8, border: '1px solid #e5e7eb', borderRadius: 8, cursor: 'pointer', fontSize: 14, flex: '0 0 auto' };
const previewStyle: React.CSSProperties = { margin: '4px 0 0', padding: 8, background: '#f9fafb', borderRadius: 4, fontFamily: 'ui-monospace, SFMono-Regular, monospace', fontSize: 12, whiteSpace: 'pre-wrap' };
const inputStyle: React.CSSProperties = { padding: '6px 8px', border: '1px solid #d1d5db', borderRadius: 4, fontSize: 14 };
const btnStyle: React.CSSProperties = { padding: '6px 12px', border: '1px solid #d1d5db', borderRadius: 4, background: '#fff', cursor: 'pointer', fontSize: 14 };
const primaryBtnStyle: React.CSSProperties = { ...btnStyle, background: '#1d4ed8', color: '#fff', borderColor: '#1d4ed8' };
const chipStyle: React.CSSProperties = { display: 'inline-flex', alignItems: 'center', gap: 4, padding: '1px 6px', background: '#f3f4f6', border: '1px solid #e5e7eb', borderRadius: 4, fontSize: 12 };
const errorStyle: React.CSSProperties = { background: '#fef2f2', color: '#b91c1c', border: '1px solid #fecaca', borderRadius: 4, padding: '8px 12px', fontSize: 14 };
const mutedStyle: React.CSSProperties = { color: '#6b7280' };
const dtStyle: React.CSSProperties = { color: '#6b7280', fontSize: 12, textTransform: 'uppercase', letterSpacing: '0.05em', alignSelf: 'center' };
