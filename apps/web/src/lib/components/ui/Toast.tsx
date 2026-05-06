import type { ReactNode } from 'react';

interface ToastProps {
  open: boolean;
  tone?: 'info' | 'success' | 'warn' | 'error';
  children: ReactNode;
}

const TONE: Record<NonNullable<ToastProps['tone']>, { background: string; color: string; border: string }> = {
  info: { background: '#1e293b', color: '#cbd5e1', border: '#334155' },
  success: { background: '#022c22', color: '#86efac', border: '#10b981' },
  warn: { background: '#78350f', color: '#fde68a', border: '#f59e0b' },
  error: { background: '#7f1d1d', color: '#fecaca', border: '#dc2626' },
};

export function Toast({ open, tone = 'info', children }: ToastProps) {
  if (!open) return null;
  const palette = TONE[tone];
  return (
    <div
      role="status"
      style={{
        position: 'fixed',
        right: 24,
        bottom: 24,
        zIndex: 200,
        padding: '10px 14px',
        background: palette.background,
        color: palette.color,
        borderRadius: 12,
        boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
        border: `1px solid ${palette.border}`,
        fontSize: 13,
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        maxWidth: 480,
      }}
    >
      {children}
    </div>
  );
}
