import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

/**
 * accounts.yamaletech.com, served by the chain gate under /accounts/ like the
 * other chain apps. It talks only to the account services, proxied here to the
 * live ones as in production.
 */
export default defineConfig({
  base: '/accounts/',
  plugins: [react()],
  server: {
    port: 5395,
    proxy: {
      '/api/custodian': { target: process.env.YAMALE_ACCOUNTS ?? 'https://pay.yamaletech.com', changeOrigin: true },
      '/api/recovery': { target: process.env.YAMALE_ACCOUNTS ?? 'https://pay.yamaletech.com', changeOrigin: true },
    },
  },
});
