import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { createApiKey, revokeApiKey, type ApiKeyWithSecret } from '@api/auth';
import { apiKeysQuery, settingsQueryKeys } from './queries';
import { toIsoDateTime, toScopes } from './utils';

interface ApiKeysSectionProps {
  setNotice: (msg: string) => void;
  setError: (msg: string) => void;
}

const DEFAULT_FORM = { name: '', scopes: '', expires_at: '' };

export function ApiKeysSection({ setNotice, setError }: ApiKeysSectionProps) {
  const qc = useQueryClient();

  const result = useQuery(apiKeysQuery);
  const apiKeys = result.data ?? [];

  const [form, setForm] = useState(DEFAULT_FORM);
  const [newKey, setNewKey] = useState<ApiKeyWithSecret | null>(null);
  const [revokingId, setRevokingId] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: () =>
      createApiKey({
        name: form.name,
        scopes: toScopes(form.scopes),
        expires_at: toIsoDateTime(form.expires_at),
      }),
    onSuccess: async (data) => {
      setNewKey(data);
      setForm(DEFAULT_FORM);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.apiKeys });
      setNotice('API key created. Copy the token now; it will not be shown again.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to create API key'),
  });

  const revokeMutation = useMutation({
    mutationFn: (id: string) => revokeApiKey(id),
    onMutate: (id) => setRevokingId(id),
    onSettled: () => setRevokingId(null),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.apiKeys });
      setNotice('API key revoked.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to revoke API key'),
  });

  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <p className="of-eyebrow">Programmatic credentials</p>
      <h2 className="of-heading-lg">API keys</h2>
      <p className="of-text-muted" style={{ marginTop: 8, maxWidth: 640 }}>
        Issue scoped programmatic credentials for automation and service integrations.
      </p>

      {newKey && (
        <div
          style={{
            marginTop: 16,
            padding: 16,
            border: '1px dashed var(--status-warning)',
            background: 'var(--status-warning-bg)',
            borderRadius: 'var(--radius-md)',
            fontSize: 13,
          }}
        >
          <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>New key token</div>
          <div
            style={{
              marginTop: 8,
              wordBreak: 'break-all',
              fontFamily: 'var(--font-mono)',
              fontSize: 12,
              color: 'var(--status-warning)',
            }}
          >
            {newKey.token}
          </div>
        </div>
      )}

      <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
        {apiKeys.map((apiKey) => (
          <div key={apiKey.id} className="of-panel-muted" style={{ padding: 16 }}>
            <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
              <div>
                <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{apiKey.name}</div>
                <div className="of-text-muted" style={{ fontSize: 12 }}>
                  {apiKey.prefix} • created {new Date(apiKey.created_at).toLocaleString()}
                </div>
              </div>
              <button
                type="button"
                className="of-btn"
                onClick={() => revokeMutation.mutate(apiKey.id)}
                disabled={revokingId === apiKey.id || apiKey.revoked_at !== null}
              >
                {apiKey.revoked_at !== null
                  ? 'Revoked'
                  : revokingId === apiKey.id
                    ? 'Revoking…'
                    : 'Revoke'}
              </button>
            </header>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
              {apiKey.scopes.map((scope) => (
                <span key={scope} className="of-chip of-status-info">
                  {scope}
                </span>
              ))}
            </div>
          </div>
        ))}
        {result.isLoading && <p className="of-text-muted">Loading API keys…</p>}
        {!result.isLoading && apiKeys.length === 0 && (
          <p className="of-text-muted">No API keys issued yet.</p>
        )}
      </div>

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
          Create API key
        </div>
        <input
          className="of-input"
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          placeholder="Key name"
          required
        />
        <input
          className="of-input"
          value={form.scopes}
          onChange={(e) => setForm((f) => ({ ...f, scopes: e.target.value }))}
          placeholder="Scopes, comma separated"
        />
        <input
          className="of-input"
          type="datetime-local"
          value={form.expires_at}
          onChange={(e) => setForm((f) => ({ ...f, expires_at: e.target.value }))}
        />
        <button
          type="submit"
          className="of-btn of-btn-primary"
          disabled={createMutation.isPending}
          style={{ alignSelf: 'start' }}
        >
          {createMutation.isPending ? 'Saving…' : 'Create API key'}
        </button>
      </form>
    </section>
  );
}
