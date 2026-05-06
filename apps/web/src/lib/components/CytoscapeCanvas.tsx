import { useEffect, useRef } from 'react';
import type {
  Core,
  ElementDefinition,
  LayoutOptions,
  StylesheetStyle,
} from 'cytoscape';

let fcoseRegistered = false;

interface CytoscapeCanvasProps {
  elements: ElementDefinition[];
  stylesheet?: StylesheetStyle[];
  layout?: LayoutOptions;
  height?: number | string;
  className?: string;
  onReady?: (cy: Core) => void;
}

export function CytoscapeCanvas({
  elements,
  stylesheet,
  layout,
  height = 360,
  className,
  onReady,
}: CytoscapeCanvasProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const cyRef = useRef<Core | null>(null);
  const onReadyRef = useRef(onReady);

  useEffect(() => {
    onReadyRef.current = onReady;
  }, [onReady]);

  // Init + dispose. Re-creates the instance only on mount/unmount.
  useEffect(() => {
    let disposed = false;

    (async () => {
      const [cytoscapeMod, fcoseMod] = await Promise.all([
        import('cytoscape'),
        import('cytoscape-fcose'),
      ]);
      if (disposed || !containerRef.current) return;

      const cytoscape = cytoscapeMod.default;
      if (!fcoseRegistered) {
        try {
          cytoscape.use(fcoseMod.default);
        } catch {
          // already registered (e.g. duplicate module instance after HMR)
        }
        fcoseRegistered = true;
      }

      const cy = cytoscape({
        container: containerRef.current,
        elements,
        style: stylesheet,
        layout: layout ?? { name: 'fcose', animate: false } as LayoutOptions,
        wheelSensitivity: 0.25,
      });

      cyRef.current = cy;
      onReadyRef.current?.(cy);
    })();

    return () => {
      disposed = true;
      cyRef.current?.destroy();
      cyRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Replace the element set when `elements` prop changes, then re-run layout.
  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) return;
    cy.elements().remove();
    cy.add(elements);
    cy.layout(layout ?? ({ name: 'fcose', animate: false } as LayoutOptions)).run();
  }, [elements, layout]);

  // Re-apply stylesheet when it changes (e.g. node display mode toggles).
  // The init effect already seeded the initial stylesheet; this updates it
  // in place without recreating the cytoscape instance.
  useEffect(() => {
    const cy = cyRef.current;
    if (!cy || !stylesheet) return;
    cy.style(stylesheet).update();
  }, [stylesheet]);

  return <div ref={containerRef} className={className} style={{ width: '100%', height }} />;
}
