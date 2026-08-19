import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

/**
 * The node's REST and RPC are proxied through the dev server rather than called
 * directly, so development does not depend on the node having CORS opened up.
 * A node that has to disable CORS to be usable is a node somebody will
 * eventually expose that way in production.
 */
export default defineConfig({
  // Served under a path behind one origin, so every emitted asset URL has to
  // carry that path. Without this the bundle asks for /assets/... at the root
  // and gets the site's 404 instead of its own JavaScript.
  base: '/explorer/',
  plugins: [react()],
  server: {
    port: 5173,
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
