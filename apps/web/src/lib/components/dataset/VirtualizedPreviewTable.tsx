import { useMemo, useRef, useState } from 'react';

import type { DatasetTransaction } from '@/lib/api/datasets';

export type ColumnDef = { name: string; field_type?: string; data_type?: string };
export type ColumnStats = {
  min?: string | number | null;
  max?: string | number | null;
  null_rate?: number;
  distinct_count?: number;
};

interface Props {
  columns: ColumnDef[];
  rows: Array<Record<string, unknown>>;
  stats?: Record<string, ColumnStats>;
  transactions?: DatasetTransaction[];
  selectedTransactionId?: string | null;
  onSelectTransaction?: (txId: string | null) => void;
  rowHeight?: number;
  viewportHeight?: number;
  fileFormat?: string | null;
  textSubFormat?: string | null;
  schemaInferred?: boolean;
}

function formatCell(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function badgeLabel(format: string | null | undefined, sub: string | null | undefined) {
  if (!format) return '';
  const upper = format.toUpperCase();
  if (upper !== 'TEXT') return upper;
  if (sub === 'json_lines') return 'TEXT-JSON';
  return 'TEXT-CSV';
}

export function VirtualizedPreviewTable({
  columns,
  rows,
  stats = {},
  transactions = [],
  selectedTransactionId = null,
  onSelectTransaction,
  rowHeight = 32,
  viewportHeight = 480,
  fileFormat = null,
  textSubFormat = null,
  schemaInferred = false,
}: Props) {
  const [scrollTop, setScrollTop] = useState(0);
  const scrollerRef = useRef<HTMLDivElement | null>(null);

  const overscan = 8;
  const visibleCount = Math.ceil(viewportHeight / rowHeight) + overscan * 2;
  const startIdx = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const endIdx = Math.min(rows.length, startIdx + visibleCount);
  const padTop = startIdx * rowHeight;
  const padBottom = Math.max(0, (rows.length - endIdx) * rowHeight);

  const visibleRows = useMemo(() => rows.slice(startIdx, endIdx), [rows, startIdx, endIdx]);

  function inferredColumns(): ColumnDef[] {
    if (columns.length > 0) return columns;
    if (rows.length === 0) return [];
    return Object.keys(rows[0]).map((name) => ({ name }));
  }

  const cols = inferredColumns();
  const badge = badgeLabel(fileFormat, textSubFormat);

  return (
    <div style={{ display: 'grid', gap: 8 }}>
      {(badge || schemaInferred || transactions.length > 0) && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center', fontSize: 11 }}>
          {badge && (
            <span style={{ padding: '2px 8px', background: '#1e293b', color: '#cbd5e1', borderRadius: 999, fontFamily: 'var(--font-mono)' }}>{badge}</span>
          )}
          {schemaInferred && (
            <span style={{ padding: '2px 8px', background: '#fef3c7', color: '#92400e', borderRadius: 999 }}>
              schema inferred
            </span>
          )}
          {transactions.length > 0 && (
            <div style={{ display: 'flex', gap: 4, overflow: 'auto', maxWidth: '100%' }}>
              <button
                type="button"
                onClick={() => onSelectTransaction?.(null)}
                style={{
                  padding: '2px 8px',
                  borderRadius: 999,
                  border: '1px solid var(--border-default)',
                  background: !selectedTransactionId ? '#1d4ed8' : 'transparent',
                  color: !selectedTransactionId ? '#fff' : 'inherit',
                  fontSize: 10,
                  cursor: 'pointer',
                  fontFamily: 'var(--font-mono)',
                  whiteSpace: 'nowrap',
                }}
              >
                latest
              </button>
              {transactions.slice(0, 16).map((t) => {
                const sel = selectedTransactionId === t.id;
                return (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => onSelectTransaction?.(t.id)}
                    title={`${t.operation} · ${t.status} · ${new Date(t.created_at).toLocaleString()}`}
                    style={{
                      padding: '2px 8px',
                      borderRadius: 999,
                      border: '1px solid var(--border-default)',
                      background: sel ? '#1d4ed8' : 'transparent',
                      color: sel ? '#fff' : 'inherit',
                      fontSize: 10,
                      cursor: 'pointer',
                      fontFamily: 'var(--font-mono)',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {t.id.slice(0, 8)}…
                  </button>
                );
              })}
            </div>
          )}
        </div>
      )}

      <div
        ref={scrollerRef}
        onScroll={(e) => setScrollTop((e.target as HTMLDivElement).scrollTop)}
        style={{
          height: viewportHeight,
          overflow: 'auto',
          border: '1px solid var(--border-default)',
          borderRadius: 12,
          background: 'var(--bg-default)',
          position: 'relative',
        }}
      >
        <table style={{ borderCollapse: 'separate', borderSpacing: 0, width: '100%', fontSize: 12 }}>
          <thead style={{ position: 'sticky', top: 0, zIndex: 1, background: 'var(--bg-default)' }}>
            <tr>
              {cols.map((c) => (
                <th
                  key={c.name}
                  style={{
                    textAlign: 'left',
                    padding: '6px 10px',
                    borderBottom: '1px solid var(--border-default)',
                    fontFamily: 'var(--font-mono)',
                    fontSize: 11,
                  }}
                >
                  <div>{c.name}</div>
                  {(c.field_type || c.data_type) && (
                    <div className="of-text-muted" style={{ fontSize: 10, fontWeight: 400 }}>
                      {c.field_type ?? c.data_type}
                    </div>
                  )}
                  {stats[c.name] && (
                    <div className="of-text-muted" style={{ fontSize: 10, fontWeight: 400, marginTop: 2 }}>
                      {stats[c.name].null_rate !== undefined && `${(stats[c.name].null_rate! * 100).toFixed(0)}% null · `}
                      {stats[c.name].distinct_count !== undefined && `${stats[c.name].distinct_count} distinct`}
                    </div>
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {padTop > 0 && (
              <tr style={{ height: padTop }}>
                <td colSpan={cols.length} />
              </tr>
            )}
            {visibleRows.map((row, i) => (
              <tr key={startIdx + i} style={{ height: rowHeight }}>
                {cols.map((c) => (
                  <td
                    key={c.name}
                    style={{
                      padding: '4px 10px',
                      borderBottom: '1px solid var(--border-subtle)',
                      maxWidth: 240,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      fontFamily: 'var(--font-mono)',
                    }}
                  >
                    {formatCell(row[c.name])}
                  </td>
                ))}
              </tr>
            ))}
            {padBottom > 0 && (
              <tr style={{ height: padBottom }}>
                <td colSpan={cols.length} />
              </tr>
            )}
            {rows.length === 0 && (
              <tr>
                <td colSpan={Math.max(1, cols.length)} className="of-text-muted" style={{ padding: 18, textAlign: 'center' }}>
                  No rows.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="of-text-muted" style={{ fontSize: 10 }}>
        {rows.length} row{rows.length === 1 ? '' : 's'} · {cols.length} column{cols.length === 1 ? '' : 's'} · windowed render
      </div>
    </div>
  );
}
