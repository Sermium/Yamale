/**
 * A local server for this console, with `/api` proxied to a real node.
 *
 * The console is one static file that talks to `/api/rest`, which is the path
 * the deployed site serves it on. Opening index.html from the filesystem gets
 * you a page with no chain behind it, and pointing it straight at
 * pay.yamalelegal.com from a file:// origin gets you CORS. So: serve the
 * directory, forward /api, and the page runs exactly as deployed.
 *
 *   node serve.js            # http://localhost:8788, against pay.yamalelegal.com
 *   YAMALE_UPSTREAM=https://host node serve.js 9000
 *
 * Development only. It serves one directory and forwards one path prefix.
 */

import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = fileURLToPath(new URL('.', import.meta.url));
const PORT = Number(process.argv[2] || 8788);
const UPSTREAM = process.env.YAMALE_UPSTREAM || 'https://pay.yamalelegal.com';

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
};

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://localhost:${PORT}`);

  if (url.pathname.startsWith('/api/')) {
    try {
      const upstream = await fetch(`${UPSTREAM}${url.pathname}${url.search}`, {
        headers: { accept: 'application/json' },
      });
      const body = Buffer.from(await upstream.arrayBuffer());
      res.writeHead(upstream.status, {
        'content-type': upstream.headers.get('content-type') || 'application/json',
      });
      res.end(body);
    } catch (e) {
      res.writeHead(502, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ message: `upstream unreachable: ${e.message}` }));
    }
    return;
  }

  // One directory, and nothing above it.
  const rel = url.pathname === '/' ? 'index.html' : normalize(url.pathname).replace(/^[/\\]+/, '');
  if (rel.includes('..')) { res.writeHead(403).end('no'); return; }
  try {
    const file = await readFile(join(HERE, rel));
    res.writeHead(200, { 'content-type': TYPES[extname(rel)] || 'application/octet-stream' });
    res.end(file);
  } catch {
    res.writeHead(404, { 'content-type': 'text/plain' });
    res.end('not found');
  }
});

server.listen(PORT, () => {
  console.log(`markets console on http://localhost:${PORT}  →  ${UPSTREAM}`);
});
