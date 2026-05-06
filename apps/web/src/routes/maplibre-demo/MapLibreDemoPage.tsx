import { useCallback, useRef } from 'react';
import type { Map as MapLibreMap } from 'maplibre-gl';

import { MapLibreCanvas } from '@components/MapLibreCanvas';

interface City {
  name: string;
  coords: [number, number];
}

const CITIES: City[] = [
  { name: 'Barcelona', coords: [2.1734, 41.3851] },
  { name: 'Madrid', coords: [-3.7038, 40.4168] },
  { name: 'Paris', coords: [2.3522, 48.8566] },
  { name: 'Berlin', coords: [13.405, 52.52] },
  { name: 'Lisbon', coords: [-9.1393, 38.7223] },
];

export function MapLibreDemoPage() {
  const mapRef = useRef<MapLibreMap | null>(null);

  const handleMapLoad = useCallback((map: MapLibreMap) => {
    mapRef.current = map;
    map.addSource('cities', {
      type: 'geojson',
      data: {
        type: 'FeatureCollection',
        features: CITIES.map((city) => ({
          type: 'Feature',
          properties: { name: city.name },
          geometry: { type: 'Point', coordinates: city.coords },
        })),
      },
    });
    map.addLayer({
      id: 'city-points',
      type: 'circle',
      source: 'cities',
      paint: {
        'circle-radius': 7,
        'circle-color': '#0369a1',
        'circle-stroke-color': '#ffffff',
        'circle-stroke-width': 2,
      },
    });
    map.addLayer({
      id: 'city-labels',
      type: 'symbol',
      source: 'cities',
      layout: {
        'text-field': ['get', 'name'],
        'text-font': ['Open Sans Regular', 'Arial Unicode MS Regular'],
        'text-offset': [0, 1.2],
        'text-anchor': 'top',
        'text-size': 12,
      },
      paint: {
        'text-color': '#1e293b',
        'text-halo-color': '#ffffff',
        'text-halo-width': 1.4,
      },
    });
  }, []);

  function flyTo(city: City) {
    mapRef.current?.flyTo({ center: city.coords, zoom: 7, speed: 1.4 });
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header>
        <p className="of-eyebrow">Capability validator</p>
        <h1 className="of-heading-xl">MapLibre wrapper demo</h1>
        <p className="of-text-muted" style={{ marginTop: 8, maxWidth: 720 }}>
          Validates <code>&lt;MapLibreCanvas&gt;</code>: lazy <code>maplibre-gl</code> import, CSS
          side-effect import, navigation control, <code>onMapLoad</code> callback for layer setup,
          and clean removal on unmount. The map style is an inline minimal background — no tile
          provider needed for the demo.
        </p>
      </header>

      <div className="of-panel" style={{ padding: 20 }}>
        <div className="of-toolbar" style={{ marginBottom: 16 }}>
          <span className="of-text-muted" style={{ fontSize: 13 }}>
            Fly to:
          </span>
          {CITIES.map((city) => (
            <button key={city.name} type="button" className="of-btn" onClick={() => flyTo(city)}>
              {city.name}
            </button>
          ))}
        </div>

        <MapLibreCanvas height={420} onMapLoad={handleMapLoad} />
      </div>
    </section>
  );
}
