import { useMemo } from 'react';

interface Props {
  mediaSetRid: string;
  schema?: 'IMAGE' | 'DICOM' | 'DOCUMENT';
  tileSize?: 256 | 512;
  minzoom?: number;
  maxzoom?: number;
  origin?: string;
}

export function RasterMediaLayer({ mediaSetRid, schema = 'IMAGE', tileSize = 256, minzoom = 0, maxzoom = 22, origin = '' }: Props) {
  const tileUrlTemplate = useMemo(() => `${origin}/tiles/${mediaSetRid}/{z}/{x}/{y}.png`, [origin, mediaSetRid]);

  const mapLibreSource = useMemo(() => ({
    type: 'raster' as const,
    tiles: [tileUrlTemplate],
    tileSize,
    minzoom,
    maxzoom,
    attribution: '© OpenFoundry media-sets-service · access pattern: geo_tile',
  }), [tileUrlTemplate, tileSize, minzoom, maxzoom]);

  return (
    <section className="rounded-3xl border border-stone-200 bg-white p-5 shadow-sm shadow-stone-200/60">
      <header>
        <p className="text-xs font-semibold uppercase tracking-[0.24em] text-cyan-700">Raster media layer</p>
        <h3 className="mt-2 text-xl font-semibold text-stone-900">Tile source · {schema}</h3>
        <p className="mt-1 text-sm text-stone-500">
          Wires a MapLibre raster source against the <code className="rounded bg-stone-100 px-1 py-0.5 text-xs">geo_tile</code> access pattern of this media set.
        </p>
      </header>
      <div className="mt-5 grid gap-4 xl:grid-cols-2">
        <div className="rounded-2xl border border-stone-200 bg-stone-50 p-4 text-sm text-stone-600">
          <p className="font-semibold text-stone-900">Tile URL template</p>
          <p className="mt-2 break-all font-mono text-xs text-stone-500">{tileUrlTemplate}</p>
          <dl className="mt-4 grid grid-cols-2 gap-2 text-xs text-stone-600">
            <div><dt className="font-semibold text-stone-900">Tile size</dt><dd>{tileSize} px</dd></div>
            <div><dt className="font-semibold text-stone-900">Zoom</dt><dd>{minzoom}–{maxzoom}</dd></div>
          </dl>
        </div>
        <div className="rounded-2xl border border-stone-200 bg-stone-50 p-4 text-sm text-stone-600">
          <p className="font-semibold text-stone-900">MapLibre source payload</p>
          <pre className="mt-2 max-h-64 overflow-auto rounded-2xl border border-stone-200 bg-white p-3 font-mono text-xs text-stone-700">{JSON.stringify(mapLibreSource, null, 2)}</pre>
        </div>
      </div>
    </section>
  );
}
