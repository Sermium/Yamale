import assert from 'node:assert/strict';
import { test } from 'node:test';

import { encodeRecord, parseRecord, type JewelRecord } from './record.ts';

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
