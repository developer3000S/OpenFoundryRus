interface LoadingStateProps {
  label?: string;
  inline?: boolean;
}

export function LoadingState({ label = 'Loading…', inline }: LoadingStateProps) {
  if (inline) {
    return (
      <span className="of-text-muted" style={{ fontSize: 12 }}>
        {label}
      </span>
    );
  }
  return <p className="of-text-muted">{label}</p>;
}

export function EmptyState({ label }: { label: string }) {
  return (
    <p className="of-text-muted" style={{ fontStyle: 'italic', fontSize: 13 }}>
      {label}
    </p>
  );
}
