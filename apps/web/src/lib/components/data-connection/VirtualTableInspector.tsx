import { capabilityChips, defaultCapabilities, tableTypeLabel, type DiscoveredEntry, type TableType, type VirtualTableProvider } from '@/lib/api/virtual-tables';

interface Props {
  provider: VirtualTableProvider;
  entry: DiscoveredEntry | null;
  onCreate?: (entry: DiscoveredEntry) => void;
}

function inferredOrFallback(entry: DiscoveredEntry): TableType {
  return entry.inferred_table_type ?? 'OTHER';
}

export function VirtualTableInspector({ provider, entry, onCreate }: Props) {
  if (!entry) {
    return (
      <aside style={inspectorStyle}>
        <div style={mutedStyle}>Select an entry on the left to preview its capabilities and schema before registering.</div>
      </aside>
    );
  }

  const tableType = inferredOrFallback(entry);

  return (
    <aside style={inspectorStyle}>
      <header>
        <h3 style={{ margin: '0 0 4px', fontSize: 16 }}>{entry.display_name}</h3>
        <code style={{ fontFamily: 'ui-monospace, SFMono-Regular, monospace', fontSize: 12, color: '#4b5563' }}>{entry.path}</code>
      </header>
      <dl style={{ display: 'grid', gridTemplateColumns: 'max-content 1fr', gap: '4px 12px', margin: 0 }}>
        <dt style={{ color: '#6b7280', fontSize: 12 }}>Kind</dt>
        <dd style={{ margin: 0, fontSize: 14 }}>{entry.kind.replace('_', ' ')}</dd>
        <dt style={{ color: '#6b7280', fontSize: 12 }}>Inferred type</dt>
        <dd style={{ margin: 0, fontSize: 14 }}>{tableTypeLabel(tableType)}</dd>
      </dl>

      {entry.registrable ? (
        <>
          {(() => {
            const caps = defaultCapabilities(provider, tableType);
            return (
              <>
                <section>
                  <h4 style={h4Style}>Capabilities (preview)</h4>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                    {capabilityChips(caps).map((chip) => (
                      <span key={chip} style={chipStyle}>{chip}</span>
                    ))}
                  </div>
                </section>
                <section>
                  <h4 style={h4Style}>Foundry compute</h4>
                  <ul style={{ listStyle: 'none', padding: 0, margin: 0, fontSize: 14 }}>
                    <li>Python single-node: {caps.foundry_compute.python_single_node ? '✓' : '✗'}</li>
                    <li>Python Spark: {caps.foundry_compute.python_spark ? '✓' : '✗'}</li>
                    <li>Pipeline Builder Spark: {caps.foundry_compute.pipeline_builder_spark ? '✓' : '✗'}</li>
                  </ul>
                </section>
                <button type="button" onClick={() => onCreate?.(entry)} style={ctaStyle}>Register virtual table</button>
              </>
            );
          })()}
        </>
      ) : (
        <p style={mutedStyle}>Drill in to a leaf table to register a virtual table from this location.</p>
      )}
    </aside>
  );
}

const inspectorStyle: React.CSSProperties = {
  border: '1px solid #e5e7eb',
  borderRadius: 8,
  background: '#fff',
  padding: 16,
  minHeight: 360,
  display: 'flex',
  flexDirection: 'column',
  gap: 12,
};

const h4Style: React.CSSProperties = { fontSize: 12, textTransform: 'uppercase', color: '#4b5563', margin: '0 0 4px', letterSpacing: '0.05em' };
const chipStyle: React.CSSProperties = { display: 'inline-block', padding: '1px 6px', fontSize: 12, borderRadius: 4, background: '#f3f4f6', border: '1px solid #e5e7eb' };
const ctaStyle: React.CSSProperties = { marginTop: 'auto', padding: '8px 12px', background: '#1d4ed8', color: '#fff', border: 'none', borderRadius: 4, fontSize: 14, cursor: 'pointer' };
const mutedStyle: React.CSSProperties = { color: '#6b7280', fontSize: 14 };
