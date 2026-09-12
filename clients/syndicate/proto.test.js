/**
 * The wire format, round-tripped.
 *
 * This file exists so that drift between proto/blockchain/syndicate/v1/*.proto
 * and the codecs in proto.js shows up as a failing test rather than as a book
 * that renders the minimum in the target's place. Each case encodes a message
 * the way the chain would and asserts the decoder recovers it.
 *
 *   node --test proto.test.js
 */

import test from 'node:test';
import assert from 'node:assert/strict';

import {
  QUERY, decodeCommitment, decodeDeal, decodeInvestor, decodeListing,
  decodeOperator, decodeParams, read, str, toHex, u64, write,
} from './proto.js';

/** Encode, then read back through the module's own reader. */
const roundTrip = (fn, decode) => decode(read(write(fn)));

/* ========================================================================== */
/*  Primitives                                                                */
/* ========================================================================== */

test('proto3 omits default values, exactly as gogoproto does', () => {
  // A zero varint and an empty string are not written at all. A decoder that
  // demanded them would fail on every message the chain actually emits.
  assert.equal(write((w) => w.num(1, 0)).length, 0);
  assert.equal(write((w) => w.string(1, '')).length, 0);
  assert.equal(write((w) => w.bool(1, false)).length, 0);
});

test('a repeated field keeps every entry, in wire order', () => {
  // The failure this rules out: a book of forty commitments read as one.
  const bytes = write((w) => { w.string(1, 'a'); w.string(1, 'b'); w.string(1, 'c'); });
  const fields = read(bytes);
  assert.equal(fields.get(1).length, 3);
  assert.equal(new TextDecoder().decode(fields.get(1)[2]), 'c');
});

test('an unknown field is ignored rather than fatal', () => {
  // A node running newer code than this page emits fields it has never heard
  // of. The correct behaviour is to parse the deal around them.
  const bytes = write((w) => { w.string(1, 'known'); w.num(99, 12345); });
  const d = read(bytes);
  assert.equal(str(d, 1), 'known');
  assert.equal(u64(d, 99), '12345');
});

test('a truncated message is refused rather than half-parsed', () => {
  const full = write((w) => w.string(1, 'abcdefghij'));
  assert.throws(() => read(full.subarray(0, full.length - 3)), /truncated/);
});

test('a large uint64 survives as a string', () => {
  // Heights and account numbers are the same shape as the ones that overflow.
  const big = '18446744073709551615';
  const d = read(write((w) => w.num(1, big)));
  assert.equal(u64(d, 1), big);
});

test('hex encoding is what CometBFT takes on the URI', () => {
  assert.equal(toHex(new Uint8Array([0x00, 0x0f, 0xff])), '000fff');
});

/* ========================================================================== */
/*  x/syndicate records                                                       */
/* ========================================================================== */

test('a Deal round-trips every field, with the Ints as strings', () => {
  // The field numbers here are transcribed from syndicate.proto. If this test
  // and that file disagree, this test is the one that is wrong — and it failing
  // is the intended way to find out.
  const d = roundTrip((w) => w
    .num(1, 7)
    .string(2, 'SYN-2026-007')
    .string(3, 'Kaolack Cold Chain')
    .string(4, 'Sahel Ventures')
    .string(5, 'yml1drawdown')
    .string(6, 'yml1operator')
    .string(7, 'uusdc')
    .string(8, '500000000000')
    .string(9, '300000000000')
    .string(10, '5000000000')
    .msg(11, (f) => f.num(1, 150).num(2, 100).num(3, 200))
    .num(12, 1790000000)
    .num(13, 2)                       // DEAL_STATE_CLOSED
    .string(14, '420000000000')
    .string(15, 'syn/7')
    .string(16, '420000000000')
    .string(17, '30000000000000000')
    .string(18, '12600000000')
    .num(19, 1788000000)
    .num(20, 1790100000), decodeDeal);

  assert.equal(d.id, '7');
  assert.equal(d.reference, 'SYN-2026-007');
  assert.equal(d.company, 'Kaolack Cold Chain');
  assert.equal(d.partner, 'Sahel Ventures');
  assert.equal(d.settlement_denom, 'uusdc');
  assert.equal(d.target, '500000000000');
  assert.equal(d.minimum, '300000000000');
  assert.equal(d.minimum_ticket, '5000000000');
  assert.deepEqual(d.fees, { sourcing_bps: 150, syndication_bps: 100, management_bps: 200 });
  assert.equal(d.state, 'DEAL_STATE_CLOSED');
  assert.equal(d.raised, '420000000000');
  assert.equal(d.denom, 'syn/7');
  assert.equal(d.supply, '420000000000');
  assert.equal(d.distributed, '12600000000');
  assert.equal(d.closed_at, '1790100000');

  // The index is a STRING and stays one. As a Number this value is already
  // past double precision, and every entitlement on the page is computed from
  // it.
  assert.equal(typeof d.distribution_index, 'string');
  assert.equal(d.distribution_index, '30000000000000000');
});

test('an open deal carries no token, and the emptiness is preserved', () => {
  // The page uses this emptiness in one place rather than the state enum: a
  // token that does not exist cannot be quoted.
  const d = roundTrip((w) => w.num(1, 3).num(13, 1), decodeDeal);
  assert.equal(d.denom, '');
  assert.equal(d.state, 'DEAL_STATE_OPEN');
  // Absent Ints read as '0', never undefined — every caller does BigInt() on
  // them and BigInt(undefined) throws.
  assert.equal(d.raised, '0');
  assert.equal(d.supply, '0');
  assert.equal(d.distribution_index, '0');
});

test('a deal with no fee schedule still decodes to zeros', () => {
  // gogoproto marks FeeSchedule non-nullable, so a zero schedule is omitted
  // from the wire entirely. A decoder returning null here would crash the fee
  // ladder on exactly the deal that charges nothing.
  const d = roundTrip((w) => w.num(1, 1), decodeDeal);
  assert.deepEqual(d.fees, { sourcing_bps: 0, syndication_bps: 0, management_bps: 0 });
});

test('a Commitment round-trips', () => {
  const c = roundTrip((w) => w
    .num(1, 7).string(2, 'yml1investor').string(3, '25000000000').num(4, 1788500000),
    decodeCommitment);
  assert.deepEqual(c, {
    deal_id: '7', investor: 'yml1investor', amount: '25000000000', committed_at: '1788500000',
  });
});

test('an Investor round-trips, and active is its own field', () => {
  const i = roundTrip((w) => w
    .string(1, 'yml1investor').string(2, 'yml1operator').string(3, 'KYC-2026-0412')
    .string(4, 'SN').bool(5, true).num(6, 1787000000), decodeInvestor);
  assert.equal(i.active, true);
  assert.equal(i.reference, 'KYC-2026-0412');
  assert.equal(i.jurisdiction, 'SN');
});

test('a withdrawn investor still has a record, and it decodes as inactive', () => {
  // The record's presence is NOT the question. Telling a struck-off investor
  // they may still bid is the mistake this distinction exists to prevent.
  const i = roundTrip((w) => w.string(1, 'yml1investor').string(4, 'SN'), decodeInvestor);
  assert.equal(i.active, false);
  assert.equal(i.address, 'yml1investor');
});

test('an Operator round-trips', () => {
  const o = roundTrip((w) => w
    .string(1, 'yml1op').string(2, 'Yamale Syndication').string(3, 'SN')
    .bool(4, true).num(5, 1786000000), decodeOperator);
  assert.equal(o.name, 'Yamale Syndication');
  assert.equal(o.active, true);
});

test('a Listing keeps the named buyer and the filler apart', () => {
  // Two different facts. `buyer` is who a private placement is offered to;
  // `filled_by` is who took it. Collapsing them would hide the case the
  // secondary market exists to make visible.
  const l = roundTrip((w) => w
    .num(1, 4).num(2, 7).string(3, 'yml1seller').string(4, 'yml1named')
    .string(5, '10000000000')
    .msg(6, (c) => c.string(1, 'uusdc').string(2, '11000000000'))
    .num(7, 2).string(8, 'yml1named').num(9, 1791000000).num(10, 1791100000),
    decodeListing);

  assert.equal(l.buyer, 'yml1named');
  assert.equal(l.filled_by, 'yml1named');
  assert.equal(l.state, 'LISTING_STATE_FILLED');
  assert.deepEqual(l.price, { denom: 'uusdc', amount: '11000000000' });
});

test('an open listing has no named buyer, which is not the same as no buyer yet', () => {
  const l = roundTrip((w) => w
    .num(1, 5).num(2, 7).string(3, 'yml1seller').string(5, '100')
    .msg(6, (c) => c.string(1, 'uusdc').string(2, '110')).num(7, 1), decodeListing);
  assert.equal(l.buyer, '', 'open to anybody onboarded');
  assert.equal(l.filled_by, '');
  assert.equal(l.state, 'LISTING_STATE_LIVE');
});

test('Params round-trip, and the periods stay strings', () => {
  const p = roundTrip((w) => w.num(1, 500).num(2, 2000).num(3, 7776000).num(4, 604800),
    decodeParams);
  assert.equal(p.max_entry_fee_bps, 500);
  assert.equal(p.max_management_fee_bps, 2000);
  assert.equal(p.max_open_period, '7776000');
  assert.equal(p.min_open_period, '604800');
});

/* ========================================================================== */
/*  Query envelopes                                                           */
/* ========================================================================== */

test('every query names a path under the proto package, not the Go package', () => {
  // `blockchain.syndicate.v1`, NOT `yamale.blockchain.syndicate.v1`. The Go
  // import path is `yamale/blockchain/x/syndicate/types` and only the proto
  // package appears on the wire; getting this wrong produces "unable to resolve
  // type URL", which reads like a chain fault and is a typo.
  for (const [name, q] of Object.entries(QUERY)) {
    assert.match(q.path, /^\/blockchain\.syndicate\.v1\.Query\/\w+$/, `${name} path`);
  }
});

test('the Entitlement request carries the deal and the holder', () => {
  const bytes = QUERY.Entitlement.request({ dealId: 7, holder: 'yml1holder' });
  const f = read(bytes);
  assert.equal(u64(f, 1), '7');
  assert.equal(str(f, 2), 'yml1holder');
});

test('the Entitlement response reports settled and unsettled together', () => {
  // A response reporting only the settled part reads zero for a holder who has
  // never transferred — most of them, most of the time, and precisely the
  // people who would conclude the platform had lost their money.
  const bytes = write((w) => w
    .string(1, '100000000000')
    .msg(2, (c) => c.string(1, 'uusdc').string(2, '3000000000'))
    .num(3, 2500));
  const r = QUERY.Entitlement.response(read(bytes));
  assert.equal(r.balance, '100000000000');
  assert.deepEqual(r.claimable, { denom: 'uusdc', amount: '3000000000' });
  assert.equal(r.share_bps, 2500);
});

test('an entitlement of nothing decodes to zeros rather than to undefined', () => {
  const r = QUERY.Entitlement.response(read(new Uint8Array()));
  assert.equal(r.balance, '0');
  assert.equal(r.claimable.amount, '0');
  assert.equal(r.share_bps, 0);
});

test('the Investor response separates the flag from the record', () => {
  const bytes = write((w) => w.bool(1, false)
    .msg(2, (i) => i.string(1, 'yml1investor').string(4, 'SN')));
  const r = QUERY.Investor.response(read(bytes));
  assert.equal(r.onboarded, false);
  assert.equal(r.investor.address, 'yml1investor', 'the record is still there');
});

test('the Listings request encodes live_only as a bool at field 2', () => {
  const all = read(QUERY.Listings.request({ dealId: 7 }));
  assert.equal(all.has(2), false, 'false is omitted, as proto3 requires');

  const live = read(QUERY.Listings.request({ dealId: 7, liveOnly: true }));
  assert.equal(Number(live.get(2)[0]), 1);
});

test('an empty Deals response is a list, not a null', () => {
  const r = QUERY.Deals.response(read(new Uint8Array()));
  assert.deepEqual(r.deals, []);
});
