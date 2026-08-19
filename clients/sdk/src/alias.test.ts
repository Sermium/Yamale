import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  ALPHABET,
  FOUNDATION_COUNTRY,
  assignedCountry,
  formatUserId,
  normaliseUserId,
  userIdCountry,
  validUserId,
} from './alias.ts';

// These tests exist because this file is the same algorithm as
// x/alias/types/id.go computed a second time. A disagreement between them does
// not show up as a crash: it shows up as a client that rejects an identifier
// the chain issued, or accepts one the chain would never have — and the person
// holding it is told their account does not exist.
//
// The fixtures below are identifiers the Go implementation produces, so a
// change to either side that breaks the agreement fails here.

/** Derived by the chain for a Nigerian account. */
const NG_ID = 'NGK3M97QRT5';
/** The same payload under a Ghanaian prefix, with the check character the
 *  chain would compute for it. */
const GH_ID = 'GHK3M97QRTA';

test('the alphabet is Crockford', () => {
  assert.equal(ALPHABET.length, 32);
  for (const banned of ['I', 'L', 'O', 'U']) {
    assert.ok(!ALPHABET.includes(banned), `alphabet contains ${banned}`);
  }
});

test('a well-formed identifier passes and its country reads back', () => {
  assert.ok(validUserId(NG_ID), `${NG_ID} was rejected`);
  assert.equal(userIdCountry(NG_ID), 'NG');
  assert.ok(validUserId(GH_ID), `${GH_ID} was rejected`);
  assert.equal(userIdCountry(GH_ID), 'GH');
});

test('it resolves however it was typed', () => {
  for (const form of [NG_ID, formatUserId(NG_ID), formatUserId(NG_ID).toLowerCase(), ` ${NG_ID} `]) {
    assert.ok(validUserId(form), `rejected in the form ${form}`);
    assert.equal(normaliseUserId(form), NG_ID);
  }
});

// The country typo the check character exists to catch. NG and NE are
// neighbours in the list and on the map, and without the prefix in the sum both
// would validate — the wrong one reaching the node and coming back "not found".
test('a wrong country fails the check character', () => {
  for (const wrong of ['NE', 'NA', 'NI', 'MG', 'GH']) {
    assert.ok(!validUserId(wrong + NG_ID.slice(2)), `NG misread as ${wrong} was accepted`);
  }
});

// Crockford folds I and L onto 1 and O onto 0. Applied to the prefix that puts
// CI and CL onto the same country, and SI and SL onto another — every one of
// them a real country. The prefix is left alone, and this is the check.
test('the country prefix is never folded', () => {
  const prefixes = new Set<string>();
  for (const cc of ['CI', 'CL', 'SI', 'SL', 'ML', 'MO']) {
    const prefix = normaliseUserId(`${cc}K3M97QRT5`).slice(0, 2);
    assert.ok(!prefixes.has(prefix), `${cc} collapsed onto ${prefix}`);
    prefixes.add(prefix);
  }
});

test('a prefix nobody assigned is refused', () => {
  for (const nonsense of ['NX', 'QK', 'XA', '00', 'N1']) {
    assert.ok(!assignedCountry(nonsense), `${nonsense} was accepted as a country`);
  }
  // The foundation's code is issuable but is not a country, so it can never be
  // recorded as an account's jurisdiction.
  assert.ok(!assignedCountry(FOUNDATION_COUNTRY));
  assert.ok(assignedCountry('NG') && assignedCountry('gh'));
});

test('the pre-jurisdiction form no longer validates', () => {
  // Everything issued before the country prefix was tombstoned by the chain's
  // v1-to-v2 migration; a client that still accepted the shape would send a
  // dead handle to the node and report a missing account.
  assert.ok(!validUserId('K3M97QRTY'));
});

test('formatting puts the country in its own group', () => {
  assert.equal(formatUserId('NGK3M97QRTB'), 'NG-K3M9-7QRT-B');
  assert.equal(formatUserId('ng-k3m9-7qrt-b'), 'NG-K3M9-7QRT-B');
});
