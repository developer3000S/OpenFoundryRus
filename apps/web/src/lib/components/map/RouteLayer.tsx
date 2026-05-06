import type { RouteResponse } from '@/lib/api/geospatial';

interface RouteLayerProps {
  routeResponse: RouteResponse | null;
}

export function RouteLayer({ routeResponse }: RouteLayerProps) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <p className="of-eyebrow" style={{ color: '#0e7490' }}>
        Routing
      </p>
      <h3 className="of-heading-md" style={{ marginTop: 4 }}>
        Shortest path and isochrone summary
      </h3>

      {routeResponse ? (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.1fr)', marginTop: 16 }}>
          <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 12 }}>
            {(
              [
                { label: 'Mode', value: routeResponse.mode },
                { label: 'Distance', value: `${routeResponse.distance_km.toFixed(1)} km` },
                { label: 'ETA', value: `${routeResponse.duration_min} min` },
              ] as const
            ).map((card) => (
              <div key={card.label} className="of-panel" style={{ padding: 12 }}>
                <p className="of-eyebrow" style={{ fontSize: 11 }}>
                  {card.label}
                </p>
                <p style={{ marginTop: 8, fontSize: 18, fontWeight: 600, color: 'var(--text-strong)' }}>
                  {card.value}
                </p>
              </div>
            ))}
          </div>

          <div className="of-panel-muted" style={{ padding: 16 }}>
            <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Isochrone anchors</div>
            <div style={{ display: 'grid', gap: 12, marginTop: 12 }}>
              {routeResponse.isochrone.map((point) => (
                <div key={point.label} className="of-panel" style={{ padding: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                    <div style={{ fontWeight: 500 }}>{point.label}</div>
                    <div className="of-text-muted" style={{ fontSize: 13 }}>
                      {point.coordinate.lat.toFixed(3)}, {point.coordinate.lon.toFixed(3)}
                    </div>
                  </div>
                  <div className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                    ETA target: {point.eta_minutes} minutes
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
          Compute a route to inspect polyline and isochrone outputs.
        </div>
      )}
    </section>
  );
}
