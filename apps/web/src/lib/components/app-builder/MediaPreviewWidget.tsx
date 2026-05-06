import { useEffect, useMemo, useState } from 'react';

import type { AppWidget } from '@/lib/api/apps';
import { getItem, type MediaItem, type MediaSetSchema } from '@/lib/api/mediaSets';
import { MediaPreview } from '@/lib/components/data/MediaPreview';

interface Props {
  widget: AppWidget;
  runtimeParameters?: Record<string, string>;
}

interface ResolvedReference {
  mediaItemRid: string;
  branch?: string | null;
  schema?: MediaSetSchema | null;
}

function interpolate(template: string, params: Record<string, string>) {
  return template.replace(/\{\{\s*([\w.-]+)\s*\}\}/g, (_, key: string) => params[key] ?? '');
}

export function MediaPreviewWidget({ widget, runtimeParameters = {} }: Props) {
  const readString = (key: string, fallback = '') => {
    const raw = widget.props[key];
    return typeof raw === 'string' ? interpolate(raw, runtimeParameters) : fallback;
  };

  const mediaString = useMemo(() => readString('media_string'), [widget, runtimeParameters]);
  const attachmentRid = useMemo(() => readString('attachment_property'), [widget, runtimeParameters]);
  const mediaReferenceRaw = useMemo(() => readString('media_reference_property'), [widget, runtimeParameters]);

  const reference = useMemo<ResolvedReference | null>(() => {
    const raw = mediaReferenceRaw.trim();
    if (!raw) return null;
    if (raw.startsWith('ri.foundry.main.media_item.')) return { mediaItemRid: raw, branch: null, schema: null };
    try {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object' && typeof parsed.mediaItemRid === 'string') {
        return {
          mediaItemRid: parsed.mediaItemRid as string,
          branch: typeof parsed.branch === 'string' ? parsed.branch : null,
          schema: typeof parsed.schema === 'string' ? (parsed.schema as MediaSetSchema) : null,
        };
      }
    } catch { /* not JSON */ }
    return null;
  }, [mediaReferenceRaw]);

  const [item, setItem] = useState<MediaItem | null>(null);
  const [loadingItem, setLoadingItem] = useState(false);
  const [itemError, setItemError] = useState('');

  useEffect(() => {
    if (!reference) {
      setItem(null);
      setItemError('');
      return;
    }
    let aborted = false;
    setLoadingItem(true);
    setItemError('');
    void getItem(reference.mediaItemRid)
      .then((next) => { if (!aborted) setItem(next); })
      .catch((cause) => { if (!aborted) setItemError(cause instanceof Error ? cause.message : 'Failed to resolve media item'); })
      .finally(() => { if (!aborted) setLoadingItem(false); });
    return () => { aborted = true; };
  }, [reference]);

  return (
    <div data-widget-id={widget.id} style={{ display: 'flex', flexDirection: 'column', gap: 10, height: '100%', padding: 12, borderRadius: 12, border: '1px solid #e2e8f0', background: '#fff', color: '#0f172a' }}>
      <header style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        <strong style={{ fontSize: 14 }}>{widget.title || 'Media preview'}</strong>
        {widget.description && <p style={{ margin: 0, fontSize: 12, color: '#64748b' }}>{widget.description}</p>}
      </header>

      {item ? (
        <div style={{ flex: 1, minHeight: 240 }}>
          <MediaPreview item={item} />
        </div>
      ) : loadingItem ? (
        <Placeholder>Loading media item…</Placeholder>
      ) : itemError ? (
        <Placeholder error>{itemError}</Placeholder>
      ) : attachmentRid ? (
        <img src={`/api/v1/ontology/actions/uploads/${encodeURIComponent(attachmentRid)}`} alt={`Attachment ${attachmentRid}`} style={rawImage} />
      ) : mediaString ? (
        /^data:/.test(mediaString) || /^https?:\/\//.test(mediaString) ? (
          <img src={mediaString} alt="Media preview" style={rawImage} />
        ) : (
          <Placeholder>Unsupported media string format. Use a media URL, data URL or media item RID.</Placeholder>
        )
      ) : (
        <Placeholder>Configure a media string, attachment, or media reference property to show a preview.</Placeholder>
      )}
    </div>
  );
}

const rawImage: React.CSSProperties = { maxWidth: '100%', maxHeight: '100%', borderRadius: 8, objectFit: 'contain' };

function Placeholder({ children, error = false }: { children: React.ReactNode; error?: boolean }) {
  return (
    <div style={{
      flex: 1,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      border: `1px dashed ${error ? '#fecaca' : '#cbd5e1'}`,
      borderRadius: 8,
      color: error ? '#b91c1c' : '#64748b',
      fontSize: 13,
      padding: 16,
      textAlign: 'center',
      background: error ? '#fef2f2' : 'transparent',
    }}>{children}</div>
  );
}
