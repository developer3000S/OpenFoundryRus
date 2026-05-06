interface JobSpecBadgeProps {
  hasMasterJobspec: boolean;
  branchesWithJobspec?: string[];
  compact?: boolean;
}

export function JobSpecBadge({ hasMasterJobspec, branchesWithJobspec = [], compact = false }: JobSpecBadgeProps) {
  const tooltip = hasMasterJobspec
    ? `JobSpec defined on master${
        branchesWithJobspec.length > 1
          ? ` (also on ${branchesWithJobspec.filter((b) => b !== 'master').join(', ')})`
          : ''
      }`
    : 'No JobSpec on master';
  const tone = hasMasterJobspec
    ? { background: '#1e3a8a', color: '#bfdbfe' }
    : { background: '#374151', color: '#9ca3af' };
  return (
    <span
      title={tooltip}
      aria-label={tooltip}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
        borderRadius: 999,
        fontSize: 11,
        fontWeight: 500,
        ...(compact ? { width: 8, height: 8, padding: 0 } : { padding: '2px 8px' }),
        ...tone,
      }}
    >
      {!compact && (
        <>
          <span aria-hidden="true">●</span>
          <span>{hasMasterJobspec ? 'JobSpec on master' : 'No JobSpec'}</span>
        </>
      )}
    </span>
  );
}
