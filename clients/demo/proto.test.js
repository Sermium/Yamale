// Tests for the hand-written protobuf.
//
// Every decoder here is written from a .proto file by hand, which means the
// failure mode is not a crash — it is a parcel that renders with the holder in
// the authority's field, or a transfer that shows one attestor when three
// signed. Both look entirely plausible on screen. So each decoder is
// round-tripped against bytes written from the same field numbers, and the
// repeated fields are given their own tests, because dropping all but the last
// entry of a repeated field is the specific bug that would make this page
// understate exactly the guarantee it is claiming.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  decodeAuthority, decodeLandParams, decodeList, decodeOne, decodeParcel,
  decodeTransfer, fromBase64, list, num, read, statusName, str, toHex, write,
} from './proto.js';

/* ========================================================================= */
/*  The wire format                                                          */
/* ========================================================================= */

test('a varint round-trips across the byte boundaries', () => {
  for (const value of [0, 1, 127, 128, 300, 16383, 16384, 119425, 2 ** 32, 2 ** 40]) {
    const bytes = write((w) => w.num(1, value));
    const back = num(read(bytes), 1);
    assert.equal(back, value, `${value} did not survive`);
  }
});

test('proto3 default values are omitted, and read back as the default', () => {
  // Not a nicety: the keeper writes nothing for a zero, so a decoder that
  // required the field present would report every parcel with vehicle_id 0 as
  // malformed rather than as un-fractionalised.
  assert.equal(write((w) => w.num(1, 0)).length, 0);
  assert.equal(write((w) => w.string(1, '')).length, 0);
  assert.equal(num(read(new Uint8Array()), 1), 0);
  assert.equal(str(read(new Uint8Array()), 1), '');
});

test('strings survive the round trip, including non-ASCII', () => {
  const text = 'Registre foncier de Mbuji-Mayi — CD';
  assert.equal(str(read(write((w) => w.string(3, text))), 3), text);
});

test('a repeated field keeps every entry, not the last one', () => {
  const bytes = write((w) => { w.string(8, 'a'); w.string(8, 'b'); w.string(8, 'c'); });
  assert.equal(list(read(bytes), 8).length, 3);
});

test('unknown fields are ignored rather than refused', () => {
  // A node running newer code emits fields this file has never heard of. The
  // correct behaviour is to read the record around them.
  const bytes = write((w) => { w.num(1, 7); w.string(99, 'from a future build'); w.string(3, 'ref'); });
  const m = read(bytes);
  assert.equal(num(m, 1), 7);
  assert.equal(str(m, 3), 'ref');
});

test('a truncated message is refused rather than half-read', () => {
  const good = write((w) => w.string(1, 'a long enough value to truncate'));
  assert.throws(() => read(good.subarray(0, good.length - 5)), /truncated/);
});

test('hex and base64 round-trip the bytes ABCI actually carries', () => {
  const bytes = Uint8Array.from([0x0a, 0x06, 0x08, 0x03, 0x10, 0x80, 0xea, 0x49]);
  assert.equal(toHex(bytes), '0a0608031080ea49');
  // The value the node returned for x/land Params, recorded in chain.js.
  assert.deepEqual(Array.from(fromBase64('CgYIAxCA6kk=')), Array.from(bytes));
  assert.equal(fromBase64('').length, 0);
});

/* ========================================================================= */
/*  parcel.proto                                                             */
/* ========================================================================= */

test('a parcel decodes every field into the field it belongs in', () => {
  const bytes = write((w) => {
    w.num(1, 2);
    w.string(2, 'a41d9c7e5b2f08c1');
    w.string(3, 'CD-KIN-2026-00413');
    w.string(4, 'yml1holder');
    w.string(5, 'yml1office');
    w.num(6, 3);                 // STATUS_DISPUTED
    w.num(8, 100202);
    w.num(11, 0);
  });
  const p = decodeParcel(bytes);
  assert.equal(p.id, 2);
  assert.equal(p.geometryHash, 'a41d9c7e5b2f08c1');
  assert.equal(p.cadastralRef, 'CD-KIN-2026-00413');
  assert.equal(p.holder, 'yml1holder');
  assert.equal(p.authority, 'yml1office');
  assert.equal(p.status, 3);
  assert.equal(p.registeredAt, 100202);
  assert.equal(p.vehicleId, 0);
  // The two address fields are adjacent and easy to transpose. A parcel showing
  // the registry office as its owner is the worst render available here.
  assert.notEqual(p.holder, p.authority);
});

test('a parcel counts all of its encumbrances, deeds and freezes', () => {
  const bytes = write((w) => {
    w.num(1, 1);
    for (let i = 0; i < 12; i += 1) w.msg(7, (e) => e.string(1, 'mortgage'));
    for (let i = 0; i < 3; i += 1) w.msg(9, (d) => d.string(1, 'grant'));
    for (let i = 0; i < 2; i += 1) w.msg(12, (f) => f.string(1, 'court order'));
  });
  const p = decodeParcel(bytes);
  // A title shown with one mortgage when it carries twelve is a lie that gets
  // somebody's house taken.
  assert.equal(p.encumbrances, 12);
  assert.equal(p.deeds, 3);
  assert.equal(p.freezes, 2);
});

test('a parcel reads its restrictions rather than counting them', () => {
  const bytes = write((w) => {
    w.num(1, 1);
    w.msg(10, (r) => { r.string(1, 'foreign_ownership_capped'); r.string(2, '4000'); });
    w.msg(10, (r) => { r.string(1, 'no_fractionalisation'); r.num(6, 1); });
  });
  const p = decodeParcel(bytes);
  assert.deepEqual(p.restrictions.map((r) => r.kind),
    ['foreign_ownership_capped', 'no_fractionalisation']);
  assert.equal(p.restrictions[0].value, '4000');
  assert.equal(p.restrictions[1].lifted, true);
});

test('a status this build has never seen renders as its number, not as registered', () => {
  assert.equal(statusName(1), 'STATUS_REGISTERED');
  assert.equal(statusName(3), 'STATUS_DISPUTED');
  assert.equal(statusName(4), 'STATUS_FROZEN');
  // A parcel wrongly shown as registered is a parcel somebody buys.
  assert.equal(statusName(9), 'STATUS_9');
});

test('a transfer keeps all three attestors', () => {
  const bytes = write((w) => {
    w.num(1, 0);
    w.num(2, 1);
    w.string(3, 'yml1seller');
    w.string(4, 'yml1buyer');
    w.num(6, 1);
    w.string(7, 'yml1validator');
    w.string(8, 'yml1office_a');
    w.string(8, 'yml1office_b');
    w.string(8, 'yml1office_c');
    w.num(9, 1787863383);
    w.num(13, 1787864080);
  });
  const t = decodeTransfer(bytes);
  assert.equal(t.validated, true);
  assert.equal(t.attestors.length, 3);
  assert.deepEqual(t.attestors, ['yml1office_a', 'yml1office_b', 'yml1office_c']);
  assert.equal(t.completedAt, 1787864080);
  assert.equal(t.objectedBy, '');
});

test('an objection is read with its reason, and stops completion', () => {
  const reason = 'the seller died in 2019 and the succession is before the tribunal de grande instance';
  const bytes = write((w) => {
    w.num(1, 1);
    w.num(2, 2);
    w.string(3, 'yml1seller');
    w.string(4, 'yml1buyer');
    w.string(10, 'yml1stranger');
    w.string(11, reason);
  });
  const t = decodeTransfer(bytes);
  assert.equal(t.objectedBy, 'yml1stranger');
  assert.equal(t.objectionReason, reason);
  assert.equal(t.completedAt, 0);
  // Neither buyer nor seller. That claim is made on the page and rests on this.
  assert.notEqual(t.objectedBy, t.from);
  assert.notEqual(t.objectedBy, t.to);
});

test('an authority decodes with its active flag', () => {
  const bytes = write((w) => {
    w.string(1, 'yml1office');
    w.string(2, 'Registre foncier de Kinshasa');
    w.string(3, 'CD');
    w.num(4, 1);
  });
  const a = decodeAuthority(bytes);
  assert.equal(a.name, 'Registre foncier de Kinshasa');
  assert.equal(a.jurisdiction, 'CD');
  assert.equal(a.active, true);
  // An inactive office writes nothing for field 4, and must not read as active.
  assert.equal(decodeAuthority(write((w) => w.string(1, 'x'))).active, false);
});

test('land params decode the quorum and the challenge window', () => {
  const p = decodeLandParams(write((w) => { w.num(1, 3); w.num(2, 600); }));
  assert.equal(p.attestationQuorum, 3);
  assert.equal(p.challengeWindowSeconds, 600);
});

/* ========================================================================= */
/*  Response wrappers                                                        */
/* ========================================================================= */

test('a list response reports the paginator total, not the page length', () => {
  // A page of 3 out of 4,000 and a corpus of 3 look identical from the array.
  const bytes = write((w) => {
    w.msg(1, (a) => a.string(1, 'one'));
    w.msg(1, (a) => a.string(1, 'two'));
    w.msg(2, (page) => page.num(2, 4000));
  });
  const decoded = decodeList(bytes, (b) => ({ v: str(read(b), 1) }));
  assert.equal(decoded.items.length, 2);
  assert.equal(decoded.total, 4000);
});

test('a list response with no paginator falls back to the page length', () => {
  const bytes = write((w) => { w.msg(1, (a) => a.string(1, 'one')); });
  assert.equal(decodeList(bytes, () => ({})).total, 1);
});

test('an empty list is nought, and says so', () => {
  const decoded = decodeList(new Uint8Array(), () => ({}));
  assert.equal(decoded.items.length, 0);
  assert.equal(decoded.total, 0);
});

test('a single-message response is null when the field is absent', () => {
  assert.equal(decodeOne(new Uint8Array(), decodeParcel), null);
  assert.equal(decodeOne(write((w) => w.msg(1, (p) => p.num(1, 7))), decodeParcel).id, 7);
});

/* ========================================================================= */
/*  Against bytes the live chain actually returned                           */
/* ========================================================================= */

test('the params bytes the node returned decode to the values it reported', () => {
  // Captured on 2026-08-27 from:
  //   /api/rpc/abci_query?path="/blockchain.land.v1.Query/Params"&data=0x
  //   → {"response":{"code":0,"value":"CgYIAxCA6kk=","height":"94464"}}
  //
  // QueryParamsResponse { Params params = 1 }; Params { quorum = 1, window = 2 }
  const params = decodeLandParams(read(fromBase64('CgYIAxCA6kk=')).get(1)[0]);
  assert.equal(params.attestationQuorum, 3);
  assert.equal(params.challengeWindowSeconds, 1209600);   // fourteen days
});

test('the challenge window has since been shortened, which is why it is read and not fixed', () => {
  // The same query on 2026-08-31 answers 600 seconds. A page that had baked the
  // fourteen days above into its copy would today be telling a room that a land
  // sale can be objected to for a fortnight when the window is ten minutes.
  // This test exists to record that the value moves, so nobody hard-codes it.
  const now = decodeLandParams(write((w) => { w.num(1, 3); w.num(2, 600); }));
  assert.equal(now.challengeWindowSeconds, 600);
  assert.notEqual(now.challengeWindowSeconds, 1209600);
});
