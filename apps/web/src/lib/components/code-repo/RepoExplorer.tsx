import type {
  PackageKind,
  RepositoryDefinition,
  RepositoryOverview,
  RepositoryVisibility,
} from '@/lib/api/code-repos';

export interface RepositoryDraft {
  id?: string;
  name: string;
  slug: string;
  description: string;
  owner: string;
  default_branch: string;
  visibility: RepositoryVisibility;
  object_store_backend: string;
  package_kind: PackageKind;
  tags_text: string;
  settings_text: string;
}

interface Props {
  overview: RepositoryOverview | null;
  repositories: RepositoryDefinition[];
  selectedRepositoryId: string;
  draft: RepositoryDraft;
  busy?: boolean;
  onSelectRepository: (repositoryId: string) => void;
  onDraftChange: (patch: Partial<RepositoryDraft>) => void;
  onSave: () => void;
  onReset: () => void;
}

const VISIBILITIES: RepositoryVisibility[] = ['private', 'public'];
const PACKAGE_KINDS: PackageKind[] = ['connector', 'transform', 'widget', 'app_template', 'ml_model', 'ai_agent'];

const darkInput: React.CSSProperties = {
  width: '100%',
  borderRadius: 16,
  border: '1px solid #44403c',
  background: '#1c1917',
  padding: '10px 14px',
  color: '#f5f5f4',
  fontSize: 13,
  outline: 'none',
};

function pretty(value: string) {
  return value.replaceAll('_', ' ');
}

function presetSettings(settings: Record<string, unknown>) {
  return JSON.stringify(settings, null, 2);
}

export function RepoExplorer({
  overview,
  repositories,
  selectedRepositoryId,
  draft,
  busy = false,
  onSelectRepository,
  onDraftChange,
  onSave,
  onReset,
}: Props) {
  function applyPreset(preset: 'rust' | 'react' | 'python') {
    if (preset === 'react') {
      onDraftChange({
        name: 'Slate React App',
        slug: 'slate-react-app',
        description: 'React + OpenFoundry SDK starter ready for Slate delivery.',
        package_kind: 'app_template',
        settings_text: presetSettings({
          runtime: 'typescript-react',
          entry_file: 'src/App.tsx',
          sdk_import: '@open-foundry/sdk/react',
          workspace_layout: 'split',
          dev_command: 'pnpm dev',
          preview_command: 'pnpm build',
          ci_required: false,
          allow_direct_commits_on_protected: false,
        }),
      });
      return;
    }
    if (preset === 'python') {
      onDraftChange({
        name: 'Python Agent Package',
        slug: 'python-agent-package',
        description: 'Python starter for automation, agents, or ML-adjacent workflows.',
        package_kind: 'ai_agent',
        settings_text: presetSettings({
          runtime: 'python',
          entry_file: 'src/python_agent_package/main.py',
          dev_command: 'python -m python_agent_package',
          preview_command: 'python -m compileall src',
          ci_required: true,
          allow_direct_commits_on_protected: false,
        }),
      });
      return;
    }
    onDraftChange({
      name: 'Foundry Widget Kit',
      slug: 'foundry-widget-kit',
      description: 'Shared widget primitives ready for marketplace publication.',
      package_kind: 'widget',
      settings_text: presetSettings({
        runtime: 'rust',
        default_path: 'src/lib.rs',
        ci_required: true,
        allow_direct_commits_on_protected: false,
      }),
    });
  }

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#0369a1' }}>
            Repository Control Plane
          </p>
          <h2 className="of-heading-md" style={{ marginTop: 6 }}>
            Object-backed repos, package kinds, and owner metadata
          </h2>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Shape repository descriptors that power protected branches, merge policies, CI triggers, and marketplace publication.
          </p>
        </div>
        <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(4, 1fr)' }}>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: '#0c0a09', color: '#f5f5f4' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#7dd3fc' }}>Repos</p>
            <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{overview?.repository_count ?? 0}</p>
          </div>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: 'var(--bg-subtle)' }}>
            <p className="of-eyebrow">Private</p>
            <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{overview?.private_repository_count ?? 0}</p>
          </div>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: '#f0f9ff', color: '#0369a1' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em' }}>Open MRs</p>
            <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{overview?.open_merge_request_count ?? 0}</p>
          </div>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: '#fffbeb', color: '#b45309' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em' }}>Kinds</p>
            <p style={{ marginTop: 6, fontSize: 13, fontWeight: 600 }}>{overview?.package_kind_mix.join(', ') || 'n/a'}</p>
          </div>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.95fr) minmax(0, 1.05fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14 }}>
          <label style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: 'var(--text-muted)', display: 'block' }}>
            Repositories
            <select
              value={selectedRepositoryId}
              onChange={(e) => onSelectRepository(e.target.value)}
              className="of-input"
              style={{ marginTop: 8, fontSize: 13 }}
            >
              <option value="">Create a new repository</option>
              {repositories.map((repository) => (
                <option key={repository.id} value={repository.id}>
                  {repository.name} · {repository.package_kind}
                </option>
              ))}
            </select>
          </label>

          <div style={{ display: 'grid', gap: 8, marginTop: 14 }}>
            {repositories.map((repository) => {
              const active = selectedRepositoryId === repository.id;
              return (
                <button
                  key={repository.id}
                  type="button"
                  onClick={() => onSelectRepository(repository.id)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 14,
                    border: `1px solid ${active ? '#0284c7' : 'var(--border-default)'}`,
                    background: active ? '#f0f9ff' : 'var(--bg-elevated)',
                    borderRadius: 16,
                    cursor: 'pointer',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{repository.name}</p>
                      <p className="of-text-muted" style={{ fontSize: 13 }}>
                        {repository.owner} · {repository.slug}
                      </p>
                    </div>
                    <span className="of-chip" style={{ background: '#0c0a09', color: '#f5f5f4', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                      {pretty(repository.package_kind)}
                    </span>
                  </div>
                  <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
                    {repository.description}
                  </p>
                </button>
              );
            })}
          </div>
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', display: 'grid', gap: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <div>
              <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#7dd3fc' }}>Metadata</p>
              <p style={{ marginTop: 6, fontSize: 13, color: '#d6d3d1' }}>
                Define the package kind, visibility, backend, and settings payload.
              </p>
            </div>
            <div style={{ display: 'flex', gap: 6 }}>
              <button type="button" onClick={onReset} disabled={busy} className="of-button" style={{ borderColor: '#44403c', color: '#d6d3d1', background: 'transparent' }}>
                New draft
              </button>
              <button type="button" onClick={onSave} disabled={busy} className="of-button of-button--primary" style={{ background: '#0ea5e9', color: '#0c0a09' }}>
                {draft.id ? 'Update repo' : 'Create repo'}
              </button>
            </div>
          </div>

          <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(3, 1fr)' }}>
            {(['rust', 'react', 'python'] as const).map((preset) => (
              <button
                key={preset}
                type="button"
                onClick={() => applyPreset(preset)}
                disabled={busy}
                style={{ borderRadius: 16, padding: '10px 14px', border: '1px solid #44403c', background: '#1c1917', textAlign: 'left', color: '#f5f5f4', cursor: busy ? 'not-allowed' : 'pointer' }}
              >
                <div style={{ fontWeight: 600 }}>
                  {preset === 'rust' ? 'Rust package' : preset === 'react' ? 'React Slate app' : 'Python automation'}
                </div>
                <div style={{ marginTop: 4, fontSize: 11, color: '#a8a29e' }}>
                  {preset === 'rust'
                    ? 'Keep the current Git + Cargo flow for connectors, transforms, or widgets.'
                    : preset === 'react'
                      ? 'Scaffold TypeScript/React with @open-foundry/sdk/react from day one.'
                      : 'Create a Python package for agents, ML helpers, or automation surfaces.'}
                </div>
              </button>
            ))}
          </div>

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Name</span>
              <input value={draft.name} onChange={(e) => onDraftChange({ name: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Slug</span>
              <input value={draft.slug} onChange={(e) => onDraftChange({ slug: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Description</span>
              <textarea
                value={draft.description}
                onChange={(e) => onDraftChange({ description: e.target.value })}
                style={{ ...darkInput, minHeight: 90, resize: 'vertical' }}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Owner</span>
              <input value={draft.owner} onChange={(e) => onDraftChange({ owner: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Default branch</span>
              <input value={draft.default_branch} onChange={(e) => onDraftChange({ default_branch: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Visibility</span>
              <select
                value={draft.visibility}
                onChange={(e) => onDraftChange({ visibility: e.target.value as RepositoryVisibility })}
                style={darkInput}
              >
                {VISIBILITIES.map((visibility) => (
                  <option key={visibility} value={visibility}>
                    {visibility}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Package kind</span>
              <select
                value={draft.package_kind}
                onChange={(e) => onDraftChange({ package_kind: e.target.value as PackageKind })}
                style={darkInput}
              >
                {PACKAGE_KINDS.map((packageKind) => (
                  <option key={packageKind} value={packageKind}>
                    {pretty(packageKind)}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Object store backend</span>
              <input
                value={draft.object_store_backend}
                onChange={(e) => onDraftChange({ object_store_backend: e.target.value })}
                style={darkInput}
              />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Tags</span>
              <input value={draft.tags_text} onChange={(e) => onDraftChange({ tags_text: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, color: '#f5f5f4', gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Settings JSON</span>
              <textarea
                value={draft.settings_text}
                onChange={(e) => onDraftChange({ settings_text: e.target.value })}
                style={{ ...darkInput, minHeight: 130, fontFamily: 'var(--font-mono)', fontSize: 11, color: '#7dd3fc', resize: 'vertical' }}
              />
            </label>
          </div>
        </div>
      </div>
    </section>
  );
}
