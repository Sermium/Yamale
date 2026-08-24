import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  displayAmount,
  formatAmount,
  formatCoinList,
  groupDecimal,
  groupInteger,
  shareOf,
  sumCoins,
  truncateDecimals,
} from './amount.ts';
import type { DenomInfo } from '../../sdk/src/denom.ts';

/**
 * The float test, and the reason this module exists.
 *
 * 9007199254740993 is 2^53 + 1: the first integer a double cannot represent.
 * Anything that routes an amount through Number() returns 9007199254740992
 * here, and the assertion below is written against the digits rather than
 * against a reimplementation of the grouping, so it fails on that substitution
 * and cannot be satisfied by copying the implementation into the test.
 */
test('grouping is exact past the range a double can hold', () => {
  assert.equal(groupInteger('9007199254740993'), '9,007,199,254,740,993');

  // Well past it: 30 digits, which no float gets within a thousand of.
  assert.equal(
    groupInteger('123456789012345678901234567890'),
    '123,456,789,012,345,678,901,234,567,890',
  );
});

test('grouping handles the short cases and the sign', () => {
  assert.equal(groupInteger('0'), '0');
  assert.equal(groupInteger('1'), '1');
  assert.equal(groupInteger('999'), '999');
  assert.equal(groupInteger('1000'), '1,000');
  assert.equal(groupInteger('-1234567'), '-1,234,567');
});

test('grouping leaves the fraction alone', () => {
  // A grouped fraction — 0.123,456 — is a different number in half of Europe.
  assert.equal(groupDecimal('1234567.891011'), '1,234,567.891011');
  assert.equal(groupDecimal('0.5'), '0.5');
});

test('truncation goes down, never up', () => {
  // Rounding up would show more money than exists, and somebody spending the
  // displayed figure gets a rejection with no visible cause.
  assert.equal(truncateDecimals('1359.844414', 2), '1359.84');
  assert.equal(truncateDecimals('1359.999999', 2), '1359.99');
  assert.equal(truncateDecimals('1359.999999', 0), '1359');
  assert.equal(truncateDecimals('1359.100000', 2), '1359.1', 'trailing zeros go');
  assert.equal(truncateDecimals('1359', 2), '1359', 'no fraction, no change');
});

test('a base-unit amount becomes the figure a person reads', () => {
  const a = displayAmount('1250500000', 'uyml');
  assert.equal(a.value, '1,250.5');
  assert.equal(a.symbol, 'YML');
  assert.equal(a.base, '1250500000');
  assert.equal(a.unknownDenom, false);
});

test('a supply larger than a double keeps every digit', () => {
  // 9,007,199,254,740.993 YML — a plausible national supply, and the point at
  // which a float starts inventing the last three digits.
  const a = displayAmount('9007199254740993000', 'uyml');
  assert.equal(a.value, '9,007,199,254,740.993');
  assert.equal(a.exact, '9,007,199,254,740.993');
});

test('a currency is printed in the units it is quoted in', () => {
  // The naira has kobo, so two places. The chain stores six.
  const ngn = displayAmount('1359844414', 'ungn');
  assert.equal(ngn.value, '1,359.84');
  assert.equal(ngn.exact, '1,359.844414', 'the full precision stays reachable');
  assert.equal(ngn.rounded, true);

  // The CFA franc has no subunit anybody uses, so none is printed.
  const xof = displayAmount('1359844414', 'uxof');
  assert.equal(xof.value, '1,359');
  assert.equal(xof.rounded, true);
});

test('an unknown denom is shown raw, never given a guessed exponent', () => {
  // The failure this prevents: assuming six decimals for an unfamiliar unit
  // understates it by a factor of a million, which on a balance page reads as
  // an account having lost everything.
  const a = displayAmount('4200000', 'usomethingnew');
  assert.equal(a.value, '4,200,000');
  assert.equal(a.symbol, 'usomethingnew');
  assert.equal(a.unknownDenom, true);
});

test('runtime metadata overrides the built-in table', () => {
  // x/stablecoin publishes denom metadata when governance approves an issuer,
  // so a currency registered after launch has to become readable without a
  // client release.
  const registry: Record<string, DenomInfo> = {
    umzn: { base: 'umzn', symbol: 'MZN', exponent: 6, minorUnits: 2, name: 'Metical' },
    'factory/xyz': { base: 'factory/xyz', symbol: 'XYZ', exponent: 3, name: 'Later token' },
  };
  assert.equal(displayAmount('5000', 'factory/xyz', registry).value, '5');
  assert.equal(displayAmount('5000', 'factory/xyz', registry).symbol, 'XYZ');
  assert.equal(displayAmount('5000', 'factory/xyz', registry).unknownDenom, false);
});

test('pool shares are named as shares, not as an unknown unit', () => {
  const a = displayAmount('24494897427', 'amm/pool/1');
  assert.equal(a.symbol, 'Pool 1 shares');
  assert.equal(a.unknownDenom, false);
});

test('one line for one coin, a phrase for several, nothing for none', () => {
  assert.equal(formatAmount('1250500000', 'uyml'), '1,250.5 YML');
  assert.equal(
    formatCoinList([
      { denom: 'uyml', amount: '1250500000' },
      { denom: 'uusd', amount: '40000000' },
    ]),
    '1,250.5 YML and 40 USD',
  );
  assert.equal(formatCoinList([]), '', 'an empty bag prints nothing, not the word "nothing"');
  assert.equal(formatCoinList(undefined), '');
});

test('totals add in base units', () => {
  const total = sumCoins([
    { denom: 'uyml', amount: '9007199254740993' },
    { denom: 'uyml', amount: '9007199254740993' },
    { denom: 'uusd', amount: '5' },
  ]);
  assert.deepEqual(total, [
    { denom: 'uyml', amount: '18014398509481986' },
    { denom: 'uusd', amount: '5' },
  ]);
});

test('a small share keeps the digits that distinguish it from zero', () => {
  // Measured on yamale-devnet-2: 174,900 YML bonded of 997,766,566 in issue.
  // The SDK's own bondedRatio quantises to four decimal places and reports
  // 0.0001, which prints as 0.010% — wrong by a factor of nearly two, and wrong
  // in the direction that makes a barely-secured chain look unsecured.
  const share = shareOf('174900000000', '997766566659667');
  assert.ok(share !== null);
  assert.equal((share! * 100).toFixed(4), '0.0175');
});

test('a share is exact for numerators past what a double holds', () => {
  assert.equal(shareOf('9007199254740993000', '18014398509481986000'), 0.5);
});

test('a share of nothing is nothing, not infinity', () => {
  // A chain with no supply recorded yet. NaN or Infinity would render as a
  // percentage on the face of the page.
  assert.equal(shareOf('100', '0'), null);
  assert.equal(shareOf('100', 'not a number'), null);
});

test('a malformed amount is skipped rather than poisoning a total', () => {
  const total = sumCoins([
    { denom: 'uyml', amount: '100' },
    { denom: 'uyml', amount: 'not a number' },
  ]);
  assert.deepEqual(total, [{ denom: 'uyml', amount: '100' }]);
});
