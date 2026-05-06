import type { BranchDefinition, CiRun, CommitDefinition } from '@/lib/api/code-repos';

export interface CommitDraft {
  branch_name: string;
  title: string;
  description: string;
  author_name: string;
  additions: string;
  deletions: string;
}

interface Props {
  commits: CommitDefinition[];
  ciRuns: CiRun[];
  branches: BranchDefinition[];
  draft: CommitDraft;
  busy?: boolean;
  onDraftChange: (patch: Partial<CommitDraft>) => void;
  onCreateCommit: () => void;
  onTriggerCi: () => void;
}

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

export function CommitHistory({
  commits,
  ciRuns,
  branches,
  draft,
  busy = false,
  onDraftChange,
  onCreateCommit,
  onTriggerCi,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#7c3aed' }}>
            Commits &amp; CI
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            History, pipeline triggers, and changed files
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Push synthetic commits into a branch and trigger the package validation pipeline on demand.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button type="button" onClick={onTriggerCi} disabled={busy} className="of-button" style={{ borderColor: '#c4b5fd', color: '#7c3aed' }}>
            Trigger CI
          </button>
          <button type="button" onClick={onCreateCommit} disabled={busy} className="of-button of-button--primary" style={{ background: '#7c3aed' }}>
            Create commit
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.94fr) minmax(0, 1.06fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 8 }}>
          {commits.map((commit) => (
            <div key={commit.sha} className="of-panel" style={{ padding: 14 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{commit.title}</p>
                  <p className="of-text-muted" style={{ fontSize: 13 }}>
                    {commit.branch_name} · {commit.sha} · {commit.author_name}
                  </p>
                </div>
                <span className="of-chip" style={{ background: '#f5f3ff', color: '#7c3aed' }}>
                  {commit.files_changed} files
                </span>
              </div>
              <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
                {commit.description}
              </p>
              <div style={{ display: 'flex', gap: 4, marginTop: 8 }}>
                <span className="of-chip">+{commit.additions}</span>
                <span className="of-chip">-{commit.deletions}</span>
              </div>
            </div>
          ))}
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4', display: 'grid', gap: 12 }}>
          <div>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#c4b5fd' }}>Commit draft</p>
          </div>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Branch</span>
              <select
                value={draft.branch_name}
                onChange={(e) => onDraftChange({ branch_name: e.target.value })}
                style={darkInput}
              >
                {branches.map((branch) => (
                  <option key={branch.name} value={branch.name}>
                    {branch.name} {branch.protected ? '· protected' : '· writable'}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Author</span>
              <input value={draft.author_name} onChange={(e) => onDraftChange({ author_name: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Title</span>
              <input value={draft.title} onChange={(e) => onDraftChange({ title: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Description</span>
              <textarea
                value={draft.description}
                onChange={(e) => onDraftChange({ description: e.target.value })}
                style={{ ...darkInput, minHeight: 80, resize: 'vertical' }}
              />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Additions</span>
              <input value={draft.additions} onChange={(e) => onDraftChange({ additions: e.target.value })} style={darkInput} />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Deletions</span>
              <input value={draft.deletions} onChange={(e) => onDraftChange({ deletions: e.target.value })} style={darkInput} />
            </label>
          </div>

          <div>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#c4b5fd' }}>
              Latest CI runs
            </p>
            <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
              {ciRuns.map((run) => (
                <div key={run.id} style={{ borderRadius: 16, padding: 12, border: '1px solid #44403c', background: '#1c1917' }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <p style={{ fontWeight: 500 }}>{run.pipeline_name}</p>
                      <p style={{ fontSize: 11, color: '#a8a29e' }}>
                        {run.branch_name} · {run.commit_sha} · {run.trigger}
                      </p>
                    </div>
                    <span
                      className="of-chip"
                      style={
                        run.status === 'passed'
                          ? { background: '#86efac', color: '#14532d', textTransform: 'uppercase', letterSpacing: '0.16em' }
                          : { background: '#fda4af', color: '#881337', textTransform: 'uppercase', letterSpacing: '0.16em' }
                      }
                    >
                      {run.status}
                    </span>
                  </div>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                    {run.checks.map((check) => (
                      <span key={check} className="of-chip" style={{ background: '#292524', color: '#d6d3d1' }}>
                        {check}
                      </span>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
