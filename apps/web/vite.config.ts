import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react-swc';
import { fileURLToPath, URL } from 'node:url';
import path from 'node:path';

// Server-side OpenSky OAuth2 client_credentials token cache. Refreshed
// proactively every 25 min (tokens live ~30 min). The frontend never
// sees the credentials — it just calls /external/opensky/* and this
// proxy injects the bearer token.
let openSkyToken: { value: string; expiresAt: number } | null = null;
let openSkyRefreshTimer: NodeJS.Timeout | null = null;

async function refreshOpenSkyToken(clientId: string, clientSecret: string) {
  try {
    const res = await fetch(
      'https://auth.opensky-network.org/auth/realms/opensky-network/protocol/openid-connect/token',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({
          grant_type: 'client_credentials',
          client_id: clientId,
          client_secret: clientSecret,
        }),
      },
    );
    if (!res.ok) {
      // eslint-disable-next-line no-console
      console.warn(`[opensky] token refresh failed: HTTP ${res.status} — ${await res.text()}`);
      return;
    }
    const data = (await res.json()) as { access_token: string; expires_in: number };
    openSkyToken = {
      value: data.access_token,
      expiresAt: Date.now() + Math.max(0, (data.expires_in - 30) * 1000),
    };
    // eslint-disable-next-line no-console
    console.log(`[opensky] token refreshed, expires in ${data.expires_in}s`);
  } catch (err) {
    // eslint-disable-next-line no-console
    console.warn('[opensky] token refresh threw:', err);
  }
}

export default defineConfig(({ mode }) => {
  // .env lives at the repo root; pnpm --filter starts Vite with cwd
  // pointing at apps/web, so we have to look upward two levels.
  const repoRoot = path.resolve(fileURLToPath(new URL('.', import.meta.url)), '../..');
  const env = loadEnv(mode, repoRoot, '');
  const openSkyId = env.OPENSKY_CLIENT_ID;
  const openSkySecret = env.OPENSKY_CLIENT_SECRET;

  if (openSkyId && openSkySecret) {
    void refreshOpenSkyToken(openSkyId, openSkySecret);
    if (openSkyRefreshTimer) clearInterval(openSkyRefreshTimer);
    openSkyRefreshTimer = setInterval(
      () => refreshOpenSkyToken(openSkyId, openSkySecret),
      25 * 60 * 1000,
    );
  } else {
    // eslint-disable-next-line no-console
    console.warn('[opensky] OPENSKY_CLIENT_ID/SECRET not set — /external/opensky proxy will pass through unauthenticated');
  }

  return {
    plugins: [react()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
        '@api': fileURLToPath(new URL('./src/lib/api', import.meta.url)),
        '@components': fileURLToPath(new URL('./src/lib/components', import.meta.url)),
        '@stores': fileURLToPath(new URL('./src/lib/stores', import.meta.url)),
        '@utils': fileURLToPath(new URL('./src/lib/utils', import.meta.url)),
      },
    },
    build: {
      chunkSizeWarningLimit: 2600,
    },
    server: {
      host: '0.0.0.0',
      port: 5174,
      proxy: {
        '/api/v1/data-connection/egress-policies': {
          target: 'http://127.0.0.1:50119',
          changeOrigin: true,
        },
        '/api/v1/data-connection': {
          target: 'http://127.0.0.1:50088',
          changeOrigin: true,
        },
        '/api/v1/auth': {
          target: 'http://127.0.0.1:50112',
          changeOrigin: true,
        },
        '/api/v1/users/me': {
          target: 'http://127.0.0.1:50112',
          changeOrigin: true,
        },
        '/api/v1/geospatial': {
          target: 'http://127.0.0.1:50131',
          changeOrigin: true,
        },
        '/external/opensky': {
          target: 'https://opensky-network.org',
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/external\/opensky/, ''),
          configure: (proxy) => {
            proxy.on('proxyReq', (proxyReq) => {
              if (openSkyToken && Date.now() < openSkyToken.expiresAt) {
                proxyReq.setHeader('Authorization', `Bearer ${openSkyToken.value}`);
              }
            });
          },
        },
        '/api': {
          target: 'http://127.0.0.1:8080',
          changeOrigin: true,
          ws: true,
        },
      },
    },
  };
});
