import type { ListingDefinition } from '@/lib/api/marketplace';

interface Props {
  listing: ListingDefinition;
  selected?: boolean;
  score: number | null;
  onSelect: () => void;
}

export function ListingCard({ listing, selected = false, score, onSelect }: Props) {
  return (
    <button
      type="button"
      onClick={onSelect}
      style={{
        width: '100%',
        textAlign: 'left',
        padding: 14,
        border: `1px solid ${selected ? '#f59e0b' : 'var(--border-default)'}`,
        background: selected ? '#fffbeb' : 'var(--bg-elevated)',
        borderRadius: 24,
        cursor: 'pointer',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
        <div>
          <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{listing.name}</p>
          <p className="of-text-muted" style={{ fontSize: 13 }}>
            {listing.publisher} · {listing.package_kind}
          </p>
        </div>
        {score !== null && (
          <span className="of-chip" style={{ background: '#0c0a09', color: '#f5f5f4', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
            {score.toFixed(2)}
          </span>
        )}
      </div>
      <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
        {listing.summary}
      </p>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
        {listing.tags.map((tag) => (
          <span key={tag} className="of-chip">
            {tag}
          </span>
        ))}
      </div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: 11, color: 'var(--text-muted)', marginTop: 10 }}>
        <span>{listing.install_count} installs</span>
        <span>{listing.average_rating.toFixed(1)} rating</span>
      </div>
    </button>
  );
}
