import type { LayerDefinition } from '@/lib/api/geospatial';

interface LayerPanelProps {
  layers: LayerDefinition[];
  selectedLayerId: string;
  onSelectLayer: (layerId: string) => void;
}

export function LayerPanel({ layers, selectedLayerId, onSelectLayer }: LayerPanelProps) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <p className="of-eyebrow" style={{ color: '#0e7490' }}>
        Layer panel
      </p>
      <h3 className="of-heading-md" style={{ marginTop: 4 }}>
        Indexed layers and vector-ready sources
      </h3>
      <p className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
        Switch between point, polygon, and line layers to drive tiles, heatmaps, clustering, and
        routing.
      </p>

      <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
        {layers.map((layer) => (
          <button
            key={layer.id}
            type="button"
            onClick={() => onSelectLayer(layer.id)}
            style={{
              width: '100%',
              textAlign: 'left',
              padding: 16,
              background: selectedLayerId === layer.id ? '#ecfeff' : 'var(--bg-panel-muted)',
              border: `1px solid ${selectedLayerId === layer.id ? '#06b6d4' : 'var(--border-default)'}`,
              borderRadius: 'var(--radius-md)',
              cursor: 'pointer',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
              <div>
                <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{layer.name}</div>
                <div className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
                  {layer.source_kind} • {layer.geometry_type} • {layer.features.length} features
                </div>
              </div>
              <span
                className={`of-chip ${layer.indexed ? 'of-status-success' : ''}`}
                style={{
                  textTransform: 'uppercase',
                  letterSpacing: '0.18em',
                  fontSize: 11,
                  fontWeight: 600,
                }}
              >
                {layer.indexed ? 'Indexed' : 'Draft'}
              </span>
            </div>
            <p className="of-text-muted" style={{ fontSize: 13, marginTop: 12 }}>
              {layer.description}
            </p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
              {layer.tags.map((tag) => (
                <span key={tag} className="of-chip" style={{ background: '#fff', fontSize: 11 }}>
                  {tag}
                </span>
              ))}
            </div>
          </button>
        ))}
      </div>
    </section>
  );
}
