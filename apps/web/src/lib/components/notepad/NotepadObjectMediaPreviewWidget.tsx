import { useEffect, useMemo, useState } from 'react';

import { getItem, type MediaItem } from '@/lib/api/mediaSets';
import { MediaPreview } from '@/lib/components/data/MediaPreview';

interface MediaReferenceJson {
  mediaSetRid?: string;
  mediaItemRid?: string;
  media_set_rid?: string;
  media_item_rid?: string;
  branch?: string;
  schema?: string;
}

interface Props {
  title?: string;
  value: MediaReferenceJson | null | undefined;
  autoExpandOnPrint?: boolean;
}

export function NotepadObjectMediaPreviewWidget({ title = 'Object media preview', value, autoExpandOnPrint = false }: Props) {
  const itemRid = useMemo(() => value?.mediaItemRid ?? value?.media_item_rid ?? null, [value]);

  const [item, setItem] = useState<MediaItem | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!itemRid) {
      setItem(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError('');
    void getItem(itemRid)
      .then((row) => { if (!cancelled) setItem(row); })
      .catch((cause) => { if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load media item'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [itemRid]);

  return (
    <section
      className={`rounded-2xl border border-slate-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-900 ${autoExpandOnPrint ? 'print:!max-h-none print:!overflow-visible' : ''}`}
    >
      <header className="mb-2 flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold">{title}</h3>
        {itemRid && (
          <span className="rounded-full bg-slate-100 px-2 py-0.5 font-mono text-[10px] text-slate-600 dark:bg-gray-800 dark:text-slate-300">{itemRid}</span>
        )}
      </header>
      {!itemRid ? (
        <div className="flex h-40 items-center justify-center rounded-xl border border-dashed border-slate-300 text-sm text-slate-400 dark:border-gray-700">No media attached to this property.</div>
      ) : loading ? (
        <div className="h-40 animate-pulse rounded-xl bg-slate-100 dark:bg-gray-800" />
      ) : error ? (
        <div className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700 dark:border-rose-900/40 dark:bg-rose-950/40 dark:text-rose-300">{error}</div>
      ) : item ? (
        <MediaPreview item={item} onError={(msg) => setError(msg)} />
      ) : null}
    </section>
  );
}
