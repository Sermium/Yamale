import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  DAY,
  actionsFor,
  bpsToPercent,
  disputeBond,
  dilutionProtected,
  isOverLand,
  isRealId,
  landGate,
  issuancePermitted,
  protectionOf,
  redeemPayout,
  saleState,
  shareholding,
  statusKey,
  statusTone,
  type Collection,
  type LandAuthorisation,
  type SaleReport,
} from './vehicle.ts';

/* ------------------------------------------------------------- fixtures */

function collection(over: Partial<Collection> = {}): Collection {
  return {
    id: 'ke-farmland',
    authority: 'yml1office',
    verification: 'VERIFY_ATTESTORS',
    attestationThreshold: 3,
    challengeWindowSeconds: 7 * DAY,
    disputeBondBps: 500,
    ...over,
  };
}

function sale(over: Partial<SaleReport> = {}): SaleReport {
  return {
    assetId: '3',
    price: { denom: 'ukes', amount: '82000000' },
    reporter: 'yml1sponsor',
    reportedAt: new Date('2026-01-01T00:00:00Z'),
    claimableAt: new Date('2026-01-08T00:00:00Z'),
    attestors: [],
    disputed: false,
    ...over,
  };
}

/* ----------------------------------------------------------------- ids */

test('a zero id is the absent case, never a record', () => {
  // Both sequences this app reads skip zero deliberately, so on this chain a
  // zero can only be an unset proto field.
  assert.equal(isRealId('0'), false);
  assert.equal(isRealId(0), false);
  assert.equal(isRealId(''), false);
  assert.equal(isRealId(undefined), false);
  assert.equal(isRealId(null), false);

  assert.equal(isRealId('1'), true);
  assert.equal(isRealId(1), true);
  // uint64 ids exceed what a double holds exactly; the check must not go
  // through Number.
  assert.equal(isRealId('18446744073709551615'), true);
});

test('a non-numeric id is not a record either', () => {
  assert.equal(isRealId('abc'), false);
  assert.equal(isRealId('-1'), false);
  assert.equal(isRealId('1.5'), false);
});

test('only a vehicle with a real parcel id is over land', () => {
  assert.equal(isOverLand({ parcelId: '0' }), false);
  assert.equal(isOverLand({ parcelId: '7' }), true);
});

/* -------------------------------------------------------- shareholding */

test('basis points convert to a percentage, not to a fraction', () => {
  assert.equal(bpsToPercent(4000), 40);
  assert.equal(bpsToPercent(10_000), 100);
  assert.equal(bpsToPercent(1), 0.01);
});

test('a holding is two percentages, and the smaller one is what is owned', () => {
  // Half the tokens in a vehicle whose tokens carry 40% of the asset.
  const s = shareholding('500000', '1000000', 4000);
  assert.equal(s.ofSupply, 50);
  assert.equal(s.ofAsset, 20);
  assert.equal(s.empty, false);
});

test('a holding of everything, in a vehicle that sold everything', () => {
  const s = shareholding('1000000', '1000000', 10_000);
  assert.equal(s.ofSupply, 100);
  assert.equal(s.ofAsset, 100);
});

test('one token in a billion does not collapse to zero', () => {
  const s = shareholding('1', '1000000000', 10_000);
  assert.ok(s.ofSupply !== null && s.ofSupply > 0,
    'a real holding must never render as 0% — it is somebody\'s money');
  assert.ok(s.ofSupply! < 0.0000002);
});

test('a supply beyond 2^53 is still divided exactly', () => {
  // Number() on these would lose the last digits and the ratio would drift.
  const s = shareholding('9007199254740993', '18014398509481986', 10_000);
  assert.ok(s.ofSupply !== null);
  assert.ok(Math.abs(s.ofSupply! - 50) < 1e-6, `got ${s.ofSupply}`);
});

test('holding nothing is empty, and says so as well as showing zero', () => {
  const s = shareholding('0', '1000000', 4000);
  assert.equal(s.ofSupply, 0);
  assert.equal(s.ofAsset, 0);
  assert.equal(s.empty, true);
});

test('no supply yet means no percentage, not a percentage of zero', () => {
  const s = shareholding('0', '0', 4000);
  assert.equal(s.ofSupply, null);
  assert.equal(s.ofAsset, null);
});

test('rubbish in gives no percentage rather than NaN on screen', () => {
  const s = shareholding('not-a-number', '1000000', 4000);
  assert.equal(s.ofSupply, null);
  assert.equal(s.empty, true);
});

/* --------------------------------------------------------- protections */

test('three attestors and a week is a strong collection', () => {
  const p = protectionOf(collection());
  assert.equal(p.level, 'strong');
  assert.ok(p.findings.some((f) => f.key === 'rwa.protect.attestors'));
});

test('a threshold of zero with a window of zero is graded as no protection', () => {
  // The case the brief calls out: it should look as dangerous as it is.
  const p = protectionOf(collection({
    verification: 'VERIFICATION_UNSPECIFIED',
    attestationThreshold: 0,
    challengeWindowSeconds: 0,
  }));
  assert.equal(p.level, 'none');
  assert.equal(p.findings[0].key, 'rwa.protect.unchecked',
    'the headline finding must lead, not be buried below the detail');
  assert.equal(p.findings[0].tone, 'bad');
});

test('a long window does not compensate for no verification', () => {
  // A score would let these cancel out. They are not substitutes.
  const p = protectionOf(collection({
    verification: 'VERIFICATION_UNSPECIFIED',
    attestationThreshold: 0,
    challengeWindowSeconds: 90 * DAY,
  }));
  assert.equal(p.level, 'none');
});

test('verification without a window is no protection either', () => {
  const p = protectionOf(collection({ challengeWindowSeconds: 0 }));
  assert.equal(p.level, 'none');
  assert.ok(p.findings.some((f) => f.key === 'rwa.protect.noWindow' && f.tone === 'bad'));
});

test('a single attestor is not a threshold', () => {
  const p = protectionOf(collection({ attestationThreshold: 1 }));
  assert.equal(p.level, 'none');
  assert.ok(p.findings.some((f) => f.key === 'rwa.protect.thresholdTooLow'));
});

test('two attestors and a week is standard, not strong', () => {
  const p = protectionOf(collection({ attestationThreshold: 2 }));
  assert.equal(p.level, 'standard');
});

test('a window under a day is weak even with good verification', () => {
  const p = protectionOf(collection({ challengeWindowSeconds: 3600 }));
  assert.equal(p.level, 'weak');
  assert.ok(p.findings.some((f) => f.key === 'rwa.protect.shortWindow' && f.tone === 'warn'));
});

test('a fixed redemption schedule needs no attestors', () => {
  const p = protectionOf(collection({
    verification: 'VERIFY_SCHEDULE',
    attestationThreshold: 0,
  }));
  assert.equal(p.level, 'strong');
  assert.ok(p.findings.some((f) => f.key === 'rwa.protect.schedule'));
  assert.ok(!p.findings.some((f) => f.key === 'rwa.protect.thresholdTooLow'),
    'the threshold field is meaningless under VERIFY_SCHEDULE and must not be graded');
});

test('a free challenge is flagged, but does not by itself lower the grade', () => {
  const p = protectionOf(collection({ disputeBondBps: 0 }));
  assert.equal(p.level, 'strong');
  assert.ok(p.findings.some((f) => f.key === 'rwa.protect.freeChallenge' && f.tone === 'warn'));
});

test('dilution protection is only claimed once shares actually exist', () => {
  assert.equal(dilutionProtected({ fractionDenom: '', status: 'STATUS_HELD' }), false);
  assert.equal(dilutionProtected({ fractionDenom: 'frac/3/KEFARM', status: 'STATUS_ACTIVE' }), true);
});

/* ----------------------------------------------------------- sale clock */

const REPORTED = { status: 'STATUS_REPORTED' } as const;

test('inside the window, a sale is disputable and not redeemable', () => {
  const s = saleState(REPORTED, sale(), collection(), new Date('2026-01-04T00:00:00Z'));
  assert.equal(s.phase, 'in-window');
  assert.equal(s.canDispute, true);
  assert.equal(s.remainingSeconds, 4 * DAY);
  assert.equal(s.totalSeconds, 7 * DAY);
});

test('the window is inclusive of its final instant, exactly as the keeper is', () => {
  // DisputeSale refuses only once block time is *after* claimable_at. Closing
  // it a second early would tell a holder they had missed a deadline they had
  // not missed.
  const at = new Date('2026-01-08T00:00:00Z');
  assert.equal(saleState(REPORTED, sale(), collection(), at).canDispute, true);

  const after = new Date('2026-01-08T00:00:01Z');
  assert.equal(saleState(REPORTED, sale(), collection(), after).canDispute, false);
});

test('past the window with attestations missing, the sale waits', () => {
  const s = saleState(REPORTED, sale({ attestors: ['a', 'b'] }), collection(),
    new Date('2026-01-09T00:00:00Z'));
  assert.equal(s.phase, 'awaiting-attestations');
  assert.equal(s.attestations, 2);
  assert.equal(s.needed, 3);
  assert.equal(s.remainingSeconds, 0);
});

test('past the window with attestations in, nothing stands in the way', () => {
  const s = saleState(REPORTED, sale({ attestors: ['a', 'b', 'c'] }), collection(),
    new Date('2026-01-09T00:00:00Z'));
  assert.equal(s.phase, 'clear');
});

test('a mode with no attestors needs none, and is not shown a requirement', () => {
  const s = saleState(REPORTED, sale(), collection({
    verification: 'VERIFY_VALUER',
    attestationThreshold: 3,
  }), new Date('2026-01-09T00:00:00Z'));
  assert.equal(s.needed, 0, 'the threshold field is meaningless outside VERIFY_ATTESTORS');
  assert.equal(s.phase, 'clear');
});

test('a dispute stops everything, including a second dispute', () => {
  const s = saleState({ status: 'STATUS_DISPUTED' }, sale({ disputed: true }), collection(),
    new Date('2026-01-04T00:00:00Z'));
  assert.equal(s.phase, 'disputed');
  assert.equal(s.canDispute, false);
});

test('a realised vehicle has no window left to run', () => {
  const s = saleState({ status: 'STATUS_REALISED' }, sale(), collection(),
    new Date('2026-01-09T00:00:00Z'));
  assert.equal(s.phase, 'realised');
  assert.equal(s.canDispute, false);
});

test('with no sale reported there is no clock and no bond', () => {
  const s = saleState({ status: 'STATUS_ACTIVE' }, null, collection(), new Date());
  assert.equal(s.phase, 'none');
  assert.equal(s.bond, '0');
  assert.equal(s.canDispute, false);
});

/* ----------------------------------------------------------------- bond */

test('the bond is the keeper\'s arithmetic, truncated the same way', () => {
  // price.MulRaw(bps).QuoRaw(10_000)
  assert.equal(disputeBond('82000000', 500), '4100000');
  assert.equal(disputeBond('1', 500), '0', 'integer division truncates toward zero');
  assert.equal(disputeBond('19999', 500), '999', '999.95 truncates to 999, never to 1000');
});

test('a bond on a price beyond 2^53 is still exact', () => {
  assert.equal(disputeBond('9007199254740993000', 500), '450359962737049650');
});

test('no bond where the collection charges none, or the input is rubbish', () => {
  assert.equal(disputeBond('82000000', 0), '0');
  assert.equal(disputeBond('nonsense', 500), '0');
  assert.equal(disputeBond('0', 500), '0');
});

/* --------------------------------------------------------------- redeem */

test('a redemption pays the surrendered fraction of what is owed', () => {
  // pos.Accrued.Mul(amount).Quo(balance)
  assert.equal(redeemPayout('1000', '500', '1000'), '500');
  assert.equal(redeemPayout('1000', '1000', '1000'), '1000');
  assert.equal(redeemPayout('999', '1', '1000'), '0', 'truncation favours the vault');
});

test('a redemption the chain would refuse returns no quote at all', () => {
  // Distinguishable from a payout of zero: they need different sentences.
  assert.equal(redeemPayout('1000', '1001', '1000'), null, 'more than held');
  assert.equal(redeemPayout('1000', '100', '0'), null, 'nothing held');
  assert.equal(redeemPayout('1000', '0', '1000'), null, 'nothing surrendered');
});

test('redeeming while owed nothing still burns, and quotes zero rather than null', () => {
  assert.equal(redeemPayout('0', '500', '1000'), '0');
});

/* --------------------------------------------------------- the land gate */

function auth(over: Partial<LandAuthorisation> = {}): LandAuthorisation {
  return {
    parcelId: '7',
    right: 'exploitation',
    maxShareBps: 4000,
    expiresAt: 1800000000,
    grantedBy: 'yml1office',
    grantedAt: 1700000000,
    withdrawn: false,
    withdrawnAt: 0,
    ...over,
  };
}

test('a vehicle that names no parcel is not governed by the registry', () => {
  assert.deepEqual(landGate('0', 'absent', 1750000000), { kind: 'not-land' });
  assert.equal(issuancePermitted({ kind: 'not-land' }), true);
});

test('a live permission is live because x/land says so, not because we recomputed it', () => {
  const g = landGate('7', { authorisation: auth(), live: true }, 1750000000);
  assert.equal(g.kind, 'live');
  assert.equal(issuancePermitted(g), true);
});

test('a withdrawn permission names withdrawal as the reason', () => {
  const g = landGate('7', { authorisation: auth({ withdrawn: true }), live: false }, 1750000000);
  assert.equal(g.kind, 'withdrawn');
  assert.equal(issuancePermitted(g), false);
});

test('an expired permission names the expiry', () => {
  const g = landGate('7', { authorisation: auth(), live: false }, 1900000000);
  assert.equal(g.kind, 'expired');
  assert.equal(issuancePermitted(g), false);
});

test('not live, not withdrawn, not expired means the parcel itself forbids it', () => {
  const g = landGate('7', { authorisation: auth(), live: false }, 1750000000);
  assert.equal(g.kind, 'restricted');
  assert.equal(issuancePermitted(g), false);
});

test('withdrawal outranks expiry when a record carries both', () => {
  const g = landGate('7', { authorisation: auth({ withdrawn: true }), live: false }, 1900000000);
  assert.equal(g.kind, 'withdrawn',
    'the office\'s decision is the finding; the clock running out afterwards is not');
});

test('a permission that was never granted is not the same as one we could not read', () => {
  assert.equal(landGate('7', 'absent', 1750000000).kind, 'absent');
  assert.equal(landGate('7', 'unreachable', 1750000000).kind, 'unreachable');
  assert.equal(issuancePermitted({ kind: 'unreachable' }), false,
    'an unreadable permission must never read as permission');
});

/* -------------------------------------------------------------- statuses */

test('every status has a sentence and a tone', () => {
  const all = [
    'STATUS_UNSPECIFIED', 'STATUS_HELD', 'STATUS_ACTIVE', 'STATUS_REPORTED',
    'STATUS_REALISED', 'STATUS_CLOSED', 'STATUS_DISPUTED',
  ] as const;
  for (const s of all) {
    assert.ok(statusKey(s).startsWith('rwa.status.'), s);
    assert.ok(['ok', 'warn', 'bad', 'mute'].includes(statusTone(s)), s);
  }
});

/* --------------------------------------------------------------- actions */

const ACTIVE = { status: 'STATUS_ACTIVE', fractionDenom: 'frac/3/KEFARM' } as const;
const REALISED = { status: 'STATUS_REALISED', fractionDenom: 'frac/3/KEFARM' } as const;

test('nothing is offered to somebody who has not connected an account', () => {
  const s = saleState(REPORTED, sale(), collection(), new Date('2026-01-04T00:00:00Z'));
  const a = actionsFor(ACTIVE, s, '1000', '500', false);
  assert.equal(a.claim.enabled, false);
  assert.equal(a.redeem.enabled, false);
  assert.equal(a.dispute.enabled, false);
  assert.equal(a.claim.whyKey, 'rwa.act.needAccount');
});

test('claim is offered only when something is actually owed', () => {
  const s = saleState({ status: 'STATUS_ACTIVE' }, null, collection(), new Date());
  assert.equal(actionsFor(ACTIVE, s, '1000', '500', true).claim.enabled, true);
  assert.equal(actionsFor(ACTIVE, s, '1000', '0', true).claim.whyKey, 'rwa.act.nothingOwed');
  assert.equal(actionsFor(ACTIVE, s, '0', '0', true).claim.whyKey, 'rwa.act.noShares');
});

test('redeem is refused until the price is final, and says which refusal it is', () => {
  const s = saleState(REPORTED, sale(), collection(), new Date('2026-01-04T00:00:00Z'));
  assert.equal(actionsFor(ACTIVE, s, '1000', '0', true).redeem.whyKey, 'rwa.act.notRealised');

  const done = saleState(REALISED, sale(), collection(), new Date('2026-02-01T00:00:00Z'));
  assert.equal(actionsFor(REALISED, done, '1000', '0', true).redeem.enabled, true);
  assert.equal(actionsFor(REALISED, done, '0', '0', true).redeem.whyKey, 'rwa.act.noShares');
});

test('dispute does not require holding shares', () => {
  // The party best placed to know a reported price is a lie is often the buyer
  // who paid the real one, and they hold nothing. The keeper does not require
  // it either.
  const s = saleState(REPORTED, sale(), collection(), new Date('2026-01-04T00:00:00Z'));
  assert.equal(actionsFor(ACTIVE, s, '0', '0', true).dispute.enabled, true);
});

test('a closed window and an existing dispute are different refusals', () => {
  const shut = saleState(REPORTED, sale(), collection(), new Date('2026-01-09T00:00:00Z'));
  assert.equal(actionsFor(ACTIVE, shut, '1000', '0', true).dispute.whyKey, 'rwa.act.windowClosed');

  const already = saleState({ status: 'STATUS_DISPUTED' }, sale({ disputed: true }),
    collection(), new Date('2026-01-04T00:00:00Z'));
  assert.equal(actionsFor(ACTIVE, already, '1000', '0', true).dispute.whyKey,
    'rwa.act.alreadyDisputed');

  const none = saleState({ status: 'STATUS_ACTIVE' }, null, collection(), new Date());
  assert.equal(actionsFor(ACTIVE, none, '1000', '0', true).dispute.whyKey, 'rwa.act.noSale');
});
