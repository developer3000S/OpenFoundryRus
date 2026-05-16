import { useCallback, useEffect, useRef, useState } from 'react';
import type { Map as MapLibreMap, StyleSpecification } from 'maplibre-gl';
import { MapboxOverlay } from '@deck.gl/mapbox';
import { ScatterplotLayer } from '@deck.gl/layers';

import { MapLibreCanvas } from '@components/MapLibreCanvas';

interface Aircraft {
  icao24: string;
  callsign: string;
  country: string;
  lon: number;
  lat: number;
  velocity: number | null;
  heading: number | null;
  altitude: number | null;
  onGround: boolean;
}

interface OpenSkyResponse {
  time: number;
  states: Array<Array<unknown> | null> | null;
}

interface PollState {
  aircraft: Aircraft[];
  lastUpdate: number | null;
  error: string | null;
  loading: boolean;
}

const OSM_STYLE: StyleSpecification = {
  version: 8,
  sources: {
    osm: {
      type: 'raster',
      tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
      tileSize: 256,
      attribution: '© OpenStreetMap contributors',
      maxzoom: 19,
    },
  },
  layers: [{ id: 'osm', type: 'raster', source: 'osm' }],
};

// OpenSky /states/all response: each state is a positional tuple.
// Schema: https://openskynetwork.github.io/opensky-api/rest.html#response
function parseState(state: Array<unknown> | null): Aircraft | null {
  if (!state) return null;
  const lon = state[5];
  const lat = state[6];
  if (typeof lon !== 'number' || typeof lat !== 'number') return null;
  return {
    icao24: typeof state[0] === 'string' ? state[0] : '',
    callsign: typeof state[1] === 'string' ? state[1].trim() : '',
    country: typeof state[2] === 'string' ? state[2] : '',
    lon,
    lat,
    velocity: typeof state[9] === 'number' ? state[9] : null,
    heading: typeof state[10] === 'number' ? state[10] : null,
    altitude: typeof state[7] === 'number' ? state[7] : null,
    onGround: state[8] === true,
  };
}

// Color a dot by altitude band: red on-ground / low → blue cruising → purple high.
function altitudeColor(a: Aircraft): [number, number, number, number] {
  if (a.onGround) return [220, 38, 38, 220]; // red
  const alt = a.altitude ?? 0;
  if (alt < 3000) return [234, 88, 12, 230]; // orange (climb/descent)
  if (alt < 9000) return [37, 99, 235, 230]; // blue (mid-cruise)
  return [124, 58, 237, 230]; // purple (high cruise)
}

const POLL_INTERVAL_MS = 30_000;

export function GlobeFlightsPage() {
  const mapRef = useRef<MapLibreMap | null>(null);
  const overlayRef = useRef<MapboxOverlay | null>(null);
  const [poll, setPoll] = useState<PollState>({
    aircraft: [],
    lastUpdate: null,
    error: null,
    loading: true,
  });
  const [selected, setSelected] = useState<Aircraft | null>(null);
  // Keep the click handler stable across re-renders so the layer's
  // onClick reference doesn't churn between polls.
  const selectedRef = useRef<Aircraft | null>(null);
  selectedRef.current = selected;

  const fetchAircraft = useCallback(async (signal: AbortSignal) => {
    setPoll((prev) => ({ ...prev, loading: true }));
    try {
      const res = await fetch('/external/opensky/api/states/all', { signal });
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      const data = (await res.json()) as OpenSkyResponse;
      const aircraft = (data.states ?? [])
        .map(parseState)
        .filter((a): a is Aircraft => a !== null);
      setPoll({
        aircraft,
        lastUpdate: data.time * 1000,
        error: null,
        loading: false,
      });
    } catch (err) {
      if ((err as Error).name === 'AbortError') return;
      setPoll((prev) => ({
        ...prev,
        loading: false,
        error: (err as Error).message,
      }));
    }
  }, []);

  // Polling loop
  useEffect(() => {
    const controller = new AbortController();
    void fetchAircraft(controller.signal);
    const id = window.setInterval(() => {
      void fetchAircraft(controller.signal);
    }, POLL_INTERVAL_MS);
    return () => {
      controller.abort();
      window.clearInterval(id);
    };
  }, [fetchAircraft]);

  const handleMapLoad = useCallback((map: MapLibreMap) => {
    mapRef.current = map;
    // NOTE: MapLibre's globe projection + deck.gl's MapboxOverlay don't
    // share a projection state — overlay positions in flat Mercator while
    // the basemap renders as a sphere, so the dots end up off-canvas.
    // For a true globe + live data view we'd need deck.gl's _GlobeView with
    // an OSM BitmapLayer rather than MapLibre. Sticking to flat Mercator
    // here keeps the demo correct.
    const overlay = new MapboxOverlay({
      layers: [],
      getTooltip: ({ object }) => {
        if (!object) return null;
        const a = object as Aircraft;
        const km = a.altitude != null ? (a.altitude / 1000).toFixed(1) : '—';
        const kmh = a.velocity != null ? Math.round(a.velocity * 3.6) : null;
        const hdg = a.heading != null ? Math.round(a.heading) : null;
        const title = a.callsign || a.icao24 || 'unknown';
        return {
          html: `<div style="font-family: system-ui, sans-serif; font-size: 12px; line-height: 1.4;">
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 4px;">${title}</div>
            <div style="color: #cbd5e1;">${a.country || 'unknown origin'}</div>
            <div style="margin-top: 4px;">${a.onGround ? 'On ground' : `Altitude: ${km} km`}</div>
            ${kmh != null ? `<div>Speed: ${kmh} km/h</div>` : ''}
            ${hdg != null ? `<div>Heading: ${hdg}°</div>` : ''}
            <div style="color: #94a3b8; margin-top: 4px;">ICAO24: ${a.icao24 || '—'}</div>
          </div>`,
          style: {
            backgroundColor: '#0f172a',
            color: '#f1f5f9',
            padding: '8px 12px',
            borderRadius: '6px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.3)',
            border: '1px solid #334155',
            pointerEvents: 'none',
          },
        };
      },
    });
    // @ts-expect-error MapLibre's addControl typing is loose at the wrapper boundary
    map.addControl(overlay);
    overlayRef.current = overlay;
  }, []);

  // Keep the pinned aircraft in sync with the latest poll — refresh
  // its position / altitude / speed when a new tick arrives.
  useEffect(() => {
    const pinned = selectedRef.current;
    if (!pinned) return;
    const fresh = poll.aircraft.find((a) => a.icao24 === pinned.icao24);
    if (fresh && fresh !== pinned) {
      setSelected(fresh);
    }
  }, [poll.aircraft]);

  // Push the current aircraft list into the deck.gl overlay whenever data changes.
  useEffect(() => {
    const overlay = overlayRef.current;
    if (!overlay) return;
    const layer = new ScatterplotLayer<Aircraft>({
      id: 'aircraft',
      data: poll.aircraft,
      getPosition: (a) => [a.lon, a.lat],
      getRadius: 18000,
      getFillColor: altitudeColor,
      radiusMinPixels: 4,
      radiusMaxPixels: 12,
      stroked: true,
      lineWidthMinPixels: 0.5,
      getLineColor: [255, 255, 255, 200],
      pickable: true,
      autoHighlight: true,
      onClick: ({ object }) => {
        setSelected((object as Aircraft) ?? null);
      },
    });
    overlay.setProps({ layers: [layer] });
  }, [poll.aircraft]);

  const lastUpdateText = poll.lastUpdate
    ? `Updated ${new Date(poll.lastUpdate).toLocaleTimeString()}`
    : poll.loading
      ? 'Loading…'
      : '—';

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header>
        <p className="of-eyebrow">Live data overlay</p>
        <h1 className="of-heading-xl">Live flights (OpenSky Network)</h1>
        <p className="of-text-muted" style={{ marginTop: 8, maxWidth: 760 }}>
          Real-time ADS-B positions from <code>opensky-network.org</code>, rendered as a
          deck.gl <code>ScatterplotLayer</code> over a MapLibre OSM basemap. Polls every
          {' '}
          {POLL_INTERVAL_MS / 1000}s. Color reflects altitude band: red = on-ground,
          orange = climb/descent, blue = mid-cruise, purple = high cruise. (Flat
          Mercator only for now — deck.gl's <code>MapboxOverlay</code> doesn't share a
          projection state with MapLibre's globe mode.)
        </p>
      </header>

      <div className="of-panel" style={{ padding: 20 }}>
        <div
          className="of-toolbar"
          data-testid="globe-flights-status"
          style={{ marginBottom: 16, display: 'flex', gap: 16, alignItems: 'center' }}
        >
          <span style={{ fontWeight: 600 }}>
            Tracking <span data-testid="aircraft-count">{poll.aircraft.length}</span>{' '}
            aircraft
          </span>
          <span className="of-text-muted" style={{ fontSize: 13 }}>
            {lastUpdateText}
          </span>
          {poll.error ? (
            <span style={{ color: '#dc2626', fontSize: 13 }} data-testid="globe-flights-error">
              error: {poll.error}
            </span>
          ) : null}
        </div>

        <div style={{ position: 'relative' }}>
          <MapLibreCanvas
            height={620}
            style={OSM_STYLE}
            center={[0, 25]}
            zoom={1.3}
            onMapLoad={handleMapLoad}
          />
          {selected ? (
            <aside
              data-testid="aircraft-detail-panel"
              style={{
                position: 'absolute',
                top: 12,
                right: 12,
                width: 260,
                padding: '14px 16px',
                backgroundColor: 'rgba(15, 23, 42, 0.92)',
                color: '#f1f5f9',
                borderRadius: 8,
                boxShadow: '0 6px 18px rgba(0,0,0,0.35)',
                border: '1px solid rgba(148, 163, 184, 0.3)',
                fontSize: 13,
                lineHeight: 1.5,
                backdropFilter: 'blur(6px)',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 8 }}>
                <strong style={{ fontSize: 15 }} data-testid="selected-callsign">
                  {selected.callsign || selected.icao24 || 'unknown'}
                </strong>
                <button
                  type="button"
                  onClick={() => setSelected(null)}
                  style={{
                    background: 'transparent',
                    color: '#cbd5e1',
                    border: 'none',
                    cursor: 'pointer',
                    fontSize: 18,
                    lineHeight: 1,
                    padding: 0,
                  }}
                  aria-label="Close aircraft details"
                >
                  ×
                </button>
              </div>
              <div style={{ color: '#cbd5e1', marginBottom: 8 }}>{selected.country || 'unknown origin'}</div>

              <dl style={{ display: 'grid', gridTemplateColumns: '90px 1fr', gap: '4px 8px', margin: 0 }}>
                <dt style={{ color: '#94a3b8' }}>State</dt>
                <dd style={{ margin: 0 }}>
                  {selected.onGround ? 'On ground' : 'Airborne'}
                </dd>

                <dt style={{ color: '#94a3b8' }}>Altitude</dt>
                <dd style={{ margin: 0 }}>
                  {selected.altitude != null
                    ? `${(selected.altitude / 1000).toFixed(1)} km / ${Math.round(selected.altitude * 3.28084).toLocaleString()} ft`
                    : '—'}
                </dd>

                <dt style={{ color: '#94a3b8' }}>Speed</dt>
                <dd style={{ margin: 0 }}>
                  {selected.velocity != null
                    ? `${Math.round(selected.velocity * 3.6)} km/h / ${Math.round(selected.velocity * 1.94384)} kt`
                    : '—'}
                </dd>

                <dt style={{ color: '#94a3b8' }}>Heading</dt>
                <dd style={{ margin: 0 }}>
                  {selected.heading != null ? `${Math.round(selected.heading)}°` : '—'}
                </dd>

                <dt style={{ color: '#94a3b8' }}>Position</dt>
                <dd style={{ margin: 0, fontVariantNumeric: 'tabular-nums' }}>
                  {selected.lat.toFixed(3)}, {selected.lon.toFixed(3)}
                </dd>

                <dt style={{ color: '#94a3b8' }}>ICAO24</dt>
                <dd style={{ margin: 0, fontFamily: 'monospace', fontSize: 12 }}>
                  {selected.icao24 || '—'}
                </dd>
              </dl>

              {selected.icao24 ? (
                <a
                  href={`https://opensky-network.org/aircraft-profile?icao24=${selected.icao24}`}
                  target="_blank"
                  rel="noreferrer"
                  style={{
                    display: 'inline-block',
                    marginTop: 10,
                    color: '#60a5fa',
                    fontSize: 12,
                  }}
                >
                  View on OpenSky →
                </a>
              ) : null}
            </aside>
          ) : (
            <div
              style={{
                position: 'absolute',
                top: 12,
                right: 12,
                padding: '6px 10px',
                backgroundColor: 'rgba(15, 23, 42, 0.65)',
                color: '#cbd5e1',
                borderRadius: 4,
                fontSize: 12,
                pointerEvents: 'none',
              }}
            >
              Click a dot to pin · hover for quick info
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
