import { useEffect, useState } from 'react';

import { ApiExplorer } from '@/lib/components/developer/ApiExplorer';
import {
  GitIntegrationManager,
  type IntegrationDraft,
  type SyncDraft,
} from '@/lib/components/developer/GitIntegrationManager';
import { SdkToolkit } from '@/lib/components/developer/SdkToolkit';
import { TerraformProviderPanel } from '@/lib/components/developer/TerraformProviderPanel';
import {
  createIntegration,
  getIntegration,
  listIntegrations,
  loadOpenApiSpec,
  loadTerraformProviderSchema,
  triggerIntegrationSync,
  updateIntegration,
  type IntegrationDetail,
  type OpenApiSpec,
  type RepositoryIntegration,
  type TerraformProviderSchema,
} from '@/lib/api/developer';
import { listRepositories, type RepositoryDefinition } from '@/lib/api/code-repos';
import { notifications } from '@stores/notifications';

function createEmptyIntegrationDraft(repository?: RepositoryDefinition | null): IntegrationDraft {
  return {
    repository_id: repository?.id ?? '',
    provider: 'github',
    external_namespace: 'openfoundry-labs',
    external_project: repository?.slug ?? 'plugin-starter',
    external_url: repository
      ? `https://github.com/openfoundry-labs/${repository.slug}`
      : 'https://github.com/openfoundry-labs/plugin-starter',
    sync_mode: 'bidirectional_mirror',
    ci_trigger_strategy: 'github_actions',
    status: 'connected',
    default_branch: repository?.default_branch ?? 'main',
    branch_mapping_text: `${repository?.default_branch ?? 'main'} -> ${repository?.default_branch ?? 'main'}`,
    webhook_url: 'https://platform.openfoundry.local/api/v1/hooks/git',
  };
}

function createEmptySyncDraft(branchName = 'main'): SyncDraft {
  return {
    trigger: 'manual',
    commit_sha: '8f4c1d2b9a6e77c1',
    branch_name: branchName,
  };
}

function parseLines(value: string) {
  return value
    .split('\n')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function draftFromIntegration(integration: RepositoryIntegration): IntegrationDraft {
  return {
    id: integration.id,
    repository_id: integration.repository_id,
    provider: integration.provider,
    external_namespace: integration.external_namespace,
    external_project: integration.external_project,
    external_url: integration.external_url,
    sync_mode: integration.sync_mode,
    ci_trigger_strategy: integration.ci_trigger_strategy,
    status: integration.status,
    default_branch: integration.default_branch,
    branch_mapping_text: integration.branch_mapping.join('\n'),
    webhook_url: integration.webhook_url,
  };
}

export function DevelopersPage() {
  const [repositories, setRepositories] = useState<RepositoryDefinition[]>([]);
  const [integrations, setIntegrations] = useState<RepositoryIntegration[]>([]);
  const [selectedIntegration, setSelectedIntegration] = useState<IntegrationDetail | null>(null);
  const [openApiSpec, setOpenApiSpec] = useState<OpenApiSpec | null>(null);
  const [terraformSchema, setTerraformSchema] = useState<TerraformProviderSchema | null>(null);
  const [loading, setLoading] = useState(true);
  const [docsLoading, setDocsLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [integrationError, setIntegrationError] = useState('');
  const [docsError, setDocsError] = useState('');
  const [draft, setDraft] = useState<IntegrationDraft>(createEmptyIntegrationDraft());
  const [syncDraft, setSyncDraft] = useState<SyncDraft>(createEmptySyncDraft());

  const busy = loading || busyAction.length > 0;

  function startNewIntegration(pool?: RepositoryDefinition[]) {
    const repository = (pool ?? repositories)[0] ?? null;
    setSelectedIntegration(null);
    setIntegrationError('');
    setDraft(createEmptyIntegrationDraft(repository));
    setSyncDraft(createEmptySyncDraft(repository?.default_branch ?? 'main'));
  }

  async function selectIntegration(integrationId: string, notify = true) {
    setBusyAction('loading-integration');
    setIntegrationError('');
    try {
      const detail = await getIntegration(integrationId);
      setSelectedIntegration(detail);
      setDraft(draftFromIntegration(detail.integration));
      setSyncDraft(createEmptySyncDraft(detail.integration.default_branch));
      if (notify) {
        notifications.info(`Loaded ${detail.integration.external_namespace}/${detail.integration.external_project}`);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load integration';
      setIntegrationError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function refreshAll(preferredId?: string) {
    setLoading(true);
    setIntegrationError('');
    try {
      const [reposResponse, intsResponse] = await Promise.all([listRepositories(), listIntegrations()]);
      setRepositories(reposResponse.items);
      setIntegrations(intsResponse.items);
      if (intsResponse.items.length) {
        const nextId = preferredId ?? selectedIntegration?.integration.id ?? intsResponse.items[0]?.id;
        if (nextId) await selectIntegration(nextId, false);
      } else {
        startNewIntegration(reposResponse.items);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load developer portal data';
      setIntegrationError(message);
      notifications.error(message);
    } finally {
      setLoading(false);
    }

    setDocsLoading(true);
    setDocsError('');
    try {
      const [spec, schema] = await Promise.all([loadOpenApiSpec(), loadTerraformProviderSchema()]);
      setOpenApiSpec(spec);
      setTerraformSchema(schema);
    } catch (error) {
      setDocsError(error instanceof Error ? error.message : 'Unable to load generated assets');
    } finally {
      setDocsLoading(false);
    }
  }

  useEffect(() => {
    void refreshAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function updateDraft(patch: Partial<IntegrationDraft>) {
    setDraft((current) => {
      const next = { ...current, ...patch };
      if (patch.repository_id && !current.id) {
        const repository = repositories.find((entry) => entry.id === patch.repository_id) ?? null;
        next.default_branch = repository?.default_branch ?? next.default_branch;
        next.external_project = repository?.slug ?? next.external_project;
        next.external_url = repository
          ? `https://github.com/openfoundry-labs/${repository.slug}`
          : next.external_url;
        next.branch_mapping_text = `${repository?.default_branch ?? next.default_branch} -> ${repository?.default_branch ?? next.default_branch}`;
        setSyncDraft((sd) => ({ ...sd, branch_name: repository?.default_branch ?? sd.branch_name }));
      }
      return next;
    });
  }

  function updateSyncDraft(patch: Partial<SyncDraft>) {
    setSyncDraft((current) => ({ ...current, ...patch }));
  }

  async function saveIntegration() {
    setBusyAction(draft.id ? 'updating-integration' : 'creating-integration');
    setIntegrationError('');
    try {
      if (draft.id) {
        const updated = await updateIntegration(draft.id, {
          external_namespace: draft.external_namespace,
          external_project: draft.external_project,
          external_url: draft.external_url,
          sync_mode: draft.sync_mode,
          ci_trigger_strategy: draft.ci_trigger_strategy,
          status: draft.status,
          default_branch: draft.default_branch,
          branch_mapping: parseLines(draft.branch_mapping_text),
          webhook_url: draft.webhook_url,
        });
        notifications.success(`Updated ${updated.external_namespace}/${updated.external_project}`);
        await refreshAll(updated.id);
      } else {
        const created = await createIntegration({
          repository_id: draft.repository_id,
          provider: draft.provider,
          external_namespace: draft.external_namespace,
          external_project: draft.external_project,
          external_url: draft.external_url,
          sync_mode: draft.sync_mode,
          ci_trigger_strategy: draft.ci_trigger_strategy,
          default_branch: draft.default_branch,
          branch_mapping: parseLines(draft.branch_mapping_text),
          webhook_url: draft.webhook_url,
        });
        notifications.success(`Created ${created.external_namespace}/${created.external_project}`);
        await refreshAll(created.id);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to save integration';
      setIntegrationError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function queueSyncRun() {
    if (!selectedIntegration) {
      notifications.warning('Select an integration before queueing a sync run');
      return;
    }
    setBusyAction('queueing-sync');
    setIntegrationError('');
    try {
      const run = await triggerIntegrationSync(selectedIntegration.integration.id, {
        trigger: syncDraft.trigger,
        commit_sha: syncDraft.commit_sha,
        branch_name: syncDraft.branch_name,
      });
      notifications.success(`Queued ${run.trigger} sync for ${run.branch_name}`);
      await refreshAll(selectedIntegration.integration.id);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to queue sync run';
      setIntegrationError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  const pathCount = openApiSpec ? Object.keys(openApiSpec.paths).length : 0;
  const resourceCount = terraformSchema ? terraformSchema.resources.length : 0;
  const providerMix =
    Array.from(new Set(integrations.map((entry) => entry.provider))).join(' + ') || 'github + gitlab';

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ maxWidth: 720 }}>
            <p className="of-eyebrow" style={{ color: '#059669' }}>
              Developer ecosystem
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              Plugin SDK, automation, and external platform delivery
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
              Bundles the Rust + WASM plugin SDK, the of CLI, proto-derived REST docs, Terraform
              provider metadata, and GitHub/GitLab sync management into one operator surface.
            </p>
          </div>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', minWidth: 320 }}>
            {[
              { label: 'Repositories', value: repositories.length, sub: 'scaffold and sync targets' },
              { label: 'Git providers', value: providerMix, sub: 'integration coverage' },
              { label: 'REST paths', value: pathCount, sub: 'generated from proto' },
              { label: 'Terraform resources', value: resourceCount, sub: 'IaC primitives' },
            ].map((stat) => (
              <div key={stat.label} className="of-panel-muted" style={{ padding: 16 }}>
                <p className="of-eyebrow">{stat.label}</p>
                <p style={{ marginTop: 8, fontSize: typeof stat.value === 'number' ? 22 : 14, fontWeight: 600, color: 'var(--text-strong)' }}>
                  {stat.value}
                </p>
                <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                  {stat.sub}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>

      <SdkToolkit />
      <ApiExplorer spec={openApiSpec} loading={docsLoading} error={docsError} />
      <TerraformProviderPanel schema={terraformSchema} loading={docsLoading} error={docsError} />
      <GitIntegrationManager
        repositories={repositories}
        integrations={integrations}
        selectedIntegration={selectedIntegration}
        draft={draft}
        syncDraft={syncDraft}
        busy={busy}
        error={integrationError}
        onSelectIntegration={(id) => void selectIntegration(id)}
        onDraftChange={updateDraft}
        onSyncDraftChange={updateSyncDraft}
        onSaveIntegration={() => void saveIntegration()}
        onTriggerSync={() => void queueSyncRun()}
        onCreateNew={() => startNewIntegration()}
      />
    </section>
  );
}
