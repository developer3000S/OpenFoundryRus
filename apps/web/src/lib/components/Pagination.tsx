interface PaginationProps {
  page: number;
  perPage: number;
  total: number;
  onChange: (next: number) => void;
}

export function Pagination({ page, perPage, total, onChange }: PaginationProps) {
  const lastPage = Math.max(1, Math.ceil(total / perPage));
  return (
    <nav style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12 }} aria-label="Pagination">
      <button
        type="button"
        onClick={() => onChange(Math.max(1, page - 1))}
        disabled={page <= 1}
        className="of-button"
        style={{ fontSize: 11 }}
      >
        ← Prev
      </button>
      <span className="of-text-muted">
        Page {page} / {lastPage} · {total} total
      </span>
      <button
        type="button"
        onClick={() => onChange(Math.min(lastPage, page + 1))}
        disabled={page >= lastPage}
        className="of-button"
        style={{ fontSize: 11 }}
      >
        Next →
      </button>
    </nav>
  );
}
