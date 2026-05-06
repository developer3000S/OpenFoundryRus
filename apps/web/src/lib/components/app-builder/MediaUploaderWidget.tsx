import { useMemo, useRef, useState } from 'react';

import type { AppWidget, WidgetEvent } from '@/lib/api/apps';
import { uploadItem } from '@/lib/api/mediaSets';
import { notifications } from '@/lib/stores/notifications';

interface Props {
  widget: AppWidget;
  runtimeParameters?: Record<string, string>;
  onAction?: (event: WidgetEvent, payload?: Record<string, unknown>) => Promise<void> | void;
}

interface StagedFile {
  id: string;
  file: File;
  status: 'staged' | 'uploading' | 'done' | 'error';
  error?: string;
  itemRid?: string;
  mediaSetRid?: string;
}

function makeId() {
  return crypto.randomUUID?.() ?? `up-${Math.random().toString(36).slice(2)}`;
}

function interpolate(template: string, params: Record<string, string>) {
  return template.replace(/\{\{\s*([\w.-]+)\s*\}\}/g, (_, key: string) => params[key] ?? '');
}

function intentClass(intent: string) {
  switch (intent) {
    case 'success': return 'bg-emerald-600 hover:bg-emerald-700';
    case 'warning': return 'bg-amber-500 hover:bg-amber-600';
    case 'danger': return 'bg-rose-600 hover:bg-rose-700';
    case 'none': return 'bg-slate-500 hover:bg-slate-600';
    default: return 'bg-blue-600 hover:bg-blue-700';
  }
}

function statusLabel(entry: StagedFile) {
  switch (entry.status) {
    case 'staged': return 'Ready';
    case 'uploading': return 'Uploading…';
    case 'done': return 'Uploaded';
    case 'error': return entry.error ?? 'Error';
  }
}

export function MediaUploaderWidget({ widget, runtimeParameters = {}, onAction }: Props) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [staged, setStaged] = useState<StagedFile[]>([]);
  const [submitting, setSubmitting] = useState(false);

  const readString = (key: string, fallback = '') => {
    const raw = widget.props[key];
    return typeof raw === 'string' ? interpolate(raw, runtimeParameters) : fallback;
  };
  const readBool = (key: string, fallback = false) => {
    const raw = widget.props[key];
    return typeof raw === 'boolean' ? raw : fallback;
  };

  const buttonText = useMemo(() => readString('text', 'Upload media'), [widget, runtimeParameters]);
  const intent = useMemo(() => readString('intent', 'primary'), [widget, runtimeParameters]);
  const allowedExtensions = useMemo(() => readString('allowed_extensions', ''), [widget, runtimeParameters]);
  const allowMultiple = useMemo(() => readBool('multi_file', false), [widget]);
  const destinationRid = useMemo(() => readString('destination_rid'), [widget, runtimeParameters]);
  const branch = useMemo(() => readString('branch', 'main'), [widget, runtimeParameters]);

  function stageFiles(files: FileList | File[]) {
    const list = Array.from(files);
    const next: StagedFile[] = list.map((file) => ({ id: makeId(), file, status: 'staged' }));
    setStaged((prev) => allowMultiple ? [...prev, ...next] : next.slice(-1));
  }

  function removeStaged(id: string) {
    setStaged((prev) => prev.filter((s) => s.id !== id));
  }

  function patch(id: string, partial: Partial<StagedFile>) {
    setStaged((prev) => prev.map((s) => (s.id === id ? { ...s, ...partial } : s)));
  }

  async function submit() {
    if (staged.length === 0 || !destinationRid) {
      notifications.warning(destinationRid ? 'No files staged' : 'Configure the upload destination before submitting');
      return;
    }
    setSubmitting(true);
    const onUpload = widget.events.find((e) => e.trigger === 'on_upload');
    try {
      for (const entry of staged) {
        if (entry.status === 'done') continue;
        patch(entry.id, { status: 'uploading', error: undefined });
        try {
          const { item } = await uploadItem(destinationRid, entry.file, { branch });
          patch(entry.id, { status: 'done', itemRid: item.rid, mediaSetRid: item.media_set_rid });
          if (onUpload && onAction) {
            await onAction(onUpload, { file_identifier: item.rid, media_set_rid: item.media_set_rid, filename: entry.file.name });
          }
        } catch (cause) {
          patch(entry.id, { status: 'error', error: cause instanceof Error ? cause.message : 'Upload failed' });
        }
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div data-widget-id={widget.id} style={{ display: 'flex', flexDirection: 'column', gap: 10, padding: 12, borderRadius: 12, border: '1px solid #e2e8f0', background: '#fff', color: '#0f172a' }}>
      <header style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        <strong style={{ fontSize: 14 }}>{widget.title || 'Upload media'}</strong>
        {widget.description && <p style={{ margin: 0, fontSize: 12, color: '#64748b' }}>{widget.description}</p>}
      </header>

      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <button
          type="button"
          className={intentClass(intent)}
          style={{ color: '#fff', border: 'none', borderRadius: 8, padding: '8px 14px', cursor: 'pointer', fontWeight: 500, fontSize: 13 }}
          onClick={() => inputRef.current?.click()}
        >
          {buttonText}
        </button>
        <input
          ref={inputRef}
          type="file"
          multiple={allowMultiple}
          accept={allowedExtensions || undefined}
          style={{ display: 'none' }}
          onChange={(e) => {
            const files = e.target.files;
            if (files && files.length > 0) {
              stageFiles(files);
              e.target.value = '';
            }
          }}
        />
      </div>

      {staged.length > 0 ? (
        <>
          <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', gap: 4, border: '1px solid #e2e8f0', borderRadius: 8, overflow: 'hidden' }}>
            {staged.map((entry, idx) => {
              const rowBg = entry.status === 'error' ? '#fef2f2' : entry.status === 'done' ? '#ecfdf5' : '#f8fafc';
              const rowColor = entry.status === 'error' ? '#b91c1c' : entry.status === 'done' ? '#047857' : 'inherit';
              return (
                <li key={entry.id} style={{ display: 'grid', gridTemplateColumns: '1fr auto auto', gap: 8, alignItems: 'center', padding: '6px 10px', fontSize: 12, background: rowBg, color: rowColor, borderBottom: idx === staged.length - 1 ? 'none' : '1px solid #e2e8f0' }}>
                  <div style={{ display: 'flex', flexDirection: 'column', minWidth: 0 }}>
                    <span style={{ fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{entry.file.name}</span>
                    <span style={{ color: '#94a3b8', fontSize: 11 }}>{Math.ceil(entry.file.size / 1024)} KB</span>
                  </div>
                  <span style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.04em' }}>{statusLabel(entry)}</span>
                  {entry.status === 'staged' ? (
                    <button type="button" onClick={() => removeStaged(entry.id)} aria-label="Remove staged file" style={{ background: 'transparent', border: 'none', color: '#64748b', fontSize: 18, cursor: 'pointer', padding: '0 4px' }}>×</button>
                  ) : <span />}
                </li>
              );
            })}
          </ul>
          <button type="button" onClick={() => void submit()} disabled={submitting || !destinationRid} style={{ background: '#1d4ed8', color: '#fff', border: 'none', borderRadius: 8, padding: '8px 14px', cursor: 'pointer', fontWeight: 600, fontSize: 13, opacity: submitting || !destinationRid ? 0.6 : 1 }}>
            {submitting ? 'Uploading…' : 'Submit'}
          </button>
          {!destinationRid && (
            <p style={{ color: '#64748b', fontStyle: 'italic', fontSize: 11, margin: 0 }}>
              Set <code style={{ background: '#f1f5f9', padding: '0 4px', borderRadius: 3 }}>destination_rid</code> in the widget props before submitting.
            </p>
          )}
        </>
      ) : (
        <p style={{ color: '#64748b', fontStyle: 'italic', fontSize: 11, margin: 0 }}>
          Files are staged locally and only uploaded when you press <strong>Submit</strong> — cancelled forms leave no orphaned items behind (per Foundry contract).
        </p>
      )}
    </div>
  );
}
