import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { AuthInfo, TxBody } from 'cosmjs-types/cosmos/tx/v1beta1/tx.js';

import { chainRegistry } from './registry.ts';
import { payment, send, treasurySpend } from './signing.ts';
import { describeFee, headline, summariseSigningRequest } from './signrequest.ts';

/**
 * The approval screen's decode, exercised the way the wallet exercises it:
 * encode a transaction with the real registry, then read it back from the bytes
 * with nothing but the bytes.
 *
 * Nothing here asserts on a string the test built itself. The point is that a
 * transaction assembled by one half of this package is legible to the other,
 * because that is the property the signing screen depends on — and it is the
 * property that fails silently when a message is added to the chain and not to
 * the registry.
 */

const ALICE = 'yml1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq';
const BOB = 'yml1zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz';

function body(messages: { typeUrl: string; value: unknown }[], memo = ''): Uint8Array {
  const registry = chainRegistry();
  return TxBody.encode(
    TxBody.fromPartial({
      messages: messages.map((m) => registry.encodeAsAny(m as never)),
      memo,
    }),
  ).finish();
}

test('a transfer is read back as an amount, a recipient and a sentence', () => {
  const summary = summariseSigningRequest(
    body([send(ALICE, BOB, [{ denom: 'uyml', amount: '1250500000' }])]),
  );

  assert.equal(summary.undecodable, undefined);
  assert.equal(summary.incomplete, false);
  assert.equal(summary.messages.length, 1);

  const decoded = summary.messages[0]!.decoded;
  assert.ok(decoded, 'the message should have decoded');
  assert.equal(decoded.kind, 'transfer');
  // The human amount, not the base units. This is the whole point of the
  // screen: 1250500000uyml must never reach it.
  assert.match(decoded.summary, /1,250\.5 YML/);
  assert.equal(decoded.counterparty, BOB);
  assert.deepEqual(decoded.coins, [{ denom: 'uyml', amount: '1250500000' }]);
});

test("this chain's own messages decode, not only the standard Cosmos ones", () => {
  // The regression that matters: paymsg, treasury, amm and the rest are absent
  // from CosmJS's default registry, so a decoder built on that alone reports
  // every payment on this chain as unreadable.
  const summary = summariseSigningRequest(
    body([
      payment({
        debtor: ALICE,
        creditor: BOB,
        endToEndId: 'INV-2026-0041',
        instructingParticipant: ALICE,
        instructedParticipant: BOB,
        denom: 'ungn',
        amount: '4500000000',
        purposeCode: 'SUPP',
        remittanceInformation: 'March hosting',
      }),
    ]),
  );

  const decoded = summary.messages[0]!.decoded;
  assert.ok(decoded);
  assert.equal(decoded.kind, 'payment');
  assert.match(decoded.summary, /4,500 NGN/);
  assert.match(decoded.summary, /March hosting/);

  // The reference and the remittance are what reconciliation needs, so they
  // have to survive as labelled detail rather than only inside the sentence.
  const reference = decoded.details?.find((d) => d.label === 'Reference');
  assert.equal(reference?.value, 'INV-2026-0041');
  const purpose = decoded.details?.find((d) => d.label === 'Purpose');
  assert.equal(purpose?.value, 'Supplier payment');
});

test('a 64-bit id survives the decode as a string', () => {
  // treasury_id is a uint64. Read through a double it would lose its low bits
  // at exactly the sizes an id reaches after a few years of use.
  const summary = summariseSigningRequest(
    body([
      treasurySpend({
        spender: ALICE,
        treasuryId: '18446744073709551615',
        recipient: BOB,
        amount: [{ denom: 'uyml', amount: '1' }],
        memo: 'rent',
      }),
    ]),
  );

  const raw = summary.messages[0]!.decoded!.raw as Record<string, unknown>;
  assert.equal(raw.treasury_id, '18446744073709551615');
});

test('an enum arrives as its name, not as the number on the wire', () => {
  const summary = summariseSigningRequest(
    body([
      {
        typeUrl: '/blockchain.treasury.v1.MsgCreateLock',
        value: {
          admin: ALICE,
          treasuryId: '1',
          beneficiary: BOB,
          denom: 'uyml',
          amount: '5000000',
          lockType: 1,
          revocable: true,
        },
      },
    ]),
  );

  const decoded = summary.messages[0]!.decoded!;
  const type = decoded.details?.find((d) => d.label === 'Type');
  // `1` means nothing to a reader; the decoder maps LOCK_TYPE_* to words, and
  // it only gets the chance if the enum came back as a name.
  assert.ok(type && type.value !== '1', `expected a named lock type, got ${type?.value}`);
});

test('the action inside a proposal is decoded, not just the wrapper', () => {
  // The exact case the old screen failed on: it said
  // /cosmos.group.v1.MsgSubmitProposal and stopped, so the thing being
  // approved — a payment out of a shared account — was never shown at all.
  const registry = chainRegistry();
  const inner = registry.encodeAsAny(
    treasurySpend({
      spender: ALICE,
      treasuryId: '3',
      recipient: BOB,
      amount: [{ denom: 'uyml', amount: '250000000' }],
      memo: 'March payroll',
    }) as never,
  );

  const summary = summariseSigningRequest(
    body([
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
    ]),
  );

  const outer = summary.messages[0]!;
  assert.equal(outer.contains.length, 1, 'the proposal should carry one nested message');
  const nested = outer.contains[0]!.decoded;
  assert.ok(nested, 'the nested message should have decoded');
  assert.match(nested.summary, /250 YML/);
  assert.match(nested.summary, /March payroll/);

  // And the one line somebody reads before approving names the act, not the
  // envelope it arrived in.
  assert.match(headline(summary), /250 YML/);
});

test('a message the registry does not know is reported as unreadable, never described', () => {
  const summary = summariseSigningRequest(
    TxBody.encode(
      TxBody.fromPartial({
        messages: [{ typeUrl: '/some.future.v9.MsgWhoKnows', value: new Uint8Array([8, 1]) }],
      }),
    ).finish(),
  );

  assert.equal(summary.incomplete, true);
  assert.equal(summary.messages[0]!.problem, 'unregistered');
  assert.equal(summary.messages[0]!.decoded, undefined);
  assert.match(headline(summary), /cannot read/i);
});

test('bytes that are not a transaction body are refused outright', () => {
  const summary = summariseSigningRequest(new Uint8Array([255, 255, 255, 255, 255, 255]));
  assert.ok(summary.undecodable, 'garbage should be reported as undecodable');
  assert.equal(summary.messages.length, 0);
  assert.equal(summary.incomplete, true);
});

test('a message whose bytes do not match its declared type is malformed, not decoded', () => {
  // A type the registry knows, carrying bytes that are not that type. The
  // difference between this and an unregistered type matters: one is a gap in
  // the wallet, the other is a transaction lying about itself.
  const summary = summariseSigningRequest(
    TxBody.encode(
      TxBody.fromPartial({
        messages: [
          // Field 1 declared as a length-delimited string of 200 bytes in a
          // 3-byte message: the reader runs off the end.
          { typeUrl: '/cosmos.bank.v1beta1.MsgSend', value: new Uint8Array([10, 200, 1]) },
        ],
      }),
    ).finish(),
  );

  assert.equal(summary.messages[0]!.problem, 'malformed');
  assert.equal(summary.incomplete, true);
});

test('the memo is carried through, because it is on the ledger forever', () => {
  const summary = summariseSigningRequest(
    body([send(ALICE, BOB, [{ denom: 'uyml', amount: '1' }])], 'INV-2026-0041'),
  );
  assert.equal(summary.memo, 'INV-2026-0041');
});

test('the fee is read from the auth info, not from what the caller claimed', () => {
  const authInfo = AuthInfo.encode(
    AuthInfo.fromPartial({
      fee: { amount: [{ denom: 'uyml', amount: '2500' }], gasLimit: 200000n, granter: BOB },
    }),
  ).finish();

  const summary = summariseSigningRequest(
    body([send(ALICE, BOB, [{ denom: 'uyml', amount: '1' }])]),
    authInfo,
  );

  assert.deepEqual(summary.fee?.amount, [{ denom: 'uyml', amount: '2500' }]);
  assert.equal(summary.fee?.gasLimit, '200000');
  assert.equal(summary.fee?.granter, BOB);

  const words = describeFee(summary.fee);
  assert.ok(words);
  assert.match(words, /0\.0025 YML/);
  assert.match(words, /allowance/);
});

test('a zero fee says nothing rather than teaching people to skip the line', () => {
  const authInfo = AuthInfo.encode(
    AuthInfo.fromPartial({ fee: { amount: [{ denom: 'uyml', amount: '0' }], gasLimit: 200000n } }),
  ).finish();

  const summary = summariseSigningRequest(
    body([send(ALICE, BOB, [{ denom: 'uyml', amount: '1' }])]),
    authInfo,
  );
  assert.equal(describeFee(summary.fee), null);
});

test('several messages are counted rather than narrated into one story', () => {
  const summary = summariseSigningRequest(
    body([
      send(ALICE, BOB, [{ denom: 'uyml', amount: '1' }]),
      send(ALICE, BOB, [{ denom: 'ungn', amount: '2' }]),
    ]),
  );
  assert.equal(summary.messages.length, 2);
  assert.match(headline(summary), /2 actions/);
});

test('an auth info that will not decode does not take the messages down with it', () => {
  const summary = summariseSigningRequest(
    body([send(ALICE, BOB, [{ denom: 'uyml', amount: '1' }])]),
    new Uint8Array([255, 255, 255, 255]),
  );
  assert.equal(summary.fee, undefined);
  assert.ok(summary.messages[0]!.decoded, 'the messages are still readable');
});
