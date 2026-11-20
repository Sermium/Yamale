import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Design } from '../api.ts';
import { filterDesigns, importErrorKey, shopOrigin } from './catalogue.ts';
import type { CatalogueDesign } from './pieces.ts';

test('a shop address becomes an https origin, or nothing', () => {
  const chain = 'yamale-devnet-2';
  assert.equal(shopOrigin('www.jewelofafrica.com', chain), 'https://www.jewelofafrica.com');
  assert.equal(shopOrigin(' https://WWW.JewelOfAfrica.com/products ', chain), 'https://www.jewelofafrica.com');
  assert.equal(shopOrigin('https://shop.example:443/', chain), 'https://shop.example');
  for (const bad of ['', 'http://shop.example', 'https://user:pw@shop.example', 'localhost', 'ftp://x.example', 'http://127.0.0.1:8099']) {
    assert.equal(shopOrigin(bad, chain), null, bad);
  }
});

test('the local devnet may read its fixture shop, and only there', () => {
  assert.equal(shopOrigin('http://127.0.0.1:8099', 'yamale-local-1'), 'http://127.0.0.1:8099');
  assert.equal(shopOrigin('http://10.0.0.5:8099', 'yamale-local-1'), null);
  assert.equal(shopOrigin('http://127.0.0.1:8099', 'yamale-devnet-2'), null);
});

test('the review list holds what still needs a person', () => {
  const d = (status: string) => ({ status } as { status: 'imported' | 'drafted' | 'changed' | 'confirmed' | 'gone' });
  const all = ['imported', 'drafted', 'changed', 'confirmed', 'gone'].map(d);
  assert.deepEqual(filterDesigns(all, 'review').map((x) => x.status), ['changed', 'drafted', 'imported']);
  assert.deepEqual(filterDesigns(all, 'confirmed').map((x) => x.status), ['confirmed']);
  assert.deepEqual(filterDesigns(all, 'gone').map((x) => x.status), ['gone']);
});

test('every code the API sends has words', () => {
  for (const c of ['unreadable', 'robots', 'refused', 'rateLimited', 'unreachable', 'busy']) assert.ok(importErrorKey(c), c);
  assert.equal(importErrorKey('something new'), null);
});

test('the API design is what the CSV judge takes (checked by the type system)', () => {
  const accepts = (_: CatalogueDesign[]) => true;
  assert.ok(accepts([] as Design[]));
});
