import { useCallback, useMemo, useState } from 'react';
import type { Core, ElementDefinition, EventObject, StylesheetStyle } from 'cytoscape';

import { CytoscapeCanvas } from '@components/CytoscapeCanvas';

const ELEMENTS: ElementDefinition[] = [
  { data: { id: 'dataset', label: 'Dataset', kind: 'object' } },
  { data: { id: 'pipeline', label: 'Pipeline', kind: 'object' } },
  { data: { id: 'notebook', label: 'Notebook', kind: 'object' } },
  { data: { id: 'dashboard', label: 'Dashboard', kind: 'object' } },
  { data: { id: 'user', label: 'User', kind: 'principal' } },
  { data: { id: 'role', label: 'Role', kind: 'principal' } },

  { data: { source: 'pipeline', target: 'dataset', label: 'writes' } },
  { data: { source: 'notebook', target: 'dataset', label: 'reads' } },
  { data: { source: 'dashboard', target: 'dataset', label: 'reads' } },
  { data: { source: 'user', target: 'role', label: 'has' } },
  { data: { source: 'role', target: 'dataset', label: 'grants' } },
  { data: { source: 'role', target: 'pipeline', label: 'grants' } },
];

const STYLESHEET: StylesheetStyle[] = [
  {
    selector: 'node',
    style: {
      'background-color': '#0369a1',
      label: 'data(label)',
      color: '#1e293b',
      'text-valign': 'bottom',
      'text-margin-y': 8,
      'font-size': 11,
      'font-weight': 600,
      width: 24,
      height: 24,
      'border-width': 2,
      'border-color': '#ffffff',
    },
  },
  {
    selector: 'node[kind = "principal"]',
    style: {
      'background-color': '#7c3aed',
      shape: 'round-rectangle',
    },
  },
  {
    selector: 'edge',
    style: {
      width: 1.4,
      'line-color': '#94a3b8',
      'curve-style': 'bezier',
      'target-arrow-shape': 'triangle',
      'target-arrow-color': '#94a3b8',
      'arrow-scale': 0.8,
      label: 'data(label)',
      'font-size': 9,
      color: '#475569',
      'text-background-color': '#ffffff',
      'text-background-opacity': 1,
      'text-background-padding': '2px',
    },
  },
  {
    selector: 'node:selected',
    style: { 'background-color': '#15803d', 'border-color': '#dcfce7', 'border-width': 4 },
  },
];

export function CytoscapeDemoPage() {
  const [selected, setSelected] = useState<string | null>(null);

  const handleReady = useCallback((cy: Core) => {
    cy.on('tap', 'node', (event: EventObject) => {
      setSelected(event.target.data('id'));
    });
    cy.on('tap', (event: EventObject) => {
      if (event.target === cy) setSelected(null);
    });
  }, []);

  const elements = useMemo(() => ELEMENTS, []);

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header>
        <p className="of-eyebrow">Capability validator</p>
        <h1 className="of-heading-xl">Cytoscape wrapper demo</h1>
        <p className="of-text-muted" style={{ marginTop: 8, maxWidth: 720 }}>
          Validates <code>&lt;CytoscapeCanvas&gt;</code>: lazy <code>cytoscape</code> +{' '}
          <code>cytoscape-fcose</code> imports, idempotent extension registration, fCoSE layout,
          tap event subscription via <code>onReady</code>, and destroy on unmount. Click a node to
          select it.
        </p>
      </header>

      <div className="of-panel" style={{ padding: 20 }}>
        <CytoscapeCanvas
          elements={elements}
          stylesheet={STYLESHEET}
          height={420}
          onReady={handleReady}
        />
      </div>

      <div className="of-panel-muted" style={{ padding: '12px 16px' }}>
        <p className="of-eyebrow">Selected node</p>
        <p style={{ marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 13 }}>
          {selected ?? '(none — tap a node)'}
        </p>
      </div>
    </section>
  );
}
