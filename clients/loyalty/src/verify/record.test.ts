import assert from 'node:assert/strict';
import { test } from 'node:test';

import { checkRecord, encodeRecord, parseRecord, recordMatches, stateForHash, type JewelRecord } from './record.ts';

const HASH = 'a'.repeat(64);
const ring: JewelRecord = {
  version: 1, jewelerId: '1', inscription: 'MD20260001', title: 'Solitaire ring', metal: '18k yellow gold',
  weightGrams: '4.2', stones: [{ kind: 'diamond', carat: '0.50', count: 1 }],
  origin: { workshop: 'Dakar, Médina', stoneSource: 'Botswana', labCertificate: 'GIA 2141438167' },
  photos: [HASH], createdAt: '2026-11-13T10:00:00.000Z',
};

test('a record survives encoding, and the same record always encodes the same', () => {
  assert.deepEqual(parseRecord(encodeRecord(ring)), ring);
  const { createdAt, photos, ...rest } = ring;
  const shuffled: JewelRecord = { createdAt, photos, ...rest };
  assert.deepEqual(encodeRecord(shuffled), encodeRecord(ring));
});

test('a record that is not version 1, or names a bad photo, is not a record', () => {
  const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
  assert.equal(parseRecord(enc({ ...ring, version: 2 })), null);
  assert.equal(parseRecord(enc({ ...ring, photos: ['A'.repeat(64)] })), null);
  assert.equal(parseRecord(enc({ ...ring, photos: [] })), null);
  assert.equal(parseRecord(enc({ ...ring, stones: [{ kind: 'ruby', carat: '1', count: 0 }] })), null);
  assert.equal(parseRecord(new TextEncoder().encode('not json')), null);
});

test('fields nobody asked for do not survive parsing', () => {
  const extra = new TextEncoder().encode(JSON.stringify({ ...ring, owner: 'Awa Diop' }));
  assert.equal('owner' in (parseRecord(extra) as object), false);
});

test('a state is shown only for the hash it was fetched for', () => {
  const held = { hash: HASH, state: { state: 'ok' as const, record: ring } };
  assert.equal(stateForHash(held, HASH), held.state);
  assert.deepEqual(stateForHash(held, 'b'.repeat(64)), { state: 'loading' });
  assert.deepEqual(stateForHash(held, null), { state: 'loading' });
  assert.deepEqual(stateForHash({ hash: null, state: { state: 'missing' } }, null), { state: 'loading' });
});

test("a record counts as a jewel's only if it names that jewel's inscription and jeweler", () => {
  const jewel = { inscription: 'MD20260001', jewelerId: '1' };
  assert.equal(recordMatches(ring, jewel), true);
  assert.equal(recordMatches(ring, { ...jewel, inscription: 'MD20260002' }), false);
  assert.equal(recordMatches(ring, { ...jewel, jewelerId: '2' }), false);
});

test("another jewel's record reads as a mismatch, and other states pass through", () => {
  const ok = { state: 'ok' as const, record: ring };
  assert.equal(checkRecord(ok, { inscription: 'MD20260001', jewelerId: '1' }), ok);
  assert.deepEqual(checkRecord(ok, { inscription: 'CHEAP-0001', jewelerId: '1' }), { state: 'mismatch' });
  assert.deepEqual(checkRecord(ok, { inscription: 'MD20260001', jewelerId: '7' }), { state: 'mismatch' });
  for (const state of ['loading', 'missing', 'mismatch', 'unreachable', 'unreadable'] as const) {
    assert.deepEqual(checkRecord({ state }, { inscription: 'X', jewelerId: '9' }), { state });
  }
});
