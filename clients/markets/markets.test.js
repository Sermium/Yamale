/**
 * Tests for the markets console's judgement.
 *
 * The swap arithmetic is the reason this file exists. A quote that disagrees
 * with x/amm's keeper by one base unit is a transaction that the trader signs
 * and the node rejects, and the direction of the disagreement decides whether
 * the trader loses a rounding unit or the pool does. So the tests below do not
 * only check that the numbers look plausible: they pin the rounding direction
 * against the exact form the keeper rejects, and they assert the constant-
 * product invariant that form violates.
 *
 *   node --test clients/markets/markets.test.js
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  BPS,
  CHAIN,
  EMPTY_AMOUNT,
  EXPIRING_FRACTION,
  IMPACT_THRESHOLDS,
  MESSAGES,
  SIGNERS,
  agreement,
  approveIssuerProposal,
  formatAmount,
  freshness,
  groupDigits,
  impactVerdict,
  mintability,
  isOwnAccount,
  ratio,
  reporting,
  resolveDenom,
  sh,
  span,
  submitRatesCommand,
  swapCommand,
  swapOut,
  swapQuote,
  toBaseUnits,
  toBaseUnitsOf,
  toDisplayAmount,
  truncatedFrom,
} from './markets.js';

// ---------------------------------------------------------------------------
// x/amm — the swap curve
// ---------------------------------------------------------------------------

/**
 * The form the keeper's comment explicitly rejects, reproduced here so a test
 * can prove the difference is real rather than theoretical.
 *
 * `reserveOut - (reserveIn * reserveOut) / (reserveIn + amountInAfterFee)`
 *
 * is algebraically identical to what the keeper computes, and arithmetically
 * different once integers truncate: this one truncates the subtrahend, which
 * rounds the *output* up.
 */
function bleedingForm(reserveIn, reserveOut, amountIn, feeBps) {
  const rIn = BigInt(reserveIn);
  const rOut = BigInt(reserveOut);
  const aIn = BigInt(amountIn);
  const afterFee = (aIn * (BPS - BigInt(feeBps))) / BPS;
  return rOut - (rIn * rOut) / (rIn + afterFee);
}

/**
 * What x/amm's keeper actually pays out, produced by running its own
 * expression through cosmossdk.io/math.
 *
 * Not hand-computed and not derived from reading the Go: `go run
 * ./tools/ammparity` evaluates the keeper's line verbatim in the same
 * arbitrary-precision integer type and prints these rows. A disagreement here
 * is therefore a disagreement between the console and the chain, not between
 * two readings of the same source.
 *
 * Rows are [reserveIn, reserveOut, amountIn, feeBps, out]. They include the
 * live pool 1 on yamale-devnet-2, reserves whose product leaves a remainder,
 * inputs of a few base units where truncation is the entire answer, and a trade
 * of half the input reserve where the curve dominates the fee.
 */
const KEEPER = [
  ['20000000000', '30000000000', '1000000000', 0, '1428571428'],
  ['20000000000', '30000000000', '1000000000', 30, '1424489212'],
  ['20000000000', '30000000000', '1000000000', 500, '1360381861'],
  ['1000000', '3000001', '7', 0, '20'],
  ['1000000', '3000001', '7', 30, '17'],
  ['1000000', '3000001', '7', 500, '17'],
  ['999999', '1000003', '13', 0, '12'],
  ['999999', '1000003', '13', 30, '11'],
  ['999999', '1000003', '13', 500, '11'],
  ['20000000000', '30000000000', '1', 0, '1'],
  ['20000000000', '30000000000', '1', 30, '0'],
  ['20000000000', '30000000000', '1', 500, '0'],
  ['20000000000', '30000000000', '10000000000', 0, '10000000000'],
  ['20000000000', '30000000000', '10000000000', 30, '9979979979'],
  ['20000000000', '30000000000', '10000000000', 500, '9661016949'],
  ['10000000000', '15000000000', '123456789', 0, '182926827'],
  ['10000000000', '15000000000', '123456789', 30, '182384718'],
  ['10000000000', '15000000000', '123456789', 500, '173886513'],
  ['7', '11', '999999', 0, '10'],
  ['7', '11', '999999', 30, '10'],
  ['7', '11', '999999', 500, '10'],
  ['1000', '1001', '3', 0, '2'],
  ['1000', '1001', '3', 30, '1'],
  ['1000', '1001', '3', 500, '1'],
];

test('the quote equals what the keeper pays, on every fixture row', () => {
  // The test this file exists for. Nothing else here proves the console and the
  // chain agree; this does, against the chain's own arithmetic.
  for (const [rIn, rOut, aIn, fee, expected] of KEEPER) {
    assert.equal(
      String(swapOut(rIn, rOut, aIn, fee)),
      expected,
      `keeper pays ${expected} for ${aIn} into ${rIn}/${rOut} at ${fee} bps`,
    );
  }
  assert.equal(KEEPER.length, 24);

  // And the rejected form disagrees with the keeper on rows where the keeper's
  // truncation actually bit — proof that matching the formula is not automatic.
  const differing = KEEPER.filter(([rIn, rOut, aIn, fee, expected]) =>
    bleedingForm(rIn, rOut, aIn, fee) !== BigInt(expected));
  assert.ok(differing.length >= 5, `only ${differing.length} rows distinguish the two forms`);
});

test('swapOut reproduces the keeper formula step for step', () => {
  // Pool 1 on yamale-devnet-2 at height 119299: 20,000 YML against 30,000 NGN,
  // 30 bps. Worked by hand:
  //   afterFee  = 1_000_000_000 * 9970 / 10000 = 997_000_000
  //   out       = 30_000_000_000 * 997_000_000 / (20_000_000_000 + 997_000_000)
  //             = 29_910_000_000_000_000_000 / 20_997_000_000
  //             = 1_424_489_212 (truncated)
  const out = swapOut('20000000000', '30000000000', '1000000000', 30);
  assert.equal(out, 1424489212n);

  const byHand = (30000000000n * 997000000n) / 20997000000n;
  assert.equal(out, byHand);
});

test('swapOut truncates rather than rounding — the pool keeps the remainder', () => {
  // Chosen so the division has a non-zero remainder: any rounding-to-nearest
  // would land a unit higher.
  const rIn = 1000000n;
  const rOut = 3000001n;
  const aIn = 7n;
  const afterFee = (aIn * 10000n) / 10000n; // zero fee, so the fee is not what is being tested
  const exactNumerator = rOut * afterFee;
  const exactDenominator = rIn + afterFee;
  assert.notEqual(exactNumerator % exactDenominator, 0n, 'test is pointless without a remainder');

  const out = swapOut(rIn, rOut, aIn, 0);
  assert.equal(out, exactNumerator / exactDenominator);
  assert.ok(out * exactDenominator <= exactNumerator, 'never above the exact quotient');
});

test('the "simpler" subtraction form rounds the other way, and by a whole unit', () => {
  // The bug the keeper comment warns about, demonstrated. If these two ever
  // agree on every input, the demonstration has stopped being one.
  const cases = [
    ['1000000', '3000001', '7', 0],
    ['20000000000', '30000000000', '1000000000', 30],
    ['999999', '1000003', '13', 30],
  ];
  let differed = 0;
  for (const [rIn, rOut, aIn, fee] of cases) {
    const keeper = swapOut(rIn, rOut, aIn, fee);
    const bleeding = bleedingForm(rIn, rOut, aIn, fee);
    assert.ok(bleeding >= keeper, 'the rejected form never pays out less');
    if (bleeding > keeper) differed++;
  }
  assert.ok(differed > 0, 'at least one case must show the extra unit the pool would lose');
});

test('swap output never exceeds the curve: k does not fall', () => {
  // The invariant the rounding direction protects. Swept across a range of
  // awkward reserves and inputs; the product of the reserves after the trade
  // must be at least what it was before. The rejected form fails this.
  let checked = 0;
  for (const rIn of [1000n, 999999n, 20000000000n, 7n]) {
    for (const rOut of [1001n, 1000003n, 30000000000n, 11n]) {
      for (const aIn of [1n, 3n, 7n, 123n, 999999n]) {
        for (const fee of [0, 30, 500]) {
          const out = swapOut(rIn, rOut, aIn, fee);
          if (out >= rOut) continue; // would drain; the keeper refuses these too
          assert.ok(
            (rIn + aIn) * (rOut - out) >= rIn * rOut,
            `k fell: rIn=${rIn} rOut=${rOut} aIn=${aIn} fee=${fee} out=${out}`,
          );
          checked++;
        }
      }
    }
  }
  assert.ok(checked > 100, `swept ${checked} combinations`);
});

test('a swap too small to clear the fee returns nothing rather than something', () => {
  // 1 base unit at 30 bps: afterFee = 1 * 9970 / 10000 = 0. The keeper would
  // divide by (reserveIn + 0) and return 0. A console that quoted a fraction
  // here would be quoting a trade that cannot happen.
  assert.equal(swapOut('20000000000', '30000000000', '1', 30), 0n);
  const q = swapQuote({ reserveIn: '20000000000', reserveOut: '30000000000', amountIn: '1', swapFeeBps: 30 });
  assert.equal(q.amountOut, 0n);
  assert.equal(q.empty, true);
});

test('swapQuote: fee, minimum received and the reserves afterwards', () => {
  const q = swapQuote({
    reserveIn: '20000000000',
    reserveOut: '30000000000',
    amountIn: '1000000000',
    swapFeeBps: 30,
    slippageBps: 50,
  });
  assert.equal(q.amountOut, 1424489212n);
  // 0.3% of 1,000 YML = 3 YML, retained in the pool.
  assert.equal(q.feeAmount, 3000000n);
  // 0.5% below the quote, truncated: 1424489212 * 9950 / 10000
  assert.equal(q.minReceived, 1417366765n);
  assert.ok(q.minReceived < q.amountOut, 'the minimum is below the quote, always');
  // The full input joins the reserve — the fee stays in the pool, for the LPs.
  assert.equal(q.newReserveIn, 21000000000n);
  assert.equal(q.newReserveOut, 30000000000n - 1424489212n);
  // The product may not fall.
  assert.ok(q.newReserveIn * q.newReserveOut >= 20000000000n * 30000000000n);
});

test('minimum received is the promise; it is floored, never rounded up', () => {
  const q = swapQuote({ reserveIn: 1000000n, reserveOut: 1000000n, amountIn: 3n, swapFeeBps: 0, slippageBps: 1 });
  assert.ok(q.minReceived <= q.amountOut);
  assert.equal(q.minReceived, (q.amountOut * 9999n) / 10000n);
});

test('price impact rises with trade size and is reported before the fee separately', () => {
  const small = swapQuote({ reserveIn: '20000000000', reserveOut: '30000000000', amountIn: '1000000', swapFeeBps: 30 });
  const large = swapQuote({ reserveIn: '20000000000', reserveOut: '30000000000', amountIn: '10000000000', swapFeeBps: 30 });
  assert.ok(large.impactBps > small.impactBps, 'a bigger trade moves the price more');
  // The total cost always includes the fee, so it is never below the impact.
  assert.ok(small.costBps >= small.impactBps);
  assert.ok(large.costBps >= large.impactBps);
  // A trade of half the input reserve against this curve costs well over 10%.
  assert.ok(large.impactBps > IMPACT_THRESHOLDS.severe, `impact was ${large.impactBps} bps`);
});

test("a tiny trade's impact is not the pool's fee wearing a disguise", () => {
  // 1 YML into a 20,000 YML pool. The curve barely moves; the fee is 30 bps.
  // Reporting 30 bps of "price impact" here would be reporting the fee twice.
  const q = swapQuote({ reserveIn: '20000000000', reserveOut: '30000000000', amountIn: '1000000', swapFeeBps: 30 });
  assert.ok(q.impactBps <= 1, `impact was ${q.impactBps} bps`);
  assert.ok(q.costBps >= 29 && q.costBps <= 31, `cost was ${q.costBps} bps, should be about the 30 bps fee`);
});

test('impactVerdict warns before signing, at stated thresholds', () => {
  assert.equal(impactVerdict(0).level, 'ok');
  assert.equal(impactVerdict(IMPACT_THRESHOLDS.notable).level, 'warn');
  assert.equal(impactVerdict(IMPACT_THRESHOLDS.high).level, 'warn');
  assert.equal(impactVerdict(IMPACT_THRESHOLDS.severe).level, 'bad');
  assert.match(impactVerdict(IMPACT_THRESHOLDS.severe).title, /10%/);
});

test('ratio divides in BigInt at a fixed scale, not in floating point', () => {
  assert.equal(ratio(30000000000n, 20000000000n, 6), '1.5');
  assert.equal(ratio(1n, 3n, 6), '0.333333');
  assert.equal(ratio(1n, 0n), null);
  // 0.1 + 0.2 territory: this must be exact, and a float would not be.
  assert.equal(ratio(3n, 10n, 18), '0.3');
});

// ---------------------------------------------------------------------------
// Denominations
// ---------------------------------------------------------------------------

test('display conversion never rounds up', () => {
  assert.equal(toDisplayAmount('299999769', 6), '299.999769');
  assert.equal(toDisplayAmount('12500000', 6), '12.5');
  assert.equal(toDisplayAmount('0', 6), '0');
  assert.equal(toDisplayAmount('1', 6), '0.000001');
});

test('a zero-decimal currency truncates, and the exact figure survives beside it', () => {
  // The case from the brief: 299999769uxof next to a genuine 300 XOF.
  assert.equal(formatAmount('299999769', 'uxof'), '299 XOF');
  assert.equal(formatAmount('300000000', 'uxof'), '300 XOF');
  // Which is why the page must be able to say what was dropped.
  assert.equal(truncatedFrom('299999769', 'uxof'), '299.999769');
  assert.equal(truncatedFrom('300000000', 'uxof'), null);
  // Kenya's shilling is quoted to two places, so it truncates differently.
  assert.equal(formatAmount('299999769', 'ukes'), '299.99 KES');
  assert.equal(truncatedFrom('299999769', 'ukes'), '299.999769');
});

test('an unknown denom is shown as itself, never given an invented exponent', () => {
  assert.equal(resolveDenom('uwhat').exponent, 0);
  assert.equal(formatAmount('1000000', 'uwhat'), '1,000,000 uwhat');
  assert.equal(resolveDenom('amm/pool/1').symbol, 'Pool 1 shares');
  assert.match(resolveDenom('ibc/ABCDEF0123').symbol, /^IBC /);
});

test('grouping goes through BigInt, so a reserve above 2^53 is not silently altered', () => {
  assert.equal(groupDigits('9007199254740993'), '9,007,199,254,740,993');
  assert.equal(groupDigits('30000000000.5'), '30,000,000,000.5');
});

test('toBaseUnits refuses what it cannot parse rather than guessing', () => {
  assert.deepEqual(toBaseUnits('1.5', 6), { base: '1500000', truncated: false });
  assert.deepEqual(toBaseUnits('1,5', 6), { base: '1500000', truncated: false });
  assert.deepEqual(toBaseUnits('0.0000001', 6), { base: '0', truncated: true });
  assert.equal(toBaseUnits('', 6), null);
  assert.equal(toBaseUnits('1.2.3', 6), null);
  assert.equal(toBaseUnits('-1', 6), null);
  assert.equal(toBaseUnits('abc', 6), null);
  // The float bug this replaces: 0.07 * 1e6 is 70000.00000000001 in binary.
  assert.equal(toBaseUnits('0.07', 6).base, '70000');
  assert.equal(toBaseUnitsOf('0.07', 'uyml').base, '70000');
});

test('a dash is not a zero', () => {
  assert.notEqual(EMPTY_AMOUNT, '0');
});

test('a validator voting through its own key is not a delegated feeder', () => {
  // Both addresses from yamale-devnet-2. x/oracle returns the validator's own
  // account when nothing has been delegated, so this is the only thing that
  // separates "has a hot key" from "is signing every vote with the operator
  // key" — and the naive prefix comparison this replaces got both wrong.
  const valoper = 'ymlvaloper1cgguvt0hvdg2602flzan9shg0g56ruje62ug5j';
  const own = 'yml1cgguvt0hvdg2602flzan9shg0g56rujev5see4';
  assert.equal(isOwnAccount(valoper, own), true);
  assert.equal(isOwnAccount(valoper, 'yml1m9xhc6zy7fxfax9t5fnykh9k2e29faj7htmqms'), false);
  assert.equal(isOwnAccount(valoper, null), false);
  assert.equal(isOwnAccount('', ''), false);

  // The form that shipped and was wrong, kept as a reminder of what it claimed.
  assert.equal(own.startsWith(valoper.slice(0, 6)), false, 'no shared six-character prefix exists');
});

// ---------------------------------------------------------------------------
// x/oracle — freshness
// ---------------------------------------------------------------------------

test('freshness: a value too old to trust is not a value', () => {
  const maxAge = 900; // the devnet's max_rate_age_seconds
  const now = 1_000_000;

  assert.equal(freshness(now - 10, now, maxAge).state, 'fresh');
  assert.equal(freshness(now - 700, now, maxAge).state, 'expiring');
  // The boundary matches the keeper: at exactly maxAge it is still usable.
  assert.equal(freshness(now - 900, now, maxAge).state, 'expiring');
  assert.equal(freshness(now - 901, now, maxAge).state, 'expired');
});

test('freshness reports how long is left, not only how old it is', () => {
  const f = freshness(1_000_000 - 600, 1_000_000, 900);
  assert.equal(f.ageSeconds, 600);
  assert.equal(f.remaining, 300, 'five minutes left, which is the number a reader needs');
  assert.equal(f.fraction, 600 / 900);
  assert.ok(f.fraction < EXPIRING_FRACTION, 'two thirds through is not yet expiring');
  assert.equal(f.state, 'fresh');

  // Three quarters through is where the console starts saying so: at a 900s
  // window that is 675 seconds old, with under four minutes of life left.
  const nearly = freshness(1_000_000 - 675, 1_000_000, 900);
  assert.equal(nearly.fraction, EXPIRING_FRACTION);
  assert.equal(nearly.state, 'expiring');
  assert.equal(nearly.remaining, 225);
});

test('freshness distinguishes "no feed" from "feed stopped"', () => {
  assert.equal(freshness(0, 1_000_000, 900).state, 'none');
  assert.equal(freshness('0', 1_000_000, 900).ageSeconds, null);
});

test('a rate stamped in the future is a clock disagreement, not a fresh rate', () => {
  const f = freshness(1_000_100, 1_000_000, 900);
  assert.equal(f.clockSkew, true);
  assert.ok(f.ageSeconds < 0);
  assert.equal(f.fraction, 0, 'a negative age must not render as a negative bar');
});

test('span reads as a duration a person understands', () => {
  assert.equal(span(1), '1 second');
  assert.equal(span(45), '45 seconds');
  assert.equal(span(600), '10 minutes');
  assert.equal(span(7200), '2 hours');
  assert.equal(span(86400 * 3), '3 days');
  assert.equal(span(86400 * 90), '3 months');
});

test('agreement separates a bare quorum from unanimity', () => {
  assert.equal(agreement(10000, 5000).level, 'ok');
  assert.match(agreement(10000, 5000).text, /unanimous/);
  assert.equal(agreement(5100, 5000).level, 'warn');
  assert.match(agreement(5100, 5000).text, /bare quorum/);
  assert.equal(agreement(0, 5000).level, 'mute');
});

test('reporting: missing every window is "never reported", not "unreliable"', () => {
  // Both devnet validators at height 119299, read from the chain.
  assert.equal(reporting(9735, 9735).level, 'bad');
  assert.match(reporting(9735, 9735).text, /never reported/);
  assert.equal(reporting(9941, 9941).text, 'has never reported a price');

  assert.equal(reporting(0, 1000).level, 'ok');
  assert.equal(reporting(100, 1000).level, 'warn');
  assert.equal(reporting(600, 1000).level, 'bad');
  assert.equal(reporting(0, 0).level, 'mute');
});

// ---------------------------------------------------------------------------
// x/stablecoin — registered is not approved
// ---------------------------------------------------------------------------

test('registration is not approval, and the console says which one is missing', () => {
  const pending = mintability('unew', { denom: 'unew', status: 'pending' }, null);
  assert.equal(pending.state, 'pending');
  assert.equal(pending.canMint, false);
  assert.match(pending.blocker, /1104/);
  assert.match(pending.blocker, /governance proposal/);
  assert.match(pending.blocker, /code 0/, 'must say that registration itself succeeded');

  const approved = mintability('uxof', { denom: 'uxof', status: 'approved' }, { denom: 'uxof', issuer: 'yml1abc' });
  assert.equal(approved.state, 'approved');
  assert.equal(approved.canMint, true);
  assert.equal(approved.issuer, 'yml1abc');
  assert.equal(approved.blocker, null);

  const none = mintability('unothing', null, null);
  assert.equal(none.state, 'unregistered');
  assert.equal(none.canMint, false);

  const refused = mintability('ubad', { denom: 'ubad', status: 'rejected' }, null);
  assert.equal(refused.state, 'rejected');
  assert.equal(refused.canMint, false);
});

test('an approved issuer record outranks a stale application status', () => {
  // Belt and braces: if the two ever disagree, the ApprovedIssuer map is the
  // one the keeper checks when it decides whether a mint succeeds.
  const m = mintability('uxof', { denom: 'uxof', status: 'pending' }, { denom: 'uxof', issuer: 'yml1abc' });
  assert.equal(m.canMint, true);
});

// ---------------------------------------------------------------------------
// Signing
// ---------------------------------------------------------------------------

test('type URLs carry the blockchain. prefix, not yamale.blockchain.', () => {
  for (const url of Object.values(MESSAGES)) {
    assert.match(url, /^\/blockchain\.(amm|oracle|stablecoin)\.v1\.Msg/, url);
    assert.doesNotMatch(url, /yamale\.blockchain/, url);
  }
});

test('every message has a signer read off the proto and a mode justified by it', () => {
  for (const [url, rule] of Object.entries(SIGNERS)) {
    assert.ok(['browser', 'proposal', 'command'].includes(rule.mode), `${url} has mode ${rule.mode}`);
    assert.ok(rule.why.length > 20, `${url} must say why`);
  }
  // The authority-gated pair can only ever be a proposal: no key exists for the
  // x/gov module account, so a button for these would be a lie.
  assert.equal(SIGNERS[MESSAGES.approveIssuer].signer, 'authority');
  assert.equal(SIGNERS[MESSAGES.approveIssuer].mode, 'proposal');
  assert.equal(SIGNERS[MESSAGES.approveAppraiser].mode, 'proposal');

  // A trader's swap is one person and one key: the browser may sign it.
  assert.equal(SIGNERS[MESSAGES.swap].signer, 'sender');
  assert.equal(SIGNERS[MESSAGES.swap].mode, 'browser');

  // A feeder is a plain key and still not a browser one; an issuer never is.
  assert.equal(SIGNERS[MESSAGES.submitRates].signer, 'feeder');
  assert.equal(SIGNERS[MESSAGES.submitRates].mode, 'command');
  assert.equal(SIGNERS[MESSAGES.mintCoin].signer, 'issuer');
  assert.equal(SIGNERS[MESSAGES.mintCoin].mode, 'command');
});

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

test('the swap command matches autocli positional order and carries a real minimum', () => {
  const cmd = swapCommand({
    poolId: 1,
    inDenom: 'uyml',
    inAmount: '1000000000',
    outDenom: 'ungn',
    minOut: '1417366765',
    from: 'trader',
  });
  assert.match(cmd, /blockchaind tx amm swap 1 uyml 1000000000 ungn 1417366765/);
  assert.match(cmd, /--chain-id yamale-devnet-2/);
  assert.doesNotMatch(cmd, /ngn 0 /, 'a minimum of zero accepts any price the block lands on');
});

test('shell quoting leaves ordinary denoms alone and protects the rest', () => {
  assert.equal(sh('uyml'), 'uyml');
  assert.equal(sh('ibc/ABC123'), 'ibc/ABC123');
  assert.equal(sh("a'b"), `'a'\\''b'`);
  assert.equal(sh('two words'), `'two words'`);
});

test('submit-rates composes one --rates flag per denom, never a JSON array', () => {
  const cmd = submitRatesCommand({
    validator: 'ymlvaloper1abc',
    rates: [
      { denom: 'uusd', rate: '1.00' },
      { denom: 'ueur', rate: '1.15' },
    ],
    from: 'feeder',
  });
  assert.equal((cmd.match(/--rates/g) || []).length, 2);
  assert.doesNotMatch(cmd, /--rates '\[/, 'an array fails with "unexpected token ["');
  assert.match(cmd, /blockchaind tx oracle submit-rates ymlvaloper1abc/);
});

test('the approve-issuer proposal names the gov account as authority, not the proposer', () => {
  const { doc, submit } = approveIssuerProposal({
    denom: 'unew',
    authority: 'yml10d07y265gmmuvt4z0w9aw880jnsr700jqvcndf',
    from: 'proposer',
  });
  const parsed = JSON.parse(doc);
  assert.equal(parsed.messages[0]['@type'], '/blockchain.stablecoin.v1.MsgApproveIssuer');
  assert.equal(parsed.messages[0].authority, 'yml10d07y265gmmuvt4z0w9aw880jnsr700jqvcndf');
  assert.equal(parsed.messages[0].approve, true);
  assert.notEqual(parsed.messages[0].authority, 'proposer');
  assert.match(parsed.summary, /1104/);
  assert.match(submit, /gov submit-proposal/);
});

test('the chain constants are in one place', () => {
  assert.equal(CHAIN.id, 'yamale-devnet-2');
  assert.equal(CHAIN.bin, 'blockchaind');
});
