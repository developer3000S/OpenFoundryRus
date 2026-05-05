import { Link } from 'react-router-dom';

const MIGRATED_ROUTES: { path: string; title: string; description: string }[] = [
  { path: '/settings', title: 'Settings', description: 'Identity, RBAC, ABAC, MFA, API keys, SSO.' },
  { path: '/auth/login', title: 'Sign in', description: 'Login + register + MFA + SSO callback.' },
  { path: '/charts-demo', title: 'Charts demo', description: 'ECharts wrapper validator.' },
];

export function Home() {
  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header className="of-hero-strip">
        <p className="of-eyebrow">OpenFoundry</p>
        <h1 className="of-heading-xl">React shell</h1>
        <p className="of-text-muted">
          Routes ported so far. The Svelte app at <code>apps/web</code> still owns everything
          else; each migrated folder gets registered in <code>src/router.tsx</code>.
        </p>
      </header>

      <div
        className="of-card-grid"
        style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))' }}
      >
        {MIGRATED_ROUTES.map((route) => (
          <Link key={route.path} to={route.path} className="of-card" style={{ textDecoration: 'none' }}>
            <p className="of-eyebrow">{route.path}</p>
            <h2 className="of-heading-md">{route.title}</h2>
            <p className="of-text-muted" style={{ fontSize: 13 }}>
              {route.description}
            </p>
          </Link>
        ))}
      </div>
    </section>
  );
}
