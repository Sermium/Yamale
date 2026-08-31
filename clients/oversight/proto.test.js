/**
 * Tests for the hand-written codec.
 *
 * The important case in here is `decodes the live chain's enforcement
 * parameters`. Every other test proves the decoder is self-consistent, which
 * is worth something but proves nothing about whether the FIELD NUMBERS are
 * right — and a wrong field number is the failure mode this codec actually
 * has. It does not throw. It renders a real value under the wrong label.
 *
 * So one test is pinned to bytes captured from yamale-devnet-2 at height
 * 119392 and asserts the decoded values against what the proto comments say
 * those parameters mean: threshold_bps must come out as 6667, because 6667 is
 * two thirds and the whole console is a claim about that number.
 *
 * Capture these vectors mechanically, never by hand. The first draft of this
 * file carried a delay tier transcribed by eye as 2624 blocks; the decoder
 * read 2880 and was right. The literal was fine and the human arithmetic was
 * not, which is the entire argument for pinning bytes rather than beliefs.
 *
 * Run: node --test clients/oversight/
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as P from './proto.js';

const bytes = (...b) => new Uint8Array(b);

// ---------------------------------------------------------------------------
// Varints.
// ---------------------------------------------------------------------------

test('reads a single-byte varint', () => {
  assert.deepEqual(P.readVarint(bytes(0x05), 0), [5n, 1]);
});

test('reads a multi-byte varint', () => {
  // 300 = 0b100101100 → 0xac 0x02
  assert.deepEqual(P.readVarint(bytes(0xac, 0x02), 0), [300n, 2]);
});

test('reads a varint at the top of the 64-bit range', () => {
  const max = bytes(0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01);
  assert.equal(P.readVarint(max, 0)[0], 18446744073709551615n);
});

test('refuses a varint that runs off the end rather than returning a short value', () => {
  assert.throws(() => P.readVarint(bytes(0x80, 0x80), 0), /past the end/);
});

test('refuses a varint wider than 64 bits instead of looping', () => {
  const runaway = new Uint8Array(20).fill(0x80);
  assert.throws(() => P.readVarint(runaway, 0), /wider than 64 bits/);
});

// ---------------------------------------------------------------------------
// Field splitting.
// ---------------------------------------------------------------------------

test('splits varint and length-delimited fields', () => {
  // field 1 varint 7; field 2 string "hi"
  const f = P.splitFields(bytes(0x08, 0x07, 0x12, 0x02, 0x68, 0x69));
  assert.equal(f.get(1)[0].varint, 7n);
  assert.deepEqual(Array.from(f.get(2)[0].bytes), [0x68, 0x69]);
});

test('keeps every occurrence of a repeated field', () => {
  const f = P.splitFields(bytes(0x08, 0x01, 0x08, 0x02, 0x08, 0x03));
  assert.equal(f.get(1).length, 3);
});

test('refuses field number zero', () => {
  assert.throws(() => P.splitFields(bytes(0x00, 0x01)), /field number 0/);
});

test('refuses a removed proto2 group rather than guessing past it', () => {
  assert.throws(() => P.splitFields(bytes(0x0b)), /unsupported wire type 3/);
});

test('refuses a length that runs past the buffer', () => {
  assert.throws(() => P.splitFields(bytes(0x12, 0x40, 0x61)), /past the end/);
});

// ---------------------------------------------------------------------------
// Schema decoding.
// ---------------------------------------------------------------------------

test('fills in proto3 defaults for every field the schema names', () => {
  const out = P.decode(new Uint8Array(0), {
    1: ['n', 'uint64'], 2: ['s', 'string'], 3: ['b', 'bool'],
    4: ['sub', { 1: ['x', 'uint64'] }], 5: ['list', 'string', { repeated: true }],
  });
  assert.deepEqual(out, { n: 0, s: '', b: false, sub: null, list: [] });
});

test('an absent submessage is null, not an object of defaults', () => {
  // The distinction matters for QueryFreezeStatusResponse, where the freeze
  // and case submessages are ALWAYS emitted — so null here means the field was
  // genuinely absent, which is a different fact from "emitted, all zero".
  const out = P.decode(new Uint8Array(0), { 2: ['freeze', P.Freeze] });
  assert.equal(out.freeze, null);
});

test('a zero-length submessage decodes to defaults, not to null', () => {
  const out = P.decode(bytes(0x12, 0x00), { 2: ['freeze', P.Freeze] });
  assert.deepEqual(out.freeze, {
    address: '', case_id: 0, expires_at_height: 0, frozen_at_height: 0,
  });
});

test('last occurrence wins for a singular field, as proto3 requires', () => {
  const out = P.decode(bytes(0x08, 0x01, 0x08, 0x09), { 1: ['n', 'uint64'] });
  assert.equal(out.n, 9);
});

test('skips fields the schema does not name', () => {
  // Forward compatibility: a field added to the chain after this console was
  // written must not break the fields it does read.
  const out = P.decode(bytes(0x08, 0x07, 0x50, 0x63), { 1: ['n', 'uint64'] });
  assert.equal(out.n, 7);
});

test('decodes a negative int64 as a negative number, not as 2^64', () => {
  // -1 as a ten-byte two's-complement varint.
  const buf = bytes(0x08, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01);
  assert.equal(P.decode(buf, { 1: ['h', 'int64'] }).h, -1);
});

test('a uint64 beyond 2^53 comes back as a string rather than a rounded number', () => {
  // 2^60. A silently rounded amount on an enforcement page is a rounded amount
  // in somebody's seizure notice.
  const big = 1152921504606846976n;
  const enc = [];
  let n = big;
  enc.push(0x08);
  for (;;) { const b = Number(n & 0x7fn); n >>= 7n; if (n === 0n) { enc.push(b); break; } enc.push(b | 0x80); }
  const out = P.decode(new Uint8Array(enc), { 1: ['n', 'uint64'] });
  assert.equal(out.n, '1152921504606846976');
});

test('decodes nested repeated messages', () => {
  // Coin{denom:"uyml", amount:"7"} twice under field 3.
  const coin = [0x0a, 0x04, 0x75, 0x79, 0x6d, 0x6c, 0x12, 0x01, 0x37];
  const buf = new Uint8Array([0x1a, coin.length, ...coin, 0x1a, coin.length, ...coin]);
  const out = P.decode(buf, { 3: ['seized', P.Coin, { repeated: true }] });
  assert.deepEqual(out.seized, [
    { denom: 'uyml', amount: '7' }, { denom: 'uyml', amount: '7' },
  ]);
});

test('refuses a field whose wire type contradicts the schema', () => {
  // A string field arriving as a varint means the schema and the chain
  // disagree about what field 2 is. Guessing would print the wrong thing.
  assert.throws(() => P.decode(bytes(0x10, 0x01), { 2: ['s', 'string'] }), /expected a length-delimited string/);
});

// ---------------------------------------------------------------------------
// Encoding requests.
// ---------------------------------------------------------------------------

test('encodes a uint64 request field', () => {
  assert.deepEqual(Array.from(P.encode({ 1: 300 })), [0x08, 0xac, 0x02]);
});

test('omits a zero, as proto3 requires', () => {
  // The server decodes the absent field back to 0, so a request for id 0 and a
  // request that leaves id out are the same bytes and the same query.
  assert.equal(P.encode({ 1: 0 }).length, 0);
});

test('encodes a string request field', () => {
  assert.deepEqual(Array.from(P.encode({ 1: 'ab' })), [0x0a, 0x02, 0x61, 0x62]);
});

test('encodes fields in ascending field-number order', () => {
  const out = Array.from(P.encode({ 2: 5, 1: 'a' }));
  assert.deepEqual(out, [0x0a, 0x01, 0x61, 0x10, 0x05]);
});

test('a pagination submessage round-trips through encodeSub', () => {
  // PageRequest{limit: 50, reverse: true} at field 1 of QueryListCaseRequest.
  const inner = P.encode({ 3: 50, 5: true });
  const outer = P.encodeSub(1, inner);
  const f = P.splitFields(outer);
  const page = P.decode(f.get(1)[0].bytes, { 3: ['limit', 'uint64'], 5: ['reverse', 'bool'] });
  assert.deepEqual(page, { limit: 50, reverse: true });
});

test('an empty submessage is omitted entirely rather than sent as a zero-length field', () => {
  assert.equal(P.encodeSub(1, new Uint8Array(0)).length, 0);
});

test('hex encoding pads single-digit bytes', () => {
  assert.equal(P.toHex(bytes(0x00, 0x0f, 0xff)), '000fff');
});

test('base64 decoding matches the node\'s value encoding', () => {
  assert.deepEqual(Array.from(P.fromBase64('CgA=')), [0x0a, 0x00]);
  assert.equal(P.fromBase64('').length, 0);
});

// ---------------------------------------------------------------------------
// The pinned live-chain vectors.
//
// Captured from https://pay.yamalelegal.com/api/rpc/abci_query at height
// 119392 on 2026-08-31, chain yamale-devnet-2. These are what the field
// numbers are actually checked against — everything above proves the decoder
// is consistent with itself.
// ---------------------------------------------------------------------------

const LIVE_ENFORCEMENT_PARAMS =
  'CpsBCOgCENAFGIs0Ij55bWwxYWZrOXpyMmhuMmpzYWM2M2g0aG02MHZsOXozZTV1NjlnbmR6ZjdjOTljc'
  + 'WdlM3Z6d2p6czN4bTh1aiiABDCAAjgBSPABUhQKDwoEdXltbBIHMTAwMDAwMBDQBVIWChEKBHV5bWwSC'
  + 'TEwMDAwMDAwMBDAFliAhwFiEQoEdXltbBIJNTAwMDAwMDAwaAU=';

test('decodes the live chain\'s enforcement parameters', () => {
  const { params } = P.decode(
    P.fromBase64(LIVE_ENFORCEMENT_PARAMS), P.QueryEnforcementParamsResponse,
  );

  // The number the whole console is about. 6667 basis points is two thirds,
  // and the module refuses anything at or below 5000 by any route.
  assert.equal(params.threshold_bps, 6667);

  assert.equal(params.voting_period_blocks, 360);
  // The safety valve on the fastest power in the module, and it must comfortably
  // exceed the voting period or a freeze would lapse mid-vote.
  assert.equal(params.provisional_freeze_blocks, 720);
  assert.ok(params.provisional_freeze_blocks >= params.voting_period_blocks);

  assert.equal(params.recovery_destination,
    'yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj');
  assert.equal(params.max_reason_length, 512);
  assert.equal(params.max_evidence_uri_length, 256);
  assert.equal(params.seize_requires_evidence, true);
  assert.equal(params.seizure_delay_blocks, 240);
  assert.equal(params.seizure_window_blocks, 17280);
  assert.equal(params.max_seizures_per_window, 5);

  // The delay schedule: longest matching tier wins, so the ORDER these arrive
  // in must not matter and the console must not assume it does.
  assert.equal(params.seizure_delay_tiers.length, 2);
  assert.deepEqual(params.seizure_delay_tiers[0],
    { threshold: { denom: 'uyml', amount: '1000000' }, delay_blocks: 720 });
  assert.deepEqual(params.seizure_delay_tiers[1],
    { threshold: { denom: 'uyml', amount: '100000000' }, delay_blocks: 2880 });

  assert.deepEqual(params.seizure_window_cap, [{ denom: 'uyml', amount: '500000000' }]);

  // Every denomination named in a tier must also be capped — the module
  // refuses a genesis where it is not, and this asserts the live chain holds
  // to that rather than trusting the validator.
  for (const tier of params.seizure_delay_tiers) {
    assert.ok(params.seizure_window_cap.some((c) => c.denom === tier.threshold.denom),
      `tier denom ${tier.threshold.denom} is not covered by the window cap`);
  }

  // No ombudsman is appointed. Field 14 is absent, which decodes to the empty
  // string — and the module reads empty as "nobody", never as "anybody". This
  // is a real fact about this chain and the console has to say it out loud
  // rather than draw an empty panel.
  assert.equal(params.ombudsman, '');
});

const LIVE_NETTING_PARAMS = 'CgA=';

test('decodes the live chain\'s netting parameters as entirely default', () => {
  const { params } = P.decode(P.fromBase64(LIVE_NETTING_PARAMS), P.QueryNettingParamsResponse);

  // A zero-length Params submessage. Every field is at its proto3 default,
  // which for this module means netting is switched off: cycle_blocks is the
  // divisor the end blocker guards against, and zero makes it return before it
  // closes anything.
  assert.equal(params.cycle_blocks, 0);
  assert.deepEqual(params.denom_policies, []);
});
