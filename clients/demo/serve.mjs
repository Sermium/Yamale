// A development server for this page, and nothing else.
//
// It exists because the page is deliberately buildless — it is one HTML file
// and four ES modules, opened over http — and because verifying it means
// pointing it at a real chain rather than at a fixture. It serves this
// directory and proxies /api to the deployment, which is exactly what the
// production gateway does.
//
//   node serve.mjs                     → http://localhost:5180
//   YAMALE_ORIGIN=https://… node serve.mjs
//
// Not part of the deployed page. The deployed page is the static files beside
// this one.

import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = fileURLToPath(new URL('.', import.meta.url));
const ORIGIN = process.env.YAMALE_ORIGIN ?? 'https://pay.yamalelegal.com';
const PORT = Number(process.env.PORT ?? 5180);

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.json': 'application/json; charset=utf-8',
};

createServer(async (req, res) => {
  const url = new URL(req.url, 'http://localhost');

  // The proxy. Same two prefixes the production gateway publishes, so a path
  // that works here works there and a 401 here is the same 401 there.
  if (url.pathname.startsWith('/api/')) {
    try {
      const upstream = await fetch(ORIGIN + url.pathname + url.search, {
        headers: { accept: 'application/json' },
        signal: AbortSignal.timeout(20000),
      });
      const body = Buffer.from(await upstream.arrayBuffer());
      res.writeHead(upstream.status, {
        'content-type': upstream.headers.get('content-type') ?? 'application/json',
      });
      res.end(body);
    } catch (e) {
      // 503 rather than 500: the page's own failure branch treats it as
      // "cannot reach the chain", which is what it is.
      res.writeHead(503, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ error: String(e.message ?? e) }));
    }
    return;
  }

  // Static, confined to this directory. normalize before join, because a
  // request for /../../etc/passwd is the first thing anybody tries.
  const rel = normalize(decodeURIComponent(url.pathname)).replace(/^[\\/]+/, '');
  const file = join(ROOT, rel === '' ? 'index.html' : rel);
  if (!file.startsWith(ROOT.endsWith(sep) ? ROOT : ROOT + sep) && file !== join(ROOT, 'index.html')) {
    res.writeHead(403).end('forbidden');
    return;
  }
  try {
    const body = await readFile(file);
    res.writeHead(200, { 'content-type': TYPES[extname(file)] ?? 'application/octet-stream' });
    res.end(body);
  } catch {
    res.writeHead(404, { 'content-type': 'text/plain' }).end('not found');
  }
}).listen(PORT, () => {
  console.log(`demo hub on http://localhost:${PORT}  (api → ${ORIGIN})`);
});
