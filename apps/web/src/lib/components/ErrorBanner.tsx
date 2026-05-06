interface ErrorBannerProps {
  error: string | null | undefined;
}

export function ErrorBanner({ error }: ErrorBannerProps) {
  if (!error) return null;
  return (
    <div
      role="alert"
      className="of-status-danger"
      style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}
    >
      {error}
    </div>
  );
}
