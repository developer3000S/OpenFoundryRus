/**
 * Fetches NORAD TLE (Two-Line Element) data from CelesTrak and uses
 * satellite.js to propagate each satellite's current orbital position.
 *
 * TLE data is refreshed every 6 hours (orbital decay is slow); position
 * propagation runs every `propagateIntervalMs` (default 10s) on the
 * cached TLE set.
 *
 * Phase 1 uses the "Active" satellite group (~12k). If perf becomes a
 * problem, swap GROUP for a smaller curated category (e.g., 'starlink',
 * 'iss', 'visual').
 */
import { useEffect, useRef, useState } from 'react';
import {
  eciToGeodetic,
  gstime,
  propagate,
  twoline2satrec,
  type SatRec,
} from 'satellite.js';

export interface Satellite {
  norad: string;
  name: string;
  lon: number;
  lat: number;
  altitude: number;
  satrec: SatRec;
}

export interface SatelliteFeed {
  satellites: Satellite[];
  lastUpdate: number | null;
  loading: boolean;
  error: string | null;
}

interface RawTleEntry {
  name: string;
  line1: string;
  line2: string;
}

function parseTleText(text: string): RawTleEntry[] {
  const lines = text.split(/\r?\n/).map((l) => l.trim()).filter(Boolean);
  const out: RawTleEntry[] = [];
  for (let i = 0; i + 2 < lines.length; i += 3) {
    const name = lines[i];
    const line1 = lines[i + 1];
    const line2 = lines[i + 2];
    if (line1.startsWith('1 ') && line2.startsWith('2 ')) {
      out.push({ name, line1, line2 });
    }
  }
  return out;
}

function propagateAll(records: { name: string; satrec: SatRec; norad: string }[], now: Date): Satellite[] {
  const gmst = gstime(now);
  const out: Satellite[] = [];
  for (const { name, satrec, norad } of records) {
    const result = propagate(satrec, now);
    if (typeof result.position === 'boolean' || !result.position) continue;
    const geo = eciToGeodetic(result.position, gmst);
    if (!Number.isFinite(geo.latitude) || !Number.isFinite(geo.longitude)) continue;
    out.push({
      norad,
      name,
      lon: (geo.longitude * 180) / Math.PI,
      lat: (geo.latitude * 180) / Math.PI,
      altitude: geo.height * 1000, // satellite.js returns km, Cesium wants m
      satrec,
    });
  }
  return out;
}

const TLE_REFRESH_MS = 6 * 60 * 60 * 1000; // 6 hours

export function useCelestrakSatellites(
  group = 'active',
  propagateIntervalMs = 10_000,
): SatelliteFeed {
  const [state, setState] = useState<SatelliteFeed>({
    satellites: [],
    lastUpdate: null,
    loading: true,
    error: null,
  });
  const recordsRef = useRef<{ name: string; satrec: SatRec; norad: string }[]>([]);
  const lastTleFetchRef = useRef<number>(0);

  useEffect(() => {
    const controller = new AbortController();

    async function refreshTle() {
      try {
        const res = await fetch(
          `/external/celestrak/NORAD/elements/gp.php?GROUP=${encodeURIComponent(group)}&FORMAT=tle`,
          { signal: controller.signal },
        );
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const text = await res.text();
        const parsed = parseTleText(text);
        const records: { name: string; satrec: SatRec; norad: string }[] = [];
        for (const t of parsed) {
          try {
            const satrec = twoline2satrec(t.line1, t.line2);
            // NORAD ID lives in columns 3-7 of line 1 (the catalog number).
            const norad = t.line1.slice(2, 7).trim();
            records.push({ name: t.name, satrec, norad });
          } catch {
            /* skip malformed entry */
          }
        }
        recordsRef.current = records;
        lastTleFetchRef.current = Date.now();
        setState((prev) => ({ ...prev, error: null }));
      } catch (err) {
        if ((err as Error).name === 'AbortError') return;
        setState((prev) => ({ ...prev, loading: false, error: (err as Error).message }));
      }
    }

    function tick() {
      if (Date.now() - lastTleFetchRef.current > TLE_REFRESH_MS && lastTleFetchRef.current !== 0) {
        void refreshTle();
      }
      if (recordsRef.current.length === 0) return;
      const sats = propagateAll(recordsRef.current, new Date());
      setState({
        satellites: sats,
        lastUpdate: Date.now(),
        loading: false,
        error: null,
      });
    }

    void refreshTle().then(tick);
    const tickId = window.setInterval(tick, propagateIntervalMs);

    return () => {
      controller.abort();
      window.clearInterval(tickId);
    };
  }, [group, propagateIntervalMs]);

  return state;
}
