// Tests for the judgement this page makes about what it read.
//
// The bulk of what follows is about ONE property, because it is the property
// the whole design rests on: there is no input to this module that produces a
// number the chain did not state. A timeout, a 401, a 503, a malformed body, a
// thrown TypeError from somewhere unrelated — every one of them has to come out
// the other side as a sentence, and none of them may come out as 0, '', '—' or
// 'null'. If somebody later "simplifies" describeFailure by adding a fallback
// that returns a dash, these are the tests that fail.
//
// The second cluster is about the numbers this page states out loud to a
// central bank: 6667 basis points, 720 blocks, 12,500,000 base units. Each of
// those has a way of being rendered wrong that flatters the system, and each is
// pinned here.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { NotFound, Unreachable } from './chain.js';
import {
  amount, blocksAbout, bps, count, describeFailure, duration, elide, isProven,
  proven, secondsPerBlock, unread, when, whenUnix,
} from './format.js';

/* ========================================================================= */
/*  No failure may become a number                                           */
/* ========================================================================= */

const FAILURES = [
  new Unreachable('x', 'unreachable', 'socket hang up'),
  new Unreachable('x', 401, 'the gateway denies this path'),
  new Unreachable('x', 403, ''),
  new Unreachable('x', 503, ''),
  new Unreachable('x', 502, ''),
  new Unreachable('x', 500, ''),
  new Unreachable('x', 'malformed', 'not JSON'),
  new NotFound('x'),
  new TypeError('fetch failed'),
  new Error(''),
];

test('every failure produces an unread proof, never a proven one', () => {
  for (const error of FAILURES) {
    const proof = describeFailure(error);
    assert.equal(proof.state, 'unread', `${error} produced ${proof.state}`);
    assert.equal(isProven(proof), false);
    // The shape a proven proof has must be absent, so no template can walk it.
    assert.equal(proof.rows, undefined);
    assert.equal(proof.height, undefined);
  }
});

test('every failure produces a sentence, and never a numeral or a dash', () => {
  for (const error of FAILURES) {
    const { detail } = describeFailure(error);
    assert.ok(detail.length > 12, `too short to be a sentence: "${detail}"`);
    assert.match(detail, /[.]$/, `not a sentence: "${detail}"`);
    assert.doesNotMatch(detail, /^\s*(0|—|-|n\/a|null|undefined)\s*$/i);
  }
});

test('a 401 is reported as the gateway denying a path, not as a login', () => {
  const proof = describeFailure(new Unreachable('land', 401, ''));
  assert.equal(proof.reason, 'denied');
  // The specific mistake this guards: a reader who sees "unauthorised" concludes
  // the record is secret. It is not — the gateway simply does not publish it.
  assert.match(proof.detail, /exists on the chain/);
  assert.doesNotMatch(proof.detail, /password/i);
});

test('a chain that does not answer is reported as unreachable, not as empty', () => {
  for (const status of ['unreachable', 502, 503]) {
    const proof = describeFailure(new Unreachable('x', status, ''));
    assert.equal(proof.reason, 'unreachable');
  }
  assert.match(describeFailure(new Unreachable('x', 'unreachable', '')).detail, /Cannot reach the chain/);
});

test('a record the chain says does not exist is distinguished from a chain that did not answer', () => {
  const absent = describeFailure(new NotFound('parcel 9'));
  const silent = describeFailure(new Unreachable('parcel 9', 'unreachable', ''));
  assert.equal(absent.reason, 'absent');
  assert.equal(silent.reason, 'unreachable');
  assert.notEqual(absent.detail, silent.detail);
  // "The chain answered and holds nothing" is a fact about the chain.
  assert.match(absent.detail, /answered/);
});

test('a proven proof is the only shape that carries rows', () => {
  const p = proven(119000, [{ label: 'a', value: 'b' }], 'note');
  assert.equal(isProven(p), true);
  assert.equal(p.height, 119000);
  assert.equal(p.rows.length, 1);
  assert.equal(isProven(unread('unreachable', 'x.')), false);
});

/* ========================================================================= */
/*  Basis points                                                             */
/* ========================================================================= */

test('basis points render without inventing or losing precision', () => {
  // 6667 is not 66.67% by rounding accident. It is the smallest integer
  // strictly above two thirds, and it is the seizure threshold, so the two
  // decimals have to survive.
  assert.equal(bps(6667), '66.67%');
  assert.equal(bps(8000), '80%');       // not 80.00%
  assert.equal(bps(10000), '100%');
  assert.equal(bps(30), '0.3%');
  assert.equal(bps(6000), '60%');
  assert.equal(bps(500), '5%');
  assert.equal(bps(0), '0%');
});

test('basis points refuse a value that is not a number', () => {
  assert.equal(bps(undefined), null);
  assert.equal(bps('nonsense'), null);
});

/* ========================================================================= */
/*  Counts and plurals                                                       */
/* ========================================================================= */

test('a count of nought is a number, not a blank', () => {
  assert.equal(count(0, 'institution'), '0 institutions');
  assert.equal(count(1, 'institution'), '1 institution');
  assert.equal(count(2, 'institution'), '2 institutions');
});

test('irregular plurals are given rather than guessed', () => {
  assert.equal(count(2, 'treasury', 'treasuries'), '2 treasuries');
  assert.equal(count(42, 'currency', 'currencies'), '42 currencies');
  assert.equal(count(1, 'currency', 'currencies'), '1 currency');
});

test('large counts carry thousands separators', () => {
  assert.equal(count(120960, 'block'), '120,960 blocks');
});

/* ========================================================================= */
/*  Blocks as durations                                                      */
/* ========================================================================= */

test('a block count is stated as blocks first and an approximation second', () => {
  const text = blocksAbout(720, 5.38);
  assert.match(text, /^720 blocks/);
  assert.match(text, /about/);
});

test('with no measured block time, the duration is omitted rather than guessed', () => {
  // The specific failure: hard-coding seven seconds on a chain running at five
  // overstates a freeze's life by a third, on the one number a supervisor
  // checks. Better to say nothing than to say a wrong duration confidently.
  assert.equal(blocksAbout(720, null), '720 blocks');
  assert.equal(blocksAbout(720, 0), '720 blocks');
  assert.equal(blocksAbout(720, NaN), '720 blocks');
  assert.doesNotMatch(blocksAbout(720, null), /about/);
});

test('the same block count reads differently at different measured block times', () => {
  const fast = blocksAbout(720, 2);
  const slow = blocksAbout(720, 7);
  assert.notEqual(fast, slow);
  assert.match(fast, /24 minutes/);
  assert.match(slow, /84 minutes/);
});

test('zero or negative blocks is not a duration', () => {
  assert.equal(blocksAbout(0, 5), null);
  assert.equal(blocksAbout(-5, 5), null);
});

test('durations step through units rather than reporting 7,000 seconds', () => {
  assert.equal(duration(45), '45 seconds');
  assert.equal(duration(600), '10 minutes');
  assert.equal(duration(7200), '2 hours');
  assert.equal(duration(650688), '8 days');   // 120,960 blocks at 5.38s
  assert.equal(duration(86400 * 90), '3 months');
});

/* ========================================================================= */
/*  Amounts                                                                  */
/* ========================================================================= */

test('micro-denominations are scaled and named', () => {
  assert.equal(amount('12500000', 'uxof'), '12.5 XOF');
  assert.equal(amount('500000000', 'uyml'), '500 YML');
  assert.equal(amount('20000000000', 'uyml'), '20,000 YML');
  assert.equal(amount('1000000', 'uyml'), '1 YML');
});

test('a fraction is truncated, never rounded up', () => {
  // A shown balance larger than the real one is the one direction that must
  // not happen: it is the number somebody spends against.
  assert.equal(amount('1999999', 'uyml'), '1.999999 YML');
  assert.equal(amount('999999', 'uyml'), '0.999999 YML');
  assert.equal(amount('1', 'uyml'), '0.000001 YML');
});

test('an amount too large for a double survives intact', () => {
  // Token supplies exceed 2^53. They are strings on the wire for that reason
  // and must not be routed through Number on the way to a screen.
  // 2^53 is 9,007,199,254,740,992. This is that many YML expressed in base
  // units, which is four orders of magnitude past where a double stops being
  // exact — routing it through Number would round it and nobody would notice.
  assert.equal(amount('90071992547409910000000', 'uyml'), '90,071,992,547,409,910 YML');
});

test('an unrecognised denomination is not given an invented exponent', () => {
  assert.equal(amount('1000000', 'tok/2/KIN518'), '1,000,000 tok/2/KIN518');
  assert.equal(amount('1000000', 'amm/pool/1'), '1,000,000 amm/pool/1');
});

test('a non-numeric amount is refused rather than rendered as NaN', () => {
  assert.equal(amount('', 'uyml'), null);
  assert.equal(amount(undefined, 'uyml'), null);
  assert.equal(amount('12.5', 'uyml'), null);
});

/* ========================================================================= */
/*  Addresses                                                                */
/* ========================================================================= */

test('an elided address keeps both ends so two can be compared by eye', () => {
  const a = 'yml1ael7jxwlvacc3daawzc2kpd6lst6w8nmml6a97';
  const short = elide(a);
  assert.equal(short, 'yml1ael7jx…t6w8nmml6a97'.replace('t6w8nmml6a97', a.slice(-10)));
  assert.ok(short.startsWith(a.slice(0, 10)), 'the head must survive');
  assert.ok(short.endsWith(a.slice(-10)), 'the tail must survive — one end alone cannot be compared');
  assert.ok(short.includes('…'));
});

test('an address short enough to show whole is not elided', () => {
  assert.equal(elide('yml1abc'), 'yml1abc');
});

/* ========================================================================= */
/*  Times                                                                    */
/* ========================================================================= */

test('an instant is rendered in UTC so two readers in a room agree', () => {
  assert.equal(when('2026-08-27T20:54:40.862924487Z'), '2026-08-27 20:54 UTC');
});

test('an unparseable instant is null rather than "Invalid Date"', () => {
  assert.equal(when('not a date'), null);
  assert.equal(when(''), null);
  assert.equal(whenUnix(0), null);
});

test('the chain stores transfer times in unix seconds', () => {
  assert.equal(whenUnix(1787864080), '2026-08-27 20:54 UTC');
});

/* ========================================================================= */
/*  Measuring the block interval                                             */
/* ========================================================================= */

test('the block interval is measured across a real span', () => {
  const earlier = { height: 117425, at: '2026-08-31T07:33:15Z' };
  const later = { height: 119425, at: '2026-08-31T10:32:35Z' };
  const s = secondsPerBlock(earlier, later);
  assert.ok(s > 5 && s < 6, `expected about 5.4s, got ${s}`);
});

test('two adjacent blocks are refused as a measurement', () => {
  // A "block time" computed from two adjacent blocks is whatever the
  // proposer's clock did, presented to three decimal places.
  assert.equal(secondsPerBlock({ height: 100, at: '2026-01-01T00:00:00Z' },
    { height: 101, at: '2026-01-01T00:00:07Z' }), null);
});

test('a missing or backwards span is refused', () => {
  assert.equal(secondsPerBlock(null, { height: 2, at: '2026-01-01T00:00:00Z' }), null);
  assert.equal(secondsPerBlock({ height: 1, at: '2026-01-01T00:10:00Z' },
    { height: 5000, at: '2026-01-01T00:00:00Z' }), null);
});
