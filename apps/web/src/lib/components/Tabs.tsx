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
    <div className="of-tabbar" role="tablist">
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
            className={`of-tab${selected ? ' of-tab-active' : ''}`}
            onClick={() => onChange(id)}
            style={{
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
