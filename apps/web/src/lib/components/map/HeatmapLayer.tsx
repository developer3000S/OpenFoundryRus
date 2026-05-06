import type {
  ClusterResponse,
  SpatialQueryResponse,
  VectorTileResponse,
} from '@/lib/api/geospatial';

interface HeatmapLayerProps {
  tile: VectorTileResponse | null;
  queryResponse?: SpatialQueryResponse | null;
  clusterResponse?: ClusterResponse | null;
}

export function HeatmapLayer({ tile, queryResponse = null, clusterResponse = null }: HeatmapLayerProps) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <p className="of-eyebrow" style={{ color: '#0e7490' }}>
        Heatmap + tiles
      </p>
      <h3 className="of-heading-md" style={{ marginTop: 4 }}>
        Vector tile summary and H3-like aggregation
      </h3>

      {tile ? (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.85fr) minmax(0, 1.15fr)', marginTop: 16 }}>
          <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 16 }}>
            <div>
              <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{tile.layer_name}</div>
              <div className="of-text-muted" style={{ fontSize: 13 }}>
                {tile.format.toUpperCase()} • zoom {tile.zoom_range[0]}-{tile.zoom_range[1]} • {tile.feature_count} features
              </div>
            </div>
            <div className="of-panel" style={{ padding: 12, fontSize: 13, color: 'var(--text-muted)' }}>
              <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Tile template</div>
              <div style={{ marginTop: 6, wordBreak: 'break-all', fontSize: 11, fontFamily: 'var(--font-mono)' }}>
                {tile.tile_url_template}
              </div>
            </div>
            {queryResponse && (
              <div className="of-panel" style={{ padding: 12, fontSize: 13, color: 'var(--text-muted)' }}>
                <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Latest query</div>
                <div style={{ marginTop: 6 }}>
                  {queryResponse.summary.matched_count} matches in {queryResponse.summary.query_time_ms} ms.
                </div>
              </div>
            )}
            {clusterResponse && (
              <div className="of-panel" style={{ padding: 12, fontSize: 13, color: 'var(--text-muted)' }}>
                <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Latest clusters</div>
                <div style={{ marginTop: 6 }}>
                  {clusterResponse.clusters.length} clusters • {clusterResponse.outliers} outliers
                </div>
              </div>
            )}
          </div>

          <div className="of-panel-muted" style={{ padding: 16 }}>
            <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Top hex bins</div>
            <div style={{ display: 'grid', gap: 12, marginTop: 12 }}>
              {tile.h3_bins.slice(0, 8).map((bin) => (
                <div key={bin.cell_id} className="of-panel" style={{ padding: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                    <div>
                      <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{bin.cell_id}</div>
                      <div className="of-text-muted" style={{ fontSize: 13 }}>
                        {bin.centroid.lat.toFixed(3)}, {bin.centroid.lon.toFixed(3)}
                      </div>
                    </div>
                    <span
                      className="of-chip of-status-info"
                      style={{ textTransform: 'uppercase', letterSpacing: '0.18em', fontSize: 11, fontWeight: 600 }}
                    >
                      {bin.count} features
                    </span>
                  </div>
                  <div
                    style={{
                      marginTop: 12,
                      height: 8,
                      borderRadius: 999,
                      background: 'var(--bg-chip)',
                      overflow: 'hidden',
                    }}
                  >
                    <div
                      style={{
                        height: '100%',
                        width: `${Math.min(bin.intensity * 20, 100)}%`,
                        background: '#0891b2',
                        borderRadius: 999,
                      }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      ) : (
        <div
          style={{
            marginTop: 16,
            padding: 32,
            border: '1px dashed var(--border-default)',
            borderRadius: 'var(--radius-md)',
            background: 'var(--bg-panel-muted)',
            textAlign: 'center',
            fontSize: 13,
            color: 'var(--text-muted)',
          }}
        >
          Select a layer to inspect vector tile metadata and heatmap bins.
        </div>
      )}
    </section>
  );
}
