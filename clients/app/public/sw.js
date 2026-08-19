/**
 * The service worker — what makes this installable, and what makes it usable on
 * a bad connection.
 *
 * Deliberately conservative about what it caches. Two rules:
 *
 *   1. **The shell is cached, the money is not.** Code and icons are safe to
 *      serve from cache; balances, transactions and quotes are not. A wallet
 *      that shows a stale balance from cache is a wallet that lies about money,
 *      so every /api/ request goes to the network and is never stored.
 *   2. **A new version wins immediately.** skipWaiting + clients.claim, because
 *      an installed app that keeps running last week's bundle against a chain
 *      that has moved on is a support problem nobody can diagnose from the
 *      outside.
 */
const VERSION = 'yamale-v1';
const SHELL = [
  '/app/',
  '/app/index.html',
  '/app/mark.svg',
  '/app/manifest.webmanifest',
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(VERSION)
      // addAll fails the whole install if one entry 404s, which would leave the
      // app uninstallable over a hashed-asset rename. Each is added best-effort.
      .then((cache) => Promise.allSettled(SHELL.map((url) => cache.add(url))))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') return;

  const url = new URL(request.url);

  // Never cache anything that talks to the chain or the faucet. Balances must
  // be true or absent, never remembered.
  if (url.pathname.startsWith('/api/')) return;

  // Navigations: network first so a fresh build is picked up, falling back to
  // the cached shell so the app still opens with no signal.
  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request).catch(() => caches.match('/app/index.html').then((r) => r || Response.error())),
    );
    return;
  }

  // Static assets are content-hashed by the build, so a cache hit is always the
  // right bytes for that URL.
  if (url.origin === self.location.origin) {
    event.respondWith(
      caches.match(request).then((hit) => hit || fetch(request).then((res) => {
        if (res.ok && res.type === 'basic') {
          const copy = res.clone();
          caches.open(VERSION).then((c) => c.put(request, copy));
        }
        return res;
      }).catch(() => hit || Response.error())),
    );
  }
});
