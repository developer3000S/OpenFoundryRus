import { defineConfig, loadEnv, type Plugin } from 'vite';
import react from '@vitejs/plugin-react-swc';
import cesium from 'vite-plugin-cesium';
import { fileURLToPath, URL } from 'node:url';
import path from 'node:path';
import fs from 'node:fs';

// Server-side OpenSky OAuth2 client_credentials token cache. Refreshed
// proactively every 25 min (tokens live ~30 min). The frontend never
// sees the credentials — it just calls /external/opensky/* and this
// proxy injects the bearer token.
let openSkyToken: { value: string; expiresAt: number } | null = null;
let openSkyRefreshTimer: NodeJS.Timeout | null = null;

// CelesTrak ToS allows only ~1 request per file per hour. We cache the
// active-satellites TLE feed for 6 hours in memory and serve from the
// cache instead of proxying every request. First request blocks while
// the cache fills; subsequent requests are O(1).
interface CelestrakCache {
  body: string;
  fetchedAt: number;
}
const celestrakCache = new Map<string, CelestrakCache>();
const CELESTRAK_TTL_MS = 6 * 60 * 60 * 1000;
const CELESTRAK_UA = 'OpenFoundry-WorldView/0.1 (local-dev; contact: mike@local.dev)';

async function fetchCelestrakCached(pathAndQuery: string): Promise<{ status: number; body: string; contentType: string }> {
  const cached = celestrakCache.get(pathAndQuery);
  if (cached && Date.now() - cached.fetchedAt < CELESTRAK_TTL_MS) {
    return { status: 200, body: cached.body, contentType: 'text/plain' };
  }
  const url = `https://celestrak.org${pathAndQuery}`;
  try {
    const res = await fetch(url, { headers: { 'User-Agent': CELESTRAK_UA } });
    if (!res.ok) {
      // If we got rate-limited but have a stale cache, return the stale data
      if (cached) {
        // eslint-disable-next-line no-console
        console.warn(`[celestrak] upstream ${res.status}; serving stale cache for ${pathAndQuery}`);
        return { status: 200, body: cached.body, contentType: 'text/plain' };
      }
      return { status: res.status, body: `upstream ${res.status}`, contentType: 'text/plain' };
    }
    const body = await res.text();
    celestrakCache.set(pathAndQuery, { body, fetchedAt: Date.now() });
    // eslint-disable-next-line no-console
    console.log(`[celestrak] cached ${pathAndQuery} (${(body.length / 1024).toFixed(1)} KB)`);
    return { status: 200, body, contentType: 'text/plain' };
  } catch (err) {
    if (cached) {
      // eslint-disable-next-line no-console
      console.warn(`[celestrak] fetch failed (${(err as Error).message}); serving stale cache`);
      return { status: 200, body: cached.body, contentType: 'text/plain' };
    }
    return { status: 502, body: (err as Error).message, contentType: 'text/plain' };
  }
}

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

function celestrakCachePlugin(): Plugin {
  return {
    name: 'celestrak-cached',
    configureServer(server) {
      // Warm the active-satellites entry from a local fallback file if
      // one exists — useful while CelesTrak has us rate-limited.
      try {
        const seed = fs.readFileSync('/tmp/tle.txt', 'utf8');
        if (seed.length > 1000) {
          celestrakCache.set('/NORAD/elements/gp.php?GROUP=active&FORMAT=tle', {
            body: seed,
            fetchedAt: Date.now() - CELESTRAK_TTL_MS / 2,
          });
          // eslint-disable-next-line no-console
          console.log(`[celestrak] warmed cache from /tmp/tle.txt (${(seed.length / 1024).toFixed(1)} KB)`);
        }
      } catch {
        /* no fallback file, fine */
      }
      server.middlewares.use('/external/celestrak', async (req, res) => {
        const result = await fetchCelestrakCached(req.url || '');
        res.statusCode = result.status;
        res.setHeader('Content-Type', result.contentType);
        res.end(result.body);
      });
    },
  };
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
    plugins: [react(), cesium(), celestrakCachePlugin()],
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
        // CelesTrak is handled by the celestrakCachePlugin below — see the
        // plugins[] array. It can't be a normal proxy because CelesTrak
        // requires a custom User-Agent + aggressive caching (1 req/hr/file).
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
