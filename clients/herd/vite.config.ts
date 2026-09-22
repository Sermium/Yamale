import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

/**
 * The field application.
 *
 * Served under /herd/ behind one origin, so every emitted asset URL carries
 * that path — without `base` the bundle asks for /assets/… at the root and gets
 * the site's 404 instead of its own JavaScript.
 */
export default defineConfig({
  base: '/herd/',
  plugins: [react()],
  server: {
    port: 5380,
    proxy: {
      '/api/rest': {
        target: process.env.YAMALE_REST ?? 'http://localhost:1317',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/rest/, ''),
      },
      '/api/rpc': {
        target: process.env.YAMALE_RPC ?? 'http://localhost:26657',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/rpc/, ''),
      },
    },
  },
});
