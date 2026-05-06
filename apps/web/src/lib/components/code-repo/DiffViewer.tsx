interface Props {
  availableBranches: string[];
  branchName: string;
  patch: string;
  busy?: boolean;
  onSelectBranch: (branchName: string) => void;
}

export function DiffViewer({ availableBranches, branchName, patch, busy = false, onSelectBranch }: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#be123c' }}>
            Diff Viewer
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Branch patch preview
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Inspect a synthetic patch against the selected branch head to review changed files before merge.
          </p>
        </div>
        <select
          value={branchName}
          onChange={(e) => onSelectBranch(e.target.value)}
          disabled={busy}
          className="of-input"
          style={{ borderRadius: 999, width: 'auto', fontSize: 13 }}
        >
          {availableBranches.map((branch) => (
            <option key={branch} value={branch}>
              {branch}
            </option>
          ))}
        </select>
      </div>

      <pre
        style={{
          marginTop: 18,
          overflowX: 'auto',
          borderRadius: 16,
          border: '1px solid #44403c',
          background: '#0c0a09',
          padding: 14,
          fontSize: 11,
          color: '#fda4af',
          fontFamily: 'var(--font-mono)',
        }}
      >
        {patch || 'Select a repository branch to render its patch.'}
      </pre>
    </section>
  );
}
