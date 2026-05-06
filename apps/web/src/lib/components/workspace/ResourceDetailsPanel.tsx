import { useEffect, useState } from 'react';

import {
  createFavorite,
  deleteFavorite,
  listResourceShares,
  type ResourceKind,
  type ResourceShare,
} from '@/lib/api/workspace';
import { ShareDialog } from './ShareDialog';

export interface ResourceSummary {
  id: string;
  name: string;
  kind: ResourceKind;
  description?: string | null;
  owner_id?: string | null;
  location?: string | null;
  tags?: string[];
  created_at?: string | null;
  updated_at?: string | null;
}

interface ResourceDetailsPanelProps {
  open: boolean;
  resource: ResourceSummary | null;
  isFavorite?: boolean;
  onClose?: () => void;
  onFavoriteToggle?: (next: boolean) => void;
}

function fmtDate(v: string | null | undefined) {
  if (!v) return '—';
  try { return new Date(v).toLocaleString(); }
  catch { return v; }
}

export function ResourceDetailsPanel({ open, resource, isFavorite = false, onClose, onFavoriteToggle }: ResourceDetailsPanelProps) {
  const [shares, setShares] = useState<ResourceShare[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [shareOpen, setShareOpen] = useState(false);

  useEffect(() => {
    if (!open || !resource) { setShares([]); return; }
    setLoading(true);
    setError('');
    listResourceShares(resource.kind, resource.id)
      .then(setShares)
      .catch((cause: unknown) => setError(cause instanceof Error ? cause.message : 'Unable to load shares'))
      .finally(() => setLoading(false));
  }, [open, resource]);

  async function toggleFavorite() {
    if (!resource) return;
    try {
      if (isFavorite) {
        await deleteFavorite(resource.kind, resource.id);
        onFavoriteToggle?.(false);
      } else {
        await createFavorite({ resource_kind: resource.kind, resource_id: resource.id });
        onFavoriteToggle?.(true);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Unable to update favorites');
    }
  }

  if (!open || !resource) return null;

  return (
    <>
      <aside style={{ position: 'fixed', top: 0, right: 0, bottom: 0, width: 380, background: '#0f172a', color: '#e2e8f0', borderLeft: '1px solid #1e293b', zIndex: 80, overflow: 'auto', padding: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
          <div>
            <p className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.04em' }}>{resource.kind}</p>
            <h3 style={{ margin: '4px 0 0', fontSize: 16, fontWeight: 600 }}>{resource.name}</h3>
            {resource.description && <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>{resource.description}</p>}
          </div>
          <button type="button" onClick={onClose} style={{ background: 'transparent', border: 'none', color: '#94a3b8', cursor: 'pointer', fontSize: 16 }} aria-label="Close">✕</button>
        </header>

        <div style={{ display: 'flex', gap: 6 }}>
          <button type="button" onClick={() => void toggleFavorite()} className="of-button" style={{ fontSize: 11 }}>
            {isFavorite ? '★ Remove favorite' : '☆ Add to favorites'}
          </button>
          <button type="button" onClick={() => setShareOpen(true)} className="of-button" style={{ fontSize: 11 }}>Share…</button>
        </div>

        <dl style={{ display: 'grid', gap: 6, fontSize: 12, margin: 0 }}>
          <div>
            <dt style={{ fontSize: 10, textTransform: 'uppercase', color: '#94a3b8' }}>RID</dt>
            <dd style={{ margin: 0, fontFamily: 'var(--font-mono)', fontSize: 11, wordBreak: 'break-all' }}>{resource.id}</dd>
          </div>
          {resource.location && (
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: '#94a3b8' }}>Location</dt>
              <dd style={{ margin: 0 }}>{resource.location}</dd>
            </div>
          )}
          {resource.owner_id && (
            <div>
              <dt style={{ fontSize: 10, textTransform: 'uppercase', color: '#94a3b8' }}>Owner</dt>
              <dd style={{ margin: 0, fontFamily: 'var(--font-mono)' }}>{resource.owner_id}</dd>
            </div>
          )}
          <div>
            <dt style={{ fontSize: 10, textTransform: 'uppercase', color: '#94a3b8' }}>Created</dt>
            <dd style={{ margin: 0 }}>{fmtDate(resource.created_at)}</dd>
          </div>
          <div>
            <dt style={{ fontSize: 10, textTransform: 'uppercase', color: '#94a3b8' }}>Updated</dt>
            <dd style={{ margin: 0 }}>{fmtDate(resource.updated_at)}</dd>
          </div>
        </dl>

        {resource.tags && resource.tags.length > 0 && (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
            {resource.tags.map((t) => (
              <span key={t} style={{ background: '#1e3a8a', color: '#bfdbfe', padding: '2px 8px', borderRadius: 999, fontSize: 11 }}>{t}</span>
            ))}
          </div>
        )}

        <section>
          <p className="of-eyebrow" style={{ fontSize: 10 }}>Shares ({shares.length})</p>
          {loading && <p className="of-text-muted" style={{ fontSize: 11, fontStyle: 'italic' }}>Loading…</p>}
          {error && <p style={{ color: '#fca5a5', fontSize: 11 }}>{error}</p>}
          <ul style={{ paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 4, marginTop: 6 }}>
            {shares.map((s) => (
              <li key={s.id} style={{ background: '#1e293b', padding: '4px 8px', borderRadius: 4, fontSize: 11 }}>
                {s.shared_with_user_id || s.shared_with_group_id || '—'} · {s.access_level}
                {s.note && ` · ${s.note}`}
              </li>
            ))}
            {shares.length === 0 && !loading && <li className="of-text-muted" style={{ fontSize: 11, fontStyle: 'italic' }}>Not shared.</li>}
          </ul>
        </section>
      </aside>

      <ShareDialog
        open={shareOpen}
        resourceKind={resource.kind}
        resourceId={resource.id}
        resourceLabel={resource.name}
        onClose={() => setShareOpen(false)}
        onShared={() => {
          setShareOpen(false);
          // refresh shares
          listResourceShares(resource.kind, resource.id).then(setShares).catch(() => {});
        }}
      />
    </>
  );
}
