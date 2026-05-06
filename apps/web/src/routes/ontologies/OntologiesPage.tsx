import { useEffect, useState } from 'react';

import {
  createProjectBranch,
  createProjectMigration,
  createProjectProposal,
  getProjectWorkingState,
  listActionTypes,
  listInterfaces,
  listLinkTypes,
  listObjectTypes,
  listProjectBranches,
  listProjectMemberships,
  listProjectMigrations,
  listProjectProposals,
  listProjectResources,
  listProjects,
  listSharedPropertyTypes,
  updateProjectBranch,
  updateProjectProposal,
  type ActionType,
  type LinkType,
  type ObjectType,
  type OntologyBranch,
  type OntologyInterface,
  type OntologyProject,
  type OntologyProjectMembership,
  type OntologyProjectMigration,
  type OntologyProjectResourceBinding,
  type OntologyProjectWorkingState,
  type OntologyProposal,
  type SharedPropertyType,
} from '@/lib/api/ontology';

interface ProjectContext {
  workingState: OntologyProjectWorkingState | null;
  memberships: OntologyProjectMembership[];
  resources: OntologyProjectResourceBinding[];
  branches: OntologyBranch[];
  proposals: OntologyProposal[];
  migrations: OntologyProjectMigration[];
}

const EMPTY_CONTEXT: ProjectContext = {
  workingState: null,
  memberships: [],
  resources: [],
  branches: [],
  proposals: [],
  migrations: [],
};

export function OntologiesPage() {
  const [projects, setProjects] = useState<OntologyProject[]>([]);
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [linkTypes, setLinkTypes] = useState<LinkType[]>([]);
  const [interfaces, setInterfaces] = useState<OntologyInterface[]>([]);
  const [actions, setActions] = useState<ActionType[]>([]);
  const [sharedProperties, setSharedProperties] = useState<SharedPropertyType[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState('');
  const [context, setContext] = useState<ProjectContext>(EMPTY_CONTEXT);
  const [tab, setTab] = useState<'overview' | 'branches' | 'proposals' | 'migrations' | 'changes'>('overview');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [branchName, setBranchName] = useState('feature/new-branch');
  const [branchDescription, setBranchDescription] = useState('');
  const [proposalTitle, setProposalTitle] = useState('Review pending changes');
  const [proposalBranchId, setProposalBranchId] = useState('');
  const [migrationSource, setMigrationSource] = useState('');
  const [migrationTarget, setMigrationTarget] = useState('');

  async function loadCatalog() {
    setLoading(true);
    setError('');
    try {
      const [pRes, otRes, ltRes, ifRes, atRes, sptRes] = await Promise.all([
        listProjects({ per_page: 200 }),
        listObjectTypes({ per_page: 200 }),
        listLinkTypes({ per_page: 200 }).catch(() => ({ data: [], total: 0 })),
        listInterfaces({ per_page: 200 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 200 })),
        listActionTypes({ per_page: 200 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 200 })),
        listSharedPropertyTypes({ per_page: 200 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 200 })),
      ]);
      setProjects(pRes.data);
      setObjectTypes(otRes.data);
      setLinkTypes(ltRes.data);
      setInterfaces(ifRes.data);
      setActions(atRes.data);
      setSharedProperties(sptRes.data);
      if (!selectedProjectId && pRes.data[0]) setSelectedProjectId(pRes.data[0].id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load ontologies');
    } finally {
      setLoading(false);
    }
  }

  async function loadProjectContext(projectId: string) {
    if (!projectId) {
      setContext(EMPTY_CONTEXT);
      return;
    }
    try {
      const [ws, memberships, resources, branches, proposals, migrations] = await Promise.all([
        getProjectWorkingState(projectId).catch(() => null),
        listProjectMemberships(projectId).catch(() => []),
        listProjectResources(projectId).catch(() => []),
        listProjectBranches(projectId).catch(() => []),
        listProjectProposals(projectId).catch(() => []),
        listProjectMigrations(projectId).catch(() => []),
      ]);
      setContext({ workingState: ws, memberships, resources, branches, proposals, migrations });
      if (branches[0]) setProposalBranchId(branches[0].id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load project context');
    }
  }

  useEffect(() => {
    void loadCatalog();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void loadProjectContext(selectedProjectId);
  }, [selectedProjectId]);

  async function createBranch() {
    if (!selectedProjectId) return;
    try {
      await createProjectBranch(selectedProjectId, {
        name: branchName,
        description: branchDescription,
        changes: context.workingState?.changes ?? [],
      });
      setBranchName('feature/new-branch');
      setBranchDescription('');
      await loadProjectContext(selectedProjectId);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to create branch');
    }
  }

  async function setBranchStatus(branchId: string, status: OntologyBranch['status']) {
    if (!selectedProjectId) return;
    try {
      await updateProjectBranch(selectedProjectId, branchId, { status });
      await loadProjectContext(selectedProjectId);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to update branch');
    }
  }

  async function createProposal() {
    if (!selectedProjectId || !proposalBranchId) return;
    try {
      await createProjectProposal(selectedProjectId, {
        branch_id: proposalBranchId,
        title: proposalTitle,
        tasks: [],
      });
      setProposalTitle('Review pending changes');
      await loadProjectContext(selectedProjectId);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to create proposal');
    }
  }

  async function setProposalStatus(proposalId: string, status: OntologyProposal['status']) {
    if (!selectedProjectId) return;
    try {
      await updateProjectProposal(selectedProjectId, proposalId, { status });
      await loadProjectContext(selectedProjectId);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to update proposal');
    }
  }

  async function createMigration() {
    if (!selectedProjectId || !migrationSource || !migrationTarget) return;
    try {
      await createProjectMigration(selectedProjectId, {
        source_project_id: migrationSource,
        target_project_id: migrationTarget,
        resources: [],
        note: 'Cross-project migration',
      });
      await loadProjectContext(selectedProjectId);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to create migration');
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <header>
        <h1 className="of-heading-xl">Ontologies</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Project-scoped ontology workspaces with branches, proposals, migrations and resource bindings.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Catalog overview</p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 8 }}>
          <span className="of-chip">Projects {projects.length}</span>
          <span className="of-chip">Object types {objectTypes.length}</span>
          <span className="of-chip">Link types {linkTypes.length}</span>
          <span className="of-chip">Interfaces {interfaces.length}</span>
          <span className="of-chip">Action types {actions.length}</span>
          <span className="of-chip">Shared properties {sharedProperties.length}</span>
        </div>
      </section>

      <section className="of-panel" style={{ padding: 16 }}>
        <p className="of-eyebrow">Select project</p>
        <select
          value={selectedProjectId}
          onChange={(e) => setSelectedProjectId(e.target.value)}
          className="of-input"
          style={{ marginTop: 8, maxWidth: 400 }}
        >
          <option value="">— select —</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.display_name} · {p.slug}
            </option>
          ))}
        </select>
      </section>

      {loading && <p className="of-text-muted">Loading…</p>}

      {selectedProjectId && (
        <>
          <nav style={{ display: 'flex', gap: 4, borderBottom: '1px solid var(--border-default)' }}>
            {(['overview', 'branches', 'proposals', 'migrations', 'changes'] as const).map((t) => {
              const active = tab === t;
              return (
                <button
                  key={t}
                  type="button"
                  onClick={() => setTab(t)}
                  style={{
                    padding: '8px 14px',
                    background: 'transparent',
                    border: 'none',
                    borderBottom: `2px solid ${active ? '#1d4ed8' : 'transparent'}`,
                    color: active ? 'var(--text-strong)' : 'var(--text-muted)',
                    cursor: 'pointer',
                    fontSize: 13,
                    fontWeight: active ? 600 : 400,
                    textTransform: 'capitalize',
                  }}
                >
                  {t}
                </button>
              );
            })}
          </nav>

          {tab === 'overview' && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Project resources & memberships</p>
              <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1fr 1fr', marginTop: 8 }}>
                <div>
                  <strong>Resources ({context.resources.length})</strong>
                  <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 12 }}>
                    {context.resources.map((r) => (
                      <li key={`${r.resource_kind}-${r.resource_id}`}>
                        {r.resource_kind} · {r.resource_id.slice(0, 12)}
                      </li>
                    ))}
                  </ul>
                </div>
                <div>
                  <strong>Memberships ({context.memberships.length})</strong>
                  <ul style={{ marginTop: 6, paddingLeft: 18, fontSize: 12 }}>
                    {context.memberships.map((m) => (
                      <li key={`${m.user_id}-${m.role}`}>
                        {m.user_id.slice(0, 12)} · {m.role}
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            </section>
          )}

          {tab === 'branches' && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Branches ({context.branches.length})</p>
              <div style={{ display: 'grid', gap: 6, marginTop: 8 }}>
                {context.branches.map((b) => (
                  <div key={b.id} className="of-panel-muted" style={{ padding: 10 }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                      <div>
                        <strong>{b.name}</strong>{' '}
                        <span className="of-chip">{b.status}</span>
                      </div>
                      <div style={{ display: 'flex', gap: 4 }}>
                        {(['draft', 'in_review', 'merged', 'closed'] as const).map((s) => (
                          <button
                            key={s}
                            type="button"
                            onClick={() => void setBranchStatus(b.id, s)}
                            className="of-button"
                            style={{ fontSize: 11 }}
                          >
                            → {s}
                          </button>
                        ))}
                      </div>
                    </div>
                    <p className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>
                      {b.description} · {b.changes.length} changes
                    </p>
                  </div>
                ))}
              </div>
              <p className="of-eyebrow" style={{ marginTop: 14 }}>Create branch from working state</p>
              <div style={{ display: 'flex', gap: 6, marginTop: 8, flexWrap: 'wrap' }}>
                <input
                  value={branchName}
                  onChange={(e) => setBranchName(e.target.value)}
                  placeholder="Branch name"
                  className="of-input"
                  style={{ width: 240 }}
                />
                <input
                  value={branchDescription}
                  onChange={(e) => setBranchDescription(e.target.value)}
                  placeholder="Description"
                  className="of-input"
                  style={{ width: 280 }}
                />
                <button type="button" onClick={() => void createBranch()} className="of-button of-button--primary">
                  Create branch
                </button>
              </div>
            </section>
          )}

          {tab === 'proposals' && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Proposals ({context.proposals.length})</p>
              <div style={{ display: 'grid', gap: 6, marginTop: 8 }}>
                {context.proposals.map((p) => (
                  <div key={p.id} className="of-panel-muted" style={{ padding: 10 }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                      <div>
                        <strong>{p.title}</strong> <span className="of-chip">{p.status}</span>
                      </div>
                      <div style={{ display: 'flex', gap: 4 }}>
                        {(['in_review', 'approved', 'merged', 'closed'] as const).map((s) => (
                          <button
                            key={s}
                            type="button"
                            onClick={() => void setProposalStatus(p.id, s)}
                            className="of-button"
                            style={{ fontSize: 11 }}
                          >
                            → {s}
                          </button>
                        ))}
                      </div>
                    </div>
                    <p className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>
                      Branch {p.branch_id.slice(0, 8)} · {p.tasks.length} tasks
                    </p>
                  </div>
                ))}
              </div>
              <p className="of-eyebrow" style={{ marginTop: 14 }}>Create proposal</p>
              <div style={{ display: 'flex', gap: 6, marginTop: 8, flexWrap: 'wrap' }}>
                <select
                  value={proposalBranchId}
                  onChange={(e) => setProposalBranchId(e.target.value)}
                  className="of-input"
                  style={{ width: 240 }}
                >
                  {context.branches.map((b) => (
                    <option key={b.id} value={b.id}>
                      {b.name}
                    </option>
                  ))}
                </select>
                <input
                  value={proposalTitle}
                  onChange={(e) => setProposalTitle(e.target.value)}
                  placeholder="Proposal title"
                  className="of-input"
                  style={{ width: 280 }}
                />
                <button type="button" onClick={() => void createProposal()} className="of-button of-button--primary">
                  Create proposal
                </button>
              </div>
            </section>
          )}

          {tab === 'migrations' && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Migrations ({context.migrations.length})</p>
              <div style={{ display: 'grid', gap: 6, marginTop: 8 }}>
                {context.migrations.map((m) => (
                  <div key={m.id} className="of-panel-muted" style={{ padding: 10 }}>
                    <strong>{m.source_project_id.slice(0, 8)}</strong> → {m.target_project_id.slice(0, 8)}{' '}
                    <span className="of-chip">{m.status}</span>
                    <p className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>
                      {m.note} · {m.resources.length} resources · {m.submitted_at}
                    </p>
                  </div>
                ))}
              </div>
              <p className="of-eyebrow" style={{ marginTop: 14 }}>Create migration</p>
              <div style={{ display: 'flex', gap: 6, marginTop: 8, flexWrap: 'wrap' }}>
                <select
                  value={migrationSource}
                  onChange={(e) => setMigrationSource(e.target.value)}
                  className="of-input"
                  style={{ width: 220 }}
                >
                  <option value="">Source project</option>
                  {projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.display_name}
                    </option>
                  ))}
                </select>
                <select
                  value={migrationTarget}
                  onChange={(e) => setMigrationTarget(e.target.value)}
                  className="of-input"
                  style={{ width: 220 }}
                >
                  <option value="">Target project</option>
                  {projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.display_name}
                    </option>
                  ))}
                </select>
                <button type="button" onClick={() => void createMigration()} className="of-button of-button--primary">
                  Submit migration
                </button>
              </div>
            </section>
          )}

          {tab === 'changes' && (
            <section className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">Working state ({context.workingState?.changes.length ?? 0} staged)</p>
              <pre style={{ marginTop: 8, padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto', maxHeight: 480 }}>
                {JSON.stringify(context.workingState, null, 2)}
              </pre>
            </section>
          )}
        </>
      )}
    </section>
  );
}
