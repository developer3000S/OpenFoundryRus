import { useEffect, useState } from 'react';

import { dataConnection, type MediaSetSyncDef } from '@/lib/api/data-connection';
import { DEFAULT_MIME_TYPES, MEDIA_SET_SCHEMAS, createMediaSet, type MediaSet, type MediaSetSchema } from '@/lib/api/mediaSets';

interface Props {
  sourceId: string;
  sourceName?: string;
  defaultProjectRid?: string;
  onSaved: (sync: MediaSetSyncDef) => void;
  onCancel: () => void;
}

type Step = 1 | 2 | 3 | 4 | 5;

export function MediaSetSyncWizard({ sourceId, sourceName = 'source', defaultProjectRid = 'ri.foundry.main.project.default', onSaved, onCancel }: Props) {
  const [step, setStep] = useState<Step>(1);
  const [kind, setKind] = useState<'MEDIA_SET_SYNC' | 'VIRTUAL_MEDIA_SET_SYNC'>('MEDIA_SET_SYNC');
  const [schema, setSchema] = useState<MediaSetSchema>('IMAGE');
  const [allowedMimeTypes, setAllowedMimeTypes] = useState<string[]>([...DEFAULT_MIME_TYPES.IMAGE]);
  const [mimePrefillFollowingSchema, setMimePrefillFollowingSchema] = useState(true);
  const [scheduleEnabled, setScheduleEnabled] = useState(false);
  const [scheduleCron, setScheduleCron] = useState('');
  const [subfolder, setSubfolder] = useState('');
  const [excludeAlreadySynced, setExcludeAlreadySynced] = useState(true);
  const [pathGlob, setPathGlob] = useState('');
  const [fileSizeLimitMb, setFileSizeLimitMb] = useState<number | null>(null);
  const [ignoreUnmatchedSchema, setIgnoreUnmatchedSchema] = useState(true);
  const [targetMediaSetRid, setTargetMediaSetRid] = useState('');
  const [createInline, setCreateInline] = useState(false);
  const [createInlineName, setCreateInlineName] = useState('');
  const [createInlineProjectRid, setCreateInlineProjectRid] = useState(defaultProjectRid);
  const [createInlineBusy, setCreateInlineBusy] = useState(false);
  const [createInlineError, setCreateInlineError] = useState('');
  const [lastCreatedSet, setLastCreatedSet] = useState<MediaSet | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');

  useEffect(() => {
    if (mimePrefillFollowingSchema) setAllowedMimeTypes([...DEFAULT_MIME_TYPES[schema]]);
  }, [schema, mimePrefillFollowingSchema]);

  function toggleMime(mime: string, checked: boolean) {
    setMimePrefillFollowingSchema(false);
    setAllowedMimeTypes((prev) => checked ? Array.from(new Set([...prev, mime])) : prev.filter((m) => m !== mime));
  }

  async function createInlineSet() {
    if (!createInlineName.trim() || !createInlineProjectRid.trim()) {
      setCreateInlineError('Name and project RID are required.');
      return;
    }
    setCreateInlineBusy(true);
    setCreateInlineError('');
    try {
      const created = await createMediaSet({
        name: createInlineName.trim(),
        project_rid: createInlineProjectRid.trim(),
        schema,
        allowed_mime_types: allowedMimeTypes,
        transaction_policy: 'TRANSACTIONLESS',
        retention_seconds: 0,
        virtual_: kind === 'VIRTUAL_MEDIA_SET_SYNC',
        source_rid: kind === 'VIRTUAL_MEDIA_SET_SYNC' ? sourceId : null,
      });
      setLastCreatedSet(created);
      setTargetMediaSetRid(created.rid);
      setCreateInline(false);
    } catch (cause) {
      setCreateInlineError(cause instanceof Error ? cause.message : 'Failed to create media set');
    } finally {
      setCreateInlineBusy(false);
    }
  }

  async function save() {
    if (!targetMediaSetRid.trim()) {
      setSaveError('Pick or create a target media set first.');
      return;
    }
    setSaving(true);
    setSaveError('');
    try {
      const persisted = await dataConnection.createMediaSetSync(sourceId, {
        kind,
        target_media_set_rid: targetMediaSetRid.trim(),
        subfolder: subfolder.trim(),
        filters: {
          exclude_already_synced: excludeAlreadySynced,
          path_glob: pathGlob.trim() || null,
          file_size_limit: fileSizeLimitMb && fileSizeLimitMb > 0 ? fileSizeLimitMb * 1024 * 1024 : null,
          ignore_unmatched_schema: ignoreUnmatchedSchema,
        },
        schedule_cron: scheduleEnabled && scheduleCron ? scheduleCron : null,
      });
      onSaved(persisted);
    } catch (cause) {
      setSaveError(cause instanceof Error ? cause.message : 'Failed to save sync');
    } finally {
      setSaving(false);
    }
  }

  const stepLabels: Record<Step, string> = { 1: 'Type', 2: 'File types', 3: 'Schedule', 4: 'Subfolder', 5: 'Filters' };

  return (
    <div role="dialog" aria-modal="true" style={wizardStyle}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 style={{ margin: 0, fontSize: 16 }}>Media set sync · {sourceName}</h2>
        <button type="button" onClick={onCancel} aria-label="Close wizard" style={linkStyle}>×</button>
      </header>

      <ol style={stepsStyle}>
        {([1, 2, 3, 4, 5] as Step[]).map((i) => (
          <li key={i} style={step === i ? stepActiveStyle : step > i ? stepDoneStyle : stepIdleStyle}>
            <span>{i}</span> {stepLabels[i]}
          </li>
        ))}
      </ol>

      <div>
        {step === 1 && (
          <fieldset style={fieldsetStyle}>
            <legend>How should bytes be handled?</legend>
            <label style={kind === 'MEDIA_SET_SYNC' ? optionSelectedStyle : optionStyle}>
              <input type="radio" name="kind" value="MEDIA_SET_SYNC" checked={kind === 'MEDIA_SET_SYNC'} onChange={() => setKind('MEDIA_SET_SYNC')} />
              <span><strong>Media set sync</strong> Copies files into Foundry storage. Items survive deletes in the source.</span>
            </label>
            <label style={kind === 'VIRTUAL_MEDIA_SET_SYNC' ? optionSelectedStyle : optionStyle}>
              <input type="radio" name="kind" value="VIRTUAL_MEDIA_SET_SYNC" checked={kind === 'VIRTUAL_MEDIA_SET_SYNC'} onChange={() => setKind('VIRTUAL_MEDIA_SET_SYNC')} />
              <span><strong>Virtual media set sync</strong> Only metadata is registered. Bytes stay in the source. Per Foundry "Virtual media sets" — no awareness of external deletions.</span>
            </label>
          </fieldset>
        )}

        {step === 2 && (
          <>
            <fieldset style={fieldsetStyle}>
              <legend>Schema</legend>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                {MEDIA_SET_SCHEMAS.map((s) => (
                  <button key={s} type="button" onClick={() => { setSchema(s); setMimePrefillFollowingSchema(true); }} style={schema === s ? pillSelectedStyle : pillStyle}>{s}</button>
                ))}
              </div>
            </fieldset>
            <fieldset style={fieldsetStyle}>
              <legend>Allowed MIME types</legend>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 4 }}>
                {DEFAULT_MIME_TYPES[schema].map((mime) => (
                  <label key={mime} style={checkboxStyle}>
                    <input type="checkbox" checked={allowedMimeTypes.includes(mime)} onChange={(e) => toggleMime(mime, e.target.checked)} />
                    <span>{mime}</span>
                  </label>
                ))}
              </div>
            </fieldset>
          </>
        )}

        {step === 3 && (
          <fieldset style={fieldsetStyle}>
            <legend>Schedule</legend>
            <label style={checkboxStyle}>
              <input type="checkbox" checked={scheduleEnabled} onChange={(e) => setScheduleEnabled(e.target.checked)} />
              <span>Enable scheduled sync</span>
            </label>
            {scheduleEnabled && (
              <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <span>Cron expression</span>
                <input type="text" value={scheduleCron} onChange={(e) => setScheduleCron(e.target.value)} placeholder="0 */6 * * *" style={inputStyle} />
              </label>
            )}
            <p style={hintStyle}>Leave disabled to run the sync only when an operator clicks <strong>Run</strong> on the source page.</p>
          </fieldset>
        )}

        {step === 4 && (
          <fieldset style={fieldsetStyle}>
            <legend>Subfolder inside the source</legend>
            <input type="text" value={subfolder} onChange={(e) => setSubfolder(e.target.value)} placeholder="Leave empty to sync from the bucket root" style={inputStyle} />
            <p style={hintStyle}>Use a slash-delimited path (e.g. <code>screenshots/2026/</code>). The configured subfolder is the prefix for every Path matches glob in step 5.</p>
          </fieldset>
        )}

        {step === 5 && (
          <>
            <fieldset style={fieldsetStyle}>
              <legend>Sync filters</legend>
              <label style={checkboxStyle}>
                <input type="checkbox" checked={excludeAlreadySynced} onChange={(e) => setExcludeAlreadySynced(e.target.checked)} />
                <span>Exclude files already synced</span>
              </label>
              <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <span>Path matches (glob)</span>
                <input type="text" placeholder="e.g. **/*.png" value={pathGlob} onChange={(e) => setPathGlob(e.target.value)} style={inputStyle} />
              </label>
              <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <span>File size limit (MB)</span>
                <input type="number" min={0} step={1} value={fileSizeLimitMb ?? ''} placeholder="No limit" onChange={(e) => { const v = Number(e.target.value); setFileSizeLimitMb(Number.isFinite(v) && v > 0 ? v : null); }} style={inputStyle} />
              </label>
              <label style={checkboxStyle}>
                <input type="checkbox" checked={ignoreUnmatchedSchema} onChange={(e) => setIgnoreUnmatchedSchema(e.target.checked)} />
                <span>Ignore items not matching schema (silently skip vs surface as error)</span>
              </label>
            </fieldset>

            <fieldset style={fieldsetStyle}>
              <legend>Target media set</legend>
              <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <span>Existing media set RID</span>
                <input type="text" value={targetMediaSetRid} onChange={(e) => setTargetMediaSetRid(e.target.value)} placeholder="ri.foundry.main.media_set.…" style={inputStyle} />
              </label>
              {!targetMediaSetRid.trim() && !createInline && (
                <button type="button" onClick={() => { setCreateInlineName(createInlineName || `${kind === 'VIRTUAL_MEDIA_SET_SYNC' ? 'Virtual ' : ''}${schema.toLowerCase()} sync target`); setCreateInline(true); }} style={linkStyle}>+ Create new media set</button>
              )}
              {createInline && (
                <div style={{ marginTop: 8, padding: 12, background: '#0f172a', border: '1px solid #1f2937', borderRadius: 6 }}>
                  <h4 style={{ margin: '0 0 8px' }}>Create media set</h4>
                  <label style={{ display: 'flex', flexDirection: 'column', gap: 4, marginBottom: 6 }}>
                    <span>Name</span>
                    <input type="text" value={createInlineName} onChange={(e) => setCreateInlineName(e.target.value)} style={inputStyle} />
                  </label>
                  <label style={{ display: 'flex', flexDirection: 'column', gap: 4, marginBottom: 6 }}>
                    <span>Project RID</span>
                    <input type="text" value={createInlineProjectRid} onChange={(e) => setCreateInlineProjectRid(e.target.value)} style={inputStyle} />
                  </label>
                  {createInlineError && <p style={errorStyle}>{createInlineError}</p>}
                  <div style={{ display: 'flex', gap: 8 }}>
                    <button type="button" onClick={() => void createInlineSet()} disabled={createInlineBusy} style={primaryBtnStyle}>{createInlineBusy ? 'Creating…' : 'Create'}</button>
                    <button type="button" onClick={() => setCreateInline(false)} style={linkStyle}>Cancel</button>
                  </div>
                  <p style={hintStyle}>The new media set is created with the schema and MIME types you picked in step 2. Virtual sync? It will be linked to this source automatically.</p>
                </div>
              )}
              {lastCreatedSet && (
                <p style={hintStyle}>Linked to freshly-created <a href={`/media-sets/${encodeURIComponent(lastCreatedSet.rid)}`}>{lastCreatedSet.name}</a>.</p>
              )}
            </fieldset>

            {saveError && <p role="alert" style={errorStyle}>{saveError}</p>}
          </>
        )}
      </div>

      <footer style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
        {step > 1 && <button type="button" onClick={() => setStep((s) => (s - 1) as Step)} style={secondaryBtnStyle}>Back</button>}
        <button type="button" onClick={onCancel} style={linkStyle}>Cancel</button>
        {step < 5 ? (
          <button type="button" onClick={() => setStep((s) => (s + 1) as Step)} style={primaryBtnStyle}>Next</button>
        ) : (
          <button type="button" onClick={() => void save()} disabled={saving || !targetMediaSetRid.trim()} style={primaryBtnStyle}>{saving ? 'Saving…' : 'Save media set sync'}</button>
        )}
      </footer>
    </div>
  );
}

const wizardStyle: React.CSSProperties = { display: 'flex', flexDirection: 'column', gap: 12, padding: 16, background: '#0b1220', border: '1px solid #1f2937', borderRadius: 12, color: '#e2e8f0', width: 'min(680px, 96vw)' };
const stepsStyle: React.CSSProperties = { display: 'flex', gap: 8, listStyle: 'none', margin: 0, padding: 0, flexWrap: 'wrap' };
const stepIdleStyle: React.CSSProperties = { display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: '#94a3b8', padding: '4px 10px', background: '#1f2937', borderRadius: 6 };
const stepActiveStyle: React.CSSProperties = { ...stepIdleStyle, background: '#1d4ed8', color: '#fff' };
const stepDoneStyle: React.CSSProperties = { ...stepIdleStyle, background: '#065f46', color: '#d1fae5' };
const fieldsetStyle: React.CSSProperties = { border: '1px solid #1f2937', borderRadius: 8, padding: 12, marginBottom: 8, background: 'transparent' };
const optionStyle: React.CSSProperties = { display: 'flex', gap: 8, alignItems: 'flex-start', padding: 8, border: '1px solid #1f2937', borderRadius: 6, cursor: 'pointer', marginBottom: 6 };
const optionSelectedStyle: React.CSSProperties = { ...optionStyle, borderColor: '#3b82f6', background: '#1e3a8a' };
const pillStyle: React.CSSProperties = { padding: '4px 10px', border: '1px solid #1f2937', borderRadius: 999, background: 'transparent', color: '#e2e8f0', cursor: 'pointer', fontSize: 12 };
const pillSelectedStyle: React.CSSProperties = { ...pillStyle, borderColor: '#3b82f6', background: '#1e3a8a' };
const checkboxStyle: React.CSSProperties = { display: 'flex', alignItems: 'center', gap: 6, fontSize: 13 };
const inputStyle: React.CSSProperties = { padding: '6px 8px', border: '1px solid #1f2937', borderRadius: 4, fontSize: 14, background: '#0f172a', color: '#e2e8f0' };
const linkStyle: React.CSSProperties = { background: 'transparent', border: 'none', color: '#60a5fa', cursor: 'pointer', fontSize: 14, padding: 0 };
const primaryBtnStyle: React.CSSProperties = { padding: '6px 12px', background: '#1d4ed8', color: '#fff', border: '1px solid #1d4ed8', borderRadius: 4, cursor: 'pointer', fontSize: 14 };
const secondaryBtnStyle: React.CSSProperties = { padding: '6px 12px', background: '#1f2937', color: '#e2e8f0', border: '1px solid #1f2937', borderRadius: 4, cursor: 'pointer', fontSize: 14 };
const hintStyle: React.CSSProperties = { color: '#94a3b8', fontSize: 12, margin: '4px 0 0' };
const errorStyle: React.CSSProperties = { color: '#fca5a5', fontSize: 13, margin: '4px 0' };
