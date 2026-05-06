import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { getControlPanel, type ControlPanelSettings } from '@/lib/api/control-panel';

export function StreamingProfilesPage() {
  const [settings, setSettings] = useState<ControlPanelSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  async function refresh() {
    setLoading(true);
    try {
      setSettings(await getControlPanel());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/control-panel" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Control panel</Link>
      <header>
        <h1 className="of-heading-xl">Streaming profiles</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Streaming profiles configuration lives inside the control-panel settings — edit via /control-panel JSON.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {loading ? (
        <p className="of-text-muted">Loading…</p>
      ) : settings ? (
        <section className="of-panel" style={{ padding: 16 }}>
          <pre style={{ padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 480 }}>
            {JSON.stringify(settings, null, 2)}
          </pre>
        </section>
      ) : null}
    </section>
  );
}
