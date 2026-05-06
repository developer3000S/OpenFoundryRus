import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { JsonEditor, parseJsonOr } from '@/lib/components/JsonEditor';
import { listSsoProviders, type SsoProviderRecord } from '@/lib/api/auth';
import {
  getControlPanel,
  getUpgradeReadiness,
  updateControlPanel,
  type AppBrandingSettings,
  type ControlPanelSettings,
  type IdentityProviderMapping,
  type ResourceManagementPolicy,
  type SupportedLocale,
  type UpdateControlPanelRequest,
  type UpgradeAssistantSettings,
  type UpgradeReadinessResponse,
} from '@/lib/api/control-panel';

const RELEASE_CHANNELS = ['stable', 'beta', 'alpha'];
const DEPLOYMENT_MODES = ['cloud', 'self_hosted', 'air_gapped'];
const LOCALES: SupportedLocale[] = ['en', 'es'];

export function ControlPanelPage() {
  const [settings, setSettings] = useState<ControlPanelSettings | null>(null);
  const [readiness, setReadiness] = useState<UpgradeReadinessResponse | null>(null);
  const [ssoProviders, setSsoProviders] = useState<SsoProviderRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // structured fields
  const [platformName, setPlatformName] = useState('');
  const [supportEmail, setSupportEmail] = useState('');
  const [docsUrl, setDocsUrl] = useState('');
  const [statusPageUrl, setStatusPageUrl] = useState('');
  const [announcementBanner, setAnnouncementBanner] = useState('');
  const [maintenanceMode, setMaintenanceMode] = useState(false);
  const [releaseChannel, setReleaseChannel] = useState('stable');
  const [defaultRegion, setDefaultRegion] = useState('');
  const [deploymentMode, setDeploymentMode] = useState('cloud');
  const [allowSelfSignup, setAllowSelfSignup] = useState(false);
  const [defaultLocale, setDefaultLocale] = useState<SupportedLocale>('en');
  const [supportedLocales, setSupportedLocales] = useState<SupportedLocale[]>([]);
  const [allowedEmailDomains, setAllowedEmailDomains] = useState('');
  const [restrictedOperations, setRestrictedOperations] = useState('');

  // JSON-driven nested fields
  const [brandingJson, setBrandingJson] = useState('{}');
  const [identityMappingsJson, setIdentityMappingsJson] = useState('[]');
  const [resourcePoliciesJson, setResourcePoliciesJson] = useState('[]');
  const [upgradeAssistantJson, setUpgradeAssistantJson] = useState('{}');

  function hydrateFromSettings(s: ControlPanelSettings) {
    setSettings(s);
    setPlatformName(s.platform_name);
    setSupportEmail(s.support_email);
    setDocsUrl(s.docs_url);
    setStatusPageUrl(s.status_page_url);
    setAnnouncementBanner(s.announcement_banner);
    setMaintenanceMode(s.maintenance_mode);
    setReleaseChannel(s.release_channel);
    setDefaultRegion(s.default_region);
    setDeploymentMode(s.deployment_mode);
    setAllowSelfSignup(s.allow_self_signup);
    setDefaultLocale(s.default_locale);
    setSupportedLocales(s.supported_locales);
    setAllowedEmailDomains(s.allowed_email_domains.join(', '));
    setRestrictedOperations(s.restricted_operations.join(', '));
    setBrandingJson(JSON.stringify(s.default_app_branding, null, 2));
    setIdentityMappingsJson(JSON.stringify(s.identity_provider_mappings, null, 2));
    setResourcePoliciesJson(JSON.stringify(s.resource_management_policies, null, 2));
    setUpgradeAssistantJson(JSON.stringify(s.upgrade_assistant, null, 2));
  }

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const [s, u, sso] = await Promise.all([
        getControlPanel(),
        getUpgradeReadiness().catch(() => null),
        listSsoProviders().catch(() => [] as SsoProviderRecord[]),
      ]);
      hydrateFromSettings(s);
      setReadiness(u);
      setSsoProviders(sso);
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
      const body: UpdateControlPanelRequest = {
        platform_name: platformName,
        support_email: supportEmail,
        docs_url: docsUrl,
        status_page_url: statusPageUrl,
        announcement_banner: announcementBanner,
        maintenance_mode: maintenanceMode,
        release_channel: releaseChannel,
        default_region: defaultRegion,
        deployment_mode: deploymentMode,
        allow_self_signup: allowSelfSignup,
        default_locale: defaultLocale,
        supported_locales: supportedLocales,
        allowed_email_domains: allowedEmailDomains.split(',').map((s) => s.trim()).filter(Boolean),
        restricted_operations: restrictedOperations.split(',').map((s) => s.trim()).filter(Boolean),
        default_app_branding: parseJsonOr<AppBrandingSettings>(brandingJson, settings?.default_app_branding ?? {} as AppBrandingSettings),
        identity_provider_mappings: parseJsonOr<IdentityProviderMapping[]>(identityMappingsJson, []),
        resource_management_policies: parseJsonOr<ResourceManagementPolicy[]>(resourcePoliciesJson, []),
        upgrade_assistant: parseJsonOr<UpgradeAssistantSettings>(upgradeAssistantJson, settings?.upgrade_assistant ?? {} as UpgradeAssistantSettings),
      };
      hydrateFromSettings(await updateControlPanel(body));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setBusy(false);
    }
  }

  function toggleLocale(loc: SupportedLocale) {
    setSupportedLocales((prev) => (prev.includes(loc) ? prev.filter((l) => l !== loc) : [...prev, loc]));
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Control panel</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Platform configuration, upgrade readiness, SSO providers.
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
        <>
          <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
            <p className="of-eyebrow">Platform</p>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))' }}>
              <label style={{ fontSize: 13 }}>
                Platform name
                <input value={platformName} onChange={(e) => setPlatformName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13 }}>
                Support email
                <input type="email" value={supportEmail} onChange={(e) => setSupportEmail(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13 }}>
                Docs URL
                <input type="url" value={docsUrl} onChange={(e) => setDocsUrl(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13 }}>
                Status page URL
                <input type="url" value={statusPageUrl} onChange={(e) => setStatusPageUrl(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13 }}>
                Default region
                <input value={defaultRegion} onChange={(e) => setDefaultRegion(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13 }}>
                Release channel
                <select value={releaseChannel} onChange={(e) => setReleaseChannel(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
                  {RELEASE_CHANNELS.map((c) => <option key={c} value={c}>{c}</option>)}
                </select>
              </label>
              <label style={{ fontSize: 13 }}>
                Deployment mode
                <select value={deploymentMode} onChange={(e) => setDeploymentMode(e.target.value)} className="of-input" style={{ marginTop: 4 }}>
                  {DEPLOYMENT_MODES.map((c) => <option key={c} value={c}>{c}</option>)}
                </select>
              </label>
              <label style={{ fontSize: 13 }}>
                Default locale
                <select value={defaultLocale} onChange={(e) => setDefaultLocale(e.target.value as SupportedLocale)} className="of-input" style={{ marginTop: 4 }}>
                  {LOCALES.map((c) => <option key={c} value={c}>{c}</option>)}
                </select>
              </label>
            </div>
            <label style={{ fontSize: 13 }}>
              Announcement banner
              <textarea value={announcementBanner} onChange={(e) => setAnnouncementBanner(e.target.value)} rows={2} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
              <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
                <input type="checkbox" checked={maintenanceMode} onChange={(e) => setMaintenanceMode(e.target.checked)} />
                Maintenance mode
              </label>
              <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
                <input type="checkbox" checked={allowSelfSignup} onChange={(e) => setAllowSelfSignup(e.target.checked)} />
                Allow self-signup
              </label>
            </div>
            <div style={{ fontSize: 13 }}>
              Supported locales
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 4 }}>
                {LOCALES.map((loc) => (
                  <label key={loc} style={{ fontSize: 11, display: 'flex', alignItems: 'center', gap: 4, padding: '2px 8px', border: '1px solid var(--border-default)', borderRadius: 999, cursor: 'pointer' }}>
                    <input type="checkbox" checked={supportedLocales.includes(loc)} onChange={() => toggleLocale(loc)} />
                    {loc}
                  </label>
                ))}
              </div>
            </div>
            <label style={{ fontSize: 13 }}>
              Allowed email domains (comma-separated)
              <input value={allowedEmailDomains} onChange={(e) => setAllowedEmailDomains(e.target.value)} placeholder="acme.com, example.org" className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Restricted operations (comma-separated)
              <input value={restrictedOperations} onChange={(e) => setRestrictedOperations(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
            </label>
          </section>

          <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 12 }}>
            <p className="of-eyebrow">Advanced (JSON)</p>
            <JsonEditor label="Default app branding" value={brandingJson} onChange={setBrandingJson} minHeight={120} />
            <JsonEditor label="Identity provider mappings" value={identityMappingsJson} onChange={setIdentityMappingsJson} minHeight={120} />
            <JsonEditor label="Resource management policies" value={resourcePoliciesJson} onChange={setResourcePoliciesJson} minHeight={120} />
            <JsonEditor label="Upgrade assistant" value={upgradeAssistantJson} onChange={setUpgradeAssistantJson} minHeight={120} />
          </section>

          <div>
            <button type="button" onClick={() => void save()} disabled={busy} className="of-button of-button--primary">
              {busy ? 'Saving…' : 'Save settings'}
            </button>
          </div>
        </>
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
