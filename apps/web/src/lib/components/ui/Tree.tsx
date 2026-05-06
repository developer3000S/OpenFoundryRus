import { useState } from 'react';

import { Glyph } from './Glyph';

interface TreeProps {
  data: unknown;
  label?: string;
  collapsedByDefault?: boolean;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isComplex(value: unknown) {
  return Array.isArray(value) || isObject(value);
}

function entries(value: unknown): Array<[string, unknown]> {
  if (isObject(value)) return Object.entries(value);
  if (Array.isArray(value)) return value.map((item, i) => [String(i), item]);
  return [];
}

function formatLeaf(value: unknown) {
  if (value === null) return 'null';
  if (value === undefined) return 'undefined';
  if (typeof value === 'string') return JSON.stringify(value);
  return String(value);
}

function leafColor(value: unknown) {
  if (value === null || value === undefined) return '#94a3b8';
  if (typeof value === 'string') return '#10b981';
  if (typeof value === 'number') return '#60a5fa';
  if (typeof value === 'boolean') return '#f59e0b';
  return '#cbd5e1';
}

export function Tree({ data, label = '', collapsedByDefault = false }: TreeProps) {
  const [expanded, setExpanded] = useState(!collapsedByDefault);

  if (!isComplex(data)) {
    return (
      <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>
        {label && <span style={{ color: '#94a3b8' }}>{label}: </span>}
        <span style={{ color: leafColor(data) }}>{formatLeaf(data)}</span>
      </div>
    );
  }

  const items = entries(data);
  const summary = Array.isArray(data) ? `[${data.length}]` : `{${Object.keys(data as object).length}}`;

  return (
    <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, lineHeight: 1.4 }}>
      {label && (
        <button
          type="button"
          onClick={() => setExpanded((e) => !e)}
          style={{ display: 'inline-flex', alignItems: 'center', gap: 4, background: 'none', border: 'none', color: 'inherit', cursor: 'pointer', padding: 0 }}
        >
          <Glyph name={expanded ? 'chevron-down' : 'chevron-right'} size={12} />
          <span style={{ fontWeight: 600 }}>{label}</span>
          <span style={{ fontSize: 11, color: '#94a3b8' }}>{summary}</span>
        </button>
      )}
      {(expanded || !label) && (
        <ul style={{ marginLeft: 12, marginTop: 4, paddingLeft: 12, listStyle: 'none', borderLeft: '1px solid #334155', display: 'grid', gap: 2 }}>
          {items.map(([key, value]) => (
            <li key={key}>
              {isComplex(value) ? (
                <Tree data={value} label={key} collapsedByDefault />
              ) : (
                <div style={{ display: 'flex', gap: 8 }}>
                  <span style={{ color: '#94a3b8' }}>{key}:</span>
                  <span style={{ color: leafColor(value) }}>{formatLeaf(value)}</span>
                </div>
              )}
            </li>
          ))}
          {items.length === 0 && <li style={{ fontSize: 11, fontStyle: 'italic', color: '#94a3b8' }}>empty</li>}
        </ul>
      )}
    </div>
  );
}
