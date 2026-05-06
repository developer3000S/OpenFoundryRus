import type { EnrollmentBranchRecord, ProductFleetRecord } from '@/lib/api/marketplace';

export interface FleetDraft {
  name: string;
  environment: string;
  workspace_targets_text: string;
  release_channel: string;
  auto_upgrade_enabled: boolean;
  maintenance_days_text: string;
  start_hour_utc: string;
  duration_minutes: string;
  branch_strategy: string;
  rollout_strategy: string;
}

export interface BranchDraft {
  fleet_id: string;
  name: string;
  repository_branch: string;
  notes: string;
}

interface Props {
  fleets: ProductFleetRecord[];
  branches: EnrollmentBranchRecord[];
  selectedListingId: string;
  busy?: boolean;
  fleetDraft: FleetDraft;
  branchDraft: BranchDraft;
  onFleetDraftChange: (patch: Partial<FleetDraft>) => void;
  onBranchDraftChange: (patch: Partial<BranchDraft>) => void;
  onCreateFleet: () => void;
  onCreateBranch: () => void;
  onSyncFleet: (fleetId: string) => void;
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

export function DeliveryStudio({
  fleets,
  branches,
  selectedListingId,
  busy = false,
  fleetDraft,
  branchDraft,
  onFleetDraftChange,
  onBranchDraftChange,
  onCreateFleet,
  onCreateBranch,
  onSyncFleet,
}: Props) {
  const selectedListingFleets = selectedListingId
    ? fleets.filter((fleet) => fleet.listing_id === selectedListingId)
    : fleets;
  const pendingTotal = fleets.reduce((total, fleet) => total + fleet.pending_upgrade_count, 0);

  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#047857' }}>
            Foundry DevOps
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Fleet rollout, maintenance windows, and enrollment branches
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Package product resources, target release channels, orchestrate upgrades across fleets, and open feature branches at the enrollment layer.
          </p>
        </div>
        <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(3, 1fr)' }}>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: '#0c0a09', color: '#f5f5f4' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#a7f3d0' }}>Fleets</p>
            <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{fleets.length}</p>
          </div>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: '#ecfdf5', color: '#047857' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em' }}>Branches</p>
            <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{branches.length}</p>
          </div>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: '#fffbeb', color: '#b45309' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em' }}>Pending</p>
            <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{pendingTotal}</p>
          </div>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.05fr) minmax(0, 0.95fr)', marginTop: 24 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 12 }}>
          <div>
            <p className="of-eyebrow">Product fleets</p>
            <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
              Each fleet can auto-track a release channel and enforce a maintenance window.
            </p>
          </div>
          <div style={{ display: 'grid', gap: 8 }}>
            {selectedListingFleets.map((fleet) => (
              <div key={fleet.id} className="of-panel" style={{ padding: 14 }}>
                <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <div>
                    <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{fleet.name}</p>
                    <p className="of-text-muted" style={{ fontSize: 13 }}>
                      {fleet.environment} · channel {fleet.release_channel} · {fleet.workspace_targets.length} workspaces
                    </p>
                  </div>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                    <span className="of-chip" style={{ background: '#ecfdf5', color: '#047857', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                      {fleet.status}
                    </span>
                    <button
                      type="button"
                      onClick={() => onSyncFleet(fleet.id)}
                      disabled={busy}
                      className="of-button"
                      style={{ borderColor: '#a7f3d0', color: '#047857', fontSize: 11 }}
                    >
                      Sync fleet
                    </button>
                  </div>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  <span className="of-chip">Current: {fleet.current_version ?? 'none'}</span>
                  <span className="of-chip">Target: {fleet.target_version ?? 'none'}</span>
                  <span className="of-chip">Pending: {fleet.pending_upgrade_count}</span>
                  <span className="of-chip">Cells: {fleet.deployment_cells.length}</span>
                  <span className="of-chip">
                    Gates passed: {fleet.promotion_gate_summary.passed}/{fleet.promotion_gate_summary.total}
                  </span>
                  {fleet.promotion_gate_summary.blocking > 0 && (
                    <span className="of-chip" style={{ background: '#fef2f2', color: '#b91c1c' }}>
                      Blocking gates: {fleet.promotion_gate_summary.blocking}
                    </span>
                  )}
                  <span className="of-chip">{fleet.auto_upgrade_enabled ? 'Auto-upgrade on' : 'Manual upgrades'}</span>
                  <span className="of-chip">
                    Window: {fleet.maintenance_window.days.join(', ')} {fleet.maintenance_window.start_hour_utc}:00 UTC
                  </span>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  {fleet.workspace_targets.map((workspace) => (
                    <span key={workspace} className="of-chip" style={{ background: 'var(--bg-elevated)' }}>
                      {workspace}
                    </span>
                  ))}
                </div>
                {fleet.deployment_cells.length > 0 && (
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                    {fleet.deployment_cells.map((cell) => (
                      <span key={cell.name} className="of-chip" style={{ background: 'var(--bg-elevated)' }}>
                        {cell.name} · {cell.cloud}/{cell.region} · {cell.status}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            ))}
            {selectedListingFleets.length === 0 && (
              <div
                style={{
                  border: '1px dashed var(--border-default)',
                  borderRadius: 16,
                  padding: 32,
                  textAlign: 'center',
                  fontSize: 13,
                  color: 'var(--text-muted)',
                  background: 'var(--bg-elevated)',
                }}
              >
                No fleets yet for the selected listing.
              </div>
            )}
          </div>
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4', display: 'grid', gap: 14 }}>
          <div>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#6ee7b7' }}>
              Create fleet
            </p>
            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 14 }}>
              <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Fleet name</span>
                <input value={fleetDraft.name} onChange={(e) => onFleetDraftChange({ name: e.target.value })} style={darkInput} />
              </label>
              {(['environment', 'release_channel'] as const).map((field) => (
                <label key={field} style={{ fontSize: 13 }}>
                  <span style={{ display: 'block', marginBottom: 6, fontWeight: 500, textTransform: 'capitalize' }}>{field.replace(/_/g, ' ')}</span>
                  <input value={fleetDraft[field]} onChange={(e) => onFleetDraftChange({ [field]: e.target.value } as Partial<FleetDraft>)} style={darkInput} />
                </label>
              ))}
              <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Workspace targets</span>
                <input
                  value={fleetDraft.workspace_targets_text}
                  onChange={(e) => onFleetDraftChange({ workspace_targets_text: e.target.value })}
                  placeholder="Ops Center - EU, Ops Center - US"
                  style={darkInput}
                />
              </label>
              <label
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '10px 14px',
                  borderRadius: 16,
                  border: '1px solid #44403c',
                  background: '#1c1917',
                  fontSize: 13,
                  gridColumn: 'span 2',
                }}
              >
                <input
                  type="checkbox"
                  checked={fleetDraft.auto_upgrade_enabled}
                  onChange={(e) => onFleetDraftChange({ auto_upgrade_enabled: e.target.checked })}
                />
                Enable auto-upgrade during maintenance windows
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Maintenance days</span>
                <input
                  value={fleetDraft.maintenance_days_text}
                  onChange={(e) => onFleetDraftChange({ maintenance_days_text: e.target.value })}
                  placeholder="sun, wed"
                  style={darkInput}
                />
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Start hour UTC</span>
                <input value={fleetDraft.start_hour_utc} onChange={(e) => onFleetDraftChange({ start_hour_utc: e.target.value })} style={darkInput} />
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Duration minutes</span>
                <input value={fleetDraft.duration_minutes} onChange={(e) => onFleetDraftChange({ duration_minutes: e.target.value })} style={darkInput} />
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Branch strategy</span>
                <input value={fleetDraft.branch_strategy} onChange={(e) => onFleetDraftChange({ branch_strategy: e.target.value })} style={darkInput} />
              </label>
              <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Rollout strategy</span>
                <input value={fleetDraft.rollout_strategy} onChange={(e) => onFleetDraftChange({ rollout_strategy: e.target.value })} style={darkInput} />
              </label>
            </div>
            <button
              type="button"
              onClick={onCreateFleet}
              disabled={busy || !selectedListingId}
              className="of-button of-button--primary"
              style={{ marginTop: 14, background: '#34d399', color: '#0c0a09' }}
            >
              Create fleet for selected listing
            </button>
          </div>

          <div style={{ borderTop: '1px solid #44403c', paddingTop: 14 }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#fcd34d' }}>
              Enrollment branch
            </p>
            <div style={{ display: 'grid', gap: 12, marginTop: 14 }}>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Fleet</span>
                <select value={branchDraft.fleet_id} onChange={(e) => onBranchDraftChange({ fleet_id: e.target.value })} style={darkInput}>
                  <option value="">Select fleet</option>
                  {fleets.map((fleet) => (
                    <option key={fleet.id} value={fleet.id}>
                      {fleet.name} · {fleet.listing_name}
                    </option>
                  ))}
                </select>
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Branch name</span>
                <input value={branchDraft.name} onChange={(e) => onBranchDraftChange({ name: e.target.value })} style={darkInput} />
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Repository branch override</span>
                <input
                  value={branchDraft.repository_branch}
                  onChange={(e) => onBranchDraftChange({ repository_branch: e.target.value })}
                  placeholder="release/ops-center/feature-x"
                  style={darkInput}
                />
              </label>
              <label style={{ fontSize: 13 }}>
                <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Notes</span>
                <textarea
                  value={branchDraft.notes}
                  onChange={(e) => onBranchDraftChange({ notes: e.target.value })}
                  style={{ ...darkInput, minHeight: 90, resize: 'vertical' }}
                />
              </label>
            </div>
            <button
              type="button"
              onClick={onCreateBranch}
              disabled={busy || !branchDraft.fleet_id}
              className="of-button of-button--primary"
              style={{ marginTop: 14, background: '#fbbf24', color: '#0c0a09' }}
            >
              Create enrollment branch
            </button>
          </div>
        </div>
      </div>

      <div className="of-panel-muted" style={{ padding: 14, marginTop: 24 }}>
        <p className="of-eyebrow">Active enrollment branches</p>
        <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', marginTop: 10 }}>
          {branches.map((branch) => (
            <div key={branch.id} className="of-panel" style={{ padding: 14 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{branch.name}</p>
                  <p className="of-text-muted" style={{ fontSize: 13 }}>
                    {branch.fleet_name} · {branch.source_release_channel} · {branch.source_version ?? 'no version yet'}
                  </p>
                </div>
                <span className="of-chip" style={{ background: '#fffbeb', color: '#b45309', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                  {branch.status}
                </span>
              </div>
              <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
                {branch.repository_branch}
              </p>
              {branch.notes && (
                <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                  {branch.notes}
                </p>
              )}
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                {branch.workspace_targets.map((workspace) => (
                  <span key={workspace} className="of-chip">
                    {workspace}
                  </span>
                ))}
              </div>
            </div>
          ))}
          {branches.length === 0 && (
            <div
              style={{
                gridColumn: '1 / -1',
                border: '1px dashed var(--border-default)',
                borderRadius: 16,
                padding: 32,
                textAlign: 'center',
                fontSize: 13,
                color: 'var(--text-muted)',
                background: 'var(--bg-elevated)',
              }}
            >
              No enrollment branches created yet.
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
