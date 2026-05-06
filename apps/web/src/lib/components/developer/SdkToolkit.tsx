const ARCHETYPES = [
  {
    name: 'Connector plugin',
    description:
      'Wrap an external system, validate credentials, and expose sync-ready metadata through the Rust + WASM SDK.',
    command: 'cargo run -p of-cli -- project init payment-connector --template connector --output plugins',
  },
  {
    name: 'Transform plugin',
    description:
      'Package deterministic row or document transforms that can run inside pipeline or notebook execution surfaces.',
    command: 'cargo run -p of-cli -- project init pii-redactor --template transform --output plugins',
  },
  {
    name: 'Widget plugin',
    description:
      'Ship a presentation layer component with manifest metadata ready for app builder and marketplace flows.',
    command: 'cargo run -p of-cli -- project init telemetry-widget --template widget --output plugins',
  },
];

const COMMAND_DECK = [
  'cargo run -p of-cli -- deploy plan gateway --environment staging',
  'cargo run -p of-cli -- script render "deploy {{service}} to {{env}}" --var service=gateway --var env=prod',
  'cargo run -p of-cli -- docs generate-openapi --output apps/web/static/generated/openapi/openfoundry.json',
  'cargo run -p of-cli -- docs generate-sdk-typescript --input apps/web/static/generated/openapi/openfoundry.json --output sdks/typescript/openfoundry-sdk',
  'cargo run -p of-cli -- docs generate-sdk-python --input apps/web/static/generated/openapi/openfoundry.json --output sdks/python/openfoundry-sdk',
  'cargo run -p of-cli -- docs generate-sdk-java --input apps/web/static/generated/openapi/openfoundry.json --output sdks/java/openfoundry-sdk',
  'cargo run -p of-cli -- terraform schema --output apps/web/static/generated/terraform/openfoundry-provider.json',
];

const COOKBOOKS = [
  {
    title: 'Mirror GitHub packages into Code Repos',
    focus: 'Set up a bidirectional sync, map release branches, and wire GitHub Actions triggers to repository integrations.',
  },
  {
    title: 'Generate API contracts for SDK consumers',
    focus: 'Regenerate the OpenAPI document and the official TypeScript, Python, and Java SDKs from the same checked-in contract.',
  },
  {
    title: 'Codify audit and Nexus surfaces with Terraform',
    focus: 'Manage audit policies, repository integrations, and cross-org peers alongside environment provisioning.',
  },
  {
    title: 'Bootstrap a WASM widget package',
    focus: 'Scaffold a new widget crate, fill in the manifest, and prepare the distribution assets consumed by app builder.',
  },
];

export function SdkToolkit() {
  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
        <div style={{ maxWidth: 720 }}>
          <p className="of-eyebrow" style={{ color: '#d97706' }}>
            SDK + CLI
          </p>
          <h2 className="of-heading-md" style={{ marginTop: 4 }}>
            Build plugins and automate delivery
          </h2>
          <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13, lineHeight: 1.7 }}>
            The plugin-sdk crate standardizes Rust + WASM manifests for connectors, transforms, and
            widgets. The of CLI scaffolds projects, renders deployment scripts, emits proto-derived
            docs, and generates the official TypeScript, Python, and Java SDKs.
          </p>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(3, 1fr)', marginTop: 20 }}>
        {ARCHETYPES.map((archetype) => (
          <div key={archetype.name} className="of-panel-muted" style={{ padding: 16 }}>
            <div style={{ fontSize: 16, fontWeight: 600, color: 'var(--text-strong)' }}>{archetype.name}</div>
            <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
              {archetype.description}
            </p>
            <pre
              style={{
                marginTop: 12,
                overflowX: 'auto',
                background: '#0c0a09',
                color: '#f5f5f4',
                padding: 12,
                borderRadius: 'var(--radius-md)',
                fontSize: 11,
                fontFamily: 'var(--font-mono)',
              }}
            >
              {archetype.command}
            </pre>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1.1fr 0.9fr', marginTop: 20 }}>
        <div className="of-panel-muted" style={{ padding: 16 }}>
          <p className="of-eyebrow">CLI cookbook</p>
          <div style={{ display: 'grid', gap: 8, marginTop: 12 }}>
            {COMMAND_DECK.map((command) => (
              <pre
                key={command}
                style={{
                  margin: 0,
                  overflowX: 'auto',
                  background: '#0c0a09',
                  color: '#f5f5f4',
                  padding: 12,
                  borderRadius: 'var(--radius-md)',
                  fontSize: 11,
                  fontFamily: 'var(--font-mono)',
                }}
              >
                {command}
              </pre>
            ))}
          </div>
        </div>
        <div className="of-panel-muted" style={{ padding: 16 }}>
          <p className="of-eyebrow">Cookbooks</p>
          <div style={{ display: 'grid', gap: 8, marginTop: 12 }}>
            {COOKBOOKS.map((cookbook) => (
              <div key={cookbook.title} className="of-panel" style={{ padding: 12 }}>
                <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{cookbook.title}</div>
                <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                  {cookbook.focus}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
