import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';
import { fileURLToPath, URL } from 'node:url';

export default defineConfig({
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
        target: 'http://127.0.0.1:50088',
        changeOrigin: true,
      },
      '/api/v1/users/me': {
        target: 'http://127.0.0.1:50088',
        changeOrigin: true,
      },
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
