export type MarkingSource =
  | { kind: 'direct' }
  | { kind: 'inherited_from_upstream'; upstream_rid: string };

export type MarkingLevel = 'public' | 'confidential' | 'pii' | 'restricted' | 'unknown';

interface MarkingBadgeProps {
  label: string;
  level?: MarkingLevel;
  source: MarkingSource;
  id?: string;
  compact?: boolean;
}

const PALETTE: Record<MarkingLevel, { background: string; color: string; border: string }> = {
  public: { background: '#1e293b', color: '#cbd5e1', border: '#475569' },
  confidential: { background: '#78350f', color: '#fde68a', border: '#b45309' },
  pii: { background: '#7f1d1d', color: '#fecaca', border: '#dc2626' },
  restricted: { background: '#991b1b', color: '#fee2e2', border: '#ef4444' },
  unknown: { background: '#27272a', color: '#d4d4d8', border: '#52525b' },
};

export function MarkingBadge({ label, level = 'unknown', source, id, compact = false }: MarkingBadgeProps) {
  const tooltip = source.kind === 'direct' ? 'Direct' : `Inherited from ${source.upstream_rid}`;
  const tone = PALETTE[level];
  const sizing = compact
    ? { padding: '1px 6px', fontSize: 10 }
    : { padding: '2px 8px', fontSize: 11 };

  return (
    <span
      data-marking-id={id}
      data-marking-level={level}
      data-marking-source={source.kind}
      title={tooltip}
      aria-label={`${label} marking — ${tooltip}`}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
        borderRadius: 999,
        textTransform: 'uppercase',
        letterSpacing: '0.04em',
        fontWeight: 500,
        background: tone.background,
        color: tone.color,
        boxShadow: `inset 0 0 0 1px ${tone.border}`,
        ...sizing,
      }}
    >
      {label}
      {source.kind === 'inherited_from_upstream' && (
        <span aria-hidden="true" style={{ opacity: 0.7 }}>↑</span>
      )}
    </span>
  );
}
