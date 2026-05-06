interface TabItem {
  key: string;
  label: string;
  badge?: number | string | null;
  disabled?: boolean;
}

interface Props {
  items: TabItem[];
  activeKey: string;
  onChange: (key: string) => void;
  ariaLabel?: string;
}

export function Tabs({ items, activeKey, onChange, ariaLabel = 'Tabs' }: Props) {
  return (
    <nav className="flex gap-1 overflow-x-auto border-b border-slate-200 dark:border-gray-800" aria-label={ariaLabel}>
      {items.map((tab) => {
        const active = activeKey === tab.key;
        return (
          <button
            key={tab.key}
            type="button"
            role="tab"
            aria-selected={active}
            disabled={tab.disabled}
            onClick={() => { if (!tab.disabled) onChange(tab.key); }}
            className={`inline-flex items-center gap-2 whitespace-nowrap border-b-2 px-3 py-2 text-sm font-medium transition-colors ${
              active
                ? 'border-blue-600 text-blue-700 dark:text-blue-300'
                : 'border-transparent text-slate-600 hover:border-slate-300 hover:text-slate-900 dark:text-gray-400 dark:hover:text-gray-100'
            } ${tab.disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
          >
            <span>{tab.label}</span>
            {tab.badge !== undefined && tab.badge !== null && tab.badge !== '' && (
              <span className="rounded-full bg-slate-100 px-1.5 text-[11px] font-medium text-slate-600 dark:bg-gray-700 dark:text-slate-300">{tab.badge}</span>
            )}
          </button>
        );
      })}
    </nav>
  );
}
