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
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 8,
        background: '#fff',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-md)',
        padding: 8,
        boxShadow: 'var(--shadow-panel)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span className="of-eyebrow">Parameters</span>
      </div>

      <div style={{ display: 'flex', flex: 1, flexWrap: 'wrap', gap: 8, minWidth: 260 }}>
        <label
          style={{
            background: '#fff',
            border: '1px solid var(--border-default)',
            borderRadius: 'var(--radius-md)',
            padding: '4px 8px',
            minWidth: 240,
            flex: '1 1 260px',
          }}
        >
          <div className="of-eyebrow">Search</div>
          <input
            type="text"
            value={draftSearch}
            onChange={(e) => setDraftSearch(e.target.value)}
            placeholder="Filter"
            style={{
              marginTop: 3,
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

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
        <button type="button" className="of-btn" onClick={resetFilters} disabled={busy}>
          Reset
        </button>
        <button type="button" className="of-btn of-btn-primary" onClick={applyFilters} disabled={busy}>
          {busy ? 'Applying…' : 'Apply'}
        </button>
      </div>
    </div>
  );
}
