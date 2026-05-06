import type { IntegrationDetail, RepositoryIntegration } from '@/lib/api/developer';
import type { RepositoryDefinition } from '@/lib/api/code-repos';

export interface IntegrationDraft {
  id?: string;
  repository_id: string;
  provider: 'github' | 'gitlab';
  external_namespace: string;
  external_project: string;
  external_url: string;
  sync_mode: string;
  ci_trigger_strategy: string;
  status: string;
  default_branch: string;
  branch_mapping_text: string;
  webhook_url: string;
}

export interface SyncDraft {
  trigger: string;
  commit_sha: string;
  branch_name: string;
}

interface GitIntegrationManagerProps {
  repositories: RepositoryDefinition[];
  integrations: RepositoryIntegration[];
  selectedIntegration: IntegrationDetail | null;
  draft: IntegrationDraft;
  syncDraft: SyncDraft;
  busy?: boolean;
  error?: string;
  onSelectIntegration: (integrationId: string) => void;
  onDraftChange: (patch: Partial<IntegrationDraft>) => void;
  onSyncDraftChange: (patch: Partial<SyncDraft>) => void;
  onSaveIntegration: () => void;
  onTriggerSync: () => void;
  onCreateNew: () => void;
}

export function GitIntegrationManager({
  repositories,
  integrations,
  selectedIntegration,
  draft,
  syncDraft,
  busy = false,
  error = '',
  onSelectIntegration,
  onDraftChange,
  onSyncDraftChange,
  onSaveIntegration,
  onTriggerSync,
  onCreateNew,
}: GitIntegrationManagerProps) {
  const repositoryName = (id: string) => repositories.find((r) => r.id === id)?.name ?? 'Unknown repository';
  const selectedRepository = repositories.find((entry) => entry.id === draft.repository_id) ?? null;

  return (
    <section className="of-panel" style={{ overflow: 'hidden' }}>
      <div style={{ borderBottom: '1px solid var(--border-default)', padding: '20px 24px' }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div style={{ maxWidth: 720 }}>
            <p className="of-eyebrow" style={{ color: '#dc2626' }}>
              GitHub + GitLab
            </p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Repository integrations and sync runs
            </h2>
            <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13, lineHeight: 1.7 }}>
              Manage external Git mirrors, map branches, and queue CI-aware sync runs directly from
              the developer portal.
            </p>
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12 }}>
            <div className="of-panel-muted" style={{ padding: '12px 16px' }}>
              <p className="of-eyebrow">Repositories</p>
              <p style={{ marginTop: 4, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>
                {repositories.length}
              </p>
            </div>
            <div className="of-panel-muted" style={{ padding: '12px 16px' }}>
              <p className="of-eyebrow">Integrations</p>
              <p style={{ marginTop: 4, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>
                {integrations.length}
              </p>
            </div>
          </div>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr' }}>
        <aside style={{ borderRight: '1px solid var(--border-default)', padding: '20px 24px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
            <p className="of-eyebrow">Connected repositories</p>
            <button type="button" className="of-btn" onClick={onCreateNew} style={{ minHeight: 26, fontSize: 11 }}>
              New
            </button>
          </div>

          <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
            {integrations.map((integration) => {
              const active = selectedIntegration?.integration.id === integration.id;
              return (
                <button
                  key={integration.id}
                  type="button"
                  onClick={() => onSelectIntegration(integration.id)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 12,
                    border: `1px solid ${active ? '#fca5a5' : 'var(--border-default)'}`,
                    background: active ? '#fef2f2' : '#fff',
                    borderRadius: 'var(--radius-md)',
                    cursor: 'pointer',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>
                      {integration.external_namespace}/{integration.external_project}
                    </div>
                    <span className="of-chip" style={{ fontSize: 10 }}>
                      {integration.provider}
                    </span>
                  </div>
                  <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                    {repositoryName(integration.repository_id)}
                  </div>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                    {[integration.sync_mode, integration.ci_trigger_strategy, integration.status].map((value) => (
                      <span key={value} className="of-chip" style={{ fontSize: 10 }}>
                        {value}
                      </span>
                    ))}
                  </div>
                </button>
              );
            })}
            {!integrations.length && (
              <div style={{ border: '1px dashed var(--border-default)', padding: 16, fontSize: 13, color: 'var(--text-muted)' }}>
                No repository integrations yet.
              </div>
            )}
          </div>
        </aside>

        <section style={{ padding: '20px 24px' }}>
          {error && (
            <div className="of-status-danger" style={{ marginBottom: 16, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
              {error}
            </div>
          )}

          <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '0.95fr 1.05fr' }}>
            <div className="of-panel-muted" style={{ padding: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                <div>
                  <p className="of-eyebrow">Configuration</p>
                  <p style={{ marginTop: 4, fontSize: 14, fontWeight: 600, color: 'var(--text-strong)' }}>
                    {draft.id ? 'Edit integration' : 'Create integration'}
                  </p>
                </div>
                {selectedRepository && (
                  <span className="of-chip" style={{ fontSize: 11 }}>
                    default: {selectedRepository.default_branch}
                  </span>
                )}
              </div>

              <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 16 }}>
                <Field label="Repository">
                  <select
                    className="of-select"
                    value={draft.repository_id}
                    onChange={(e) => onDraftChange({ repository_id: e.target.value })}
                    disabled={Boolean(draft.id)}
                  >
                    <option value="">Select repository</option>
                    {repositories.map((r) => (
                      <option key={r.id} value={r.id}>
                        {r.name}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="Provider">
                  <select
                    className="of-select"
                    value={draft.provider}
                    onChange={(e) => onDraftChange({ provider: e.target.value as 'github' | 'gitlab' })}
                    disabled={Boolean(draft.id)}
                  >
                    <option value="github">GitHub</option>
                    <option value="gitlab">GitLab</option>
                  </select>
                </Field>
                <Field label="Namespace">
                  <input
                    className="of-input"
                    value={draft.external_namespace}
                    onChange={(e) => onDraftChange({ external_namespace: e.target.value })}
                  />
                </Field>
                <Field label="Project">
                  <input
                    className="of-input"
                    value={draft.external_project}
                    onChange={(e) => onDraftChange({ external_project: e.target.value })}
                  />
                </Field>
                <Field label="Remote URL" fullWidth>
                  <input
                    className="of-input"
                    value={draft.external_url}
                    onChange={(e) => onDraftChange({ external_url: e.target.value })}
                  />
                </Field>
                <Field label="Sync mode">
                  <input
                    className="of-input"
                    value={draft.sync_mode}
                    onChange={(e) => onDraftChange({ sync_mode: e.target.value })}
                  />
                </Field>
                <Field label="CI trigger">
                  <input
                    className="of-input"
                    value={draft.ci_trigger_strategy}
                    onChange={(e) => onDraftChange({ ci_trigger_strategy: e.target.value })}
                  />
                </Field>
                <Field label="Status">
                  <input
                    className="of-input"
                    value={draft.status}
                    onChange={(e) => onDraftChange({ status: e.target.value })}
                  />
                </Field>
                <Field label="Default branch">
                  <input
                    className="of-input"
                    value={draft.default_branch}
                    onChange={(e) => onDraftChange({ default_branch: e.target.value })}
                  />
                </Field>
                <Field label="Webhook URL" fullWidth>
                  <input
                    className="of-input"
                    value={draft.webhook_url}
                    onChange={(e) => onDraftChange({ webhook_url: e.target.value })}
                  />
                </Field>
                <Field label="Branch mapping" fullWidth>
                  <textarea
                    className="of-textarea"
                    value={draft.branch_mapping_text}
                    onChange={(e) => onDraftChange({ branch_mapping_text: e.target.value })}
                    rows={4}
                  />
                </Field>
              </div>

              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 16 }}>
                <button type="button" className="of-btn of-btn-primary" onClick={onSaveIntegration} disabled={busy}>
                  {draft.id ? 'Save integration' : 'Create integration'}
                </button>
                <button type="button" className="of-btn" onClick={onCreateNew} disabled={busy}>
                  Reset
                </button>
              </div>
            </div>

            <div style={{ display: 'grid', gap: 16 }}>
              <div className="of-panel-muted" style={{ padding: 16 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                  <div>
                    <p className="of-eyebrow">Sync queue</p>
                    <p style={{ marginTop: 4, fontSize: 14, fontWeight: 600, color: 'var(--text-strong)' }}>
                      {selectedIntegration ? 'Trigger external sync' : 'Select an integration'}
                    </p>
                  </div>
                  {selectedIntegration?.integration.last_synced_at && (
                    <span className="of-chip" style={{ fontSize: 11 }}>
                      last synced {new Date(selectedIntegration.integration.last_synced_at).toLocaleString()}
                    </span>
                  )}
                </div>

                {selectedIntegration ? (
                  <>
                    <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 16 }}>
                      <Field label="Trigger">
                        <input
                          className="of-input"
                          value={syncDraft.trigger}
                          onChange={(e) => onSyncDraftChange({ trigger: e.target.value })}
                        />
                      </Field>
                      <Field label="Branch">
                        <input
                          className="of-input"
                          value={syncDraft.branch_name}
                          onChange={(e) => onSyncDraftChange({ branch_name: e.target.value })}
                        />
                      </Field>
                      <Field label="Commit SHA" fullWidth>
                        <input
                          className="of-input"
                          value={syncDraft.commit_sha}
                          onChange={(e) => onSyncDraftChange({ commit_sha: e.target.value })}
                        />
                      </Field>
                    </div>
                    <div style={{ marginTop: 16 }}>
                      <button
                        type="button"
                        className="of-btn"
                        onClick={onTriggerSync}
                        disabled={busy}
                        style={{ background: '#dc2626', color: '#fff', borderColor: '#b91c1c' }}
                      >
                        Queue sync run
                      </button>
                    </div>
                  </>
                ) : (
                  <div style={{ marginTop: 16, border: '1px dashed var(--border-default)', padding: 16, fontSize: 13, color: 'var(--text-muted)' }}>
                    Pick an integration from the left column or create a new one.
                  </div>
                )}
              </div>

              <div className="of-panel-muted" style={{ padding: 16 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                  <p className="of-eyebrow">Recent sync runs</p>
                  <p className="of-text-muted" style={{ fontSize: 13 }}>
                    {selectedIntegration?.sync_runs.length ?? 0} runs
                  </p>
                </div>
                <div style={{ display: 'grid', gap: 8, marginTop: 12 }}>
                  {(selectedIntegration?.sync_runs ?? []).map((run) => (
                    <div key={run.id} className="of-panel" style={{ padding: 12 }}>
                      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                        <div>
                          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>
                            {run.summary}
                          </div>
                          <div className="of-text-muted" style={{ marginTop: 4, fontSize: 11 }}>
                            branch {run.branch_name} · commit {run.commit_sha.slice(0, 8)}
                          </div>
                        </div>
                        <span className="of-chip" style={{ fontSize: 10 }}>
                          {run.status}
                        </span>
                      </div>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                        {run.checks.map((check) => (
                          <span key={check} className="of-chip" style={{ fontSize: 10 }}>
                            {check}
                          </span>
                        ))}
                      </div>
                    </div>
                  ))}
                  {!selectedIntegration?.sync_runs.length && (
                    <div style={{ border: '1px dashed var(--border-default)', padding: 16, fontSize: 13, color: 'var(--text-muted)' }}>
                      No sync runs recorded yet.
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </section>
  );
}

function Field({ label, children, fullWidth }: { label: string; children: React.ReactNode; fullWidth?: boolean }) {
  return (
    <label style={{ display: 'block', fontSize: 13, gridColumn: fullWidth ? '1 / -1' : undefined }}>
      <div className="of-eyebrow" style={{ marginBottom: 6 }}>
        {label}
      </div>
      {children}
    </label>
  );
}
