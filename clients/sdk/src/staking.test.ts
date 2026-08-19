import assert from 'node:assert/strict';
import { test } from 'node:test';

import { minimumReceived, quoteSwap, toPool, toProposal, toValidator } from './staking.ts';

// ---------------------------------------------------------------- swaps

const pool = toPool({
  id: '0',
  denom_a: 'uyml',
  denom_b: 'uusd',
  reserve_a: '1000000',
  reserve_b: '1000000',
  total_shares: '1000000',
  swap_fee_bps: '0',
});

// The chain returns 90909 for this trade — confirmed against a running node.
// A client that rounded the other way would quote 90910, and every trade at the
// quoted amount would be rejected for slippage. Pinning the exact figure is
// what keeps the two implementations honest.
test('a swap quote matches the chain, including its rounding direction', () => {
  const quote = quoteSwap(pool, '100000', 'uyml');
  assert.ok(quote);
  assert.equal(quote.amountOut, '90909');
});

test('the fee reduces the output', () => {
  const withFee = toPool({ ...poolRaw(), swap_fee_bps: '30' });
  const free = quoteSwap(pool, '100000', 'uyml')!;
  const charged = quoteSwap(withFee, '100000', 'uyml')!;
  assert.ok(BigInt(charged.amountOut) < BigInt(free.amountOut));
});

test('swapping the other way uses the other reserve', () => {
  const lopsided = toPool({ ...poolRaw(), reserve_a: '1000000', reserve_b: '4000000' });
  const forward = quoteSwap(lopsided, '100000', 'uyml')!;
  const back = quoteSwap(lopsided, '100000', 'uusd')!;
  assert.ok(BigInt(forward.amountOut) > BigInt(back.amountOut), 'the deeper side returns more');
});

test('a trade large relative to the pool reports high price impact', () => {
  const small = quoteSwap(pool, '1000', 'uyml')!;
  const large = quoteSwap(pool, '500000', 'uyml')!;
  assert.ok(small.priceImpact < 0.01, 'a small trade barely moves the price');
  assert.ok(large.priceImpact > 0.3, 'a huge trade must warn loudly');
});

test('unusable input is refused rather than guessed at', () => {
  assert.equal(quoteSwap(pool, '0', 'uyml'), null);
  assert.equal(quoteSwap(pool, 'abc', 'uyml'), null);
  assert.equal(quoteSwap(toPool({ ...poolRaw(), reserve_a: '0' }), '100', 'uyml'), null);
});

test('pool price accounts for differing denom exponents', () => {
  // 1 YML for 4 USD, both six decimals.
  const p = toPool({ ...poolRaw(), reserve_a: '1000000', reserve_b: '4000000' });
  assert.equal(p.price, 4);
});

function poolRaw() {
  return {
    id: '0',
    denom_a: 'uyml',
    denom_b: 'uusd',
    reserve_a: '1000000',
    reserve_b: '1000000',
    total_shares: '1000000',
    swap_fee_bps: '0',
  };
}

// ------------------------------------------------------------ validators

test('voting power is a share, and dangerous concentration is flagged', () => {
  const total = 1_000_000n;
  const big = toValidator({ operator_address: 'v1', tokens: '400000', description: { moniker: 'Big' } }, total);
  const small = toValidator({ operator_address: 'v2', tokens: '100000', description: { moniker: 'Small' } }, total);

  assert.equal(big.votingPower, 0.4);
  assert.equal(small.votingPower, 0.1);
  assert.equal(big.concerningPower, true, 'over a third can stall the chain alone');
  assert.equal(small.concerningPower, false);
});

test('commission is read as a fraction', () => {
  const v = toValidator(
    { operator_address: 'v1', tokens: '1', commission: { commission_rates: { rate: '0.100000000000000000' } } },
    1n,
  );
  assert.equal(v.commission, 0.1);
});

test('a chain with nothing bonded does not divide by zero', () => {
  const v = toValidator({ operator_address: 'v1', tokens: '0' }, 0n);
  assert.equal(v.votingPower, 0);
  assert.equal(v.concerningPower, false);
});

// ------------------------------------------------------------- proposals

const describe = (msg: any) => `does ${msg['@type']}`;

test('a proposal reports its tally as shares of votes cast', () => {
  const p = toProposal(
    {
      id: '3',
      title: 'Raise the treasury lock cap',
      summary: 'Allow more concurrent locks.',
      status: 'PROPOSAL_STATUS_VOTING_PERIOD',
      submit_time: '2026-08-01T00:00:00Z',
      voting_end_time: '2026-08-03T00:00:00Z',
      final_tally_result: {
        yes_count: '600',
        no_count: '200',
        abstain_count: '100',
        no_with_veto_count: '100',
      },
      messages: [{ '@type': '/blockchain.treasury.v1.MsgUpdateParams' }],
    },
    describe,
  );

  assert.equal(p.status, 'voting');
  assert.equal(p.statusLabel, 'Open for voting');
  assert.equal(p.tally.yes, 0.6);
  assert.equal(p.tally.no, 0.2);
  assert.equal(p.totalVoted, '1000');
  assert.deepEqual(p.actions, ['does /blockchain.treasury.v1.MsgUpdateParams']);
});

test('an untallied proposal reports zeroes rather than dividing by zero', () => {
  const p = toProposal({ id: '1', status: 'PROPOSAL_STATUS_DEPOSIT_PERIOD' }, describe);
  assert.equal(p.tally.yes, 0);
  assert.equal(p.totalVoted, '0');
  assert.equal(p.statusLabel, 'Waiting for enough deposit to go to a vote');
});

test('a status the client does not know is surfaced, not hidden', () => {
  const p = toProposal({ id: '9', status: 'PROPOSAL_STATUS_SOMETHING_NEW' }, describe);
  assert.equal(p.status, 'unknown');
  assert.equal(p.statusLabel, 'PROPOSAL_STATUS_SOMETHING_NEW');
});

// The minimum received is the number that goes into the signed message. Quoting
// only the expected amount teaches people to sign without a floor.
test('the minimum received is the quote less the tolerance, rounded down', () => {
  assert.equal(minimumReceived('90909', 100), '89999', '90909 less 1% is 89999.91 — floored, never rounded up');
  assert.equal(minimumReceived('90909', 50), '90454');
  assert.equal(minimumReceived('90909', 0), '90909');
});

test('a nonsensical tolerance cannot produce a floor above the quote', () => {
  assert.equal(minimumReceived('1000', -50), '1000', 'a negative tolerance is clamped, not applied backwards');
  assert.equal(minimumReceived('1000', 20000), '0', 'and one over 100% floors at nothing');
});

test('an unusable quote yields no floor rather than throwing', () => {
  assert.equal(minimumReceived('not-a-number', 100), '0');
  assert.equal(minimumReceived('0', 100), '0');
});
