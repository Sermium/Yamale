import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { groupDigits, toBaseUnits, toBaseUnitsOf, toDisplayAmount } from './denom.ts';
import { formatMoney, setLocale } from './i18n.ts';

/**
 * The conversion between what a person types and what the chain moves.
 *
 * Asserted against figures worked out by hand rather than by re-running the
 * implementation's own arithmetic in the test: the bug this replaces was that
 * `Math.round(Number(x) * 10 ** 6)` agrees with itself perfectly and disagrees
 * with the money.
 */

test('a typed amount becomes base units without passing through a float', () => {
  assert.deepEqual(toBaseUnits('1250.50', 6), { base: '1250500000', truncated: false });
  assert.deepEqual(toBaseUnits('0.07', 6), { base: '70000', truncated: false });
  assert.deepEqual(toBaseUnits('1', 6), { base: '1000000', truncated: false });
  assert.deepEqual(toBaseUnits('0', 6), { base: '0', truncated: false });
  assert.deepEqual(toBaseUnits('.5', 6), { base: '500000', truncated: false });
  assert.deepEqual(toBaseUnits('7.', 6), { base: '7000000', truncated: false });
  assert.deepEqual(toBaseUnits('000012.5', 6), { base: '12500000', truncated: false });
});

test('base units above what a double can hold are exact', () => {
  // 2^53 is 9007199254740992. Ten million YML at six decimals is an order of
  // magnitude past it, and this chain's supply figures live up there.
  assert.deepEqual(toBaseUnits('90071992547.409929', 6), {
    base: '90071992547409929',
    truncated: false,
  });
  // The old float form produced ...992 for this, losing the final digit.
  assert.deepEqual(toBaseUnits('9007199254.740993', 6), {
    base: '9007199254740993',
    truncated: false,
  });
});

test('a decimal comma is a decimal separator, and both together are refused', () => {
  assert.deepEqual(toBaseUnits('1250,50', 6), { base: '1250500000', truncated: false });
  // Ambiguous: 1,250.50 and 1.250,50 are the same characters in two locales.
  assert.equal(toBaseUnits('1,250.50', 6), null);
  assert.equal(toBaseUnits('1.250,50', 6), null);
});

test('group separators a locale inserts are stripped, not parsed as digits', () => {
  // French renders 1 250,50 with a narrow no-break space; pasting it back into
  // the field has to mean 1250.50, not 125050.
  assert.deepEqual(toBaseUnits('1 250,50', 6), { base: '1250500000', truncated: false });
  assert.deepEqual(toBaseUnits('1 250,50', 6), { base: '1250500000', truncated: false });
  assert.deepEqual(toBaseUnits("1'250.50", 6), { base: '1250500000', truncated: false });
});

test('precision the denom cannot hold is reported, not silently dropped', () => {
  assert.deepEqual(toBaseUnits('1.1234567', 6), { base: '1123456', truncated: true });
  // Trailing zeros past the exponent lose nothing, so they are not a truncation.
  assert.deepEqual(toBaseUnits('1.1234560000', 6), { base: '1123456', truncated: false });
});

test('anything that is not a non-negative decimal is refused rather than guessed', () => {
  for (const bad of ['', '  ', '-1', 'abc', '1e6', '1.2.3', '.', '1-2', '1 2 3.4.5']) {
    assert.equal(toBaseUnits(bad, 6), null, `${JSON.stringify(bad)} should be refused`);
  }
});

test('the exponent comes from the denom, never from a hardcoded million', () => {
  // uyml is six decimals; a pool share denom is zero, and treating one as the
  // other is a factor-of-a-million error in whichever direction it happens.
  assert.deepEqual(toBaseUnitsOf('1.5', 'uyml'), { base: '1500000', truncated: false });
  assert.deepEqual(toBaseUnitsOf('3', 'amm/pool/1'), { base: '3', truncated: false });
  assert.deepEqual(toBaseUnitsOf('3.5', 'amm/pool/1'), { base: '3', truncated: true });
});

test('a round trip through base units and back returns what was typed', () => {
  for (const typed of ['1250.5', '0.000001', '999999999999.999999', '0']) {
    const units = toBaseUnits(typed, 6);
    assert.ok(units, typed);
    assert.equal(toDisplayAmount(units.base, 6), Number(typed) === 0 ? '0' : trimZeros(typed));
  }
});

function trimZeros(value: string): string {
  return value.includes('.') ? value.replace(/0+$/, '').replace(/\.$/, '') : value;
}

test('grouping an integer part larger than a double stays exact', () => {
  // The failing case for the old Number() form, which rendered ...993 as ...992.
  assert.equal(groupDigits('9007199254740993'), '9,007,199,254,740,993');
  assert.equal(groupDigits('9007199254740993.5'), '9,007,199,254,740,993.5');
  assert.equal(groupDigits('-1234567.89'), '-1,234,567.89');
  assert.equal(groupDigits('0'), '0');
});

test('formatted money keeps every digit the chain holds', () => {
  setLocale('en');
  assert.equal(formatMoney('9007199254740993', 6), '9,007,199,254.740993');
  assert.equal(formatMoney('1250500000', 6), '1,250.5');
  assert.equal(formatMoney('0', 6), '0');
  assert.equal(formatMoney(1n, 6), '0.000001');
  setLocale('en');
});

test('money is punctuated by the locale, not by string concatenation', () => {
  setLocale('fr');
  const french = formatMoney('1250500000', 6);
  // A French reader writes a comma for the decimal mark; asserting on the
  // exact separator character would pin the test to one ICU version, so this
  // asserts the property that matters.
  assert.ok(french.includes(','), `expected a decimal comma in ${french}`);
  assert.ok(!french.includes('.'), `expected no decimal point in ${french}`);
  setLocale('en');
});

test('a negative balance keeps its sign', () => {
  setLocale('en');
  assert.equal(formatMoney('-1250500000', 6), '-1,250.5');
  setLocale('en');
});

test('a value that is not a number survives instead of rendering as NaN', () => {
  setLocale('en');
  // Reached when a REST response carries a field the client did not expect.
  assert.equal(formatMoney('not-a-number', 6), 'not-a-number');
  setLocale('en');
});
