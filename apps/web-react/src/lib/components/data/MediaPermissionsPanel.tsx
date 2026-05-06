import { useState } from 'react';

import type { MediaSet } from '@/lib/api/mediaSets';
import { MarkingBadge } from '@/lib/components/dataset/MarkingBadge';
import { ShareDialog } from '@/lib/components/workspace/ShareDialog';

import { EditMarkingsModal } from './EditMarkingsModal';

interface Props {
  mediaSet: MediaSet;
  onChanged: (next: MediaSet) => void;
}

function levelOf(name: string): 'public' | 'confidential' | 'pii' | 'restricted' | 'unknown' {
  const lower = name.toLowerCase();
  if (lower === 'public') return 'public';
  if (lower === 'confidential') return 'confidential';
  if (lower === 'pii') return 'pii';
  if (lower === 'secret' || lower === 'restricted') return 'restricted';
  return 'unknown';
}

export function MediaPermissionsPanel({ mediaSet, onChanged }: Props) {
  const [showEditMarkings, setShowEditMarkings] = useState(false);
  const [showShareDialog, setShowShareDialog] = useState(false);

  return (
    <section className="space-y-6">
      <div className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-900">
        <header className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">Markings</h2>
            <p className="mt-1 text-xs text-slate-500">
              Cedar enforces clearance against this set's markings. An item with no per-item override inherits the full set; granular per-item markings tighten access further.
            </p>
          </div>
          <button type="button" className="rounded-xl bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700" onClick={() => setShowEditMarkings(true)}>
            Edit markings
          </button>
        </header>

        <div className="mt-4 space-y-3 text-xs">
          <div>
            <div className="text-[11px] uppercase tracking-[0.18em] text-slate-400">Direct (this set)</div>
            <div className="mt-1 flex flex-wrap gap-2">
              {mediaSet.markings.length === 0 ? (
                <span className="text-slate-400 italic">No markings — anyone with project access</span>
              ) : (
                mediaSet.markings.map((m) => (
                  <MarkingBadge key={m} label={m.toUpperCase()} level={levelOf(m)} source={{ kind: 'direct' }} id={m} />
                ))
              )}
            </div>
          </div>

          <div>
            <div className="text-[11px] uppercase tracking-[0.18em] text-slate-400">Inherited (project + tenant)</div>
            <div className="mt-1 flex flex-wrap gap-2 text-slate-400 italic">
              Inheritance from the parent project lands when the project ontology is wired (H4). For now, only direct markings on this set are enforced.
            </div>
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-dashed border-slate-300 bg-slate-50 p-4 text-xs dark:border-gray-700 dark:bg-gray-800/40">
        <header className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold">User & group sharing</h2>
            <p className="mt-1 text-slate-500">
              Per-principal share lists are a separate axis from Cedar markings (markings gate clearance, shares grant tenant access). The <code className="font-mono">media_set</code> share kind goes live alongside the rest of the workspace sharing surface.
            </p>
          </div>
          <button type="button" className="rounded-xl border border-slate-300 px-3 py-1.5 text-xs hover:bg-slate-100 dark:border-gray-700 dark:hover:bg-gray-800" onClick={() => setShowShareDialog(true)}>
            Open share dialog
          </button>
        </header>
      </div>

      {showEditMarkings && (
        <EditMarkingsModal
          mediaSet={mediaSet}
          onClose={() => setShowEditMarkings(false)}
          onSaved={(updated) => { setShowEditMarkings(false); onChanged(updated); }}
        />
      )}

      <ShareDialog
        open={showShareDialog}
        resourceKind="other"
        resourceId={mediaSet.rid}
        resourceLabel={mediaSet.name}
        onClose={() => setShowShareDialog(false)}
      />
    </section>
  );
}
