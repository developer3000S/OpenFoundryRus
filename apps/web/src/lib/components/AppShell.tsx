import { Outlet } from 'react-router-dom';

import { Sidebar } from './Sidebar';
import { Toaster } from './Toaster';
import { Topbar } from './Topbar';

export function AppShell() {
  return (
    <div className="of-shell" style={{ display: 'flex' }}>
      <Sidebar />
      <main className="of-main">
        <Topbar />
        <Outlet />
      </main>
      <Toaster />
    </div>
  );
}
