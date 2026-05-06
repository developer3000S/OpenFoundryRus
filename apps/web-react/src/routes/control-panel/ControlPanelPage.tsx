import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { listSsoProviders, type SsoProviderRecord } from '@/lib/api/auth';
import {
  getControlPanel,
  getUpgradeReadiness,
  updateControlPanel,
  type ControlPanelSettings,
  type UpgradeReadinessResponse,
} from '@/lib/api/control-panel';

export function ControlPanelPage() {
  const [settings, setSettings] = useState<ControlPanelSettings | null>(null);
  const [readiness, setReadiness] = useState<UpgradeReadinessResponse | null>(null);
  const [ssoProviders, setSsoProviders] = useState<SsoProviderRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [draft, setDraft] = useState('');

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const [s, u, sso] = await Promise.all([
        getControlPanel(),
        getUpgradeReadiness(),
        listSsoProviders().catch(() => [] as SsoProviderRecord[]),
      ]);
      setSettings(s);
      setReadiness(u);
      setSsoProviders(sso);
      setDraft(JSON.stringify(s, null, 2));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function save() {
    setBusy(true);
    setError('');
    try {
      const parsed = JSON.parse(draft);
      const next = await updateControlPanel(parsed);
      setSettings(next);
      setDraft(JSON.stringify(next, null, 2));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Control panel</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Platform configuration, upgrade readiness, SSO providers, plus links to the streaming profiles and data
          health surfaces.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {loading && <p className="of-text-muted">Loading…</p>}

      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <Link to="/control-panel/streaming-profiles" className="of-button">Streaming profiles →</Link>
        <Link to="/control-panel/data-health" className="of-button">Data health →</Link>
      </div>

      {settings && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Settings (JSON edit)</p>
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            className="of-input"
            style={{ marginTop: 8, fontFamily: 'var(--font-mono)', fontSize: 11, minHeight: 320 }}
          />
          <button type="button" onClick={() => void save()} disabled={busy} className="of-button of-button--primary" style={{ marginTop: 8 }}>
            Save
          </button>
        </section>
      )}

      {readiness && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Upgrade readiness</p>
          <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 320 }}>
            {JSON.stringify(readiness, null, 2)}
          </pre>
        </section>
      )}

      {ssoProviders.length > 0 && (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">SSO providers ({ssoProviders.length})</p>
          <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 13 }}>
            {ssoProviders.map((p) => (
              <li key={p.id}>
                <strong>{p.name}</strong> · {p.provider_type} · {p.enabled ? 'enabled' : 'disabled'}
              </li>
            ))}
          </ul>
        </section>
      )}
    </section>
  );
}
