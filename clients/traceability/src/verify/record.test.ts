import assert from 'node:assert/strict';
import { test } from 'node:test';

import { checkRecord, encodeRecord, parseRecord, recordMatches, stateForHash, stoneLineKey, type JewelRecord } from './record.ts';

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

test('a record of an unknown version, or naming a bad photo, is not a record', () => {
  const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
  assert.equal(parseRecord(enc({ ...ring, version: 3 })), null);
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

const v2: JewelRecord = {
  ...ring, version: 2, inscription: 'JOA0710001',
  origin: { workshop: 'Jewel Of Africa', stoneSource: '', labCertificate: 'GIA ' + '1'.repeat(150) },
  description: 'Twelve significant emeralds totaling 32 carats.\n18kt Yellow Gold.',
  productUrl: 'https://www.jewelofafrica.com/products/emerald-parrot-pendant-071', sku: 'JOA-071',
  listPrice: { amount: '17678.00', currency: 'USD', at: '2026-11-16', label: 'list price at registration' },
  pieceNotes: 'Clasp re-soldered before sale.',
};

test('a v2 record survives encoding, in one fixed key order', () => {
  assert.deepEqual(parseRecord(encodeRecord(v2)), v2);
  const text = new TextDecoder().decode(encodeRecord(v2));
  assert.ok(text.indexOf('"createdAt"') < text.indexOf('"description"'));
  assert.ok(text.endsWith('"pieceNotes":"Clasp re-soldered before sale."}'));
});

test('v1 encodes exactly as before', () => {
  assert.equal(new TextDecoder().decode(encodeRecord(ring)),
    '{"version":1,"jewelerId":"1","inscription":"MD20260001","title":"Solitaire ring","metal":"18k yellow gold","weightGrams":"4.2","stones":[{"kind":"diamond","carat":"0.50","count":1}],"origin":{"workshop":"Dakar, Médina","stoneSource":"Botswana","labCertificate":"GIA 2141438167"},"photos":["' + HASH + '"],"createdAt":"2026-11-13T10:00:00.000Z"}');
});

test('a v2 record is checked as strictly as v1', () => {
  const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
  assert.equal(parseRecord(enc({ ...v2, productUrl: 'http://shop.example/x' })), null);
  assert.equal(parseRecord(enc({ ...v2, productUrl: 'javascript:alert(1)' })), null);
  assert.equal(parseRecord(enc({ ...v2, description: 'x'.repeat(5001) })), null);
  assert.equal(parseRecord(enc({ ...v2, listPrice: { ...v2.listPrice, label: 'our price' } })), null);
  assert.equal(parseRecord(enc({ ...v2, listPrice: { ...v2.listPrice, currency: 'usd' } })), null);
  assert.equal(parseRecord(enc({ ...v2, pieceNotes: 'x'.repeat(501) })), null);
  assert.equal(parseRecord(enc({ ...v2, origin: { ...v2.origin, labCertificate: 'x'.repeat(201) } })), null);
  assert.deepEqual(parseRecord(enc({ ...v2, listPrice: null, productUrl: '' })), { ...v2, listPrice: null, productUrl: '' });
  assert.equal(parseRecord(enc({ ...ring, origin: { ...ring.origin, labCertificate: 'x'.repeat(81) } })), null, 'v1 keeps 80');
});

test('a list price dated on a day that does not exist is not a record', () => {
  const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
  for (const at of ['2026-13-45', '2026-02-30', '2026-00-10']) {
    assert.equal(parseRecord(enc({ ...v2, listPrice: { ...v2.listPrice, at } })), null, at);
  }
});

test('a catalogue (v2) record may leave a stone\'s count and carats not stated; a hand-registered (v1) one may not', () => {
  const unstated = { ...v2, stones: [{ kind: 'Diamond', carat: '', count: 0 }, { kind: 'Emerald', carat: '5.2', count: 0 }, { kind: 'Ruby', carat: '', count: 3 }] } as JewelRecord;
  assert.deepEqual(parseRecord(encodeRecord(unstated)), unstated);
  const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
  assert.equal(parseRecord(enc({ ...ring, stones: [{ kind: 'Diamond', carat: '1', count: 0 }] })), null);
  assert.equal(parseRecord(enc({ ...v2, stones: [{ kind: 'Diamond', carat: '', count: -1 }] })), null);
  assert.equal(parseRecord(enc({ ...v2, stones: [{ kind: 'Diamond', carat: '', count: 1.5 }] })), null);
});

test('each stone line says what is not stated, and only that', () => {
  assert.equal(stoneLineKey({ kind: 'Emerald', carat: '32', count: 12 }), 'traceability.stoneLine');
  assert.equal(stoneLineKey({ kind: 'Diamond', carat: '', count: 422 }), 'traceability.stoneLineNoCarat');
  assert.equal(stoneLineKey({ kind: 'Emerald', carat: '5.2', count: 0 }), 'traceability.stoneLineNoCount');
  assert.equal(stoneLineKey({ kind: 'Tsavorite', carat: '', count: 0 }), 'traceability.stoneLineUnstated');
});
