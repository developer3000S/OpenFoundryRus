import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Viewer, type CesiumComponentRef } from 'resium';
import {
  Cartesian3,
  Color,
  CustomDataSource,
  Entity as CesiumEntity,
  HeightReference,
  Ion,
  ScreenSpaceEventHandler,
  ScreenSpaceEventType,
  type Viewer as CesiumViewer,
} from 'cesium';

import { LANDMARKS, type Landmark } from './landmarks';
import { useOpenSkyAircraft, type Aircraft } from './useOpenSkyAircraft';
import { useCelestrakSatellites, type Satellite } from './useCelestrakSatellites';

// Cesium ships with a free default Ion access token baked in for new
// installs. We don't depend on Ion in Phase 1 — leave the default in
// place so terrain/imagery fallbacks work. If we ever exceed the
// default token's quota we can register our own at cesium.com/ion.
if (!Ion.defaultAccessToken || Ion.defaultAccessToken === 'your_access_token') {
  // No-op; just signals intent — Cesium handles missing tokens
  // gracefully by falling back to OSM imagery.
}

type Selection =
  | { kind: 'aircraft'; data: Aircraft }
  | { kind: 'satellite'; data: Satellite }
  | null;

const AIRCRAFT_COLOR_AIRBORNE = Color.fromCssColorString('#3b82f6');
const AIRCRAFT_COLOR_LOW = Color.fromCssColorString('#f97316');
const AIRCRAFT_COLOR_HIGH = Color.fromCssColorString('#a855f7');
const AIRCRAFT_COLOR_GROUND = Color.fromCssColorString('#ef4444');
// Satellites color-coded by orbital regime. Translucent so the globe
// stays visible underneath the LEO shell (~80% of catalog).
const SATELLITE_COLOR_LEO = Color.fromCssColorString('#fbbf24').withAlpha(0.55);
const SATELLITE_COLOR_MEO = Color.fromCssColorString('#fb923c').withAlpha(0.7);
const SATELLITE_COLOR_GEO = Color.fromCssColorString('#f43f5e').withAlpha(0.85);
const SATELLITE_COLOR_HEO = Color.fromCssColorString('#a3e635').withAlpha(0.85);

function satelliteColor(altMeters: number): Color {
  if (altMeters < 2_000_000) return SATELLITE_COLOR_LEO;
  if (altMeters < 35_000_000) return SATELLITE_COLOR_MEO;
  if (altMeters < 36_500_000) return SATELLITE_COLOR_GEO;
  return SATELLITE_COLOR_HEO;
}

function aircraftColor(a: Aircraft): Color {
  if (a.onGround) return AIRCRAFT_COLOR_GROUND;
  const alt = a.altitude ?? 0;
  if (alt < 3000) return AIRCRAFT_COLOR_LOW;
  if (alt > 9000) return AIRCRAFT_COLOR_HIGH;
  return AIRCRAFT_COLOR_AIRBORNE;
}

export function WorldViewPage() {
  const aircraftSourceRef = useRef<CustomDataSource | null>(null);
  const satellitesSourceRef = useRef<CustomDataSource | null>(null);
  const aircraftEntityMapRef = useRef<Map<string, CesiumEntity>>(new Map());
  const satelliteEntityMapRef = useRef<Map<string, CesiumEntity>>(new Map());
  const [viewer, setViewer] = useState<CesiumViewer | null>(null);

  // Callback ref — resium calls this once Cesium's async init resolves,
  // which is later than the component mount tick. Holding the viewer in
  // state means downstream effects naturally re-run when it appears.
  const handleViewerRef = useCallback((ref: CesiumComponentRef<CesiumViewer> | null) => {
    setViewer(ref?.cesiumElement ?? null);
  }, []);

  const aircraftFeed = useOpenSkyAircraft();
  const satelliteFeed = useCelestrakSatellites();
  const [selected, setSelected] = useState<Selection>(null);

  // Wire up data sources once the viewer is ready.
  useEffect(() => {
    if (!viewer) return;

    const aircraftSource = new CustomDataSource('aircraft');
    const satellitesSource = new CustomDataSource('satellites');
    viewer.dataSources.add(aircraftSource);
    viewer.dataSources.add(satellitesSource);
    aircraftSourceRef.current = aircraftSource;
    satellitesSourceRef.current = satellitesSource;

    // Dev-only window handle for Playwright/console debugging
    if (typeof window !== 'undefined' && import.meta.env.DEV) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (window as any).__worldview = { viewer, aircraftSource, satellitesSource };
    }

    return () => {
      viewer.dataSources.remove(aircraftSource, true);
      viewer.dataSources.remove(satellitesSource, true);
      aircraftEntityMapRef.current.clear();
      satelliteEntityMapRef.current.clear();
    };
  }, [viewer]);

  // Sync click handler with the latest feed data. Re-arms whenever the
  // arrays change so the lookup always sees the current snapshot.
  useEffect(() => {
    if (!viewer) return;
    const handler = new ScreenSpaceEventHandler(viewer.scene.canvas);
    const acById = new Map(aircraftFeed.aircraft.map((a) => [a.icao24, a]));
    const satByNorad = new Map(satelliteFeed.satellites.map((s) => [s.norad, s]));
    handler.setInputAction((evt: { position: { x: number; y: number } }) => {
      const picked = viewer.scene.pick(evt.position);
      const id = picked?.id?.id as string | undefined;
      if (!id) {
        setSelected(null);
        return;
      }
      const a = acById.get(id);
      if (a) {
        setSelected({ kind: 'aircraft', data: a });
        return;
      }
      const s = satByNorad.get(id);
      if (s) {
        setSelected({ kind: 'satellite', data: s });
        return;
      }
    }, ScreenSpaceEventType.LEFT_CLICK);
    return () => handler.destroy();
  }, [viewer, aircraftFeed.aircraft, satelliteFeed.satellites]);

  // Push aircraft positions into Cesium. Reuses entities by icao24 so
  // we update in place rather than recreate every tick.
  useEffect(() => {
    const source = aircraftSourceRef.current;
    if (!source) return;
    const map = aircraftEntityMapRef.current;
    const seen = new Set<string>();
    for (const a of aircraftFeed.aircraft) {
      seen.add(a.icao24);
      const pos = Cartesian3.fromDegrees(a.lon, a.lat, Math.max(a.altitude ?? 0, 0));
      let entity = map.get(a.icao24);
      if (!entity) {
        entity = source.entities.add({
          id: a.icao24,
          position: pos,
          point: {
            pixelSize: 6,
            color: aircraftColor(a),
            outlineColor: Color.WHITE,
            outlineWidth: 1,
            heightReference: HeightReference.NONE,
          },
        });
        map.set(a.icao24, entity);
      } else {
        entity.position = pos as unknown as typeof entity.position;
        if (entity.point) entity.point.color = aircraftColor(a) as unknown as typeof entity.point.color;
      }
    }
    // Reap aircraft that fell out of coverage
    for (const [id, entity] of map) {
      if (!seen.has(id)) {
        source.entities.remove(entity);
        map.delete(id);
      }
    }
  }, [aircraftFeed.aircraft]);

  // Push satellite positions into Cesium with the same diff-in-place pattern.
  useEffect(() => {
    const source = satellitesSourceRef.current;
    if (!source) return;
    const map = satelliteEntityMapRef.current;
    const seen = new Set<string>();
    for (const s of satelliteFeed.satellites) {
      seen.add(s.norad);
      const pos = Cartesian3.fromDegrees(s.lon, s.lat, s.altitude);
      const color = satelliteColor(s.altitude);
      let entity = map.get(s.norad);
      if (!entity) {
        entity = source.entities.add({
          id: s.norad,
          position: pos,
          point: {
            pixelSize: 2,
            color,
            outlineWidth: 0,
          },
        });
        map.set(s.norad, entity);
      } else {
        entity.position = pos as unknown as typeof entity.position;
        if (entity.point) entity.point.color = color as unknown as typeof entity.point.color;
      }
    }
    for (const [id, entity] of map) {
      if (!seen.has(id)) {
        source.entities.remove(entity);
        map.delete(id);
      }
    }
  }, [satelliteFeed.satellites]);

  const flyTo = useCallback(
    (landmark: Landmark) => {
      if (!viewer) return;
      viewer.camera.flyTo({
        destination: Cartesian3.fromDegrees(landmark.lon, landmark.lat, landmark.altitude),
        duration: 2.5,
      });
    },
    [viewer],
  );

  const lastFeedTime = useMemo(() => {
    const t = Math.max(aircraftFeed.lastUpdate ?? 0, satelliteFeed.lastUpdate ?? 0);
    return t > 0 ? new Date(t).toLocaleTimeString() : null;
  }, [aircraftFeed.lastUpdate, satelliteFeed.lastUpdate]);

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header>
        <p className="of-eyebrow">Live geospatial intelligence</p>
        <h1 className="of-heading-xl">WorldView (Cesium)</h1>
        <p className="of-text-muted" style={{ marginTop: 8, maxWidth: 760 }}>
          A 3D globe with live OpenSky ADS-B aircraft and CelesTrak satellite orbits.
          Click any dot to track it. Yellow = satellite (altitude ≥ 200km).
          Blue/orange/purple/red = aircraft by altitude band. Phase 1 — no Google 3D
          tiles, no shaders, no time-scrubber yet.
        </p>
      </header>

      <div className="of-panel" style={{ padding: 16 }}>
        <div
          style={{
            marginBottom: 12,
            display: 'flex',
            gap: 16,
            alignItems: 'center',
            flexWrap: 'wrap',
          }}
        >
          <span style={{ fontWeight: 600 }} data-testid="aircraft-count">
            ✈ {aircraftFeed.aircraft.length.toLocaleString()} aircraft
          </span>
          <span style={{ fontWeight: 600 }} data-testid="satellite-count">
            🛰 {satelliteFeed.satellites.length.toLocaleString()} satellites
          </span>
          <span className="of-text-muted" style={{ fontSize: 13 }}>
            {lastFeedTime ? `Updated ${lastFeedTime}` : 'Loading…'}
          </span>
          {aircraftFeed.error ? (
            <span style={{ color: '#dc2626', fontSize: 13 }}>aircraft: {aircraftFeed.error}</span>
          ) : null}
          {satelliteFeed.error ? (
            <span style={{ color: '#dc2626', fontSize: 13 }}>satellites: {satelliteFeed.error}</span>
          ) : null}
        </div>

        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 12 }}>
          <span className="of-text-muted" style={{ fontSize: 13, alignSelf: 'center' }}>
            Fly to:
          </span>
          {LANDMARKS.map((l) => (
            <button
              key={l.id}
              type="button"
              className="of-btn"
              onClick={() => flyTo(l)}
              style={{ padding: '4px 10px', fontSize: 13 }}
            >
              {l.label}
            </button>
          ))}
        </div>

        <div style={{ position: 'relative', height: 640 }}>
          <Viewer
            ref={handleViewerRef}
            full={false}
            style={{ width: '100%', height: '100%' }}
            animation={false}
            timeline={false}
            geocoder={false}
            baseLayerPicker={false}
            sceneModePicker={false}
            navigationHelpButton={false}
            homeButton={false}
            fullscreenButton={false}
            infoBox={false}
            selectionIndicator={false}
          />
          {selected ? (
            <SelectionPanel selection={selected} onClose={() => setSelected(null)} />
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
              Click a dot to pin · drag to rotate · scroll to zoom
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function SelectionPanel({ selection, onClose }: { selection: NonNullable<Selection>; onClose: () => void }) {
  const isAircraft = selection.kind === 'aircraft';
  const title = isAircraft
    ? selection.data.callsign || selection.data.icao24 || 'aircraft'
    : selection.data.name || `NORAD ${selection.data.norad}`;

  return (
    <aside
      data-testid="selection-panel"
      style={{
        position: 'absolute',
        top: 12,
        right: 12,
        width: 280,
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
        <strong style={{ fontSize: 15 }} data-testid="selected-title">
          {title}
        </strong>
        <button
          type="button"
          onClick={onClose}
          style={{
            background: 'transparent',
            color: '#cbd5e1',
            border: 'none',
            cursor: 'pointer',
            fontSize: 18,
            lineHeight: 1,
            padding: 0,
          }}
          aria-label="Close"
        >
          ×
        </button>
      </div>
      <div style={{ color: '#cbd5e1', marginBottom: 8 }}>
        {isAircraft
          ? selection.data.country || 'unknown origin'
          : 'Satellite (CelesTrak ACTIVE)'}
      </div>

      {isAircraft ? <AircraftDetails a={selection.data} /> : <SatelliteDetails s={selection.data} />}
    </aside>
  );
}

function AircraftDetails({ a }: { a: Aircraft }) {
  return (
    <dl style={{ display: 'grid', gridTemplateColumns: '90px 1fr', gap: '4px 8px', margin: 0 }}>
      <dt style={{ color: '#94a3b8' }}>State</dt>
      <dd style={{ margin: 0 }}>{a.onGround ? 'On ground' : 'Airborne'}</dd>

      <dt style={{ color: '#94a3b8' }}>Altitude</dt>
      <dd style={{ margin: 0 }}>
        {a.altitude != null
          ? `${(a.altitude / 1000).toFixed(1)} km / ${Math.round(a.altitude * 3.28084).toLocaleString()} ft`
          : '—'}
      </dd>

      <dt style={{ color: '#94a3b8' }}>Speed</dt>
      <dd style={{ margin: 0 }}>
        {a.velocity != null
          ? `${Math.round(a.velocity * 3.6)} km/h / ${Math.round(a.velocity * 1.94384)} kt`
          : '—'}
      </dd>

      <dt style={{ color: '#94a3b8' }}>Heading</dt>
      <dd style={{ margin: 0 }}>{a.heading != null ? `${Math.round(a.heading)}°` : '—'}</dd>

      <dt style={{ color: '#94a3b8' }}>Position</dt>
      <dd style={{ margin: 0, fontVariantNumeric: 'tabular-nums' }}>
        {a.lat.toFixed(3)}, {a.lon.toFixed(3)}
      </dd>

      <dt style={{ color: '#94a3b8' }}>ICAO24</dt>
      <dd style={{ margin: 0, fontFamily: 'monospace', fontSize: 12 }}>{a.icao24 || '—'}</dd>
    </dl>
  );
}

function SatelliteDetails({ s }: { s: Satellite }) {
  // Quick orbital classification from altitude
  const orbitKind =
    s.altitude < 2_000_000 ? 'LEO'
      : s.altitude < 35_000_000 ? 'MEO'
        : s.altitude < 36_000_000 ? 'GEO-band'
          : 'HEO';
  return (
    <dl style={{ display: 'grid', gridTemplateColumns: '90px 1fr', gap: '4px 8px', margin: 0 }}>
      <dt style={{ color: '#94a3b8' }}>NORAD</dt>
      <dd style={{ margin: 0, fontFamily: 'monospace' }}>{s.norad}</dd>

      <dt style={{ color: '#94a3b8' }}>Altitude</dt>
      <dd style={{ margin: 0 }}>{`${(s.altitude / 1000).toFixed(0)} km`}</dd>

      <dt style={{ color: '#94a3b8' }}>Orbit</dt>
      <dd style={{ margin: 0 }}>{orbitKind}</dd>

      <dt style={{ color: '#94a3b8' }}>Position</dt>
      <dd style={{ margin: 0, fontVariantNumeric: 'tabular-nums' }}>
        {s.lat.toFixed(2)}, {s.lon.toFixed(2)}
      </dd>

      <a
        href={`https://www.n2yo.com/satellite/?s=${s.norad}`}
        target="_blank"
        rel="noreferrer"
        style={{ gridColumn: '1 / -1', color: '#60a5fa', fontSize: 12, marginTop: 6 }}
      >
        View on n2yo.com →
      </a>
    </dl>
  );
}
