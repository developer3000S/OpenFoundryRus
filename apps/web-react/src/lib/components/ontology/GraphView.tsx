import { useMemo } from 'react';

import { CytoscapeCanvas } from '@/lib/components/CytoscapeCanvas';
import type { LinkType, ObjectType } from '@/lib/api/ontology';

interface GraphViewProps {
  objectTypes: ObjectType[];
  linkTypes: LinkType[];
  selectedTypeId?: string | null;
  onNodeSelect?: (typeId: string) => void;
  onEdgeSelect?: (linkTypeId: string) => void;
}

export function GraphView({ objectTypes, linkTypes, selectedTypeId = null, onNodeSelect, onEdgeSelect }: GraphViewProps) {
  const elements = useMemo(() => {
    const ids = new Set(objectTypes.map((t) => t.id));
    const nodes = objectTypes.map((t) => ({
      data: { id: t.id, label: t.display_name || t.name, color: t.color || '#1d4ed8' },
      classes: t.id === selectedTypeId ? 'selected' : '',
    }));
    const edges = linkTypes
      .filter((l) => ids.has(l.source_type_id) && ids.has(l.target_type_id))
      .map((l) => ({
        data: { id: l.id, source: l.source_type_id, target: l.target_type_id, label: l.display_name || l.name },
      }));
    return [...nodes, ...edges];
  }, [objectTypes, linkTypes, selectedTypeId]);

  const stylesheet = useMemo(
    () => [
      {
        selector: 'node',
        style: {
          'background-color': 'data(color)',
          label: 'data(label)',
          color: '#f1f5f9',
          'text-valign': 'center',
          'text-halign': 'center',
          'text-wrap': 'wrap',
          'text-max-width': '120px',
          'font-size': 11,
          'font-weight': 600,
          width: 120,
          height: 44,
          shape: 'round-rectangle',
          'border-color': '#475569',
          'border-width': 2,
        },
      },
      {
        selector: 'node.selected',
        style: { 'border-color': '#fbbf24', 'border-width': 4 },
      },
      {
        selector: 'edge',
        style: {
          width: 1.5,
          'line-color': '#475569',
          'target-arrow-color': '#475569',
          'target-arrow-shape': 'triangle',
          'curve-style': 'bezier',
          label: 'data(label)',
          'font-size': 9,
          color: '#94a3b8',
        },
      },
    ],
    [],
  );

  return (
    <CytoscapeCanvas
      elements={elements}
      stylesheet={stylesheet as unknown as import('cytoscape').StylesheetStyle[]}
      height={520}
      onReady={(cy) => {
        cy.removeListener('tap');
        cy.on('tap', 'node', (evt) => onNodeSelect?.(evt.target.id()));
        cy.on('tap', 'edge', (evt) => onEdgeSelect?.(evt.target.id()));
      }}
    />
  );
}
