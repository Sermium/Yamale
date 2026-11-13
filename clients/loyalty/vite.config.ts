import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

/**
 * The jewel app, served at the root of its own host (loyalty.yamaletech.com),
 * so `base` is '/' and the certificate card's path /j/<jeweler>/<jewel> is a
 * real path, not a fragment: the fragment is reserved for claim keys.
 *
 * Everything the app talks to is proxied under /api, as it will be in
 * production, so the browser never needs CORS and the claim key in a
 * fragment never meets a cross-origin request.
 */
const strip = (prefix: string) => (path: string) => path.replace(new RegExp(`^${prefix}`), '');

export default defineConfig({
  base: '/',
  plugins: [react()],
  server: {
    port: 5390,
    proxy: {
      '/api/rpc': { target: process.env.YAMALE_RPC ?? 'http://127.0.0.1:26957', changeOrigin: true, rewrite: strip('/api/rpc') },
      '/api/loyalty': { target: process.env.LOYALTY_API ?? 'http://127.0.0.1:8095', changeOrigin: true, rewrite: strip('/api/loyalty') },
      '/api/custodian': { target: process.env.YAMALE_CUSTODIAN ?? 'http://127.0.0.1:8099', changeOrigin: true, rewrite: strip('/api/custodian') },
      '/api/recovery': { target: process.env.YAMALE_RECOVERY ?? 'http://127.0.0.1:8100', changeOrigin: true, rewrite: strip('/api/recovery') },
    },
  },
});
