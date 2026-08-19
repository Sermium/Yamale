import assert from 'node:assert/strict';
import { test } from 'node:test';

import { CHAIN_MESSAGE_TYPES, chainRegistry } from './registry.ts';
import { BasicAllowance } from './generated/cosmos/feegrant/v1beta1/feegrant.ts';
import {
  claimLock,
  grantFeeAllowance,
  openCase,
  payment,
  revokeFeeAllowance,
  submitRates,
  swap,
  treasurySpend,
  voteCase,
} from './signing.ts';

// These tests encode and decode for real rather than checking that a type URL
// is present in a list. The failure they exist to catch is not a missing
// registration — that shows up immediately as "Unregistered type url" — it is a
// registered message whose *fields* do not survive the round trip. A field name
// that differs from the generated type is dropped silently by protobuf
// encoding, so a payment would reach the chain with an empty end-to-end id and
// be rejected for a reason that reads as unrelated to the mistake.

const registry = chainRegistry();

/** Encode as the signer would, then decode the bytes back. */
function roundTrip(msg: { typeUrl: string; value: unknown }): any {
  const bytes = registry.encode(msg as never);
  return registry.decode({ typeUrl: msg.typeUrl, value: bytes });
}

test('a payment survives encoding with every field intact', () => {
  const decoded = roundTrip(
    payment({
      debtor: 'yml1debtor',
      endToEndId: 'INV-2026-0042',
      instructingParticipant: 'yml1bankone',
      instructedParticipant: 'yml1banktwo',
      creditor: 'yml1creditor',
      denom: 'ueur',
      amount: '125000',
      purposeCode: 'SUPP',
      remittanceInformation: 'Invoice 42',
    }),
  );

  assert.equal(decoded.debtor, 'yml1debtor');
  assert.equal(decoded.endToEndId, 'INV-2026-0042');
  assert.equal(decoded.instructingParticipant, 'yml1bankone');
  assert.equal(decoded.instructedParticipant, 'yml1banktwo');
  assert.equal(decoded.creditor, 'yml1creditor');
  assert.equal(decoded.denom, 'ueur');
  assert.equal(decoded.amount, '125000');
  assert.equal(decoded.purposeCode, 'SUPP');
  assert.equal(decoded.remittanceInformation, 'Invoice 42');
});

// The slippage floor is the only thing protecting a trader from a pool that
// moved between quote and execution. A swap that arrived with min_amount_out
// zeroed would still execute — at any price.
test('a swap carries its slippage floor', () => {
  const decoded = roundTrip(
    swap({
      sender: 'yml1trader',
      poolId: '3',
      tokenInDenom: 'uyml',
      tokenInAmount: '1000000',
      tokenOutDenom: 'ueur',
      minAmountOut: '89999',
    }),
  );

  assert.equal(decoded.poolId, '3');
  assert.equal(decoded.tokenInAmount, '1000000');
  assert.equal(decoded.minAmountOut, '89999');
});

test('a treasury spend carries its coins and recipient', () => {
  const decoded = roundTrip(
    treasurySpend({
      spender: 'yml1spender',
      treasuryId: '7',
      recipient: 'yml1supplier',
      amount: [{ denom: 'uyml', amount: '250000' }],
      memo: 'March hosting',
    }),
  );

  assert.equal(decoded.treasuryId, '7');
  assert.equal(decoded.recipient, 'yml1supplier');
  assert.deepEqual(decoded.amount, [{ denom: 'uyml', amount: '250000' }]);
  assert.equal(decoded.memo, 'March hosting');
});

test('a lock claim names the beneficiary and the lock', () => {
  const decoded = roundTrip(claimLock('yml1beneficiary', '2'));
  assert.equal(decoded.beneficiary, 'yml1beneficiary');
  assert.equal(decoded.lockId, '2');
});

// Repeated fields are the ones most likely to be lost to a name mismatch,
// because an empty list encodes to nothing at all rather than to an error.
test('a rate submission keeps every rate in the round', () => {
  const decoded = roundTrip(
    submitRates('yml1feeder', 'ymlvaloper1me', [
      { denom: 'ueur', rate: '1.080000000000000000' },
      { denom: 'uchf', rate: '1.140000000000000000' },
    ]),
  );

  assert.equal(decoded.feeder, 'yml1feeder');
  assert.equal(decoded.validator, 'ymlvaloper1me');
  assert.equal(decoded.rates.length, 2);
  assert.equal(decoded.rates[0].denom, 'ueur');
  assert.equal(decoded.rates[0].rate, '1.080000000000000000');
  assert.equal(decoded.rates[1].denom, 'uchf');
});

// The registry has to keep the standard messages as well: a signer built only
// from this chain's types could no longer send a transfer or cast a vote.
test('the standard Cosmos messages are still registered', () => {
  for (const typeUrl of [
    '/cosmos.bank.v1beta1.MsgSend',
    '/cosmos.staking.v1beta1.MsgDelegate',
    '/cosmos.gov.v1.MsgVote',
  ]) {
    assert.doesNotThrow(() => registry.lookupType(typeUrl), `${typeUrl} is not registered`);
  }
});

test('every message this chain adds is registered', () => {
  for (const [typeUrl] of CHAIN_MESSAGE_TYPES) {
    assert.ok(registry.lookupType(typeUrl), `${typeUrl} is not registered`);
  }
});

// A type URL is a string, and a string with a typo in it registers just as
// happily as a correct one. These are compared against the package names in the
// .proto files rather than against themselves.
test('type urls match the proto packages they come from', () => {
  for (const [typeUrl] of CHAIN_MESSAGE_TYPES) {
    // Fee grants are Cosmos messages carried here because CosmJS's default
    // registry omits them; they are not this chain's own.
    if (typeUrl.startsWith('/cosmos.feegrant.')) continue;
    assert.match(
      typeUrl,
      /^\/blockchain\.(alias|amm|builderfee|emission|enforcement|oracle|paymsg|stablecoin|treasury|validatorgov)\.v1\.Msg[A-Z]/,
      `${typeUrl} does not look like one of this chain's messages`,
    );
  }
});

// The enforcement messages move somebody's assets on a validator vote, so the
// fields that decide who and how much have to survive encoding exactly.
test('an enforcement case carries its target, action and evidence', () => {
  const decoded = roundTrip(
    openCase({
      opener: 'yml1me',
      target: 'yml1scammer',
      action: 'seize',
      reason: 'drained a pool',
      evidenceUri: 'https://example.org/case',
      evidenceHash: '9f2c0e1b',
    }),
  );

  assert.equal(decoded.target, 'yml1scammer');
  assert.equal(decoded.action, 2, 'seize, not freeze — the difference is whether assets move');
  assert.equal(decoded.reason, 'drained a pool');
  assert.equal(decoded.evidenceHash, '9f2c0e1b');
});

test('a case vote carries the option the validator chose', () => {
  assert.equal(roundTrip(voteCase('yml1me', '3', 'yes')).option, 1);
  assert.equal(roundTrip(voteCase('yml1me', '3', 'no')).option, 2);
  assert.equal(roundTrip(voteCase('yml1me', '3', 'abstain')).option, 3);
  assert.equal(roundTrip(voteCase('yml1me', '3', 'yes')).caseId, '3');
});

// A fee grant is what lets a customer holding only naira move it at all, and
// the allowance is a nested Any — the field most likely to be silently dropped
// by a hand-written encoder, which would produce an unlimited grant instead of
// a capped one.
test('a fee allowance survives encoding with its limit intact', () => {
  const expires = new Date('2026-12-31T23:59:59.000Z');
  const decoded = roundTrip(
    grantFeeAllowance({
      granter: 'yml1bank',
      grantee: 'yml1customer',
      spendLimit: [{ denom: 'uyml', amount: '1000000' }],
      expiresAt: expires,
    }),
  );

  assert.equal(decoded.granter, 'yml1bank');
  assert.equal(decoded.grantee, 'yml1customer');
  assert.equal(decoded.allowance.typeUrl, '/cosmos.feegrant.v1beta1.BasicAllowance');

  // The limit is inside the Any, so it has to be unpacked to be checked. An
  // allowance that arrived with an empty spend_limit would be *unlimited*,
  // which is the opposite of what was asked for and would not look wrong.
  const allowance = BasicAllowance.decode(decoded.allowance.value);
  assert.deepEqual(allowance.spendLimit, [{ denom: 'uyml', amount: '1000000' }]);
  assert.equal(allowance.expiration?.getTime(), expires.getTime());
});

test('revoking a fee allowance names both parties', () => {
  const decoded = roundTrip(revokeFeeAllowance('yml1bank', 'yml1customer'));
  assert.equal(decoded.granter, 'yml1bank');
  assert.equal(decoded.grantee, 'yml1customer');
});
