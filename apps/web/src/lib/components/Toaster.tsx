import { notifications, useToasts, type Toast } from '@stores/notifications';

const TONE_CLASS: Record<Toast['type'], string> = {
  success: 'of-status-success',
  error: 'of-status-danger',
  info: 'of-status-info',
  warning: 'of-status-warning',
};

export function Toaster() {
  const toasts = useToasts();

  if (toasts.length === 0) return null;

  return (
    <div
      role="status"
      aria-live="polite"
      style={{
        position: 'fixed',
        right: 16,
        bottom: 16,
        zIndex: 60,
        display: 'flex',
        flexDirection: 'column',
        gap: 8,
        maxWidth: 360,
      }}
    >
      {toasts.map((toast) => (
        <button
          key={toast.id}
          type="button"
          onClick={() => notifications.dismiss(toast.id)}
          className={`of-panel ${TONE_CLASS[toast.type]}`}
          style={{
            padding: '10px 14px',
            border: '1px solid currentColor',
            fontSize: 13,
            textAlign: 'left',
            cursor: 'pointer',
          }}
        >
          {toast.message}
        </button>
      ))}
    </div>
  );
}
