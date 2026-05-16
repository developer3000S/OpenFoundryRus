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
    const overlay = new MapboxOverlay({ layers: [] });
    // @ts-expect-error MapLibre's addControl typing is loose at the wrapper boundary
    map.addControl(overlay);
    overlayRef.current = overlay;
  }, []);

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
      radiusMinPixels: 2,
      radiusMaxPixels: 8,
      stroked: true,
      lineWidthMinPixels: 0.5,
      getLineColor: [255, 255, 255, 200],
      pickable: true,
      autoHighlight: true,
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

        <MapLibreCanvas
          height={620}
          style={OSM_STYLE}
          center={[0, 25]}
          zoom={1.3}
          onMapLoad={handleMapLoad}
        />
      </div>
    </section>
  );
}
