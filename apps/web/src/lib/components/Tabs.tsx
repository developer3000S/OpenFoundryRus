import type { ReactNode } from 'react';

export interface TabDefinition<T extends string> {
  id: T;
  label?: ReactNode;
}

interface TabsProps<T extends string> {
  tabs: ReadonlyArray<T | TabDefinition<T>>;
  active: T;
  onChange: (next: T) => void;
}

export function Tabs<T extends string>({ tabs, active, onChange }: TabsProps<T>) {
  return (
    <div role="tablist" style={{ display: 'flex', flexWrap: 'wrap', gap: 4, borderBottom: '1px solid var(--border-default)' }}>
      {tabs.map((entry) => {
        const id = (typeof entry === 'string' ? entry : entry.id) as T;
        const label = typeof entry === 'string' ? entry : (entry.label ?? entry.id);
        const selected = active === id;
        return (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={selected}
            onClick={() => onChange(id)}
            style={{
              fontSize: 12,
              borderBottom: selected ? '2px solid #1d4ed8' : '2px solid transparent',
              background: 'transparent',
              border: 'none',
              padding: '8px 16px',
              cursor: 'pointer',
              color: selected ? 'var(--text-default)' : 'var(--text-muted)',
              textTransform: typeof label === 'string' ? 'capitalize' : undefined,
            }}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}
