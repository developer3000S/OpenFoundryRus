interface KernelStatusProps {
  kernel?: string;
  status?: string | null;
}

function toneClass(status: string | null) {
  switch (status) {
    case 'busy':
      return 'of-status-warning';
    case 'idle':
      return 'of-status-success';
    case 'dead':
      return 'of-status-danger';
    default:
      return '';
  }
}

export function KernelStatus({ kernel = 'python', status = null }: KernelStatusProps) {
  return (
    <span className={`of-chip ${toneClass(status)}`} style={{ fontSize: 11, fontWeight: 500 }}>
      <span style={{ width: 8, height: 8, borderRadius: 999, background: 'currentColor', opacity: 0.7 }} />
      {kernel} {status ?? 'offline'}
    </span>
  );
}
