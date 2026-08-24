import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { assessHealth, faultTolerance, stalledAtHeight } from './health.ts';

const NOW = new Date('2026-08-23T19:00:00Z');
const at = (secondsAgo: number) => new Date(NOW.getTime() - secondsAgo * 1000).toISOString();

const status = (secondsAgo: number, over: Partial<{ catchingUp: boolean; height: number }> = {}) => ({
  chainId: 'yamale-devnet-2',
  latestHeight: over.height ?? 34928,
  latestTime: at(secondsAgo),
  catchingUp: over.catchingUp ?? false,
});

// --- the refusal a halted node answers with -----------------------------------

const STALLED =
  '{"code":3,"message":"invalid height: context did not contain latest block height in either ' +
  'check state or finalize block state (2733)","details":[]}';

test('the height a halted node names is read out of its refusal', () => {
  assert.equal(stalledAtHeight(STALLED), 2733);
  assert.equal(stalledAtHeight(JSON.parse(STALLED)), 2733, 'a parsed body too');
});

test('an unrelated failure is not mistaken for a stall', () => {
  // The consequence of getting this wrong: the page pins itself to a height it
  // invented and shows historical state under a "stopped" banner on a chain
  // that is running perfectly.
  assert.equal(stalledAtHeight('{"code":5,"message":"not found"}'), null);
  assert.equal(stalledAtHeight(''), null);
  assert.equal(stalledAtHeight(null), null);
  assert.equal(stalledAtHeight(undefined), null);
  // A 401 from the reverse proxy is an HTML page, not a chain error.
  assert.equal(stalledAtHeight('<html><title>401 Authorization Required</title>'), null);
});

test('height zero is refused', () => {
  // Zero means "latest" in the block-height header, which is the request that
  // just failed — retrying it would loop.
  assert.equal(
    stalledAtHeight('context did not contain latest block height in either state (0)'),
    null,
  );
});

// --- what state the chain is in -----------------------------------------------

test('a chain producing blocks reads as live', () => {
  const h = assessHealth({ status: status(3), blockSeconds: 5, now: NOW });
  assert.equal(h.state, 'live');
  assert.equal(h.severity, 'ok');
  assert.equal(h.height, 34928);
  assert.equal(h.ageSeconds, 3);
});

test('a late block is slow, and a much later one is stopped', () => {
  assert.equal(assessHealth({ status: status(20), blockSeconds: 5, now: NOW }).state, 'slow');
  assert.equal(assessHealth({ status: status(200), blockSeconds: 5, now: NOW }).state, 'stopped');
});

test('the thresholds scale with the interval the chain actually keeps', () => {
  // 200 seconds is a stopped five-second chain, a late thirty-second one, and a
  // healthy ninety-second one. Fixed thresholds have to be wrong about two of
  // those three.
  assert.equal(assessHealth({ status: status(200), blockSeconds: 5, now: NOW }).state, 'stopped');
  assert.equal(assessHealth({ status: status(200), blockSeconds: 30, now: NOW }).state, 'slow');
  assert.equal(assessHealth({ status: status(200), blockSeconds: 90, now: NOW }).state, 'live');
});

test('a fast chain is not called stopped over one missed proposer', () => {
  // A 500ms chain would otherwise be "stopped" four seconds after a hiccup.
  const h = assessHealth({ status: status(12), blockSeconds: 0.5, now: NOW });
  assert.equal(h.state, 'live', 'the floor under the multiple holds');
});

test('a replaying node says so', () => {
  const h = assessHealth({ status: status(2, { catchingUp: true }), blockSeconds: 5, now: NOW });
  assert.equal(h.state, 'catching-up');
  assert.equal(h.severity, 'warn');
});

test('a replaying node whose last block is ancient is stopped, not syncing', () => {
  // "Catching up" reads as "wait a moment". A day-old tip does not mean that,
  // and dressing it as a sync hides an outage behind a reassuring word.
  const h = assessHealth({ status: status(86_400, { catchingUp: true }), blockSeconds: 5, now: NOW });
  assert.equal(h.state, 'stopped');
});

test('a node refusing state queries is stopped even while its RPC answers', () => {
  // This is the real halt signature: CometBFT still reports a height, and every
  // figure on the page is historical. Reporting "live" here would be the most
  // misleading thing the interface could do.
  const h = assessHealth({ status: status(2), blockSeconds: 5, stalledAt: 2733, now: NOW });
  assert.equal(h.state, 'stopped');
  assert.equal(h.readingHistory, true);
  assert.equal(h.stalledAt, 2733);
});

test('no RPC and no refusal is unreachable, not stopped', () => {
  // Distinguishing these matters: one is the chain's problem and one is the
  // reader's network, and telling a ministry its chain has halted because their
  // office wifi dropped is a phone call nobody wants to receive.
  const h = assessHealth({ status: null, now: NOW });
  assert.equal(h.state, 'unreachable');
  assert.equal(h.height, null);
});

test('no RPC but a named stall height is a halted chain', () => {
  const h = assessHealth({ status: null, stalledAt: 2733, now: NOW });
  assert.equal(h.state, 'stopped');
  assert.equal(h.height, 2733, 'the height it is stuck at is the height to show');
  assert.equal(h.readingHistory, true);
});

test('an unparseable timestamp does not invent an age', () => {
  const h = assessHealth({
    status: { chainId: 'x', latestHeight: 5, latestTime: 'not a date', catchingUp: false },
    now: NOW,
  });
  assert.equal(h.ageSeconds, null);
  assert.equal(h.state, 'live', 'unknown age falls back to what is known, not to alarm');
});

test('with nothing measured the chain\'s own interval is assumed', () => {
  const h = assessHealth({ status: status(3), now: NOW });
  assert.equal(h.expectedSeconds, 5);
  assert.equal(h.state, 'live');
});

// --- how much of the set can be lost ------------------------------------------

test('a three-validator set with even stake stops when one goes down', () => {
  // Measured on this network. Two of three is 66.6%, and a commit needs more
  // than two thirds — so the chain halts on the first failure.
  const h = faultTolerance([1 / 3, 1 / 3, 1 / 3]);
  assert.equal(h.spare, 0);
  assert.equal(h.fragile, true);
});

test('four evenly-held validators survive one loss', () => {
  const h = faultTolerance([0.25, 0.25, 0.25, 0.25]);
  assert.equal(h.spare, 1);
  assert.equal(h.fragile, false);
});

test('tolerance is counted from stake, not from headcount', () => {
  // Ten validators, one holding 40%. A count-based answer says three can be
  // lost; the truth is that losing the big one alone stops the chain.
  const shares = [0.4, ...Array.from({ length: 9 }, () => 0.6 / 9)];
  const h = faultTolerance(shares);
  assert.equal(h.fragile, true);
  assert.equal(h.spare, 0);
});

test('an empty set is fragile rather than infinitely robust', () => {
  assert.deepEqual(faultTolerance([]), { spare: 0, fragile: true });
  assert.deepEqual(faultTolerance([0, 0]), { spare: 0, fragile: true });
});
