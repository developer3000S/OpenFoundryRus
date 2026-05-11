import { useMemo, useState } from 'react';

import type { WidgetCatalogItem } from '@/lib/api/apps';
import { Glyph, type GlyphName } from '@/lib/components/ui/Glyph';

interface WidgetCatalogProps {
  items: WidgetCatalogItem[];
  onSelect: (item: WidgetCatalogItem) => void;
}

const GLYPH_BY_TYPE: Record<string, GlyphName> = {
  agent: 'sparkles',
  button: 'run',
  chart: 'graph',
  container: 'cube',
  form: 'document',
  image: 'image',
  map: 'object',
  media_preview: 'image',
  media_uploader: 'artifact',
  metric: 'sparkles',
  scenario: 'settings',
  table: 'list',
  text: 'document',
};

function catalogGlyph(item: WidgetCatalogItem): GlyphName {
  return (item.display?.icon as GlyphName | undefined) ?? GLYPH_BY_TYPE[item.widget_type] ?? 'cube';
}

export function getWidgetCatalogItems(items: WidgetCatalogItem[]) {
  return items;
}

export function WidgetCatalog({ items, onSelect }: WidgetCatalogProps) {
  const catalog = useMemo(() => getWidgetCatalogItems(items), [items]);
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('all');

  const categories = useMemo(
    () => ['all', ...Array.from(new Set(catalog.map((item) => item.category))).sort()],
    [catalog],
  );

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return catalog.filter((item) => {
      const inCategory = category === 'all' || item.category === category;
      const haystack = `${item.label} ${item.widget_type} ${item.description} ${item.category}`.toLowerCase();
      return inCategory && (!needle || haystack.includes(needle));
    });
  }, [catalog, category, query]);

  return (
    <section style={{ display: 'grid', gap: 10 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <div>
          <p className="of-eyebrow" style={{ margin: 0 }}>Widget catalog</p>
          <p className="of-text-muted" style={{ margin: '3px 0 0', fontSize: 12 }}>
            {catalog.length} available widgets
          </p>
        </div>
      </div>

      <input
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder="Search widgets"
        className="of-input"
      />

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 5 }}>
        {categories.map((entry) => (
          <button
            key={entry}
            type="button"
            className={`of-button ${category === entry ? 'of-button--primary' : ''}`}
            onClick={() => setCategory(entry)}
            style={{ minHeight: 26, padding: '0 8px', textTransform: 'capitalize' }}
          >
            {entry}
          </button>
        ))}
      </div>

      <div style={{ display: 'grid', gap: 8 }}>
        {filtered.map((item) => (
          <button
            key={item.widget_type}
            type="button"
            onClick={() => onSelect(item)}
            style={{
              display: 'grid',
              gridTemplateColumns: '28px minmax(0, 1fr)',
              gap: 9,
              width: '100%',
              border: '1px solid var(--border-subtle)',
              borderRadius: 'var(--radius-md)',
              background: 'var(--bg-panel)',
              color: 'var(--text-default)',
              padding: 10,
              textAlign: 'left',
            }}
          >
            <span
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: 28,
                height: 28,
                border: '1px solid var(--border-subtle)',
                borderRadius: 'var(--radius-sm)',
                background: 'var(--bg-panel-muted)',
                color: 'var(--status-info)',
              }}
            >
              <Glyph name={catalogGlyph(item)} size={16} />
            </span>
            <span style={{ display: 'grid', gap: 4, minWidth: 0 }}>
              <span style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}>
                <strong style={{ color: 'var(--text-strong)' }}>{item.label || item.widget_type}</strong>
                <code style={{ color: 'var(--text-soft)', fontSize: 11 }}>{item.widget_type}</code>
              </span>
              <span className="of-text-muted" style={{ fontSize: 12, lineHeight: 1.4 }}>
                {item.description || 'Widget building block'}
              </span>
              {item.supported_bindings.length > 0 ? (
                <span style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                  {item.supported_bindings.slice(0, 3).map((binding) => (
                    <span key={binding} className="of-chip" style={{ minHeight: 20, fontSize: 11 }}>
                      {binding}
                    </span>
                  ))}
                </span>
              ) : null}
            </span>
          </button>
        ))}
        {filtered.length === 0 ? (
          <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>
            No widgets match the current filters.
          </p>
        ) : null}
      </div>
    </section>
  );
}
