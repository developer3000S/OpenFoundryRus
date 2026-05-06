import type { ProductFleetRecord } from '@/lib/api/marketplace';

interface Props {
  versions: string[];
  version: string;
  workspaceName: string;
  releaseChannel: string;
  fleetId: string;
  enrollmentBranch: string;
  fleets: ProductFleetRecord[];
  busy?: boolean;
  onVersionChange: (version: string) => void;
  onWorkspaceNameChange: (workspaceName: string) => void;
  onReleaseChannelChange: (releaseChannel: string) => void;
  onFleetChange: (fleetId: string) => void;
  onEnrollmentBranchChange: (branchName: string) => void;
  onInstall: () => void;
}

export function InstallDialog({
  versions,
  version,
  workspaceName,
  releaseChannel,
  fleetId,
  enrollmentBranch,
  fleets,
  busy = false,
  onVersionChange,
  onWorkspaceNameChange,
  onReleaseChannelChange,
  onFleetChange,
  onEnrollmentBranchChange,
  onInstall,
}: Props) {
  return (
    <div className="of-panel-muted" style={{ padding: 14 }}>
      <p className="of-eyebrow" style={{ color: '#047857' }}>
        One-click install
      </p>
      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 14 }}>
        <label style={{ fontSize: 13 }}>
          <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Version</span>
          <select value={version} onChange={(e) => onVersionChange(e.target.value)} className="of-input">
            {versions.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 13 }}>
          <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Workspace</span>
          <input value={workspaceName} onChange={(e) => onWorkspaceNameChange(e.target.value)} className="of-input" />
        </label>
        <label style={{ fontSize: 13 }}>
          <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Release channel</span>
          <input value={releaseChannel} onChange={(e) => onReleaseChannelChange(e.target.value)} className="of-input" />
        </label>
        <label style={{ fontSize: 13 }}>
          <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Fleet</span>
          <select value={fleetId} onChange={(e) => onFleetChange(e.target.value)} className="of-input">
            <option value="">Direct install</option>
            {fleets.map((fleet) => (
              <option key={fleet.id} value={fleet.id}>
                {fleet.name} · {fleet.release_channel}
              </option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
          <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Enrollment branch</span>
          <input
            value={enrollmentBranch}
            onChange={(e) => onEnrollmentBranchChange(e.target.value)}
            placeholder="feature/ops-branch (optional)"
            className="of-input"
          />
        </label>
      </div>
      <button
        type="button"
        onClick={onInstall}
        disabled={busy || versions.length === 0}
        className="of-button of-button--primary"
        style={{ marginTop: 14, background: '#10b981', color: '#0c0a09' }}
      >
        Install package
      </button>
    </div>
  );
}
