import { Outlet } from 'react-router-dom';

export function AuthLayout() {
  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24,
        background: 'var(--bg-app)',
      }}
    >
      <Outlet />
    </div>
  );
}
