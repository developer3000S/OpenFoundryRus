import type { ReactNode } from 'react';

interface PageHeaderProps {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
}

export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
      <div>
        <h1 className="of-heading-xl">{title}</h1>
        {description && (
          <p className="of-text-muted" style={{ marginTop: 4, maxWidth: 720 }}>
            {description}
          </p>
        )}
      </div>
      {actions && <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>{actions}</div>}
    </header>
  );
}
