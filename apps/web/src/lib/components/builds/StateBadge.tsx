import { BUILD_STATE_COLORS, JOB_STATE_COLORS, type BuildState, type JobState } from '@/lib/api/buildsV1';

interface Props {
  kind: 'build' | 'job';
  state: BuildState | JobState | string;
  size?: 'sm' | 'md';
}

const FALLBACK = { bg: '#374151', text: '#e5e7eb' };

export function StateBadge({ kind, state, size = 'md' }: Props) {
  const palette = kind === 'build'
    ? (BUILD_STATE_COLORS[state as BuildState] ?? FALLBACK)
    : (JOB_STATE_COLORS[state as JobState] ?? FALLBACK);
  const pulse = kind === 'build' ? Boolean((BUILD_STATE_COLORS[state as BuildState] ?? {}).pulse) : false;

  const sizeStyle: React.CSSProperties = size === 'sm' ? { padding: '1px 6px', fontSize: 10 } : { padding: '2px 8px', fontSize: 11 };

  return (
    <span
      data-state={state}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        borderRadius: 999,
        background: palette.bg,
        color: palette.text,
        fontWeight: 600,
        fontFamily: "ui-monospace, 'SF Mono', Consolas, monospace",
        letterSpacing: '0.02em',
        whiteSpace: 'nowrap',
        ...sizeStyle,
      }}
    >
      {pulse && (
        <span style={{
          width: 6,
          height: 6,
          borderRadius: '50%',
          background: palette.text,
          marginRight: 6,
          animation: 'of-pulse 1.4s ease-in-out infinite',
        }} />
      )}
      {state}
      <style>{`@keyframes of-pulse { 0%,100% { opacity: 0.3; } 50% { opacity: 1; } }`}</style>
    </span>
  );
}
