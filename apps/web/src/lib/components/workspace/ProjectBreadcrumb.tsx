import { Glyph } from '@/lib/components/ui/Glyph';

export interface BreadcrumbItem {
  id: string;
  label: string;
  href?: string;
}

interface ProjectBreadcrumbProps {
  items: BreadcrumbItem[];
  onNavigate?: (item: BreadcrumbItem, index: number) => void;
}

export function ProjectBreadcrumb({ items, onNavigate }: ProjectBreadcrumbProps) {
  return (
    <nav aria-label="Breadcrumb" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 4, fontSize: 13 }}>
      {items.map((item, index) => (
        <span key={item.id} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
          {index > 0 && (
            <span aria-hidden="true" style={{ color: '#475569' }}>
              <Glyph name="chevron-right" size={12} />
            </span>
          )}
          {index === items.length - 1 ? (
            <span style={{ fontWeight: 600 }}>{item.label}</span>
          ) : item.href ? (
            <a
              href={item.href}
              onClick={(e) => {
                if (onNavigate) {
                  e.preventDefault();
                  onNavigate(item, index);
                }
              }}
              style={{ color: '#60a5fa', textDecoration: 'none' }}
            >
              {item.label}
            </a>
          ) : (
            <button type="button" onClick={() => onNavigate?.(item, index)} style={{ background: 'transparent', border: 'none', color: '#60a5fa', cursor: 'pointer', textDecoration: 'underline', padding: 0, fontSize: 13 }}>
              {item.label}
            </button>
          )}
        </span>
      ))}
    </nav>
  );
}
