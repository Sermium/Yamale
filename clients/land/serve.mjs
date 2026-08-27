// A local server for driving this console against the live chain.
//
// Not part of the console and not deployed with it. It exists because the two
// halves of "does this work" cannot be checked from the same place: the page is
// a working-tree file, and the chain it reads is behind a proxy on another
// origin. Serving the working tree and forwarding /api to that proxy puts them
// on one origin, which is also the shape the real deployment has — so a browser
// driving this is exercising the same relative URLs the deployed page uses,
// rather than a CORS variant of them.
//
//   node serve.mjs [port] [upstream]
//
// Defaults to 8130 and https://pay.yamalelegal.com. Reads only: nothing here
// forwards a POST, because a mistake in a throwaway proxy should not be able to
// broadcast a transaction.

import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';

const PORT = Number(process.argv[2] ?? 8130);
const UPSTREAM = process.argv[3] ?? 'https://pay.yamalelegal.com';
const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname.slice(1)), '..');

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.json': 'application/json; charset=utf-8',
};

http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://localhost:${PORT}`);

  if (url.pathname.startsWith('/api/')) {
    try {
      const upstream = await fetch(`${UPSTREAM}${req.url}`, { redirect: 'manual' });
      const body = Buffer.from(await upstream.arrayBuffer());
      res.writeHead(upstream.status, {
        'content-type': upstream.headers.get('content-type') ?? 'application/octet-stream',
      });
      res.end(body);
    } catch (e) {
      res.writeHead(502, { 'content-type': 'text/plain' });
      res.end(`upstream: ${e.message}`);
    }
    return;
  }

  let file = path.join(ROOT, decodeURIComponent(url.pathname));
  if (!file.startsWith(ROOT)) { res.writeHead(403).end('no'); return; }
  if (fs.existsSync(file) && fs.statSync(file).isDirectory()) file = path.join(file, 'index.html');

  // Anything the working tree does not hold comes from upstream. That is what
  // makes the wallet reachable at /wallet on this origin, which the signing
  // path requires: the console opens `${origin}/wallet/connect`, and the whole
  // security model of that protocol is that both ends check the origin.
  if (!fs.existsSync(file)) {
    try {
      const upstream = await fetch(`${UPSTREAM}${req.url}`, { redirect: 'follow' });
      const body = Buffer.from(await upstream.arrayBuffer());
      res.writeHead(upstream.status, {
        'content-type': upstream.headers.get('content-type') ?? 'application/octet-stream',
      });
      res.end(body);
    } catch (e) {
      res.writeHead(404, { 'content-type': 'text/plain' });
      res.end(`not here, and upstream said: ${e.message}`);
    }
    return;
  }

  res.writeHead(200, {
    'content-type': TYPES[path.extname(file)] ?? 'application/octet-stream',
    // Never cached. The whole point of this server is to look at an edit, and a
    // browser holding the previous index.html means a measurement of a file
    // that is no longer on disk — which is worse than not measuring.
    'cache-control': 'no-store, must-revalidate',
  });
  res.end(fs.readFileSync(file));
}).listen(PORT, () => {
  console.log(`land console on http://localhost:${PORT}/land/ · /api → ${UPSTREAM}`);
});
