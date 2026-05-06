import 'maplibre-gl/dist/maplibre-gl.css';

import { useEffect, useRef } from 'react';
import type { Map as MapLibreMap, StyleSpecification } from 'maplibre-gl';

const DEFAULT_STYLE: StyleSpecification = {
  version: 8,
  sources: {},
  layers: [
    {
      id: 'background',
      type: 'background',
      paint: { 'background-color': '#f5efe4' },
    },
  ],
};

interface MapLibreCanvasProps {
  style?: StyleSpecification | string;
  center?: [number, number];
  zoom?: number;
  height?: number | string;
  className?: string;
  onMapLoad?: (map: MapLibreMap) => void;
}

export function MapLibreCanvas({
  style = DEFAULT_STYLE,
  center = [2.1734, 41.3851],
  zoom = 3.3,
  height = 360,
  className,
  onMapLoad,
}: MapLibreCanvasProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<MapLibreMap | null>(null);
  const onMapLoadRef = useRef(onMapLoad);

  useEffect(() => {
    onMapLoadRef.current = onMapLoad;
  }, [onMapLoad]);

  useEffect(() => {
    let disposed = false;

    (async () => {
      const maplibre = await import('maplibre-gl');
      if (disposed || !containerRef.current) return;

      const map = new maplibre.Map({
        container: containerRef.current,
        style,
        center,
        zoom,
        attributionControl: false,
      });

      map.addControl(new maplibre.NavigationControl({ showCompass: false }), 'top-right');
      mapRef.current = map;

      map.on('load', () => {
        if (!disposed) onMapLoadRef.current?.(map);
      });
    })();

    return () => {
      disposed = true;
      mapRef.current?.remove();
      mapRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return <div ref={containerRef} className={className} style={{ width: '100%', height }} />;
}
