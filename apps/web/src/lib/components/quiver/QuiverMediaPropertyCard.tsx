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
  objectLabel: string;
  propertyName: string;
  value: MediaReferenceJson | null | undefined;
}

export function QuiverMediaPropertyCard({ objectLabel, propertyName, value }: Props) {
  const itemRid = useMemo(() => value?.mediaItemRid ?? value?.media_item_rid ?? null, [value]);
  const setRid = useMemo(() => value?.mediaSetRid ?? value?.media_set_rid ?? null, [value]);

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
    <article
      data-set-rid={setRid ?? ''}
      data-item-rid={itemRid ?? ''}
      className="rounded-2xl border border-slate-200 bg-white p-3 shadow-sm dark:border-gray-700 dark:bg-gray-900"
    >
      <header className="flex items-baseline justify-between gap-3">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold">{propertyName}</h3>
          <p className="truncate text-xs text-slate-500">{objectLabel}</p>
        </div>
        {itemRid && (
          <span className="rounded-full bg-slate-100 px-2 py-0.5 font-mono text-[10px] text-slate-600 dark:bg-gray-800 dark:text-slate-300">
            {itemRid.split('.').pop()?.slice(0, 8) ?? itemRid}
          </span>
        )}
      </header>

      <div className="mt-3 min-h-[140px]">
        {!itemRid ? (
          <div className="flex h-32 items-center justify-center rounded-xl border border-dashed border-slate-300 text-xs text-slate-400 dark:border-gray-700">No media attached.</div>
        ) : loading ? (
          <div className="h-32 animate-pulse rounded-xl bg-slate-100 dark:bg-gray-800" />
        ) : error ? (
          <div className="rounded-xl border border-rose-200 bg-rose-50 p-2 text-xs text-rose-700 dark:border-rose-900/40 dark:bg-rose-950/40 dark:text-rose-300">{error}</div>
        ) : item ? (
          <MediaPreview item={item} onError={(msg) => setError(msg)} />
        ) : null}
      </div>
    </article>
  );
}
