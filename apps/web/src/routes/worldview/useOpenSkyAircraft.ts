/**
 * Polls live ADS-B positions from the OpenSky Network via the
 * /external/opensky Vite proxy (the proxy attaches the OAuth2 bearer
 * token server-side; credentials never reach the client).
 *
 * Phase 1 is a direct REST poll. When live data should flow through
 * OpenFoundry's data pipeline, swap this hook's implementation for
 * one that reads from a dataset RID — the returned shape stays
 * identical and the UI doesn't need to change.
 */
import { useCallback, useEffect, useState } from 'react';

export interface Aircraft {
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

export interface AircraftFeed {
  aircraft: Aircraft[];
  lastUpdate: number | null;
  loading: boolean;
  error: string | null;
}

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

export function useOpenSkyAircraft(pollIntervalMs = 30_000): AircraftFeed {
  const [state, setState] = useState<AircraftFeed>({
    aircraft: [],
    lastUpdate: null,
    loading: true,
    error: null,
  });

  const fetchOnce = useCallback(async (signal: AbortSignal) => {
    try {
      const res = await fetch('/external/opensky/api/states/all', { signal });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as OpenSkyResponse;
      const aircraft = (data.states ?? [])
        .map(parseState)
        .filter((a): a is Aircraft => a !== null);
      setState({
        aircraft,
        lastUpdate: data.time * 1000,
        loading: false,
        error: null,
      });
    } catch (err) {
      if ((err as Error).name === 'AbortError') return;
      setState((prev) => ({ ...prev, loading: false, error: (err as Error).message }));
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void fetchOnce(controller.signal);
    const id = window.setInterval(() => void fetchOnce(controller.signal), pollIntervalMs);
    return () => {
      controller.abort();
      window.clearInterval(id);
    };
  }, [fetchOnce, pollIntervalMs]);

  return state;
}
