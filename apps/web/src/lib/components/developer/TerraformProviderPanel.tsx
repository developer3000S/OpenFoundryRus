import type { TerraformProviderSchema } from '@/lib/api/developer';

const EXAMPLE = `provider "openfoundry" {
  api_url   = "https://platform.openfoundry.local"
  token     = var.openfoundry_token
  workspace = "production"
}

resource "openfoundry_repository_integration" "github_widget_kit" {
  repository_id       = "0196839d-d210-7f8c-8a1d-7ab001030001"
  provider            = "github"
  external_project    = "foundry-widget-kit"
  sync_mode           = "bidirectional_mirror"
  ci_trigger_strategy = "github_actions"
}`;

interface TerraformProviderPanelProps {
  schema: TerraformProviderSchema | null;
  loading?: boolean;
  error?: string;
}

export function TerraformProviderPanel({ schema, loading = false, error = '' }: TerraformProviderPanelProps) {
  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div style={{ maxWidth: 720 }}>
          <p className="of-eyebrow" style={{ color: '#0284c7' }}>
            Terraform provider
          </p>
          <h2 className="of-heading-md" style={{ marginTop: 4 }}>
            Infrastructure-as-code surface
          </h2>
          <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13, lineHeight: 1.7 }}>
            The generated provider schema turns OpenFoundry resources into stable IaC primitives.
            Platform teams can version repository integrations, audit policies, and Nexus peers the
            same way they manage networks, databases, and queues.
          </p>
        </div>
        <div className="of-status-info" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          <div style={{ fontWeight: 600 }}>Generator</div>
          <div style={{ marginTop: 4 }}>just terraform-schema</div>
        </div>
      </div>

      {loading ? (
        <div style={{ marginTop: 24, fontSize: 13, color: 'var(--text-muted)' }}>Loading provider schema…</div>
      ) : error ? (
        <div style={{ marginTop: 24, fontSize: 13, color: 'var(--status-danger)' }}>{error}</div>
      ) : !schema ? (
        <div style={{ marginTop: 24, fontSize: 13, color: 'var(--text-muted)' }}>
          The provider schema has not been generated yet.
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 24, gridTemplateColumns: '0.95fr 1.05fr', marginTop: 24 }}>
          <div style={{ display: 'grid', gap: 16 }}>
            <div className="of-panel-muted" style={{ padding: 16 }}>
              <p className="of-eyebrow">Provider configuration</p>
              <div style={{ display: 'grid', gap: 12, marginTop: 12 }}>
                {Object.entries(schema.provider.configuration).map(([name, description]) => (
                  <div key={name} className="of-panel" style={{ padding: 12 }}>
                    <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{name}</div>
                    <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                      {description}
                    </p>
                  </div>
                ))}
              </div>
            </div>

            <div className="of-panel-muted" style={{ padding: 16 }}>
              <p className="of-eyebrow">Example</p>
              <pre style={{ marginTop: 12, overflowX: 'auto', background: '#0c0a09', color: '#f5f5f4', padding: 12, borderRadius: 'var(--radius-md)', fontSize: 11, fontFamily: 'var(--font-mono)' }}>
                {EXAMPLE}
              </pre>
            </div>
          </div>

          <div style={{ display: 'grid', gap: 16 }}>
            {[
              { label: 'Resources', items: schema.resources },
              { label: 'Data sources', items: schema.data_sources },
            ].map((section) => (
              <div key={section.label} className="of-panel-muted" style={{ padding: 16 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                  <p className="of-eyebrow">{section.label}</p>
                  <p className="of-text-muted" style={{ fontSize: 13 }}>
                    {section.items.length}{' '}
                    {section.label === 'Resources' ? 'managed surfaces' : 'exported views'}
                  </p>
                </div>
                <div style={{ display: 'grid', gap: 12, marginTop: 12 }}>
                  {section.items.map((entry) => (
                    <div key={entry.name} className="of-panel" style={{ padding: 12 }}>
                      <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-strong)' }}>
                        {entry.name}
                      </div>
                      <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                        {entry.description}
                      </p>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
                        {Object.entries(entry.attributes).map(([name, description]) => (
                          <span key={name} className="of-chip" title={description as string}>
                            {name}
                          </span>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </section>
  );
}
