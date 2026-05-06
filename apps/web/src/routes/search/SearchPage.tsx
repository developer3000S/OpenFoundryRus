import { Link } from 'react-router-dom';

interface DiscoveryCard {
  path: string;
  kicker: string;
  title: string;
  description: string;
}

const CARDS: DiscoveryCard[] = [
  {
    path: '/object-explorer',
    kicker: 'Explore',
    title: 'Object Explorer',
    description:
      'Search ontology objects, pivot across relationships, and reopen saved explorations.',
  },
  {
    path: '/queries',
    kicker: 'Analyze',
    title: 'Queries',
    description: 'Run SQL-style searches and inspect structured results.',
  },
  {
    path: '/ontology',
    kicker: 'Schema',
    title: 'Ontology',
    description: 'Inspect types, links, and schema details that shape search behavior.',
  },
];

export function SearchPage() {
  return (
    <div className="of-content-grid">
      <section className="of-panel" style={{ padding: 24 }}>
        <p className="of-eyebrow">Workspace</p>
        <h1 className="of-heading-lg" style={{ marginTop: 8 }}>
          Search
        </h1>
        <p
          className="of-text-muted"
          style={{ marginTop: 8, maxWidth: 720, fontSize: 14, lineHeight: 1.7 }}
        >
          Use the dedicated search entry in the shell to jump into discovery workflows, object
          exploration, and query-driven analysis.
        </p>
      </section>

      <section
        className="of-card-grid"
        style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', marginTop: 16 }}
      >
        {CARDS.map((card) => (
          <Link key={card.path} to={card.path} className="of-card" style={{ textDecoration: 'none' }}>
            <div className="of-kicker">{card.kicker}</div>
            <div className="of-heading-sm">{card.title}</div>
            <p className="of-text-muted" style={{ margin: 0, fontSize: 13 }}>
              {card.description}
            </p>
          </Link>
        ))}
      </section>
    </div>
  );
}
