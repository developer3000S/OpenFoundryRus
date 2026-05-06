import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { createSsoProvider, deleteSsoProvider } from '@api/auth';
import { usePermissions } from '@/lib/auth/permissions';
import { settingsQueryKeys, ssoProvidersQuery } from './queries';
import { parseJson, toOptionalString, toScopes } from './utils';

interface SsoProvidersSectionProps {
  setNotice: (msg: string) => void;
  setError: (msg: string) => void;
}

const DEFAULT_FORM = {
  slug: '',
  name: '',
  provider_type: 'oidc',
  enabled: true,
  client_id: '',
  client_secret: '',
  issuer_url: '',
  authorization_url: '',
  token_url: '',
  userinfo_url: '',
  scopes: 'openid,profile,email',
  saml_metadata_url: '',
  saml_entity_id: '',
  saml_sso_url: '',
  saml_certificate: '',
  attribute_mapping: '{\n  "subject": "sub",\n  "email": "email",\n  "name": "name"\n}',
};

export function SsoProvidersSection({ setNotice, setError }: SsoProvidersSectionProps) {
  const perms = usePermissions();
  const qc = useQueryClient();

  const result = useQuery({ ...ssoProvidersQuery, enabled: perms.canReadSso });
  const providers = result.data ?? [];

  const [form, setForm] = useState(DEFAULT_FORM);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: () => {
      let attributeMapping: Record<string, unknown>;
      try {
        attributeMapping = parseJson(form.attribute_mapping);
      } catch (err) {
        return Promise.reject(
          new Error(
            err instanceof Error
              ? `Invalid attribute mapping JSON: ${err.message}`
              : 'Invalid attribute mapping JSON',
          ),
        );
      }
      return createSsoProvider({
        slug: form.slug,
        name: form.name,
        provider_type: form.provider_type,
        enabled: form.enabled,
        client_id: toOptionalString(form.client_id),
        client_secret: toOptionalString(form.client_secret),
        issuer_url: toOptionalString(form.issuer_url),
        authorization_url: toOptionalString(form.authorization_url),
        token_url: toOptionalString(form.token_url),
        userinfo_url: toOptionalString(form.userinfo_url),
        scopes: toScopes(form.scopes),
        saml_metadata_url: toOptionalString(form.saml_metadata_url),
        saml_entity_id: toOptionalString(form.saml_entity_id),
        saml_sso_url: toOptionalString(form.saml_sso_url),
        saml_certificate: toOptionalString(form.saml_certificate),
        attribute_mapping: attributeMapping,
      });
    },
    onSuccess: async () => {
      setForm(DEFAULT_FORM);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.ssoProviders });
      setNotice('SSO provider created.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to create SSO provider'),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteSsoProvider(id),
    onMutate: (id) => setDeletingId(id),
    onSettled: () => setDeletingId(null),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.ssoProviders });
      setNotice('SSO provider deleted.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to delete SSO provider'),
  });

  if (!perms.canReadSso) return null;

  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <p className="of-eyebrow">SSO</p>
      <h2 className="of-heading-lg">Provider connections</h2>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fit, minmax(420px, 1fr))', marginTop: 24 }}>
        {providers.map((provider) => (
          <article key={provider.id} className="of-panel-muted" style={{ padding: 20 }}>
            <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
              <div>
                <h3 className="of-heading-sm">{provider.name}</h3>
                <p className="of-text-muted" style={{ fontSize: 13 }}>
                  {provider.provider_type} • /{provider.slug}
                </p>
              </div>
              <span className={`of-chip ${provider.enabled ? 'of-status-success' : ''}`}>
                {provider.enabled ? 'Enabled' : 'Disabled'}
              </span>
            </header>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
              {provider.scopes.map((scope) => (
                <span key={scope} className="of-chip">
                  {scope}
                </span>
              ))}
            </div>
            <pre
              className="of-scrollbar"
              style={{
                margin: '12px 0 0',
                overflow: 'auto',
                padding: 12,
                background: '#fff',
                border: '1px solid var(--border-subtle)',
                borderRadius: 'var(--radius-sm)',
                fontSize: 12,
              }}
            >
              {JSON.stringify(provider.attribute_mapping, null, 2)}
            </pre>
            {perms.canManageSso && (
              <button
                type="button"
                className="of-btn of-btn-danger"
                style={{ marginTop: 12 }}
                onClick={() => deleteMutation.mutate(provider.id)}
                disabled={deletingId === provider.id}
              >
                {deletingId === provider.id ? 'Deleting…' : 'Delete provider'}
              </button>
            )}
          </article>
        ))}
        {result.isLoading && <p className="of-text-muted">Loading providers…</p>}
        {!result.isLoading && providers.length === 0 && (
          <p className="of-text-muted">No SSO providers configured.</p>
        )}
      </div>

      {perms.canManageSso && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            createMutation.mutate();
          }}
          style={{
            display: 'grid',
            gap: 10,
            gridTemplateColumns: '1fr 1fr',
            marginTop: 24,
            padding: 20,
            border: '1px dashed var(--border-default)',
            borderRadius: 'var(--radius-md)',
          }}
        >
          <div className="of-eyebrow" style={{ gridColumn: '1 / -1' }}>
            Create provider
          </div>
          <input
            className="of-input"
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            placeholder="Display name"
            required
          />
          <input
            className="of-input"
            value={form.slug}
            onChange={(e) => setForm((f) => ({ ...f, slug: e.target.value }))}
            placeholder="Slug"
            required
          />
          <select
            className="of-select"
            value={form.provider_type}
            onChange={(e) => setForm((f) => ({ ...f, provider_type: e.target.value }))}
          >
            <option value="oidc">OIDC</option>
            <option value="saml">SAML</option>
          </select>
          <label
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              padding: '0 12px',
              border: '1px solid var(--border-default)',
              borderRadius: 'var(--radius-sm)',
              fontSize: 13,
              minHeight: 38,
            }}
          >
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
            />
            Enabled
          </label>
          <input
            className="of-input"
            value={form.client_id}
            onChange={(e) => setForm((f) => ({ ...f, client_id: e.target.value }))}
            placeholder="Client ID"
          />
          <input
            className="of-input"
            value={form.client_secret}
            onChange={(e) => setForm((f) => ({ ...f, client_secret: e.target.value }))}
            placeholder="Client secret"
          />
          <input
            className="of-input"
            value={form.issuer_url}
            onChange={(e) => setForm((f) => ({ ...f, issuer_url: e.target.value }))}
            placeholder="Issuer URL"
          />
          <input
            className="of-input"
            value={form.authorization_url}
            onChange={(e) => setForm((f) => ({ ...f, authorization_url: e.target.value }))}
            placeholder="Authorization URL"
          />
          <input
            className="of-input"
            value={form.token_url}
            onChange={(e) => setForm((f) => ({ ...f, token_url: e.target.value }))}
            placeholder="Token URL"
          />
          <input
            className="of-input"
            value={form.userinfo_url}
            onChange={(e) => setForm((f) => ({ ...f, userinfo_url: e.target.value }))}
            placeholder="Userinfo URL"
          />
          <input
            className="of-input"
            value={form.scopes}
            onChange={(e) => setForm((f) => ({ ...f, scopes: e.target.value }))}
            placeholder="Scopes, comma separated"
            style={{ gridColumn: '1 / -1' }}
          />
          <input
            className="of-input"
            value={form.saml_metadata_url}
            onChange={(e) => setForm((f) => ({ ...f, saml_metadata_url: e.target.value }))}
            placeholder="SAML metadata URL"
          />
          <input
            className="of-input"
            value={form.saml_entity_id}
            onChange={(e) => setForm((f) => ({ ...f, saml_entity_id: e.target.value }))}
            placeholder="SAML entity ID"
          />
          <input
            className="of-input"
            value={form.saml_sso_url}
            onChange={(e) => setForm((f) => ({ ...f, saml_sso_url: e.target.value }))}
            placeholder="SAML SSO URL"
          />
          <input
            className="of-input"
            value={form.saml_certificate}
            onChange={(e) => setForm((f) => ({ ...f, saml_certificate: e.target.value }))}
            placeholder="SAML certificate"
          />
          <textarea
            className="of-textarea"
            value={form.attribute_mapping}
            onChange={(e) => setForm((f) => ({ ...f, attribute_mapping: e.target.value }))}
            rows={7}
            style={{ gridColumn: '1 / -1', fontFamily: 'var(--font-mono)' }}
          />
          <button
            type="submit"
            className="of-btn of-btn-primary"
            disabled={createMutation.isPending}
            style={{ gridColumn: '1 / -1', justifySelf: 'start' }}
          >
            {createMutation.isPending ? 'Saving…' : 'Create provider'}
          </button>
        </form>
      )}
    </section>
  );
}
