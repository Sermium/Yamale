import assert from 'node:assert/strict';
import { test } from 'node:test';

import { clampCount, countFromInput, designComplete, draftComplete, EMPTY_DRAFT, MAX_STONES, type Draft } from './draft.ts';

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

test('a hand-registered record still states every stone: the jeweler holds the piece', () => {
  const d = { ...EMPTY_DRAFT, title: 'Ring', metal: 'Gold', weightGrams: '4', origin: { workshop: 'W', stoneSource: '', labCertificate: '' } };
  assert.equal(draftComplete({ ...d, stones: [{ kind: 'Diamond', carat: '1', count: 1 }] }), true);
  assert.equal(draftComplete({ ...d, stones: [{ kind: 'Diamond', carat: '', count: 1 }] }), false);
  assert.equal(draftComplete({ ...d, stones: [{ kind: 'Diamond', carat: '1', count: 0 }] }), false);
  assert.equal(draftComplete({ ...d, stones: [{ kind: 'Diamond', carat: '', count: 0 }] }, 2), true, 'a catalogue record corrected');
});

test('a design may leave its weight to the pieces, and nothing else', () => {
  const f = { metal: '18kt Yellow Gold', weightGrams: '', stones: [{ kind: 'Emerald', carat: '32', count: 12 }] };
  assert.equal(designComplete('Emerald Parrot Pendant', f), true);
  assert.equal(designComplete('', f), false);
  assert.equal(designComplete('P', { ...f, metal: ' ' }), false);
  assert.equal(designComplete('P', { ...f, weightGrams: 'heavy' }), false);
  // The website often does not say: a catalogue record then reads "not stated".
  assert.equal(designComplete('P', { ...f, stones: [{ kind: 'Diamond', carat: '', count: 422 }] }), true);
  assert.equal(designComplete('P', { ...f, stones: [{ kind: 'Diamond', carat: '1', count: 0 }] }), true);
  assert.equal(designComplete('P', { ...f, stones: [{ kind: 'Diamond', carat: '', count: 0 }] }), true);
  assert.equal(designComplete('P', { ...f, stones: [{ kind: ' ', carat: '1', count: 1 }] }), false, 'a stone is named');
  assert.equal(designComplete('P', { ...f, stones: [{ kind: 'Diamond', carat: 'a few', count: 1 }] }), false);
  assert.equal(designComplete('P', { ...f, stones: [{ kind: 'Diamond', carat: '', count: 10000 }] }), false);
});

test('a v2 draft may carry a longer lab certificate', () => {
  const long = { ...ok, origin: { ...ok.origin, labCertificate: 'x'.repeat(150) } };
  assert.equal(draftComplete(long), false);
  assert.equal(draftComplete(long, 2), true);
});

test('an empty count field means "not stated" on a catalogue design, and 1 on a hand-registered one', () => {
  assert.equal(countFromInput('', true), 0);
  assert.equal(countFromInput('  ', false), 1);
  assert.equal(countFromInput('12', true), 12);
  assert.equal(countFromInput('0', true), 1, 'a typed 0 is not "not stated": leave the field empty for that');
  assert.equal(countFromInput('123456', true), 9999);
});
