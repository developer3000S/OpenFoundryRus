/**
 * Curated points of interest for the landmark jump bar. Each entry
 * specifies the Cesium camera target (lon, lat) and the height to fly
 * to in meters. We use moderate altitudes (~1.5km–3km AGL feel) so the
 * city is recognizable but you can still see surrounding traffic.
 */
export interface Landmark {
  id: string;
  label: string;
  lon: number;
  lat: number;
  altitude: number;
}

export const LANDMARKS: Landmark[] = [
  { id: 'sf', label: 'San Francisco', lon: -122.4194, lat: 37.7749, altitude: 25000 },
  { id: 'nyc', label: 'New York', lon: -74.006, lat: 40.7128, altitude: 25000 },
  { id: 'london', label: 'London', lon: -0.1278, lat: 51.5074, altitude: 25000 },
  { id: 'tokyo', label: 'Tokyo', lon: 139.6503, lat: 35.6762, altitude: 25000 },
  { id: 'singapore', label: 'Singapore', lon: 103.8198, lat: 1.3521, altitude: 25000 },
  { id: 'dubai', label: 'Dubai', lon: 55.2708, lat: 25.2048, altitude: 25000 },
  { id: 'sydney', label: 'Sydney', lon: 151.2093, lat: -33.8688, altitude: 25000 },
  { id: 'tehran', label: 'Tehran', lon: 51.389, lat: 35.6892, altitude: 25000 },
];
