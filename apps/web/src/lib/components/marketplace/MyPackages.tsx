import type { InstallRecord } from '@/lib/api/marketplace';

interface Props {
  installs: InstallRecord[];
}

export function MyPackages({ installs }: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div>
        <p className="of-eyebrow" style={{ color: '#047857' }}>
          Installed Packages
        </p>
        <h3 className="of-heading-md" style={{ marginTop: 6 }}>
          Workspace rollout history
        </h3>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
          Track completed installs and their dependency plans across workspaces.
        </p>
      </div>

      <div style={{ display: 'grid', gap: 10, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', marginTop: 18 }}>
        {installs.map((install) => (
          <div key={install.id} className="of-panel-muted" style={{ padding: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
              <div>
                <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{install.listing_name}</p>
                <p className="of-text-muted" style={{ fontSize: 13 }}>
                  {install.workspace_name} · {install.version} · {install.release_channel}
                </p>
              </div>
              <span className="of-chip" style={{ background: '#ecfdf5', color: '#047857', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                {install.status}
              </span>
            </div>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
              {install.fleet_name && <span className="of-chip">Fleet: {install.fleet_name}</span>}
              {install.enrollment_branch && <span className="of-chip">Branch: {install.enrollment_branch}</span>}
              {install.auto_upgrade_enabled && <span className="of-chip">Auto-upgrade</span>}
              {install.maintenance_window && (
                <span className="of-chip">
                  Window: {install.maintenance_window.days.join(', ')} {install.maintenance_window.start_hour_utc}:00 UTC
                </span>
              )}
            </div>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
              {install.dependency_plan.map((dependency, index) => (
                <span key={index} className="of-chip">
                  {dependency.package_slug} {dependency.version_req}
                </span>
              ))}
            </div>
            {install.activation && (
              <div className="of-panel" style={{ padding: 10, marginTop: 10, border: '1px solid #a7f3d0' }}>
                <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>Activation</div>
                <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                  {install.activation.kind} · {install.activation.status}
                </div>
                {install.activation.public_url && (
                  <a href={install.activation.public_url} style={{ marginTop: 8, display: 'inline-flex', color: '#047857', fontSize: 13 }}>
                    Open runtime
                  </a>
                )}
                {install.activation.notes && (
                  <p className="of-text-muted" style={{ marginTop: 8, fontSize: 11 }}>
                    {install.activation.notes}
                  </p>
                )}
              </div>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}
