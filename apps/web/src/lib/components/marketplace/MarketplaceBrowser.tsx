import type { CategoryDefinition, ListingDefinition, MarketplaceOverview } from '@/lib/api/marketplace';
import { ListingCard } from './ListingCard';

interface Props {
  overview: MarketplaceOverview | null;
  categories: CategoryDefinition[];
  listings: ListingDefinition[];
  selectedListingId: string;
  searchQuery: string;
  selectedCategory: string;
  scoreById: Record<string, number>;
  busy?: boolean;
  onSearchQueryChange: (query: string) => void;
  onCategoryChange: (category: string) => void;
  onSearch: () => void;
  onSelectListing: (listingId: string) => void;
}

export function MarketplaceBrowser({
  overview,
  categories,
  listings,
  selectedListingId,
  searchQuery,
  selectedCategory,
  scoreById,
  busy = false,
  onSearchQueryChange,
  onCategoryChange,
  onSearch,
  onSelectListing,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#c2410c' }}>
            Marketplace Browser
          </p>
          <h2 className="of-heading-md" style={{ marginTop: 6 }}>
            Discovery across connectors, widgets, templates, models, and agents
          </h2>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Search and filter private listings backed by the new marketplace service.
          </p>
        </div>
        <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(3, 1fr)' }}>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: '#0c0a09', color: '#f5f5f4' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#fdba74' }}>Listings</p>
            <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{overview?.listing_count ?? 0}</p>
          </div>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: '#fff7ed', color: '#9a3412' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em' }}>Categories</p>
            <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{overview?.category_count ?? 0}</p>
          </div>
          <div style={{ borderRadius: 16, padding: '10px 14px', background: '#ecfdf5', color: '#047857' }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em' }}>Installs</p>
            <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{overview?.total_installs ?? 0}</p>
          </div>
        </div>
      </div>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 18 }}>
        <input
          value={searchQuery}
          onChange={(e) => onSearchQueryChange(e.target.value)}
          placeholder="Search widget, connector, agent…"
          className="of-input"
          style={{ flex: 1, minWidth: 200, borderRadius: 999 }}
        />
        <select
          value={selectedCategory}
          onChange={(e) => onCategoryChange(e.target.value)}
          className="of-input"
          style={{ width: 'auto', borderRadius: 999 }}
        >
          <option value="all">All categories</option>
          {categories.map((category) => (
            <option key={category.slug} value={category.slug}>
              {category.name}
            </option>
          ))}
        </select>
        <button type="button" onClick={onSearch} disabled={busy} className="of-button of-button--primary" style={{ background: '#f97316', color: '#0c0a09', borderRadius: 999 }}>
          Search
        </button>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.9fr) minmax(0, 1.1fr)', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14 }}>
          <p className="of-eyebrow">Categories</p>
          <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', marginTop: 10 }}>
            {categories.map((category) => {
              const active = selectedCategory === category.slug;
              return (
                <button
                  key={category.slug}
                  type="button"
                  onClick={() => onCategoryChange(category.slug)}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 12,
                    border: `1px solid ${active ? '#f97316' : 'var(--border-default)'}`,
                    background: active ? '#fff7ed' : 'var(--bg-elevated)',
                    borderRadius: 16,
                    cursor: 'pointer',
                  }}
                >
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{category.name}</p>
                  <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                    {category.description}
                  </p>
                  <p style={{ marginTop: 8, fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: 'var(--text-muted)' }}>
                    {category.listing_count} listings
                  </p>
                </button>
              );
            })}
          </div>
        </div>

        <div style={{ display: 'grid', gap: 8 }}>
          {listings.map((listing) => (
            <ListingCard
              key={listing.id}
              listing={listing}
              selected={selectedListingId === listing.id}
              score={scoreById[listing.id] ?? null}
              onSelect={() => onSelectListing(listing.id)}
            />
          ))}
          {listings.length === 0 && (
            <div
              style={{
                border: '1px dashed var(--border-default)',
                borderRadius: 16,
                padding: 32,
                textAlign: 'center',
                fontSize: 13,
                color: 'var(--text-muted)',
                background: 'var(--bg-subtle)',
              }}
            >
              No listings match the current discovery query.
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
