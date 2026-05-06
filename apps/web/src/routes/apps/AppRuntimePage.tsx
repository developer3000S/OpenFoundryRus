import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';

import { AppRenderer } from '@/lib/components/apps/AppRenderer';
import { getPublishedApp, type AppDefinition } from '@/lib/api/apps';

export function AppRuntimePage() {
  const { slug = '' } = useParams<{ slug: string }>();
  const [app, setApp] = useState<AppDefinition | null>(null);
  const [versionNumber, setVersionNumber] = useState<number | null>(null);
  const [publishedAt, setPublishedAt] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!slug) return;
    setLoading(true);
    setError('');
    getPublishedApp(slug)
      .then((res) => {
        setApp(res.app);
        setVersionNumber(res.published_version_number);
        setPublishedAt(res.published_at);
      })
      .catch((cause: unknown) => setError(cause instanceof Error ? cause.message : 'Failed to load published app'))
      .finally(() => setLoading(false));
  }, [slug]);

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">Published App Runtime</h1>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
            End-user rendering surface. Apps slug: <code>{slug}</code>
          </p>
        </div>
        {versionNumber !== null && (
          <div style={{ display: 'flex', gap: 6 }}>
            <span style={{ padding: '4px 10px', borderRadius: 999, border: '1px solid var(--border-default)', fontSize: 12 }}>Version {versionNumber}</span>
            <span style={{ padding: '4px 10px', borderRadius: 999, border: '1px solid var(--border-default)', fontSize: 12 }}>Published {publishedAt}</span>
          </div>
        )}
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {loading ? (
        <p className="of-text-muted">Loading published app…</p>
      ) : app ? (
        <AppRenderer app={app} mode="published" />
      ) : null}
    </section>
  );
}
