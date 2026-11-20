#!/usr/bin/env node
// A stand-in for Jewel of Africa's shop, for the local devnet: the feed saved
// on 2026-11-16 (tools/traceabilityapi/testdata/catalogue), with every image
// replaced by a PNG drawn here. traceability-api reads it through
// --catalogue-dev-origin http://127.0.0.1:8099.
import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
import { deflateSync, crc32 } from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const data = join(here, '..', '..', 'tools', 'traceabilityapi', 'testdata', 'catalogue');
const ORIGIN = 'http://127.0.0.1:8099';
const feed = JSON.parse(readFileSync(join(data, 'shopify-products.json'), 'utf8'));
feed.products.forEach((p, i) => p.images.forEach((im, j) => { im.src = `${ORIGIN}/img/${i}-${j}.png`; }));
const meta = readFileSync(join(data, 'shopify-meta.json'));

function chunk(type, body) {
  const len = Buffer.alloc(4); len.writeUInt32BE(body.length);
  const tb = Buffer.from(type);
  const crc = Buffer.alloc(4); crc.writeUInt32BE(crc32(Buffer.concat([tb, body])) >>> 0);
  return Buffer.concat([len, tb, body, crc]);
}

function png(seed) {
  const w = 96, h = 96;
  const raw = Buffer.alloc((w * 3 + 1) * h);
  for (let y = 0; y < h; y++) {
    raw[y * (w * 3 + 1)] = 0;
    for (let x = 0; x < w; x++) {
      const o = y * (w * 3 + 1) + 1 + x * 3;
      raw[o] = (seed * 37 + x) & 255; raw[o + 1] = (seed * 91 + y) & 255; raw[o + 2] = 140;
    }
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4); ihdr[8] = 8; ihdr[9] = 2;
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(raw)), chunk('IEND', Buffer.alloc(0))]);
}

createServer((req, res) => {
  const u = new URL(req.url, ORIGIN);
  if (u.pathname === '/robots.txt') { res.end('User-agent: *\nDisallow: /admin\n'); return; }
  if (u.pathname === '/products.json') {
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify(u.searchParams.get('page') === '1' ? feed : { products: [] }));
    return;
  }
  if (u.pathname === '/meta.json') { res.setHeader('Content-Type', 'application/json'); res.end(meta); return; }
  const m = /^\/img\/(\d+)-(\d+)\.png$/.exec(u.pathname);
  if (m) { res.setHeader('Content-Type', 'image/png'); res.end(png(Number(m[1]) * 7 + Number(m[2]))); return; }
  res.statusCode = 404; res.end('not here');
}).listen(8099, '127.0.0.1', () => console.log(`fixture shop on ${ORIGIN}`));
