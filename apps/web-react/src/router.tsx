import { createBrowserRouter } from 'react-router-dom';

import { AppShell } from '@components/AppShell';
import { AuthLayout } from '@components/AuthLayout';
import { Home } from './routes/Home';
import { NotFound } from './routes/NotFound';

export const router = createBrowserRouter([
  {
    path: '/auth',
    element: <AuthLayout />,
    children: [
      {
        path: 'login',
        lazy: async () => ({ Component: (await import('./routes/auth/LoginPage')).LoginPage }),
      },
      {
        path: 'register',
        lazy: async () => ({ Component: (await import('./routes/auth/RegisterPage')).RegisterPage }),
      },
      {
        path: 'mfa',
        lazy: async () => ({ Component: (await import('./routes/auth/MfaPage')).MfaPage }),
      },
      {
        path: 'callback',
        lazy: async () => ({ Component: (await import('./routes/auth/CallbackPage')).CallbackPage }),
      },
    ],
  },
  {
    path: '/',
    element: <AppShell />,
    errorElement: <NotFound />,
    children: [
      { index: true, element: <Home /> },
      {
        path: 'settings',
        lazy: async () => ({ Component: (await import('./routes/settings/SettingsPage')).SettingsPage }),
      },
      {
        path: 'charts-demo',
        lazy: async () => ({ Component: (await import('./routes/charts-demo/ChartsDemoPage')).ChartsDemoPage }),
      },
      // Migration pattern: add a route here as you port each SvelteKit folder under apps/web/src/routes/.
      { path: '*', element: <NotFound /> },
    ],
  },
]);
