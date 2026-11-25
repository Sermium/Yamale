import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Design } from '../../api.ts';
import { canConfirmDesign, confirmAll, filterDesigns, importErrorKey, missingForConfirm, readyToConfirm, shopOrigin } from './catalogue.ts';
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
  for (const c of ['unreadable', 'robots', 'refused', 'rateLimited', 'unreachable', 'busy', 'interrupted']) assert.ok(importErrorKey(c), c);
  assert.equal(importErrorKey('something new'), null);
});

test('the API design is what the CSV judge takes (checked by the type system)', () => {
  const accepts = (_: CatalogueDesign[]) => true;
  assert.ok(accepts([] as Design[]));
});

test('a photo that failed to download blocks confirming until it is left out', () => {
  type S = 'loading' | 'ok' | 'failed';
  const img = (state: S, chosen = true) => ({ state, chosen });
  const base = { gone: false, rights: true, complete: true, busy: false, images: [img('ok'), img('ok'), img('failed')] };
  assert.equal(canConfirmDesign(base), false, 'a chosen failed image');
  assert.equal(canConfirmDesign({ ...base, images: [img('ok'), img('ok'), img('failed', false)] }), true, 'the same image unticked');
  assert.equal(canConfirmDesign({ ...base, images: [img('ok'), img('loading')] }), false, 'still loading');
  assert.equal(canConfirmDesign({ ...base, images: [img('ok', false), img('failed', false)] }), false, 'no usable chosen image');
  assert.equal(canConfirmDesign({ ...base, images: [img('ok')], rights: false }), false, 'no rights tick');
  assert.equal(canConfirmDesign({ ...base, images: [img('ok')], gone: true }), false, 'gone');
  assert.equal(canConfirmDesign({ ...base, images: [img('ok')], complete: false }), false, 'incomplete');
  assert.equal(canConfirmDesign({ ...base, images: [img('ok')], busy: true }), false, 'busy');
  assert.equal(canConfirmDesign({ ...base, images: [img('ok')] }), true);
});

const fieldsOf = (metal: string, stones: { kind: string; carat: string; count: number }[] = [], weightGrams = '') => ({ metal, weightGrams, stones });

test('what stops a design being confirmed is named', () => {
  const d = (title: string, f: ReturnType<typeof fieldsOf>, photos = 1) => ({ title, fields: f, imageUrls: Array.from({ length: photos }, (_, n) => `u${n}`) });
  assert.deepEqual(missingForConfirm(d('Ring', fieldsOf('18kt Gold', [{ kind: 'Emerald', carat: '5.2', count: 1 }]))), []);
  assert.deepEqual(missingForConfirm(d('Ring', fieldsOf('', [{ kind: 'Amethyst', carat: '', count: 4 }]))), ['metal'], 'carats not stated');
  assert.deepEqual(missingForConfirm(d('Ring', fieldsOf('Gold', [{ kind: 'Diamond', carat: '3.5', count: 0 }]))), [], 'count not stated');
  assert.deepEqual(missingForConfirm(d('Ring', fieldsOf('Gold', [{ kind: '', carat: '1', count: 1 }]))), ['stones'], 'a stone without a name');
  assert.deepEqual(missingForConfirm(d('Ring', fieldsOf('Gold', [{ kind: 'Ruby', carat: 'some', count: 1 }]))), ['stones'], 'carats that are not a number');
  assert.deepEqual(missingForConfirm(d('', fieldsOf('Gold', [], 'heavy'), 0)), ['title', 'weight', 'photos']);
});

test('"Confirm all" takes only complete drafts, never a changed or confirmed design', () => {
  const ok = fieldsOf('Gold', [{ kind: 'Ruby', carat: '1', count: 2 }]);
  const mk = (id: string, status: string, f = ok) => ({ id, status, title: 'T', fields: f, imageUrls: ['a'] });
  const ready = readyToConfirm([mk('a', 'drafted'), mk('b', 'imported'), mk('c', 'changed'), mk('d', 'confirmed'), mk('e', 'gone'), mk('f', 'drafted', fieldsOf(''))]);
  assert.deepEqual(ready.map((x) => x.id), ['a', 'b']);
});

test('confirmAll goes design by design, leaves out unreadable photos, records failures and stops when asked', async () => {
  const designs = [
    { id: 'a', imageUrls: ['1', '2'] },
    { id: 'b', imageUrls: ['1'] },          // its only photo cannot be read
    { id: 'c', imageUrls: ['1'] },          // the confirm is refused
    { id: 'd', imageUrls: ['1'] },
    { id: 'e', imageUrls: ['1'] },
  ];
  const confirmed: { id: string; photos: string[] }[] = [];
  let stop = false;
  const res = await confirmAll(designs, {
    image: async (d, n) => (d.id === 'b' ? null : d.id === 'a' && n === 1 ? null : new Blob([`${d.id}${n}`])),
    upload: async (b) => ({ ok: true, hash: `h:${await b.text()}` }),
    confirm: async (d, photos) => {
      if (d.id === 'c') return { ok: false, raw: '409 stale' };
      confirmed.push({ id: d.id, photos });
      if (d.id === 'd') stop = true;
      return { ok: true };
    },
    progress: () => {},
    stopped: () => stop,
  });
  assert.deepEqual(confirmed, [{ id: 'a', photos: ['h:a0'] }, { id: 'd', photos: ['h:d0'] }]);
  assert.deepEqual(res.confirmed, ['a', 'd']);
  assert.deepEqual(res.failed.map((f) => f.id), ['b', 'c']);
  assert.equal(res.failed[1]!.raw, '409 stale');
  assert.deepEqual(res.notRun, ['e']);
});

test('an upload that fails fails its design, and the next design still runs', async () => {
  const res = await confirmAll([{ id: 'a', imageUrls: ['1', '2'] }, { id: 'b', imageUrls: ['1'] }], {
    image: async (d, n) => new Blob([`${d.id}${n}`]),
    upload: async (b) => ((await b.text()) === 'a1' ? { ok: false, raw: '500 disk' } : { ok: true, hash: 'h' }),
    confirm: async () => ({ ok: true }),
    progress: () => {}, stopped: () => false,
  });
  assert.deepEqual(res, { confirmed: ['b'], failed: [{ id: 'a', raw: '500 disk' }], notRun: [] });
});

test('a design already confirmed, and untouched since, cannot be confirmed again', () => {
  const base = { gone: false, rights: true, complete: true, busy: false, images: [{ state: 'ok' as const, chosen: true }] };
  assert.equal(canConfirmDesign({ ...base, settled: true }), false, 'confirmed and unchanged');
  assert.equal(canConfirmDesign({ ...base, settled: false }), true, 'edited after confirming');
});
