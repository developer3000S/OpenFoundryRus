import { useEffect, useState } from 'react';

import { getDownloadUrl, type MediaItem } from '@/lib/api/mediaSets';

interface Props {
  item: MediaItem;
  largeImageThresholdPx?: number;
  onError?: (message: string) => void;
}

type Kind = 'image' | 'audio' | 'video' | 'pdf' | 'csv' | 'office' | 'other';

function dispatchKind(mime: string): Kind {
  if (mime.startsWith('image/')) return 'image';
  if (mime.startsWith('audio/')) return 'audio';
  if (mime.startsWith('video/')) return 'video';
  if (mime === 'application/pdf') return 'pdf';
  if (mime === 'text/csv') return 'csv';
  if (mime.startsWith('application/vnd.openxmlformats-officedocument.')) return 'office';
  return 'other';
}

function thumbnailAccessPatternUrl(rid: string): string {
  return `/api/v1/access-patterns/run?kind=thumbnail&item=${encodeURIComponent(rid)}`;
}

export function MediaPreview({ item, largeImageThresholdPx = 4096, onError }: Props) {
  const [url, setUrl] = useState<string | null>(null);
  const [loadingUrl, setLoadingUrl] = useState(false);
  const [urlError, setUrlError] = useState('');
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);
  const [imageNaturalWidth, setImageNaturalWidth] = useState(0);
  const [pdfPage, setPdfPage] = useState(1);

  const kind = dispatchKind(item.mime_type);

  useEffect(() => {
    let aborted = false;
    setLoadingUrl(true);
    setUrlError('');
    setUrl(null);
    setZoom(1);
    setRotation(0);
    setPdfPage(1);
    void getDownloadUrl(item.rid)
      .then((response) => {
        if (aborted) return;
        setUrl(response.url);
      })
      .catch((cause) => {
        if (aborted) return;
        const msg = cause instanceof Error ? cause.message : 'Failed to resolve download URL';
        setUrlError(msg);
        onError?.(msg);
      })
      .finally(() => {
        if (!aborted) setLoadingUrl(false);
      });
    return () => { aborted = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.rid]);

  const shouldPreferThumbnail = kind === 'image' && imageNaturalWidth > largeImageThresholdPx;

  if (loadingUrl) {
    return (
      <section className="flex h-full flex-col gap-3" data-kind={kind}>
        <div className="flex flex-1 items-center justify-center">
          <div className="h-32 w-32 animate-pulse rounded-2xl bg-slate-100 dark:bg-gray-800" />
        </div>
      </section>
    );
  }

  if (urlError) {
    return (
      <section className="flex h-full flex-col gap-3" data-kind={kind}>
        <div className="flex flex-1 items-center justify-center rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700 dark:border-rose-900/40 dark:bg-rose-950/40 dark:text-rose-300">
          {urlError}
        </div>
      </section>
    );
  }

  if (!url) {
    return (
      <section className="flex h-full flex-col gap-3" data-kind={kind}>
        <div className="flex flex-1 items-center justify-center text-sm text-slate-500">No preview URL</div>
      </section>
    );
  }

  return (
    <section className="flex h-full flex-col gap-3" data-kind={kind}>
      {kind === 'image' && (
        <>
          <div className="flex flex-1 items-center justify-center overflow-auto rounded-xl bg-slate-50 p-4 dark:bg-gray-800">
            <img
              src={shouldPreferThumbnail ? thumbnailAccessPatternUrl(item.rid) : url}
              alt={`Preview of ${item.path}`}
              className="max-h-full max-w-full select-none"
              style={{ transform: `scale(${zoom}) rotate(${rotation}deg)`, transition: 'transform 120ms ease-out' }}
              onLoad={(e) => setImageNaturalWidth((e.currentTarget as HTMLImageElement).naturalWidth)}
              onError={() => onError?.('image element failed to load')}
            />
          </div>
          <div className="flex items-center gap-3 text-xs text-slate-500">
            <label className="flex items-center gap-2">
              Zoom
              <input type="range" min={0.25} max={4} step={0.05} value={zoom} onChange={(e) => setZoom(Number(e.target.value))} aria-label="Zoom" />
              <span className="tabular-nums">{(zoom * 100).toFixed(0)}%</span>
            </label>
            <button type="button" className="rounded border border-slate-300 px-2 py-0.5 hover:bg-slate-100 dark:border-gray-700 dark:hover:bg-gray-800" onClick={() => setRotation((r) => (r + 90) % 360)} aria-label="Rotate 90°">
              Rotate {rotation}°
            </button>
            {shouldPreferThumbnail && (
              <span className="text-[11px] italic text-slate-400">Showing thumbnail (image &gt; {largeImageThresholdPx}px wide)</span>
            )}
          </div>
        </>
      )}

      {kind === 'pdf' && (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-slate-300 p-6 text-center dark:border-gray-700">
          <p className="text-sm text-slate-700 dark:text-slate-200">Inline PDF viewer ships with H4. Open the file in a new tab to read it now.</p>
          <a href={url} target="_blank" rel="noopener" className="rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700">Open externally</a>
          <div className="flex items-center gap-2 text-xs text-slate-500">
            <button type="button" className="rounded border border-slate-300 px-2 py-0.5 hover:bg-slate-100 dark:border-gray-700 dark:hover:bg-gray-800" onClick={() => setPdfPage((p) => Math.max(1, p - 1))}>Prev</button>
            <span>Page {pdfPage} / 1</span>
            <button type="button" className="rounded border border-slate-300 px-2 py-0.5 hover:bg-slate-100 dark:border-gray-700 dark:hover:bg-gray-800" onClick={() => setPdfPage((p) => p + 1)}>Next</button>
          </div>
        </div>
      )}

      {kind === 'audio' && (
        <div className="flex flex-1 flex-col items-stretch justify-center gap-3 rounded-xl bg-slate-50 p-4 dark:bg-gray-800">
          <audio controls src={url} className="w-full" onError={() => onError?.('audio element failed to load')} />
          <div className="h-16 w-full rounded bg-gradient-to-r from-sky-200 via-sky-400 to-sky-200 dark:from-sky-900 dark:via-sky-700 dark:to-sky-900" aria-label="Waveform placeholder" />
          <p className="text-[11px] italic text-slate-400">Waveform rendering activates once the `waveform` access pattern is registered.</p>
        </div>
      )}

      {kind === 'video' && (
        <div className="flex flex-1 items-center justify-center overflow-hidden rounded-xl bg-black">
          <video controls src={url} className="max-h-full max-w-full" onError={() => onError?.('video element failed to load')} />
        </div>
      )}

      {(kind === 'csv' || kind === 'office') && (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-slate-300 p-6 text-center dark:border-gray-700">
          <p className="text-sm text-slate-700 dark:text-slate-200">Preview not available for this file type. Download to view it locally.</p>
          <a href={url} download={item.path} className="rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700">Download</a>
        </div>
      )}

      {kind === 'other' && (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-slate-300 p-6 text-center dark:border-gray-700">
          <p className="text-sm text-slate-700 dark:text-slate-200">Inline preview is not supported for {item.mime_type || 'this MIME type'}.</p>
          <a href={url} download={item.path} className="rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700">Download</a>
        </div>
      )}
    </section>
  );
}
