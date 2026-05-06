import { useState } from 'react';

import { patchSetMarkings, previewSetMarkings, type MarkingsPreviewResponse, type MediaSet } from '@/lib/api/mediaSets';
import { notifications } from '@/lib/stores/notifications';

interface Props {
  mediaSet: MediaSet;
  onClose: () => void;
  onSaved: (updated: MediaSet) => void;
}

const KNOWN_MARKINGS = ['public', 'confidential', 'pii', 'secret'];

export function EditMarkingsModal({ mediaSet, onClose, onSaved }: Props) {
  const [selected, setSelected] = useState<string[]>(mediaSet.markings.map((m) => m.toLowerCase()));
  const [preview, setPreview] = useState<MarkingsPreviewResponse | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [previewError, setPreviewError] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');

  function toggle(marking: string, checked: boolean) {
    setSelected((prev) => checked ? Array.from(new Set([...prev, marking])) : prev.filter((m) => m !== marking));
    setPreview(null);
  }

  async function runPreview() {
    setPreviewing(true);
    setPreviewError('');
    try {
      setPreview(await previewSetMarkings(mediaSet.rid, selected));
    } catch (cause) {
      setPreviewError(cause instanceof Error ? cause.message : 'Preview failed');
    } finally {
      setPreviewing(false);
    }
  }

  async function save() {
    setSaving(true);
    setSaveError('');
    try {
      const updated = await patchSetMarkings(mediaSet.rid, selected);
      notifications.success(`Markings updated for ${updated.name}`);
      onSaved(updated);
    } catch (cause) {
      setSaveError(cause instanceof Error ? cause.message : 'Failed to save markings');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 z-[110] flex items-start justify-center overflow-y-auto bg-black/60 p-6" role="presentation">
      <div role="dialog" aria-modal="true" aria-label="Edit markings" className="w-full max-w-lg space-y-4 rounded-2xl border border-slate-200 bg-white p-5 text-sm shadow-xl dark:border-gray-700 dark:bg-gray-900">
        <header className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">Edit markings</h2>
            <p className="mt-1 text-xs text-slate-500">Cedar enforces clearance: <code className="font-mono">user.clearances ⊇ resource.markings</code>. Removing a marking may grant access; adding one may take it away.</p>
          </div>
          <button type="button" className="rounded p-1 text-slate-500 hover:bg-slate-100 dark:hover:bg-gray-800" aria-label="Close" onClick={onClose}>×</button>
        </header>

        <fieldset className="space-y-1">
          <legend className="text-xs uppercase tracking-[0.18em] text-slate-400">Markings</legend>
          {KNOWN_MARKINGS.map((m) => (
            <label key={m} className="flex items-center gap-2">
              <input type="checkbox" checked={selected.includes(m)} onChange={(e) => toggle(m, e.target.checked)} />
              <span className="font-mono text-xs uppercase">{m}</span>
            </label>
          ))}
        </fieldset>

        <div className="flex flex-wrap items-center gap-2">
          <button type="button" className="rounded-xl border border-slate-300 px-3 py-1.5 text-xs hover:bg-slate-50 dark:border-gray-700 dark:hover:bg-gray-800" onClick={() => void runPreview()} disabled={previewing}>
            {previewing ? 'Previewing…' : 'Preview impact'}
          </button>
          <button type="button" className="rounded-xl bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-60" onClick={() => void save()} disabled={saving}>
            {saving ? 'Saving…' : 'Save markings'}
          </button>
        </div>

        {previewError && <p className="text-xs text-rose-600">{previewError}</p>}
        {saveError && <p className="text-xs text-rose-600">{saveError}</p>}

        {preview && (
          <div className="rounded-xl border border-slate-200 bg-slate-50 p-3 text-xs dark:border-gray-700 dark:bg-gray-800/40">
            <p className="font-medium">Dry-run preview</p>
            <ul className="mt-2 space-y-1">
              <li>Added: <span className="font-mono">{preview.added.join(', ') || '—'}</span></li>
              <li>Removed: <span className="font-mono">{preview.removed.join(', ') || '—'}</span></li>
              <li className={preview.users_losing_access > 0 ? 'font-semibold text-rose-600' : 'text-slate-500'}>
                {preview.users_losing_access} user{preview.users_losing_access === 1 ? '' : 's'} will lose access
              </li>
            </ul>
          </div>
        )}
      </div>
    </div>
  );
}
