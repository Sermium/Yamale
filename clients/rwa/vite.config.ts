import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

/**
 * The node is reached through the dev server rather than directly, for the same
 * reason as the other clients: a node that has to open CORS to be usable is a
 * node somebody eventually exposes that way in production.
 *
 * Two upstreams, and the split is not cosmetic. `/api/rpc` carries every read
 * this app makes of x/tokenisation and x/land, because those queries answer
 * unauthenticated over ABCI; `/api/rest` carries only the two bank reads — a
 * balance and a supply — which the proxy's allowlist does serve. Pointing both
 * at REST would produce a browser login box on the vehicle page, which is a 401
 * wearing a costume. See src/chain.ts.
 */
export default defineConfig({
  // Served under a path behind one origin, so every emitted asset URL has to
  // carry that path. Without this the bundle asks for /assets/... at the root
  // and gets the site's 404 instead of its own JavaScript.
  base: '/rwa/',
  plugins: [react()],
  server: {
    port: 5378,
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
