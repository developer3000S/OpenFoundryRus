import { useEffect, useState } from 'react';

import {
  createDefaultDateRange,
  type DashboardDateRange,
  type DashboardFilterState,
} from '@/lib/utils/dashboards';

import { DateRangeFilter } from './DateRangeFilter';

interface FilterBarProps {
  search: string;
  dateRange: DashboardDateRange;
  busy?: boolean;
  onApply?: (filters: DashboardFilterState) => void;
  onReset?: () => void;
}

export function FilterBar({ search, dateRange, busy = false, onApply, onReset }: FilterBarProps) {
  const [draftSearch, setDraftSearch] = useState(search);
  const [draftDateRange, setDraftDateRange] = useState<DashboardDateRange>(dateRange);

  // Re-seed the draft whenever the parent applies new filters externally.
  useEffect(() => {
    setDraftSearch(search);
    setDraftDateRange({ ...dateRange });
  }, [search, dateRange]);

  function applyFilters() {
    onApply?.({ search: draftSearch.trim(), dateRange: draftDateRange });
  }

  function resetFilters() {
    setDraftSearch('');
    setDraftDateRange(createDefaultDateRange());
    onReset?.();
  }

  return (
    <div
      style={{
        display: 'grid',
        gap: 12,
        background: '#fff',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-md)',
        padding: 16,
        boxShadow: 'var(--shadow-panel)',
      }}
    >
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <div className="of-eyebrow">Global Filters</div>
          <h2 className="of-heading-sm" style={{ marginTop: 4, fontSize: 16 }}>
            Propagate one filter context across every widget
          </h2>
        </div>

        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          <button type="button" className="of-btn" onClick={resetFilters} disabled={busy}>
            Reset
          </button>
          <button type="button" className="of-btn of-btn-primary" onClick={applyFilters} disabled={busy}>
            {busy ? 'Applying…' : 'Apply Filters'}
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 1.2fr) minmax(0, 1fr)' }}>
        <label
          style={{
            background: '#fff',
            border: '1px solid var(--border-default)',
            borderRadius: 'var(--radius-md)',
            padding: '8px 12px',
          }}
        >
          <div className="of-eyebrow">Search</div>
          <input
            type="text"
            value={draftSearch}
            onChange={(e) => setDraftSearch(e.target.value)}
            placeholder="Use {{search}} in widget SQL or rely on table filtering"
            style={{
              marginTop: 6,
              width: '100%',
              border: 0,
              background: 'transparent',
              fontSize: 13,
              color: 'var(--text-strong)',
              outline: 'none',
            }}
          />
        </label>

        <DateRangeFilter
          value={draftDateRange}
          onChange={(value) => setDraftDateRange(value)}
          disabled={busy}
        />
      </div>

      <div
        style={{
          border: '1px dashed var(--border-default)',
          borderRadius: 'var(--radius-md)',
          padding: '8px 12px',
          fontSize: 12,
          color: 'var(--text-muted)',
        }}
      >
        Query placeholders available in every widget: <code>{'{{search}}'}</code>,{' '}
        <code>{'{{date_from}}'}</code>, <code>{'{{date_to}}'}</code>
      </div>
    </div>
  );
}
