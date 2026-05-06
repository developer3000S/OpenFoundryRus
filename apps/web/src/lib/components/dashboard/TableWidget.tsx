import { useEffect, useMemo, useState } from 'react';

import type { QueryResult } from '@/lib/api/queries';
import { toNumber, type DashboardTableWidget } from '@/lib/utils/dashboards';

interface TableWidgetProps {
  widget: DashboardTableWidget;
  result: QueryResult | null;
  globalSearch?: string;
}

export function TableWidget({ widget, result, globalSearch = '' }: TableWidgetProps) {
  const [localSearch, setLocalSearch] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [sortColumn, setSortColumn] = useState(widget.defaultSortColumn);
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>(widget.defaultSortDirection);

  // Sync sort when the widget config changes.
  useEffect(() => {
    setSortColumn(widget.defaultSortColumn);
    setSortDirection(widget.defaultSortDirection);
  }, [widget.defaultSortColumn, widget.defaultSortDirection]);

  const columns = result?.columns ?? [];

  const visibleColumns = useMemo(() => {
    const configured = Array.isArray(widget.columns)
      ? widget.columns.filter(
          (column) => column && typeof column.key === 'string' && column.key.length > 0,
        )
      : [];
    if (configured.length > 0) {
      return configured
        .map((column) => ({
          name: column.key,
          label: column.label || column.key,
          index: columns.findIndex((candidate) => candidate.name === column.key),
        }))
        .filter((column) => column.index >= 0);
    }
    return columns.map((column, index) => ({ name: column.name, label: column.name, index }));
  }, [widget.columns, columns]);

  const columnIndexMap = useMemo(
    () => new Map(columns.map((column, index) => [column.name, index] as const)),
    [columns],
  );

  const effectiveSearch = `${globalSearch} ${localSearch}`.trim().toLowerCase();

  const filteredRows = useMemo(() => {
    if (!result) return [] as string[][];
    if (!effectiveSearch) return result.rows;
    return result.rows.filter((row) =>
      row.some((cell) => cell.toLowerCase().includes(effectiveSearch)),
    );
  }, [result, effectiveSearch]);

  const sortedRows = useMemo(() => {
    const index = columnIndexMap.get(sortColumn) ?? 0;
    return [...filteredRows].sort((left, right) => {
      const leftNumeric = toNumber(left[index]);
      const rightNumeric = toNumber(right[index]);
      if (leftNumeric !== null && rightNumeric !== null) {
        return sortDirection === 'asc' ? leftNumeric - rightNumeric : rightNumeric - leftNumeric;
      }
      const comparison = left[index].localeCompare(right[index]);
      return sortDirection === 'asc' ? comparison : -comparison;
    });
  }, [filteredRows, sortColumn, sortDirection, columnIndexMap]);

  const pageSize = Math.max(widget.pageSize, 1);
  const totalPages = Math.max(1, Math.ceil(sortedRows.length / pageSize));

  // Reset page when filters / sorts / size / data change.
  useEffect(() => {
    setCurrentPage(1);
  }, [effectiveSearch, sortColumn, sortDirection, pageSize, result?.rows.length]);

  const pagedRows = useMemo(
    () =>
      sortedRows
        .slice((currentPage - 1) * pageSize, currentPage * pageSize)
        .map((row) => visibleColumns.map((column) => row[column.index] ?? '')),
    [sortedRows, currentPage, pageSize, visibleColumns],
  );

  function toggleSort(column: string) {
    if (sortColumn === column) {
      setSortDirection((d) => (d === 'asc' ? 'desc' : 'asc'));
      return;
    }
    setSortColumn(column);
    setSortDirection('asc');
  }

  return (
    <div style={{ display: 'flex', height: '100%', minHeight: 280, flexDirection: 'column', gap: 12 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <div className="of-text-muted" style={{ fontSize: 12 }}>
          {sortedRows.length} rows after filters
        </div>
        <input
          type="text"
          className="of-input"
          value={localSearch}
          onChange={(e) => setLocalSearch(e.target.value)}
          placeholder="Filter visible rows"
          style={{ minHeight: 32, fontSize: 13, width: 'auto', minWidth: 200 }}
        />
      </div>

      {result && visibleColumns.length > 0 ? (
        <>
          <div
            className="of-scrollbar"
            style={{
              minHeight: 0,
              flex: 1,
              overflow: 'auto',
              border: '1px solid var(--border-default)',
              borderRadius: 'var(--radius-sm)',
            }}
          >
            <table className="of-table">
              <thead>
                <tr>
                  {visibleColumns.map((column) => (
                    <th key={column.name}>
                      <button
                        type="button"
                        onClick={() => toggleSort(column.name)}
                        style={{
                          display: 'inline-flex',
                          alignItems: 'center',
                          gap: 4,
                          background: 'transparent',
                          border: 0,
                          padding: 0,
                          color: 'inherit',
                          font: 'inherit',
                          textTransform: 'inherit',
                          cursor: 'pointer',
                        }}
                      >
                        <span>{column.label}</span>
                        {sortColumn === column.name && <span>{sortDirection === 'asc' ? '↑' : '↓'}</span>}
                      </button>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {pagedRows.map((row, index) => (
                  <tr key={index}>
                    {row.map((cell, cellIndex) => (
                      <td key={cellIndex}>{cell}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: 13 }}>
            <span className="of-text-muted">
              Page {currentPage} of {totalPages}
            </span>
            <div style={{ display: 'flex', gap: 8 }}>
              <button
                type="button"
                className="of-btn"
                onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                disabled={currentPage <= 1}
              >
                Prev
              </button>
              <button
                type="button"
                className="of-btn"
                onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                disabled={currentPage >= totalPages}
              >
                Next
              </button>
            </div>
          </div>
        </>
      ) : (
        <div
          style={{
            display: 'flex',
            flex: 1,
            alignItems: 'center',
            justifyContent: 'center',
            border: '1px dashed var(--border-default)',
            borderRadius: 'var(--radius-md)',
            color: 'var(--text-muted)',
            fontSize: 13,
          }}
        >
          This table widget is waiting for query results.
        </div>
      )}
    </div>
  );
}
