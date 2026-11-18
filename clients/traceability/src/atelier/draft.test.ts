import assert from 'node:assert/strict';
import { test } from 'node:test';

import { clampCount, draftComplete, EMPTY_DRAFT, MAX_STONES, type Draft } from './draft.ts';

const ok: Draft = {
  ...EMPTY_DRAFT, title: 'Ring', metal: 'Gold', weightGrams: '7.5',
  origin: { workshop: 'Dakar', stoneSource: '', labCertificate: '' },
};

test('a stone count is a whole number from 1 to 9999', () => {
  assert.equal(clampCount(1.5), 1);
  assert.equal(clampCount(2.9), 2);
  assert.equal(clampCount(10000), 9999);
  assert.equal(clampCount(0), 1);
  assert.equal(clampCount(-3), 1);
  assert.equal(clampCount(NaN), 1);
  assert.equal(clampCount(40), 40);
});

test('a draft is complete only when the reader would accept the record', () => {
  assert.equal(draftComplete(ok), true);
  assert.equal(draftComplete({ ...ok, title: ' ' }), false);
  const stone = { kind: 'Diamond', carat: '0.25', count: 1 };
  assert.equal(draftComplete({ ...ok, stones: [stone] }), true);
  assert.equal(draftComplete({ ...ok, stones: [{ ...stone, count: 1.5 }] }), false);
  assert.equal(draftComplete({ ...ok, stones: [{ ...stone, count: 10000 }] }), false);
  assert.equal(draftComplete({ ...ok, stones: Array(MAX_STONES).fill(stone) }), true);
  assert.equal(draftComplete({ ...ok, stones: Array(MAX_STONES + 1).fill(stone) }), false);
  assert.equal(draftComplete({ ...ok, title: 'x'.repeat(121) }), false);
  assert.equal(draftComplete({ ...ok, weightGrams: '1234567890.12' }), false);
});
