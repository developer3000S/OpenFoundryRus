import { Outlet } from 'react-router-dom';

import { Sidebar } from './Sidebar';
import { Toaster } from './Toaster';
import { Topbar } from './Topbar';

export function AppShell() {
  return (
    <div className="of-shell" style={{ display: 'flex' }}>
      <Sidebar />
      <main style={{ display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0 }}>
        <Topbar />
        <Outlet />
      </main>
      <Toaster />
    </div>
  );
}
