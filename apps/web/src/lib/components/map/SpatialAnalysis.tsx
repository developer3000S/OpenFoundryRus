import type {
  ClusterAlgorithm,
  ClusterResponse,
  GeocodeResponse,
  LayerDefinition,
  RouteMode,
  SpatialOperation,
  SpatialQueryResponse,
} from '@/lib/api/geospatial';

export interface AnalysisDraft {
  operation: SpatialOperation;
  point_lat: string;
  point_lon: string;
  radius_km: number;
  limit: number;
  cluster_algorithm: ClusterAlgorithm;
  cluster_count: number;
  cluster_radius_km: number;
  geocode_query: string;
  origin_lat: string;
  origin_lon: string;
  destination_lat: string;
  destination_lon: string;
  route_mode: RouteMode;
  route_max_minutes: number;
}

const OPERATIONS: SpatialOperation[] = ['within', 'intersects', 'nearest', 'buffer'];
const ALGORITHMS: ClusterAlgorithm[] = ['dbscan', 'kmeans'];
const ROUTE_MODES: RouteMode[] = ['drive', 'bike', 'walk'];

interface SpatialAnalysisProps {
  selectedLayer: LayerDefinition | null;
  draft: AnalysisDraft;
  busy?: boolean;
  queryResponse: SpatialQueryResponse | null;
  clusterResponse: ClusterResponse | null;
  geocodeResponse: GeocodeResponse | null;
  reverseGeocodeResponse: GeocodeResponse | null;
  onDraftChange: (patch: Partial<AnalysisDraft>) => void;
  onRunQuery: () => void;
  onRunCluster: () => void;
  onRunRoute: () => void;
  onGeocode: () => void;
  onReverseGeocode: () => void;
}

export function SpatialAnalysis({
  selectedLayer,
  draft,
  busy = false,
  queryResponse,
  clusterResponse,
  geocodeResponse,
  reverseGeocodeResponse,
  onDraftChange,
  onRunQuery,
  onRunCluster,
  onRunRoute,
  onGeocode,
  onReverseGeocode,
}: SpatialAnalysisProps) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div>
            <p className="of-eyebrow" style={{ color: '#0e7490' }}>
              Spatial analysis
            </p>
            <h3 className="of-heading-md" style={{ marginTop: 4 }}>
              Queries, clustering, geocoding, and routing
            </h3>
            <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
              {selectedLayer
                ? `${selectedLayer.name} is the active layer for spatial analysis.`
                : 'Select a layer to enable spatial analysis controls.'}
            </p>
          </div>
          {geocodeResponse && (
            <div
              className="of-status-info"
              style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13, fontWeight: 500 }}
            >
              <div>{geocodeResponse.address}</div>
              <div style={{ marginTop: 4, fontSize: 12 }}>
                {geocodeResponse.coordinate.lat.toFixed(4)}, {geocodeResponse.coordinate.lon.toFixed(4)} •{' '}
                {Math.round(geocodeResponse.confidence * 100)}%
              </div>
            </div>
          )}
        </div>

        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))' }}>
          {/* Spatial query column */}
          <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 12 }}>
            <div style={{ fontWeight: 600, fontSize: 14 }}>Spatial query</div>
            <Field label="Operation">
              <select
                className="of-select"
                value={draft.operation}
                onChange={(e) => onDraftChange({ operation: e.target.value as SpatialOperation })}
              >
                {OPERATIONS.map((op) => (
                  <option key={op} value={op}>
                    {op}
                  </option>
                ))}
              </select>
            </Field>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
              <Field label="Latitude">
                <input
                  className="of-input"
                  value={draft.point_lat}
                  onChange={(e) => onDraftChange({ point_lat: e.target.value })}
                />
              </Field>
              <Field label="Longitude">
                <input
                  className="of-input"
                  value={draft.point_lon}
                  onChange={(e) => onDraftChange({ point_lon: e.target.value })}
                />
              </Field>
              <Field label="Radius km">
                <input
                  type="number"
                  className="of-input"
                  value={draft.radius_km}
                  onChange={(e) => onDraftChange({ radius_km: Number(e.target.value) })}
                />
              </Field>
              <Field label="Limit">
                <input
                  type="number"
                  className="of-input"
                  value={draft.limit}
                  onChange={(e) => onDraftChange({ limit: Number(e.target.value) })}
                />
              </Field>
            </div>
            <button
              type="button"
              className="of-btn of-btn-primary"
              onClick={onRunQuery}
              disabled={busy || !selectedLayer}
            >
              Run spatial query
            </button>
            {queryResponse && (
              <p className="of-text-muted" style={{ fontSize: 13 }}>
                {queryResponse.summary.matched_count} matches in {queryResponse.summary.query_time_ms} ms.
              </p>
            )}
          </div>

          {/* Clustering + geocoding column */}
          <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 12 }}>
            <div style={{ fontWeight: 600, fontSize: 14 }}>Clustering + geocoding</div>
            <Field label="Algorithm">
              <select
                className="of-select"
                value={draft.cluster_algorithm}
                onChange={(e) => onDraftChange({ cluster_algorithm: e.target.value as ClusterAlgorithm })}
              >
                {ALGORITHMS.map((alg) => (
                  <option key={alg} value={alg}>
                    {alg}
                  </option>
                ))}
              </select>
            </Field>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
              <Field label="Cluster count">
                <input
                  type="number"
                  className="of-input"
                  value={draft.cluster_count}
                  onChange={(e) => onDraftChange({ cluster_count: Number(e.target.value) })}
                />
              </Field>
              <Field label="Radius km">
                <input
                  type="number"
                  className="of-input"
                  value={draft.cluster_radius_km}
                  onChange={(e) => onDraftChange({ cluster_radius_km: Number(e.target.value) })}
                />
              </Field>
            </div>
            <button
              type="button"
              className="of-btn"
              onClick={onRunCluster}
              disabled={busy || !selectedLayer}
            >
              Run clustering
            </button>
            <Field label="Address search">
              <input
                className="of-input"
                value={draft.geocode_query}
                onChange={(e) => onDraftChange({ geocode_query: e.target.value })}
                placeholder="Madrid, Barcelona, Berlin"
              />
            </Field>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
              <button type="button" className="of-btn" onClick={onGeocode} disabled={busy}>
                Geocode
              </button>
              <button type="button" className="of-btn" onClick={onReverseGeocode} disabled={busy}>
                Reverse geocode
              </button>
            </div>
            {clusterResponse && (
              <p className="of-text-muted" style={{ fontSize: 13 }}>
                {clusterResponse.clusters.length} clusters, {clusterResponse.outliers} outliers.
              </p>
            )}
            {reverseGeocodeResponse && (
              <p className="of-text-muted" style={{ fontSize: 13 }}>
                Nearest address: {reverseGeocodeResponse.address}
              </p>
            )}
          </div>

          {/* Routing column */}
          <div className="of-panel-muted" style={{ padding: 16, display: 'grid', gap: 12 }}>
            <div style={{ fontWeight: 600, fontSize: 14 }}>Routing</div>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
              <Field label="Origin lat">
                <input
                  className="of-input"
                  value={draft.origin_lat}
                  onChange={(e) => onDraftChange({ origin_lat: e.target.value })}
                />
              </Field>
              <Field label="Origin lon">
                <input
                  className="of-input"
                  value={draft.origin_lon}
                  onChange={(e) => onDraftChange({ origin_lon: e.target.value })}
                />
              </Field>
              <Field label="Destination lat">
                <input
                  className="of-input"
                  value={draft.destination_lat}
                  onChange={(e) => onDraftChange({ destination_lat: e.target.value })}
                />
              </Field>
              <Field label="Destination lon">
                <input
                  className="of-input"
                  value={draft.destination_lon}
                  onChange={(e) => onDraftChange({ destination_lon: e.target.value })}
                />
              </Field>
            </div>
            <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
              <Field label="Mode">
                <select
                  className="of-select"
                  value={draft.route_mode}
                  onChange={(e) => onDraftChange({ route_mode: e.target.value as RouteMode })}
                >
                  {ROUTE_MODES.map((mode) => (
                    <option key={mode} value={mode}>
                      {mode}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Max minutes">
                <input
                  type="number"
                  className="of-input"
                  value={draft.route_max_minutes}
                  onChange={(e) => onDraftChange({ route_max_minutes: Number(e.target.value) })}
                />
              </Field>
            </div>
            <button type="button" className="of-btn of-btn-primary" onClick={onRunRoute} disabled={busy}>
              Compute route
            </button>
          </div>
        </div>
      </div>
    </section>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label style={{ display: 'block', fontSize: 13 }}>
      <div className="of-eyebrow" style={{ marginBottom: 6 }}>
        {label}
      </div>
      {children}
    </label>
  );
}
