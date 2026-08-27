import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { TxBody } from 'cosmjs-types/cosmos/tx/v1beta1/tx.js';

import { arriving, consequencesOf, leaving, locking, totalCoins } from './consequence.ts';
import { chainRegistry } from './registry.ts';
import { send, treasurySpend } from './signing.ts';
import { summariseSigningRequest } from './signrequest.ts';

/**
 * The ledger a signer reads, exercised through real bytes.
 *
 * Every case here goes message → protobuf → decode → consequences, because the
 * property being tested is not "the classifier returns what I typed" — it is
 * that a transaction assembled by this package is legible as *what it does to
 * me* by the screen that has to ask.
 */

const ALICE = 'yml1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq';
const BOB = 'yml1zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz';

function ledger(messages: { typeUrl: string; value: unknown }[], signer: string) {
  const registry = chainRegistry();
  const bytes = TxBody.encode(
    TxBody.fromPartial({ messages: messages.map((m) => registry.encodeAsAny(m as never)) }),
  ).finish();
  return consequencesOf(summariseSigningRequest(bytes), signer);
}

test('a transfer out of the signing account is money leaving it', () => {
  const c = ledger([send(ALICE, BOB, [{ denom: 'uyml', amount: '250000000' }])], ALICE);

  assert.deepEqual(leaving(c), [{ denom: 'uyml', amount: '250000000' }]);
  assert.deepEqual(arriving(c), []);
  assert.equal(c.reversibility, 'irreversible');
  assert.equal(c.incomplete, false);
});

test('the same transfer, read by the recipient, is money arriving', () => {
  // The bug this exists to prevent: classifying by message type rather than by
  // whose address is on which side, so a screen tells the payee that 250 YML is
  // about to leave them.
  const c = ledger([send(ALICE, BOB, [{ denom: 'uyml', amount: '250000000' }])], BOB);

  assert.deepEqual(arriving(c), [{ denom: 'uyml', amount: '250000000' }]);
  assert.deepEqual(leaving(c), []);
});

test('a transfer between two other accounts is not attributed to the signer', () => {
  const c = ledger([send(ALICE, BOB, [{ denom: 'uyml', amount: '1' }])], 'yml1someoneelse');

  assert.deepEqual(leaving(c), []);
  assert.deepEqual(arriving(c), []);
  assert.equal(c.movements[0]!.side, 'elsewhere');
});

test('a treasury payment leaves the treasury, not the treasurer who signs it', () => {
  // A spender's own balance is not at stake. Putting `250 YML` under "leaves
  // this account" would make every treasurer think it was.
  const c = ledger(
    [
      treasurySpend({
        spender: ALICE,
        treasuryId: '3',
        recipient: BOB,
        amount: [{ denom: 'uyml', amount: '250000000' }],
        memo: 'March payroll',
      }),
    ],
    ALICE,
  );

  assert.deepEqual(leaving(c), []);
  assert.equal(c.movements[0]!.from, 'treasury 3');
  assert.equal(c.movements[0]!.side, 'elsewhere');
  assert.equal(c.reversibility, 'irreversible');
});

test('committing funds is reported as locked, with who gets them and when', () => {
  const releases = 1893456000; // 2030-01-01
  const c = ledger(
    [
      {
        typeUrl: '/blockchain.treasury.v1.MsgCreateLock',
        value: {
          admin: ALICE,
          treasuryId: '1',
          beneficiary: BOB,
          denom: 'uyml',
          amount: '5000000',
          lockType: 2,
          endTime: releases,
          revocable: false,
        },
      },
    ],
    ALICE,
  );

  assert.deepEqual(locking(c), [{ denom: 'uyml', amount: '5000000' }]);
  assert.equal(c.locks[0]!.beneficiary, BOB);
  assert.equal(c.locks[0]!.releasesAt, releases);
  assert.equal(c.locks[0]!.releaseUnknown, false);
  // Irrevocable: the treasury cannot get it back, which is the point of it.
  assert.equal(c.reversibility, 'irreversible');
});

test('a revocable commitment is not called irreversible', () => {
  const c = ledger(
    [
      {
        typeUrl: '/blockchain.treasury.v1.MsgCreateLock',
        value: {
          admin: ALICE,
          treasuryId: '1',
          beneficiary: BOB,
          denom: 'uyml',
          amount: '5000000',
          lockType: 2,
          endTime: 1893456000,
          revocable: true,
        },
      },
    ],
    ALICE,
  );
  assert.equal(c.reversibility, 'revocable');
  assert.equal(c.locks[0]!.revocable, true);
});

test('a commitment with no end time says the release date is unknown rather than 1970', () => {
  const c = ledger(
    [
      {
        typeUrl: '/blockchain.treasury.v1.MsgCreateLock',
        value: {
          admin: ALICE,
          treasuryId: '1',
          beneficiary: BOB,
          denom: 'uyml',
          amount: '1',
          lockType: 1,
          revocable: false,
        },
      },
    ],
    ALICE,
  );
  assert.equal(c.locks[0]!.releasesAt, undefined);
  assert.equal(c.locks[0]!.releaseUnknown, true);
});

test('staking is a delay, not a payment', () => {
  const c = ledger(
    [
      {
        typeUrl: '/cosmos.staking.v1beta1.MsgDelegate',
        value: {
          delegatorAddress: ALICE,
          validatorAddress: 'ymlvaloper1xyz',
          amount: { denom: 'uyml', amount: '100000000' },
        },
      },
    ],
    ALICE,
  );

  assert.deepEqual(leaving(c), [], 'staked funds are still yours');
  assert.deepEqual(locking(c), [{ denom: 'uyml', amount: '100000000' }]);
  assert.equal(c.reversibility, 'delayed');
});

test('a proposal moves nothing when it is signed, even carrying a payment', () => {
  // The whole reason the two questions are separate. The nested spend is real
  // and is described elsewhere on the screen, but signing the proposal does not
  // pay anybody — a group vote does — so the loudest warning in the product
  // must not fire here.
  const registry = chainRegistry();
  const inner = registry.encodeAsAny(
    treasurySpend({
      spender: ALICE,
      treasuryId: '3',
      recipient: BOB,
      amount: [{ denom: 'uyml', amount: '250000000' }],
      memo: '',
    }) as never,
  );

  const c = ledger(
    [
      {
        typeUrl: '/cosmos.group.v1.MsgSubmitProposal',
        value: {
          groupPolicyAddress: ALICE,
          proposers: [ALICE],
          metadata: '',
          messages: [inner],
          exec: 0,
        },
      },
    ],
    ALICE,
  );

  assert.equal(c.reversibility, 'proposal');
  assert.deepEqual(c.movements, []);
  assert.equal(c.incomplete, false);
});

test('one irreversible message makes the whole transaction irreversible', () => {
  const c = ledger(
    [
      { typeUrl: '/cosmos.gov.v1.MsgVote', value: { proposalId: '1', voter: ALICE, option: 1 } },
      send(ALICE, BOB, [{ denom: 'uyml', amount: '1' }]),
    ],
    ALICE,
  );
  assert.equal(c.reversibility, 'irreversible');
});

test('an unrecognised message makes the ledger incomplete rather than empty', () => {
  const c = consequencesOf(
    summariseSigningRequest(
      TxBody.encode(
        TxBody.fromPartial({
          messages: [{ typeUrl: '/some.future.v9.MsgWhoKnows', value: new Uint8Array([8, 1]) }],
        }),
      ).finish(),
    ),
    ALICE,
  );

  assert.equal(c.incomplete, true);
  assert.equal(c.reversibility, 'unknown');
});

test('a known transfer beside an unknown message is still incomplete', () => {
  // The dangerous case: a screen that totals the part it understood and says
  // nothing about the part it did not.
  const c = consequencesOf(
    summariseSigningRequest(
      TxBody.encode(
        TxBody.fromPartial({
          messages: [
            chainRegistry().encodeAsAny(
              send(ALICE, BOB, [{ denom: 'uyml', amount: '1' }]) as never,
            ),
            { typeUrl: '/some.future.v9.MsgWhoKnows', value: new Uint8Array([8, 1]) },
          ],
        }),
      ).finish(),
    ),
    ALICE,
  );

  assert.deepEqual(leaving(c), [{ denom: 'uyml', amount: '1' }]);
  assert.equal(c.incomplete, true);
  // Worst wins: an irreversible transfer beside something nobody can read is
  // reported as irreversible, not as unknown.
  assert.equal(c.reversibility, 'irreversible');
});

test('bytes that are not a transaction produce no ledger at all', () => {
  const c = consequencesOf(summariseSigningRequest(new Uint8Array([255, 255, 255])), ALICE);
  assert.equal(c.incomplete, true);
  assert.equal(c.reversibility, 'unknown');
  assert.deepEqual(c.movements, []);
});

test('a fee allowance is a power handed over, not an amount moved', () => {
  const c = ledger(
    [
      {
        typeUrl: '/cosmos.feegrant.v1beta1.MsgGrantAllowance',
        value: { granter: ALICE, grantee: BOB },
      },
    ],
    ALICE,
  );
  assert.deepEqual(c.movements, []);
  assert.equal(c.authority[0]!.grantee, BOB);
  assert.equal(c.reversibility, 'revocable');
});

test('amounts are summed by denom, as BigInt, and never across denoms', () => {
  // Above 2^53 on purpose: a total supply or a treasury balance reaches this,
  // and Number would silently round it on the one line somebody is checking.
  const summed = totalCoins([
    { denom: 'uyml', amount: '9007199254740993' },
    { denom: 'uyml', amount: '1' },
    { denom: 'ungn', amount: '5' },
  ]);
  assert.deepEqual(summed, [
    { denom: 'uyml', amount: '9007199254740994' },
    { denom: 'ungn', amount: '5' },
  ]);
});

test('a coin that is not a number is dropped rather than turned into NaN', () => {
  assert.deepEqual(totalCoins([{ denom: 'uyml', amount: 'nonsense' }]), []);
});
