/**
 * Tests for the console's reasoning.
 *
 * The threshold arithmetic here is a mirror of `Params.RequiredPower` in
 * x/enforcement/types/params.go, and the netting window state is a mirror of
 * the condition described in docs/scope/gaps.md open question 2. Both are
 * mirrors, so both can drift, and a drifted mirror is worse than no console:
 * it produces a confident wrong number. These tests are what catches the
 * drift.
 *
 * Run: node --test clients/oversight/
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as O from './oversight.js';

// ---------------------------------------------------------------------------
// requiredPower — the mirror of the keeper.
// ---------------------------------------------------------------------------

test('two thirds of a round number', () => {
  // 6667 bps of 300 = 200.01 → 201 after rounding up.
  assert.equal(O.requiredPower(300, 6667), 201);
});

test('rounds UP, so a case cannot pass one unit short of the bar', () => {
  // 6667 * 3 = 20001; 20001/10000 = 2.0001. Truncation would say 2 and let a
  // validator with 2 power pass a case needing more than two thirds of 3.
  assert.equal(O.requiredPower(3, 6667), 3);
  assert.notEqual(O.requiredPower(3, 6667), 2);
});

test('an exact division does not gain a spurious extra unit', () => {
  // 5000 bps of 200 = exactly 100. Rounding up unconditionally would say 101.
  assert.equal(O.requiredPower(200, 5000), 100);
});

test('a total of zero needs zero, and does not divide by anything', () => {
  assert.equal(O.requiredPower(0, 6667), 0);
  assert.equal(O.requiredPower(-5, 6667), 0);
});

test('the live chain: two validators, and neither can seize alone', () => {
  // Measured on yamale-devnet-2 at height 119318: powers 100000 and 74900.
  const set = [{ power: 100000 }, { power: 74900 }];
  const c = O.seizureCoalition(set, 6667);

  assert.equal(c.totalPower, 174900);
  // 174900 * 6667 = 1166058300; / 10000 = 116605.83 → 116606 after rounding up.
  assert.equal(c.required, 116606);

  // The asymmetry, in two numbers. One validator stops money; two are needed
  // to take it.
  assert.equal(c.toFreeze, 1);
  assert.equal(c.minimumToSeize, 2);
  assert.equal(c.largestCanSeizeAlone, false);

  // The largest holds 57.17% — comfortably short of two thirds.
  assert.equal(c.largestSingleShareBps, 5718);
});

test('a validator holding two thirds alone is reported as able to seize alone', () => {
  // The honest failure case. If the set ever concentrates like this, the
  // console must say so rather than keep printing "two thirds are needed" as
  // if that implied more than one signature.
  const c = O.seizureCoalition([{ power: 80 }, { power: 20 }], 6667);
  assert.equal(c.minimumToSeize, 1);
  assert.equal(c.largestCanSeizeAlone, true);
});

test('an empty validator set cannot freeze and cannot seize', () => {
  const c = O.seizureCoalition([], 6667);
  assert.equal(c.toFreeze, 0);
  assert.equal(c.minimumToSeize, null);
  assert.equal(c.largestSingleShareBps, 0);
});

test('validators with zero power are excluded from the coalition arithmetic', () => {
  const c = O.seizureCoalition([{ power: 100 }, { power: 0 }], 6667);
  assert.equal(c.validators, 1);
  assert.equal(c.totalPower, 100);
});

// ---------------------------------------------------------------------------
// caseStanding — including the case that is already lost.
// ---------------------------------------------------------------------------

const params = { threshold_bps: 6667 };

test('a case at the bar is met', () => {
  const s = O.caseStanding({
    total_power_at_open: 300, yes_power: 201, no_power: 0, abstain_power: 0,
  }, params);
  assert.equal(s.required, 201);
  assert.equal(s.met, true);
  assert.equal(s.shortBy, 0);
});

test('a case one unit below the bar is not met', () => {
  const s = O.caseStanding({
    total_power_at_open: 300, yes_power: 200, no_power: 0, abstain_power: 0,
  }, params);
  assert.equal(s.met, false);
  assert.equal(s.shortBy, 1);
});

test('a case is unreachable once enough power has voted no', () => {
  // 300 bonded, 201 needed. 100 against leaves 200 possible — one short.
  // The keeper rejects here eagerly; a console drawing only the yes bar would
  // show 0% and imply the case is still in play.
  const s = O.caseStanding({
    total_power_at_open: 300, yes_power: 0, no_power: 100, abstain_power: 0,
  }, params);
  assert.equal(s.reachable, false);
});

test('abstentions are spent power and count against reachability', () => {
  // A validator that abstained cannot come back and vote yes.
  const s = O.caseStanding({
    total_power_at_open: 300, yes_power: 0, no_power: 0, abstain_power: 100,
  }, params);
  assert.equal(s.reachable, false);
  assert.equal(s.abstainBps, 3333);
});

test('a fresh case with nothing cast is reachable and not met', () => {
  const s = O.caseStanding({
    total_power_at_open: 300, yes_power: 0, no_power: 0, abstain_power: 0,
  }, params);
  assert.equal(s.reachable, true);
  assert.equal(s.met, false);
  assert.equal(s.uncast, 300);
});

test('a case opened against a set with no bonded power is neither met nor reachable', () => {
  const s = O.caseStanding({
    total_power_at_open: 0, yes_power: 0, no_power: 0, abstain_power: 0,
  }, params);
  assert.equal(s.met, false);
  assert.equal(s.reachable, false);
  assert.equal(s.yesBps, 0); // and no division by zero
});

// ---------------------------------------------------------------------------
// Freezes.
// ---------------------------------------------------------------------------

test('a freeze expiring at zero is permanent, not missing', () => {
  // makePermanent zeroes this field when a case passes. Reading the zero as an
  // absent value would render the most serious state on the page as a blank.
  const f = O.freezeCountdown(
    { expires_at_height: 0, frozen_at_height: 100 }, 500, 5,
  );
  assert.equal(f.permanent, true);
  assert.equal(f.blocksLeft, null);
  assert.equal(f.heldFor, 400);
});

test('a live freeze reports blocks left and an estimated duration', () => {
  const f = O.freezeCountdown(
    { expires_at_height: 1000, frozen_at_height: 280 }, 640, 5,
  );
  assert.equal(f.blocksLeft, 360);
  assert.equal(f.lapsed, false);
  assert.equal(f.estimate, 1800);
});

test('a freeze past its expiry height is lapsed', () => {
  // The end blocker lifts every freeze whose expiry is at or below the current
  // height, so a lapsed freeze that persists is worth seeing.
  const f = O.freezeCountdown({ expires_at_height: 100, frozen_at_height: 1 }, 100, 5);
  assert.equal(f.lapsed, true);
  assert.equal(f.blocksLeft, 0);
});

test('no estimate is offered without a measured block interval', () => {
  const f = O.freezeCountdown({ expires_at_height: 1000, frozen_at_height: 1 }, 640, 0);
  assert.equal(f.estimate, null);
});

test('a freeze carrying case id zero has no case attached', () => {
  // Enforcement case ids start at 1, so 0 here is not "case 0".
  assert.equal(O.freezeHasCase({ case_id: 0 }), false);
  assert.equal(O.freezeHasCase({ case_id: 1 }), true);
});

// ---------------------------------------------------------------------------
// The seizure window.
// ---------------------------------------------------------------------------

test('the count cap and the value cap are reported separately', () => {
  const h = O.windowHeadroom({
    window_start_height: 100, current_height: 200,
    cap: [{ denom: 'uyml', amount: '500000000' }],
    seized: [{ denom: 'uyml', amount: '100000000' }],
    remaining: [{ denom: 'uyml', amount: '400000000' }],
    seizure_count: 5, max_seizures: 5,
  });
  // Exhausted by count even though four fifths of the value cap is unused.
  assert.equal(h.countExhausted, true);
  assert.equal(h.denoms[0].remaining, '400000000');
});

test('a denomination the cap does not name is reported as uncapped by value', () => {
  // Only the count cap holds it back. Saying nothing would read as "zero
  // seized of zero allowed", which is the opposite of the truth.
  const h = O.windowHeadroom({
    cap: [{ denom: 'uyml', amount: '500' }],
    seized: [{ denom: 'uyml', amount: '10' }, { denom: 'uusd', amount: '99' }],
    remaining: [], seizure_count: 1, max_seizures: 5,
  });
  assert.deepEqual(h.uncapped, [{ denom: 'uusd', seized: '99' }]);
  assert.equal(h.countExhausted, false);
});

// ---------------------------------------------------------------------------
// x/netting: the window, and the gap the project wrote down.
// ---------------------------------------------------------------------------

test('cycle_blocks of zero with an empty window is a configuration, not a fault', () => {
  const s = O.nettingWindowState({
    params: { cycle_blocks: 0 },
    cycle: { status: 1, opened_at_height: 1, outcomes: [] },
    closesAtHeight: 0, held: [], height: 119400,
  });
  assert.equal(s.state, 'off');
  assert.match(s.headline, /switched off/);
});

test('cycle_blocks of zero with obligations trapped in the open window is a stall', () => {
  // docs/scope/gaps.md open question 2: the end blocker returns before closing
  // anything, so this window never settles and its collateral cannot be
  // released by any message.
  const s = O.nettingWindowState({
    params: { cycle_blocks: 0 },
    cycle: {
      status: 1, opened_at_height: 60,
      outcomes: [{ denom: 'ungn', status: 1, gross_amount: '39300000' }],
    },
    closesAtHeight: 0, held: [], height: 119400,
  });
  assert.equal(s.state, 'stalled');
  assert.deepEqual(s.trappedDenoms, ['ungn']);
  assert.equal(s.stalledForBlocks, 119340);
  assert.match(s.headline, /cannot close/);
});

test('a zero gross amount in an outcome does not count as trapped traffic', () => {
  // gross_amount is always emitted as a decimal string, so "0" is the empty
  // case and must not raise the alarm.
  const s = O.nettingWindowState({
    params: { cycle_blocks: 0 },
    cycle: { status: 1, opened_at_height: 1, outcomes: [{ denom: 'ungn', gross_amount: '0' }] },
    closesAtHeight: 0, held: [], height: 500,
  });
  assert.equal(s.state, 'off');
});

test('held slices frozen by a disabled end blocker are a stall in their own right', () => {
  const s = O.nettingWindowState({
    params: { cycle_blocks: 0 },
    cycle: { status: 1, opened_at_height: 1, outcomes: [] },
    closesAtHeight: 0,
    held: [{ cycle_id: 3, denom: 'ungn', reason: 'insufficient reserve' }],
    height: 500,
  });
  assert.equal(s.state, 'stalled');
  assert.equal(s.frozenHeldSlices, 1);
});

test('an end height already in the past is NOT a stall while netting is enabled', () => {
  // The false positive the naive check produces. closes_at_height is
  // recomputed from the CURRENT cycle_blocks, so shortening it mid-window
  // legitimately yields a height already passed; the end blocker still closes
  // at the next multiple.
  const s = O.nettingWindowState({
    params: { cycle_blocks: 50 },
    cycle: { status: 1, opened_at_height: 400, outcomes: [] },
    closesAtHeight: 450, held: [], height: 600,
  });
  assert.equal(s.state, 'open');
});

test('a held slice on an enabled chain is held, not stalled', () => {
  const s = O.nettingWindowState({
    params: { cycle_blocks: 50 },
    cycle: { status: 3, opened_at_height: 400, outcomes: [{ denom: 'ungn', status: 3 }] },
    closesAtHeight: 450,
    held: [{ cycle_id: 4, denom: 'ungn', reason: 'insufficient reserve' }],
    height: 420,
  });
  assert.equal(s.state, 'held');
  assert.deepEqual(s.trappedDenoms, ['ungn']);
  assert.match(s.detail, /retries a held slice unchanged/);
});

// ---------------------------------------------------------------------------
// Exposure.
// ---------------------------------------------------------------------------

test('exposure derives availability and utilisation from reserve and locked', () => {
  const [e] = O.exposure([{
    denom: 'ungn', reserve: '20000000', locked: '3000000',
    available: '17000000', net_position: '-3000000',
  }]);
  assert.equal(e.available, '17000000');
  assert.equal(e.owes, true);
  assert.equal(e.utilisationBps, 1500);
  assert.equal(e.fullyCommitted, false);
});

test('a fully committed participant can submit nothing further', () => {
  const [e] = O.exposure([{
    denom: 'ungn', reserve: '100', locked: '100', available: '0', net_position: '-100',
  }]);
  assert.equal(e.utilisationBps, 10000);
  assert.equal(e.fullyCommitted, true);
  assert.equal(e.available, '0');
});

test('exposure handles amounts beyond what a double can hold', () => {
  // These are decimal strings on the wire precisely so they are not rounded,
  // and this console must not undo that by parsing them as numbers.
  const [e] = O.exposure([{
    denom: 'ungn', reserve: '123456789012345678901234567890',
    locked: '0', available: '0', net_position: '0',
  }]);
  assert.equal(e.available, '123456789012345678901234567890');
});

test('a participant with no reserve reports zero utilisation rather than dividing by zero', () => {
  const [e] = O.exposure([{ denom: 'ungn', reserve: '0', locked: '0', available: '0', net_position: '0' }]);
  assert.equal(e.utilisationBps, 0);
  assert.equal(e.fullyCommitted, false);
});

// ---------------------------------------------------------------------------
// The refusals panel, and the honesty rule attached to it.
// ---------------------------------------------------------------------------

test('every refusal names where it is enforced', () => {
  for (const r of O.REFUSALS) {
    assert.ok(r.what && r.how && r.where, `incomplete refusal: ${r.what}`);
  }
});

test('only the four constitutionally pinned rules claim to be pinned', () => {
  // x/enforcement/types/constitutional.go pins exactly threshold_bps,
  // recovery_destination, voting_period_blocks and provisional_freeze_blocks.
  // The module guide says the delay, the window cap and the ombudsman are
  // INTENDED to become invariants — they are not today, and a console telling
  // a reader governance cannot change them would be reassuring them falsely
  // about the exact powers they came to check.
  const pinned = O.REFUSALS.filter((r) => r.pinned);
  assert.equal(pinned.length, 2);
  for (const r of pinned) assert.match(r.pinNote, /constitution/i);

  const window = O.REFUSALS.find((r) => /expropriation/.test(r.what));
  assert.equal(window.pinned, false);
  assert.match(window.pinNote, /not yet/);

  const ombudsman = O.REFUSALS.find((r) => /can never start one/.test(r.what));
  assert.equal(ombudsman.pinned, false);
  assert.match(ombudsman.pinNote, /not one yet/);
});

// ---------------------------------------------------------------------------
// Signing.
// ---------------------------------------------------------------------------

test('the console signs nothing', () => {
  // Every enforcement authority on this chain is a validator operator key or
  // an x/group policy account. A browser button collapses an office's
  // procedure into a click nobody can audit afterwards.
  for (const s of O.SIGNING) {
    assert.notEqual(s.decision, 'sign', `${s.type} would be signed in the browser`);
    assert.ok(['command', 'propose'].includes(s.decision));
    assert.ok(s.signer, `${s.type} has no signer field recorded`);
    assert.ok(s.why.length > 40, `${s.type} has no reasoning recorded`);
  }
});

test('every message type carries the blockchain. prefix, not yamale.blockchain.', () => {
  // The REST gateway paths carry /yamale/blockchain/..., which belongs to the
  // HTTP annotation and to nothing else. The proto package — and therefore the
  // Any type URL — is blockchain.<module>.v1.
  for (const s of O.SIGNING) {
    assert.match(s.type, /^\/blockchain\.(enforcement|netting)\.v1\.Msg/);
    assert.doesNotMatch(s.type, /yamale/);
  }
});

test('the permissionless sweep is recorded as permissionless and still not signed here', () => {
  const sweep = O.SIGNING.find((s) => s.type.endsWith('MsgSweep'));
  assert.match(sweep.who, /Anyone/);
  assert.equal(sweep.decision, 'command');
  // And the reasoning must mention the refusal that makes the permissionless
  // path safe — a sweep is refused while a case is HELD.
  assert.match(sweep.why, /HELD/);
});

// ---------------------------------------------------------------------------
// Presentation that carries a judgement.
// ---------------------------------------------------------------------------

test('two thirds renders as 66.67%, not as 67%', () => {
  assert.equal(O.bpsPercent(6667), '66.67%');
});

test('a duration is not invented without a measured block interval', () => {
  assert.equal(O.blocksAsDuration(720, 0), null);
  assert.equal(O.blocksAsDuration(720, null), null);
  assert.equal(O.blocksAsDuration(0, 5), null);
});

test('durations pick a sensible unit', () => {
  assert.equal(O.blocksAsDuration(10, 5), 'about 50 seconds');
  assert.equal(O.blocksAsDuration(360, 5), 'about 30 minutes');
  assert.equal(O.blocksAsDuration(720, 5), 'about 1 hour');
  assert.equal(O.blocksAsDuration(120960, 5), 'about 7 days');
});

test('amounts are grouped and keep their denomination in base units', () => {
  // No conversion to a display unit: this console does not know the exponent
  // of an arbitrary denomination, and guessing misstates a seizure by orders
  // of magnitude.
  //
  // The separator is referenced rather than typed. A literal here would be an
  // invisible character in a test file, and the first person to normalise the
  // whitespace would break the assertion without being able to see why.
  const sp = O.GROUP_SEPARATOR;
  assert.equal(O.amount({ denom: 'uyml', amount: '500000000' }), `500${sp}000${sp}000 uyml`);
  assert.equal(O.amount({ denom: 'ungn', amount: '-3000000' }), `−3${sp}000${sp}000 ungn`);
  assert.equal(O.amount(null), '—');
});

test('the group separator does not let an amount wrap across a line', () => {
  // U+202F, a narrow no-break space. An ordinary space would let "500 000 000"
  // break into what looks like two numbers, and these are the numbers a reader
  // compares by eye.
  assert.equal(O.GROUP_SEPARATOR, ' ');
});

test('a status the console was written before renders as unknown, not as unspecified', () => {
  assert.equal(O.statusName(99), 'UNKNOWN_99');
  assert.equal(O.statusName(8), 'VETOED');
});

test('a vetoed or reversed case is toned as a check that worked', () => {
  assert.equal(O.statusTone(8), 'ok'); // VETOED
  assert.equal(O.statusTone(6), 'ok'); // REVERSED
  assert.equal(O.statusTone(2), 'bad'); // PASSED — a seizure was decided
  assert.equal(O.statusTone(1), 'warn'); // VOTING — live
});

test('every refusal is attributed to the module it belongs to', () => {
  // Each half of the console shows its own refusals. An untagged one would
  // silently vanish from both.
  for (const r of O.REFUSALS) {
    assert.ok(['enforcement', 'netting'].includes(r.module), 'untagged: ' + r.what);
  }
  assert.ok(O.REFUSALS.some((r) => r.module === 'netting'));
  assert.ok(O.REFUSALS.some((r) => r.module === 'enforcement'));
});

test('an unreadable amount is reported as unreadable, never as zero', () => {
  // 0 posted and we-could-not-read-what-was-posted are opposite answers to the
  // only question this panel is asked. One malformed entry must also not take
  // the rest of the table down with it.
  const rows = O.exposure([
    { denom: 'ungn', reserve: 'not-a-number', locked: '0', available: '0', net_position: '0' },
    { denom: 'uyml', reserve: '500', locked: '100', available: '400', net_position: '-100' },
  ]);
  assert.equal(rows[0].unreadable, true);
  assert.equal(rows[0].reserve, undefined);
  assert.equal(rows[1].unreadable, false);
  assert.equal(rows[1].available, '400');
});

test('an empty amount string is unreadable rather than zero', () => {
  const [r] = O.exposure([{ denom: 'ungn', reserve: '', locked: '0', net_position: '0' }]);
  assert.equal(r.unreadable, true);
});
