import { test } from 'node:test';
import assert from 'node:assert/strict';

import { groupHoldings, mapLimit, type Held } from './holdings.ts';

const h = (id: string, inscription: string, title: string | null, sku = '', more: Partial<Held> = {}): Held =>
  ({ id, jewelerId: '1', inscription, title, sku, stolen: false, onOffer: false, ...more });

test('pieces of one design are one row, sorted by name, serials in natural order', () => {
  const groups = groupHoldings([
    h('1', 'JOA0710010', 'Emerald Parrot Pendant', 'JOA-071'),
    h('2', 'JOA0290001', 'Aquamarine Ring', 'JOA-029'),
    h('3', 'JOA0710002', 'Emerald Parrot Pendant', 'JOA-071'),
    h('4', 'JOA0710009', 'Emerald Parrot Pendant', 'JOA-071'),
  ]);
  assert.deepEqual(groups.map((g) => [g.title, g.items.length]), [['Aquamarine Ring', 1], ['Emerald Parrot Pendant', 3]]);
  assert.deepEqual(groups[1]!.items.map((i) => i.inscription), ['JOA0710002', 'JOA0710009', 'JOA0710010']);
});

test('the design code groups, not the title: two designs may share a name', () => {
  const groups = groupHoldings([h('1', 'A0001', 'Ring', 'R-1'), h('2', 'A0002', 'Ring', 'R-2'), h('3', 'A0003', 'Ring', 'R-1')]);
  assert.deepEqual(groups.map((g) => g.items.map((i) => i.id)), [['1', '3'], ['2']]);
});

test('hand-registered jewels without a code group by title; another jeweler never joins', () => {
  const groups = groupHoldings([h('1', 'B0001', 'Bangle'), h('2', 'B0002', 'Bangle'), h('3', 'B0003', 'Bangle', '', { jewelerId: '2' })]);
  assert.deepEqual(groups.map((g) => g.items.map((i) => i.id)), [['1', '2'], ['3']]);
});

test('a jewel whose record could not be read stands alone, after the named ones', () => {
  const groups = groupHoldings([h('9', 'X0009', null), h('8', 'X0008', null), h('1', 'A0001', 'Zircon Ring')]);
  assert.deepEqual(groups.map((g) => g.items.map((i) => i.id)), [['1'], ['8'], ['9']]);
});

test('a row counts its stolen and on-offer pieces', () => {
  const [g] = groupHoldings([h('1', 'A1', 'R', 'S'), h('2', 'A2', 'R', 'S', { stolen: true }), h('3', 'A3', 'R', 'S', { onOffer: true })]);
  assert.equal(g!.stolen, 1);
  assert.equal(g!.onOffer, 1);
});

test('mapLimit keeps order and never runs more than the limit at once', async () => {
  let running = 0, peak = 0;
  const out = await mapLimit([5, 1, 4, 2, 3, 6, 7], 3, async (n) => {
    running++; peak = Math.max(peak, running);
    await new Promise((r) => setTimeout(r, n));
    running--;
    return n * 10;
  });
  assert.deepEqual(out, [50, 10, 40, 20, 30, 60, 70]);
  assert.ok(peak <= 3, `peak ${peak}`);
});
