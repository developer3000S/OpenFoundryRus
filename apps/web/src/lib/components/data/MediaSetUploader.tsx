import { useRef, useState } from 'react';

import { uploadItem, type MediaItem, type MediaSet } from '@/lib/api/mediaSets';
import { notifications } from '@/lib/stores/notifications';

interface Props {
  mediaSet: MediaSet;
  onUploaded?: (item: MediaItem) => void;
}

type UploadStatus = 'queued' | 'uploading' | 'done' | 'rejected' | 'error';

interface UploadRow {
  id: string;
  name: string;
  size: number;
  mime: string;
  status: UploadStatus;
  progress: number;
  detail?: string;
}

function makeId() {
  return crypto.randomUUID?.() ?? `up-${Math.random().toString(36).slice(2)}`;
}

function formatBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function statusClass(row: UploadRow) {
  switch (row.status) {
    case 'done': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300';
    case 'rejected':
    case 'error': return 'bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300';
    case 'uploading': return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300';
    default: return 'bg-slate-100 text-slate-700 dark:bg-gray-800 dark:text-slate-300';
  }
}

function statusLabel(row: UploadRow) {
  switch (row.status) {
    case 'queued': return 'Queued';
    case 'uploading': return `Uploading… ${Math.round(row.progress * 100)}%`;
    case 'done': return 'Uploaded';
    case 'rejected': return 'Rejected';
    case 'error': return 'Failed';
  }
}

export function MediaSetUploader({ mediaSet, onUploaded }: Props) {
  const [dragging, setDragging] = useState(false);
  const [rows, setRows] = useState<UploadRow[]>([]);
  const inputRef = useRef<HTMLInputElement | null>(null);

  function isMimeAllowed(file: File): boolean {
    if (mediaSet.allowed_mime_types.length === 0) return true;
    const mime = file.type || 'application/octet-stream';
    return mediaSet.allowed_mime_types.some((allowed) => allowed.toLowerCase() === mime.toLowerCase());
  }

  function patchRow(id: string, patch: Partial<UploadRow>) {
    setRows((prev) => prev.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  }

  async function uploadOne(row: UploadRow, file: File) {
    patchRow(row.id, { status: 'uploading', progress: 0 });
    try {
      const { item } = await uploadItem(mediaSet.rid, file, {
        branch: 'main',
        onProgress: (fraction) => patchRow(row.id, { progress: fraction }),
      });
      patchRow(row.id, { status: 'done', progress: 1 });
      if (item.deduplicated_from) {
        notifications.warning(`Path overwrites existing item: ${item.path}`);
      } else {
        notifications.success(`Uploaded ${item.path}`);
      }
      onUploaded?.(item);
    } catch (cause) {
      patchRow(row.id, { status: 'error', detail: cause instanceof Error ? cause.message : String(cause) });
      notifications.error(`Failed to upload ${file.name}`);
    }
  }

  async function ingest(files: FileList | File[]) {
    const list = Array.from(files);
    for (const file of list) {
      const row: UploadRow = {
        id: makeId(),
        name: file.name,
        size: file.size,
        mime: file.type || 'application/octet-stream',
        status: 'queued',
        progress: 0,
      };
      setRows((prev) => [...prev, row]);
      if (!isMimeAllowed(file)) {
        patchRow(row.id, { status: 'rejected', detail: `MIME ${file.type || 'unknown'} not allowed by ${mediaSet.schema} schema` });
        notifications.error(`${file.name}: MIME type not allowed`);
        continue;
      }
      await uploadOne(row, file);
    }
  }

  return (
    <div className="space-y-4">
      <div
        role="button"
        tabIndex={0}
        aria-label="Upload media files"
        className={`rounded-2xl border-2 border-dashed p-8 text-center transition-colors ${dragging ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20' : 'border-slate-300 dark:border-gray-700'}`}
        onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
        onDragLeave={(e) => { e.preventDefault(); setDragging(false); }}
        onDrop={(e) => { e.preventDefault(); setDragging(false); if (e.dataTransfer) void ingest(e.dataTransfer.files); }}
        onClick={() => inputRef.current?.click()}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') inputRef.current?.click(); }}
      >
        <p className="text-sm font-semibold text-slate-700 dark:text-slate-200">Drop files here to upload</p>
        <p className="mt-1 text-xs text-slate-500">
          Or
          <button type="button" className="underline hover:text-blue-600" onClick={(e) => { e.stopPropagation(); inputRef.current?.click(); }}>choose from your computer</button>
        </p>
        {mediaSet.allowed_mime_types.length > 0 && (
          <p className="mt-3 text-[11px] text-slate-400">Accepted MIME types: {mediaSet.allowed_mime_types.join(', ')}</p>
        )}
        <input
          ref={inputRef}
          type="file"
          multiple
          className="hidden"
          onChange={(e) => {
            if (e.target.files && e.target.files.length > 0) {
              void ingest(e.target.files);
              e.target.value = '';
            }
          }}
        />
      </div>

      {rows.length > 0 && (
        <ul className="space-y-2">
          {rows.map((row) => (
            <li key={row.id} className="rounded-xl border border-slate-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-900">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium text-slate-700 dark:text-slate-100">{row.name}</div>
                  <div className="text-[11px] text-slate-500">{row.mime} · {formatBytes(row.size)}</div>
                  {row.detail && <div className="mt-1 text-[11px] text-rose-600 dark:text-rose-300">{row.detail}</div>}
                </div>
                <span className={`rounded-full px-2.5 py-1 text-xs font-medium ${statusClass(row)}`}>{statusLabel(row)}</span>
              </div>
              {(row.status === 'uploading' || (row.status === 'done' && row.progress > 0)) && (
                <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-200 dark:bg-gray-700">
                  <div className="h-full bg-blue-500 transition-[width] duration-150 ease-linear" style={{ width: `${Math.round(row.progress * 100)}%` }} />
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
