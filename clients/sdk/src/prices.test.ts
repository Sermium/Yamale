import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  DEFAULT_MAX_RATE_AGE,
  describeFreshness,
  freshnessOf,
  toAppraisal,
  toRate,
  comparePoolToRates,
  valueAt,
} from './prices.ts';

const NOW = 1_700_000_000;

function rawRate(overrides: Record<string, unknown> = {}) {
  return {
    denom: 'uusd',
    rate: '1.050000000000000000',
    updated_at: String(NOW),
    updated_height: '48',
    voting_power_bps: '10000',
    ...overrides,
  };
}

// The chain returned exactly this payload on a live devnet. Pinning it is what
// keeps the client's reading of a rate honest against the module's writing of
// one.
test('a rate is read as the chain writes it', () => {
  const rate = toRate(rawRate(), NOW);
  assert.equal(rate.denom, 'uusd');
  assert.equal(rate.value, 1.05);
  assert.equal(rate.updatedHeight, 48);
  assert.equal(rate.votingPower, 1);
  assert.equal(rate.ageSeconds, 0);
  assert.equal(rate.freshness, 'fresh');
  assert.equal(rate.stale, false);
});

// The rule the client applies has to be the rule the chain applies, or a screen
// will show a price the chain has already stopped accepting.
test('staleness turns over at the same age the chain uses', () => {
  const justInside = toRate(rawRate(), NOW + DEFAULT_MAX_RATE_AGE);
  assert.equal(justInside.stale, false);

  const justOutside = toRate(rawRate(), NOW + DEFAULT_MAX_RATE_AGE + 1);
  assert.equal(justOutside.stale, true);
  assert.equal(justOutside.freshness, 'stale');
});

test('a rate warns before it expires', () => {
  const halfway = toRate(rawRate(), NOW + DEFAULT_MAX_RATE_AGE / 2 + 1);
  assert.equal(halfway.freshness, 'ageing');
  assert.equal(halfway.stale, false, 'ageing is a warning, not a refusal');
});

// Clock skew must never make something look more current than it is.
test('a rate from the future is treated as unusable, not as fresh', () => {
  const rate = toRate(rawRate({ updated_at: String(NOW + 600) }), NOW);
  assert.equal(rate.stale, true);
});

test('a rate agreed by a bare quorum is flagged', () => {
  assert.equal(toRate(rawRate({ voting_power_bps: '10000' }), NOW).thinlyAgreed, false);
  assert.equal(toRate(rawRate({ voting_power_bps: '5100' }), NOW).thinlyAgreed, true);
});

test('a governance-changed maximum age is respected', () => {
  const rate = toRate(rawRate(), NOW + 100, 60);
  assert.equal(rate.stale, true, 'the client must follow the chain’s parameter, not a hard-coded one');
});

// ------------------------------------------------------------- valuing

// 4 YML at 0.4213 USD is 1.6852, floored to 1.68. Scaling is the whole point:
// applying the rate to 4000000 base units directly would be off by a million.
test('an amount is scaled by its exponent before the rate is applied', () => {
  const rate = toRate(rawRate({ denom: 'uyml', rate: '0.421300000000000000' }), NOW);
  assert.equal(valueAt('4000000', rate), 1.68);
});

test('valuing rounds down, so nothing is shown as worth more than it is', () => {
  const rate = toRate(rawRate({ denom: 'uyml', rate: '0.333333333333333333' }), NOW);
  assert.equal(valueAt('1000000', rate), 0.33);
});

test('a stale rate produces no figure at all', () => {
  const rate = toRate(rawRate({ denom: 'uyml' }), NOW + DEFAULT_MAX_RATE_AGE + 1);
  assert.equal(valueAt('4000000', rate), null);
});

// ---------------------------------------------------------- valuations

test('a valuation ages from the date it describes, not the date it was filed', () => {
  const ninetyDays = 90 * 86_400;
  const appraisal = toAppraisal(
    {
      class_id: 'realestate',
      nft_id: 'building-1',
      value: '2500000000',
      value_denom: 'uusd',
      appraiser: 'yml1valuer',
      valued_at: String(NOW - ninetyDays),
      submitted_at: String(NOW),
      method: 'RICS Red Book',
    },
    NOW,
  );

  assert.equal(appraisal.ageSeconds, ninetyDays, 'filed today, but ninety days old');
  assert.equal(appraisal.freshness, 'ageing');
  assert.equal(appraisal.stale, false);
});

test('a valuation past its window is refused however recently it arrived', () => {
  const appraisal = toAppraisal(
    { class_id: 'c', nft_id: 'n', value: '1', value_denom: 'uusd', valued_at: String(NOW - 200 * 86_400) },
    NOW,
  );
  assert.equal(appraisal.stale, true);
});

test('a revoked signer is surfaced without discarding the valuation', () => {
  const appraisal = toAppraisal(
    { class_id: 'c', nft_id: 'n', value: '1000', value_denom: 'uusd', valued_at: String(NOW) },
    NOW,
    undefined,
    false,
  );
  assert.equal(appraisal.value, '1000', 'the record survives');
  assert.equal(appraisal.appraiserStillApproved, false, 'but the reader can see the authority is gone');
});

// ------------------------------------------------------------ wording

test('freshness is explained without assuming the reader knows what an oracle is', () => {
  const ago = (s: number) => `${Math.round(s / 60)} minutes ago`;

  assert.match(describeFreshness(toRate(rawRate(), NOW + 180), ago), /Agreed 3 minutes ago by 100% of validators/);
  assert.match(
    describeFreshness(toRate(rawRate(), NOW + DEFAULT_MAX_RATE_AGE + 1), ago),
    /Too old to rely on/,
  );
  assert.match(
    describeFreshness(toRate(rawRate({ updated_at: '0' }), NOW), ago),
    /No price has been agreed yet/,
  );
});

test('freshnessOf treats an unset maximum as never expiring', () => {
  assert.equal(freshnessOf(10_000_000, 0), 'fresh');
});

// ------------------------------------------------- pool vs agreed rate

test('a pool trading in line with the agreed rates raises nothing', () => {
  const yml = toRate(rawRate({ denom: 'uyml', rate: '0.4213' }), NOW);
  const usd = toRate(rawRate({ denom: 'uusd', rate: '1.00' }), NOW);

  // 0.4213 USD per YML ÷ 1.00 USD per USD = 0.4213 USD per YML.
  const c = comparePoolToRates(0.4213, yml, usd);
  assert.ok(c);
  assert.equal(Math.round(c.fairPrice * 10000) / 10000, 0.4213);
  assert.equal(c.notable, false);
});

test('a pool that has drifted from the agreed rates is flagged with its direction', () => {
  const yml = toRate(rawRate({ denom: 'uyml', rate: '0.40' }), NOW);
  const usd = toRate(rawRate({ denom: 'uusd', rate: '1.00' }), NOW);

  const high = comparePoolToRates(0.48, yml, usd)!;
  assert.equal(high.notable, true);
  assert.ok(high.divergence > 0, 'the pool prices YML above the agreed rate');

  const low = comparePoolToRates(0.32, yml, usd)!;
  assert.ok(low.divergence < 0);
});

// Comparing against a price that does not exist is worse than not comparing.
test('no comparison is made without both rates', () => {
  const usd = toRate(rawRate({ denom: 'uusd' }), NOW);
  assert.equal(comparePoolToRates(0.42, undefined, usd), null);
  assert.equal(comparePoolToRates(null, usd, usd), null);
});

test('a comparison against a stale rate says so rather than hiding', () => {
  const stale = toRate(rawRate({ denom: 'uyml', rate: '0.40' }), NOW + DEFAULT_MAX_RATE_AGE + 1);
  const usd = toRate(rawRate({ denom: 'uusd', rate: '1.00' }), NOW);

  const c = comparePoolToRates(0.48, stale, usd)!;
  assert.equal(c.stale, true);
  assert.equal(c.notable, true, 'the gap is still real; how much to trust it is the caveat');
});
