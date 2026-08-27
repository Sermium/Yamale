import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { TYPE_URLS, messagePlan, messageValue, type Draft } from './message.ts';

const CLAIM: Draft = {
  kind: 'claim', holder: 'yml1holder', assetId: '3', owed: '4800000', owedDenom: 'ukes',
};
const REDEEM: Draft = {
  kind: 'redeem', holder: 'yml1holder', assetId: '3', amount: '250000',
  shareDenom: 'tok/3/KEFARM', payout: '20500000', payoutDenom: 'ukes',
};
const DISPUTE: Draft = {
  kind: 'dispute', challenger: 'yml1holder', assetId: '3',
  reason: 'the deed shows 96,000,000 KES', bond: '4100000', bondDenom: 'ukes',
};

test('the type url prefix is blockchain., not yamale.blockchain.', () => {
  // The proto package every one of this module's .proto files declares. Getting
  // it wrong produces an unregistered-type-url failure that reads as a missing
  // encoder rather than as a typo.
  for (const url of Object.values(TYPE_URLS)) {
    assert.match(url, /^\/blockchain\.tokenisation\.v1\.Msg[A-Z]/);
    assert.doesNotMatch(url, /yamale/);
  }
});

test('a claim shows both fields it signs and the payout it does not', () => {
  const plan = messagePlan(CLAIM);
  assert.equal(plan.typeUrl, TYPE_URLS.claim);

  const signed = plan.rows.filter((r) => r.carried === 'ledger').map((r) => r.field);
  assert.deepEqual(signed, ['holder', 'asset_id'],
    'MsgClaim has exactly two fields; a third would be an invention');

  const paid = plan.rows.find((r) => r.field === 'paid');
  assert.equal(paid?.carried, 'derived',
    'the amount paid is nowhere in the signed bytes — the keeper decides it');
  assert.equal(paid?.denom, 'ukes');
});

test('a redemption states what is destroyed as well as what is signed', () => {
  const plan = messagePlan(REDEEM);

  const amount = plan.rows.find((r) => r.field === 'amount');
  assert.equal(amount?.carried, 'ledger');
  assert.equal(amount?.value, '250000', 'base units, unformatted — the view converts');

  const burned = plan.rows.find((r) => r.field === 'burned');
  assert.equal(burned?.carried, 'derived');
  assert.equal(burned?.value, '250000');
  assert.ok(burned?.noteKey, 'a derived consequence has to explain itself');

  const paid = plan.rows.find((r) => r.field === 'paid');
  assert.equal(paid?.value, '20500000');
  assert.equal(paid?.denom, 'ukes');
});

test('a dispute reason is marked as reaching the ledger', () => {
  // Somebody typing a reason should know it is being published permanently and
  // read by whoever resolves the dispute, not filed with us.
  const reason = messagePlan(DISPUTE).rows.find((r) => r.field === 'reason');
  assert.equal(reason?.carried, 'ledger');
  assert.equal(reason?.value, 'the deed shows 96,000,000 KES');
});

test('a dispute bond is derived, and it is the row that matters most', () => {
  // The bond leaves the challenger's account in the same block and appears
  // nowhere in MsgDisputeSale. It is the consequence somebody would otherwise
  // discover afterwards.
  const bond = messagePlan(DISPUTE).rows.find((r) => r.field === 'bond');
  assert.equal(bond?.carried, 'derived');
  assert.equal(bond?.value, '4100000');
  assert.equal(bond?.denom, 'ukes');
  assert.equal(bond?.noteKey, 'rwa.msg.bondNote');
});

test('every derived row explains itself and every signed row does not need to', () => {
  for (const draft of [CLAIM, REDEEM, DISPUTE]) {
    for (const row of messagePlan(draft).rows) {
      if (row.carried === 'derived') {
        assert.ok(row.noteKey, `${draft.kind}.${row.field} is derived with nothing said about it`);
      }
    }
  }
});

test('the signed value carries exactly the generated field names', () => {
  // A field name that differs from the generated type is dropped silently by
  // protobuf encoding — an amount of nothing, after somebody agreed to exit.
  assert.deepEqual(messageValue(CLAIM), { holder: 'yml1holder', assetId: '3' });
  assert.deepEqual(messageValue(REDEEM), {
    holder: 'yml1holder', assetId: '3', amount: '250000',
  });
  assert.deepEqual(messageValue(DISPUTE), {
    challenger: 'yml1holder', assetId: '3', reason: 'the deed shows 96,000,000 KES',
  });
});

test('a signed value never carries a derived figure', () => {
  // The bond and the payout must not leak into the message: MsgDisputeSale has
  // no bond field, and a stray key would encode as nothing while looking like
  // it had been sent.
  const dispute = messageValue(DISPUTE);
  assert.equal('bond' in dispute, false);
  const redeem = messageValue(REDEEM);
  assert.equal('payout' in redeem, false);
  assert.equal('paid' in redeem, false);
});
