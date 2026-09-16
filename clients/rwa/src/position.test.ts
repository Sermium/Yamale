import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  POSITIONS_PREFIX,
  costOf,
  int,
  pendingAccrual,
  positionKey,
  returnOn,
  type Position,
} from './position.ts';

function position(over: Partial<Position> = {}): Position {
  return {
    assetId: '4',
    holder: 'yml1holder',
    accrued: '0',
    costBasis: '0',
    costQty: '0',
    tokenSeconds: '0',
    tokenSecondsPeriod: '0',
    lastTouch: 0,
    ...over,
  };
}

/* ------------------------------------------------------------- the integers */

test('a chain integer that was never written reads as zero rather than throwing', () => {
  // Every one of these fields arrives as '' on a position written before the
  // field existed. A thrown BigInt in a render path takes the page with it.
  assert.equal(int(''), 0n);
  assert.equal(int(undefined), 0n);
  assert.equal(int(null), 0n);
  assert.equal(int('nonsense'), 0n);
  assert.equal(int('  42 '), 42n);
});

/* ========================================================================== */
/*  Cost                                                                      */
/* ========================================================================== */

test('a quantity of zero is COST NOT RECORDED and never a cost of zero', () => {
  // The headline rule. Cost zero reports an infinite return, which is the most
  // flattering and most wrong figure this module could produce.
  const cost = costOf(position({ costQty: '0', costBasis: '0' }), '9000000');
  assert.equal(cost.state, 'none');
  assert.equal(cost.uncovered, '9000000');
  assert.equal(cost.zeroCost, false, 'unrecorded is not the same fact as free');
});

test('a position that does not exist at all is COST NOT RECORDED', () => {
  const cost = costOf(null, '500');
  assert.equal(cost.state, 'none');
  assert.equal(cost.uncovered, '500');
});

test('a fully covered holding reports what was paid and what it covers', () => {
  const cost = costOf(position({ costQty: '1000', costBasis: '250000' }), '1000');
  assert.equal(cost.state, 'full');
  assert.equal(cost.basis, '250000');
  assert.equal(cost.covered, '1000');
  assert.equal(cost.uncovered, '0');
});

test('a partly covered holding never averages a known cost with an unknown one', () => {
  // Five million subscribed at a recorded price, four million arrived as a
  // plain transfer from somebody with no basis. The ordinary case.
  const cost = costOf(position({ costQty: '5000000', costBasis: '500000' }), '9000000');
  assert.equal(cost.state, 'partial');
  assert.equal(cost.covered, '5000000');
  assert.equal(cost.uncovered, '4000000');
  // The basis stays attached to the five million it was paid for. Spreading it
  // over all nine would report a cost nobody paid for any of them.
  assert.equal(cost.basis, '500000');
});

test('a basis covering more than the balance is clamped, never negative', () => {
  // The keeper's invariant should prevent this, but a position read at one
  // height beside a bank balance read at another can disagree by a transfer.
  const cost = costOf(position({ costQty: '1000', costBasis: '5000' }), '400');
  assert.equal(cost.covered, '400');
  assert.equal(cost.uncovered, '0');
});

test('a recorded cost of zero is a fact, and is not the same as no cost', () => {
  // A free allocation recorded as a subscription at a price of nothing. Real,
  // deliberate, and still yields no percentage.
  const cost = costOf(position({ costQty: '1000', costBasis: '0' }), '1000');
  assert.equal(cost.state, 'full');
  assert.equal(cost.zeroCost, true);
  assert.equal(cost.basis, '0', 'a recorded zero is still a recorded quantity');
});

/* ========================================================================== */
/*  Return                                                                    */
/* ========================================================================== */

test('no basis means no return figure, and a stated reason', () => {
  const cost = costOf(position(), '1000');
  const r = returnOn(cost, '500', '1000');
  assert.equal(r.percent, null);
  assert.equal(r.income, null);
  assert.equal(r.whyKey, 'rwa.cost.noBasis',
    'a dash in a return column reads as zero to most people and broken to the rest');
});

test('a cost of zero yields no return rather than an infinite one', () => {
  const cost = costOf(position({ costQty: '1000', costBasis: '0' }), '1000');
  const r = returnOn(cost, '999999', '1000');
  assert.equal(r.percent, null);
  assert.equal(r.whyKey, 'rwa.cost.zeroBasis');
});

test('an unanswered entitlement query is not a return of zero', () => {
  const cost = costOf(position({ costQty: '1000', costBasis: '100' }), '1000');
  const r = returnOn(cost, null, '1000');
  assert.equal(r.percent, null);
  assert.equal(r.whyKey, 'rwa.cost.owedUnknown');
});

test('a return on a fully covered holding is income over cost', () => {
  const cost = costOf(position({ costQty: '1000', costBasis: '10000' }), '1000');
  const r = returnOn(cost, '2500', '1000');
  assert.equal(r.income, '2500');
  assert.equal(r.percent, 25);
});

test('a return is reported on the COVERED part only', () => {
  // Nine million held, five million covered by a basis of 500,000. Income of
  // 900,000 across the whole holding is 500,000 attributable to the covered
  // tokens -- a 100% return on what was actually paid.
  //
  // Reporting 900,000 over 500,000 would be 180%, a return on tokens the
  // holder never paid for.
  const cost = costOf(position({ costQty: '5000000', costBasis: '500000' }), '9000000');
  const r = returnOn(cost, '900000', '9000000');
  assert.equal(r.income, '500000');
  assert.equal(r.percent, 100);
});

test('a small return does not collapse to zero', () => {
  const cost = costOf(position({ costQty: '1000', costBasis: '1000000000' }), '1000');
  const r = returnOn(cost, '1000', '1000');
  assert.ok(r.percent !== null && r.percent > 0, 'somebody earned that');
});

test('income attributable to the covered part truncates toward the vault', () => {
  // Every division in x/tokenisation truncates, so a figure quoted here is
  // never larger than what the chain would actually pay.
  const cost = costOf(position({ costQty: '1', costBasis: '1' }), '3');
  const r = returnOn(cost, '10', '3');
  assert.equal(r.income, '3', '10 * 1 / 3 = 3, not 4');
});

/* ========================================================================== */
/*  Pending accrual                                                           */
/* ========================================================================== */

const VAULT = { periodIndex: '3', periodStart: 1_000 };

test('token-seconds accrue from the last touch inside the same period', () => {
  const p = pendingAccrual(
    position({ tokenSeconds: '5000', tokenSecondsPeriod: '3', lastTouch: 1_200 }),
    VAULT, '100', 1_300,
  );
  // 5000 already banked, plus 100 tokens for 100 more seconds.
  assert.equal(p.tokenSeconds, '15000');
  assert.equal(p.periodIndex, '3');
});

test('seconds belonging to a CLOSED period are money now, and are not carried forward', () => {
  // The branch that matters most. Carrying them would double-count a
  // distribution the holder has already been credited for.
  const p = pendingAccrual(
    position({ tokenSeconds: '999999', tokenSecondsPeriod: '2', lastTouch: 500 }),
    VAULT, '100', 1_300,
  );
  // Fresh from the period's start (1000), not from the last touch (500).
  assert.equal(p.tokenSeconds, '30000');
});

test('a holder who touched after the period opened accrues from the touch', () => {
  const p = pendingAccrual(
    position({ tokenSeconds: '400', tokenSecondsPeriod: '1', lastTouch: 1_100 }),
    VAULT, '10', 1_300,
  );
  // max(periodStart 1000, lastTouch 1100) = 1100; 10 * 200 = 2000.
  assert.equal(p.tokenSeconds, '2000');
});

test('a zero balance accrues nothing and reports no time held', () => {
  const p = pendingAccrual(
    position({ tokenSecondsPeriod: '3', lastTouch: 1_000 }), VAULT, '0', 2_000,
  );
  assert.equal(p.tokenSeconds, '0');
  assert.equal(p.heldSeconds, null, 'dividing by a zero balance says nothing');
});

test('a clock that has gone backwards does not subtract accrual', () => {
  const p = pendingAccrual(
    position({ tokenSeconds: '700', tokenSecondsPeriod: '3', lastTouch: 2_000 }),
    VAULT, '100', 1_500,
  );
  assert.equal(p.tokenSeconds, '700');
});

test('no position means no accrual', () => {
  const p = pendingAccrual(null, VAULT, '1000', 5_000);
  assert.equal(p.tokenSeconds, '0');
});

test('time held is the integral divided by the balance', () => {
  const p = pendingAccrual(
    position({ tokenSeconds: '0', tokenSecondsPeriod: '3', lastTouch: 1_000 }),
    VAULT, '250', 1_000 + 86_400,
  );
  assert.equal(p.heldSeconds, 86_400, 'a day, said in a unit a person has intuitions about');
});

test('period zero is flagged as the first, since nothing has been declared yet', () => {
  const p = pendingAccrual(position(), { periodIndex: '0', periodStart: 10 }, '1', 20);
  assert.equal(p.first, true);
  assert.equal(pendingAccrual(position(), VAULT, '1', 2_000).first, false);
});

test('nothing in this module converts token-seconds into a currency', async () => {
  // The rule, asserted rather than trusted to review. A forecast standing
  // beside a settled figure in the same units cannot be told apart from it.
  const module = await import('./position.ts');
  const pending = pendingAccrual(
    position({ tokenSecondsPeriod: '3', lastTouch: 1_000 }), VAULT, '100', 2_000,
  );
  assert.equal(typeof pending.tokenSeconds, 'string');
  assert.ok(!('amount' in pending) && !('value' in pending) && !('denom' in pending));
  assert.ok(!Object.keys(module).some((k) => /rate|toCurrency|toAmount|forecast/i.test(k)));
});

/* ========================================================================== */
/*  The store key                                                             */
/* ========================================================================== */

test('a position key is the prefix, the id big-endian, then the address', () => {
  // Verified against yamale-devnet-2: a subspace walk of prefix 4 returns keys
  // of exactly this shape, and this key reads back the expected position.
  const key = positionKey('3', 'yml1sayjzake5a0adj28wenjz8ek9xvr9eywpe9atm');
  assert.equal(key[0], POSITIONS_PREFIX);
  assert.deepEqual([...key.slice(1, 9)], [0, 0, 0, 0, 0, 0, 0, 3]);
  assert.equal(
    new TextDecoder().decode(key.slice(9)),
    'yml1sayjzake5a0adj28wenjz8ek9xvr9eywpe9atm',
  );
});

test('the asset id is big-endian, which is what keeps the map numerically ordered', () => {
  // Little-endian would land every asset in the wrong place and read back a
  // different holder's position rather than failing.
  const key = positionKey('258', 'a');
  assert.deepEqual([...key.slice(1, 9)], [0, 0, 0, 0, 0, 0, 1, 2]);
});

test('an id beyond a double is encoded exactly', () => {
  const key = positionKey('18446744073709551615', 'a');
  assert.deepEqual([...key.slice(1, 9)], [255, 255, 255, 255, 255, 255, 255, 255]);
});

test('the address carries no length prefix and no terminator', () => {
  // It is the LAST element of the pair, so collections encodes it terminally:
  // raw bytes, because nothing follows it.
  const key = positionKey('1', 'abc');
  assert.equal(key.length, 1 + 8 + 3);
});
