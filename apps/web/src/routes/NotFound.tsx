import { Link, useRouteError } from 'react-router-dom';

export function NotFound() {
  const error = useRouteError() as { statusText?: string; message?: string } | undefined;
  return (
    <section className="of-page">
      <div className="of-panel" style={{ padding: 24 }}>
        <p className="of-eyebrow">404</p>
        <h1 className="of-heading-lg">Route not migrated yet</h1>
        <p className="of-text-muted">
          {error?.statusText ?? error?.message ?? 'This page has not been ported from the Svelte app.'}
        </p>
        <p>
          <Link to="/" className="of-link">
            Back to home
          </Link>
        </p>
      </div>
    </section>
  );
}
