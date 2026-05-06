import {
  createDefaultDateRange,
  resolveDateRange,
  type DashboardDatePreset,
  type DashboardDateRange,
} from '@/lib/utils/dashboards';

const PRESET_OPTIONS: Array<{ value: DashboardDatePreset; label: string }> = [
  { value: 'last_7_days', label: 'Last 7 days' },
  { value: 'last_30_days', label: 'Last 30 days' },
  { value: 'last_90_days', label: 'Last 90 days' },
  { value: 'this_month', label: 'This month' },
  { value: 'quarter_to_date', label: 'Quarter to date' },
  { value: 'custom', label: 'Custom range' },
];

interface DateRangeFilterProps {
  value: DashboardDateRange;
  disabled?: boolean;
  onChange?: (value: DashboardDateRange) => void;
}

export function DateRangeFilter({ value, disabled = false, onChange }: DateRangeFilterProps) {
  const resolved = resolveDateRange(value);

  function handlePresetChange(e: React.ChangeEvent<HTMLSelectElement>) {
    const preset = e.target.value as DashboardDatePreset;
    if (preset === 'custom') {
      const fallback = createDefaultDateRange();
      onChange?.({
        mode: 'absolute',
        preset,
        from: value.from || fallback.from,
        to: value.to || fallback.to,
      });
      return;
    }
    const next: DashboardDateRange = { mode: 'relative', preset, from: value.from, to: value.to };
    const nextRange = resolveDateRange(next);
    onChange?.({ ...next, from: nextRange.from, to: nextRange.to });
  }

  function handleAbsoluteChange(key: 'from' | 'to', nextValue: string) {
    onChange?.({
      mode: 'absolute',
      preset: 'custom',
      from: key === 'from' ? nextValue : value.from,
      to: key === 'to' ? nextValue : value.to,
    });
  }

  const showCustomInputs = value.mode === 'absolute' || value.preset === 'custom';

  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        gap: 12,
        background: '#fff',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-md)',
        padding: '8px 12px',
      }}
    >
      <div>
        <div className="of-eyebrow">Date Range</div>
        <div style={{ marginTop: 4, fontSize: 13, fontWeight: 500, color: 'var(--text-strong)' }}>
          {resolved.label}
        </div>
      </div>

      <select className="of-select" value={value.preset} onChange={handlePresetChange} disabled={disabled} style={{ width: 'auto' }}>
        {PRESET_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>

      {showCustomInputs && (
        <>
          <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13 }}>
            <span>From</span>
            <input
              type="date"
              className="of-input"
              value={value.from}
              onChange={(e) => handleAbsoluteChange('from', e.target.value)}
              disabled={disabled}
              style={{ width: 'auto' }}
            />
          </label>
          <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13 }}>
            <span>To</span>
            <input
              type="date"
              className="of-input"
              value={value.to}
              onChange={(e) => handleAbsoluteChange('to', e.target.value)}
              disabled={disabled}
              style={{ width: 'auto' }}
            />
          </label>
        </>
      )}
    </div>
  );
}
