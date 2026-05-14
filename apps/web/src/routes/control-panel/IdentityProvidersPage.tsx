// SG.3: SSO provider administration — Control Panel UI.
//
// Lets administrators register, edit, refresh and remove SAML/OIDC
// identity providers, manage per-provider email-domain routing, and
// run the public login-troubleshoot probe against any email.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { JsonEditor, parseJsonOr } from '@/lib/components/JsonEditor';
import {
  checkSsoProviderHealth,
  createSsoProvider,
  deleteSsoProvider,
  listSsoProviders,
  refreshSsoProviderMetadata,
  troubleshootSsoLogin,
  updateSsoProvider,
  type LoginTroubleshootResponse,
  type SsoProviderHealth,
  type SsoProviderRecord,
} from '@/lib/api/auth';

const PROVIDER_TYPES = ['oidc', 'saml'] as const;

type ProviderForm = {
  slug: string;
  name: string;
  provider_type: 'oidc' | 'saml';
  client_id: string;
  client_secret: string;
  issuer_url: string;
  saml_metadata_url: string;
  saml_entity_id: string;
  saml_sso_url: string;
  saml_certificate: string;
  scopes: string;
  domains: string;
  attribute_mapping: string;
};

const EMPTY_FORM: ProviderForm = {
  slug: '',
  name: '',
  provider_type: 'oidc',
  client_id: '',
  client_secret: '',
  issuer_url: '',
  saml_metadata_url: '',
  saml_entity_id: '',
  saml_sso_url: '',
  saml_certificate: '',
  scopes: 'openid, email, profile',
  domains: '',
  attribute_mapping: JSON.stringify(
    {
      subject: 'sub',
      email: 'email',
      name: 'name',
      groups: { claim: 'groups' },
    },
    null,
    2,
  ),
};

export function IdentityProvidersPage() {
  const [providers, setProviders] = useState<SsoProviderRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);
  const [form, setForm] = useState<ProviderForm>(EMPTY_FORM);
  const [healthCache, setHealthCache] = useState<Record<string, SsoProviderHealth>>({});

  const refresh = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setProviders(await listSsoProviders());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load providers');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function create() {
    if (!form.slug.trim() || !form.name.trim()) {
      setError('slug and name are required');
      return;
    }
    setCreating(true);
    try {
      const scopes = splitCSV(form.scopes);
      const domains = splitCSV(form.domains);
      const mapping = parseJsonOr<Record<string, unknown>>(form.attribute_mapping, {});
      await createSsoProvider({
        slug: form.slug.trim(),
        name: form.name.trim(),
        provider_type: form.provider_type,
        enabled: true,
        client_id: form.client_id || undefined,
        client_secret: form.client_secret || undefined,
        issuer_url: form.issuer_url || undefined,
        saml_metadata_url: form.saml_metadata_url || undefined,
        saml_entity_id: form.saml_entity_id || undefined,
        saml_sso_url: form.saml_sso_url || undefined,
        saml_certificate: form.saml_certificate || undefined,
        scopes: scopes.length > 0 ? scopes : undefined,
        domains: domains.length > 0 ? domains : undefined,
        attribute_mapping: Object.keys(mapping).length > 0 ? mapping : undefined,
      });
      setForm(EMPTY_FORM);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to create provider');
    } finally {
      setCreating(false);
    }
  }

  async function toggleEnabled(p: SsoProviderRecord) {
    try {
      await updateSsoProvider(p.id, { enabled: !p.enabled });
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to update provider');
    }
  }

  async function refreshMetadata(p: SsoProviderRecord) {
    try {
      await refreshSsoProviderMetadata(p.id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to refresh metadata');
    }
  }

  async function probeHealth(p: SsoProviderRecord) {
    try {
      const result = await checkSsoProviderHealth(p.id);
      setHealthCache((prev) => ({ ...prev, [p.id]: result }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to probe health');
    }
  }

  async function remove(p: SsoProviderRecord) {
    try {
      await deleteSsoProvider(p.id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to delete provider');
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/control-panel" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
        ← Control panel
      </Link>

      <header>
        <h1 className="of-heading-xl">Identity providers</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Manage SAML and OIDC providers, domain routing, claim mapping, and metadata refresh.
          See{' '}
          <a href="/docs/security-governance/security-overview" target="_blank" rel="noreferrer">
            security overview
          </a>{' '}
          and the{' '}
          <a href="/docs/security-governance/identity-and-access" target="_blank" rel="noreferrer">
            identity and access
          </a>{' '}
          guide before changing provider configuration.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <TroubleshootSection onError={setError} />

      <ProviderList
        providers={providers}
        loading={loading}
        healthCache={healthCache}
        onToggle={toggleEnabled}
        onRefresh={refreshMetadata}
        onProbe={probeHealth}
        onDelete={remove}
      />

      <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 12 }}>
        <h2 className="of-heading-lg" style={{ fontSize: 14, textTransform: 'uppercase', letterSpacing: '0.12em' }}>
          Register provider
        </h2>
        <CreateForm form={form} onChange={setForm} onSubmit={create} busy={creating} />
      </section>
    </section>
  );
}

function ProviderList({
  providers,
  loading,
  healthCache,
  onToggle,
  onRefresh,
  onProbe,
  onDelete,
}: {
  providers: SsoProviderRecord[];
  loading: boolean;
  healthCache: Record<string, SsoProviderHealth>;
  onToggle: (p: SsoProviderRecord) => void | Promise<void>;
  onRefresh: (p: SsoProviderRecord) => void | Promise<void>;
  onProbe: (p: SsoProviderRecord) => void | Promise<void>;
  onDelete: (p: SsoProviderRecord) => void | Promise<void>;
}) {
  if (loading) {
    return <p className="of-text-muted">Loading providers…</p>;
  }
  if (providers.length === 0) {
    return (
      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-text-muted">No providers registered yet.</p>
      </section>
    );
  }
  return (
    <section style={{ display: 'grid', gap: 12 }}>
      {providers.map((p) => {
        const health = healthCache[p.id];
        return (
          <article key={p.id} className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
            <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
              <div>
                <h3 className="of-heading-lg" style={{ fontSize: 16 }}>
                  {p.name}{' '}
                  <span className="of-text-muted" style={{ fontSize: 12 }}>
                    ({p.slug})
                  </span>
                </h3>
                <p className="of-text-muted" style={{ fontSize: 12 }}>
                  {p.provider_type.toUpperCase()} ·{' '}
                  {p.enabled ? 'enabled' : 'disabled'}
                  {p.client_secret_configured ? ' · secret configured' : ''}
                  {p.saml_certificate_configured ? ' · certificate configured' : ''}
                </p>
              </div>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                <button className="of-button of-button--ghost" onClick={() => void onToggle(p)}>
                  {p.enabled ? 'Disable' : 'Enable'}
                </button>
                {p.provider_type === 'saml' && (
                  <button className="of-button of-button--ghost" onClick={() => void onRefresh(p)}>
                    Refresh metadata
                  </button>
                )}
                <button className="of-button of-button--ghost" onClick={() => void onProbe(p)}>
                  Check health
                </button>
                <button className="of-button of-button--ghost" onClick={() => void onDelete(p)} style={{ color: '#b91c1c' }}>
                  Delete
                </button>
              </div>
            </header>
            <dl style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 12px', fontSize: 12 }}>
              {p.issuer_url && (
                <>
                  <dt className="of-text-muted">Issuer</dt>
                  <dd><code>{p.issuer_url}</code></dd>
                </>
              )}
              {p.saml_sso_url && (
                <>
                  <dt className="of-text-muted">SAML SSO URL</dt>
                  <dd><code>{p.saml_sso_url}</code></dd>
                </>
              )}
              {p.saml_entity_id && (
                <>
                  <dt className="of-text-muted">Entity ID</dt>
                  <dd><code>{p.saml_entity_id}</code></dd>
                </>
              )}
              {p.domains && p.domains.length > 0 && (
                <>
                  <dt className="of-text-muted">Domains</dt>
                  <dd>{p.domains.join(', ')}</dd>
                </>
              )}
              {p.metadata_last_refreshed_at && (
                <>
                  <dt className="of-text-muted">Metadata refreshed</dt>
                  <dd>{new Date(p.metadata_last_refreshed_at).toLocaleString()}</dd>
                </>
              )}
              {p.metadata_last_error && (
                <>
                  <dt className="of-text-muted">Last refresh error</dt>
                  <dd style={{ color: '#b91c1c' }}>{p.metadata_last_error}</dd>
                </>
              )}
              {p.certificate_expires_at && (
                <>
                  <dt className="of-text-muted">Cert expires</dt>
                  <dd>{new Date(p.certificate_expires_at).toLocaleString()}</dd>
                </>
              )}
            </dl>
            {health && (
              <pre style={{ padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
                {JSON.stringify(health, null, 2)}
              </pre>
            )}
          </article>
        );
      })}
    </section>
  );
}

function CreateForm({
  form,
  onChange,
  onSubmit,
  busy,
}: {
  form: ProviderForm;
  onChange: (next: ProviderForm) => void;
  onSubmit: () => void | Promise<void>;
  busy: boolean;
}) {
  const isOidc = form.provider_type === 'oidc';
  const isSaml = form.provider_type === 'saml';
  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
        <label style={{ fontSize: 12 }}>
          Slug
          <input
            className="of-input"
            value={form.slug}
            onChange={(e) => onChange({ ...form, slug: e.target.value })}
            style={{ marginTop: 4 }}
          />
        </label>
        <label style={{ fontSize: 12 }}>
          Display name
          <input
            className="of-input"
            value={form.name}
            onChange={(e) => onChange({ ...form, name: e.target.value })}
            style={{ marginTop: 4 }}
          />
        </label>
        <label style={{ fontSize: 12 }}>
          Provider type
          <select
            className="of-input"
            value={form.provider_type}
            onChange={(e) => onChange({ ...form, provider_type: e.target.value as 'oidc' | 'saml' })}
            style={{ marginTop: 4 }}
          >
            {PROVIDER_TYPES.map((t) => (
              <option key={t} value={t}>
                {t.toUpperCase()}
              </option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 12 }}>
          Email domains (comma-separated)
          <input
            className="of-input"
            value={form.domains}
            onChange={(e) => onChange({ ...form, domains: e.target.value })}
            placeholder="acme.example.com, partner.example.com"
            style={{ marginTop: 4 }}
          />
        </label>
      </div>

      {isOidc && (
        <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
          <label style={{ fontSize: 12 }}>
            Issuer URL
            <input
              className="of-input"
              value={form.issuer_url}
              onChange={(e) => onChange({ ...form, issuer_url: e.target.value })}
              placeholder="https://idp.example.com"
              style={{ marginTop: 4 }}
            />
          </label>
          <label style={{ fontSize: 12 }}>
            Client ID
            <input
              className="of-input"
              value={form.client_id}
              onChange={(e) => onChange({ ...form, client_id: e.target.value })}
              style={{ marginTop: 4 }}
            />
          </label>
          <label style={{ fontSize: 12 }}>
            Client secret
            <input
              className="of-input"
              type="password"
              value={form.client_secret}
              onChange={(e) => onChange({ ...form, client_secret: e.target.value })}
              style={{ marginTop: 4 }}
            />
          </label>
          <label style={{ fontSize: 12 }}>
            Scopes (comma-separated)
            <input
              className="of-input"
              value={form.scopes}
              onChange={(e) => onChange({ ...form, scopes: e.target.value })}
              style={{ marginTop: 4 }}
            />
          </label>
        </div>
      )}

      {isSaml && (
        <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
          <label style={{ fontSize: 12 }}>
            Metadata URL
            <input
              className="of-input"
              value={form.saml_metadata_url}
              onChange={(e) => onChange({ ...form, saml_metadata_url: e.target.value })}
              placeholder="https://idp.example.com/saml/metadata"
              style={{ marginTop: 4 }}
            />
          </label>
          <label style={{ fontSize: 12 }}>
            Entity ID
            <input
              className="of-input"
              value={form.saml_entity_id}
              onChange={(e) => onChange({ ...form, saml_entity_id: e.target.value })}
              style={{ marginTop: 4 }}
            />
          </label>
          <label style={{ fontSize: 12 }}>
            SAML SSO URL
            <input
              className="of-input"
              value={form.saml_sso_url}
              onChange={(e) => onChange({ ...form, saml_sso_url: e.target.value })}
              style={{ marginTop: 4 }}
            />
          </label>
          <label style={{ fontSize: 12 }}>
            Signing certificate (base64 / PEM)
            <textarea
              className="of-input"
              value={form.saml_certificate}
              onChange={(e) => onChange({ ...form, saml_certificate: e.target.value })}
              style={{ marginTop: 4, minHeight: 80, fontFamily: 'var(--font-mono)', fontSize: 11 }}
            />
          </label>
        </div>
      )}

      <label style={{ fontSize: 12 }}>
        Attribute mapping
        <JsonEditor
          value={form.attribute_mapping}
          onChange={(v) => onChange({ ...form, attribute_mapping: v })}
        />
      </label>

      <div>
        <button className="of-button of-button--primary" disabled={busy} onClick={() => void onSubmit()}>
          {busy ? 'Creating…' : 'Register provider'}
        </button>
      </div>
    </div>
  );
}

function TroubleshootSection({ onError }: { onError: (msg: string) => void }) {
  const [email, setEmail] = useState('');
  const [result, setResult] = useState<LoginTroubleshootResponse | null>(null);
  const [busy, setBusy] = useState(false);

  const severity = useMemo(() => result?.state ?? 'ok', [result]);

  async function probe() {
    if (!email.trim()) {
      return;
    }
    setBusy(true);
    try {
      setResult(await troubleshootSsoLogin(email.trim()));
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to troubleshoot');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 12 }}>
      <h2 className="of-heading-lg" style={{ fontSize: 14, textTransform: 'uppercase', letterSpacing: '0.12em' }}>
        Login troubleshoot
      </h2>
      <p className="of-text-muted" style={{ fontSize: 12 }}>
        Classify why a sign-in attempt might fail: unknown domain, disabled user, disabled provider, stale metadata, expired or expiring certificate, or unreachable issuer.
      </p>
      <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end', flexWrap: 'wrap' }}>
        <label style={{ fontSize: 12, flex: '1 1 280px' }}>
          Email
          <input
            className="of-input"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="user@example.com"
            style={{ marginTop: 4 }}
          />
        </label>
        <button className="of-button" onClick={() => void probe()} disabled={busy}>
          {busy ? 'Checking…' : 'Troubleshoot'}
        </button>
      </div>
      {result && (
        <div style={{ display: 'grid', gap: 8 }}>
          <p style={{ fontSize: 13 }}>
            State: <strong>{severity}</strong> · domain <code>{result.domain}</code> · matched{' '}
            {result.matched_providers.length} provider(s) · user exists:{' '}
            {result.user_exists ? 'yes' : 'no'}
            {result.user_disabled ? ' (disabled)' : ''}
          </p>
          {result.diagnostics.length > 0 && (
            <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 4, fontSize: 12 }}>
              {result.diagnostics.map((d, i) => (
                <li
                  key={`${d.code}-${i}`}
                  style={{
                    padding: '6px 10px',
                    borderRadius: 8,
                    background:
                      d.severity === 'error'
                        ? 'rgba(239, 68, 68, 0.1)'
                        : d.severity === 'warning'
                          ? 'rgba(245, 158, 11, 0.1)'
                          : 'rgba(99, 102, 241, 0.1)',
                  }}
                >
                  <strong>{d.severity.toUpperCase()}</strong>: {d.message}{' '}
                  <span className="of-text-muted">({d.code})</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  );
}

function splitCSV(value: string): string[] {
  return value
    .split(',')
    .map((v) => v.trim())
    .filter((v) => v.length > 0);
}
