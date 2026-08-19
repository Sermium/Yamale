import assert from 'node:assert/strict';
import { test } from 'node:test';

import { beginUnbonding, claimRewards, delegate, send, vote } from './signing.ts';
import { describeChain } from './wallet.ts';

// The message constructors exist so no interface assembles a typeUrl by hand.
// A typo there produces a transaction the chain rejects for a reason that reads
// as unrelated to the mistake, so the exact strings are pinned.

test('a transfer is built as the chain expects it', () => {
  const msg = send('yml1from', 'yml1to', [{ denom: 'uyml', amount: '1000000' }]);
  assert.equal(msg.typeUrl, '/cosmos.bank.v1beta1.MsgSend');
  assert.deepEqual(msg.value, {
    fromAddress: 'yml1from',
    toAddress: 'yml1to',
    amount: [{ denom: 'uyml', amount: '1000000' }],
  });
});

test('staking messages carry the delegator and validator the right way round', () => {
  const stake = delegate('yml1me', 'ymlvaloper1them', { denom: 'uyml', amount: '5' });
  assert.equal(stake.typeUrl, '/cosmos.staking.v1beta1.MsgDelegate');
  assert.deepEqual(stake.value, {
    delegatorAddress: 'yml1me',
    validatorAddress: 'ymlvaloper1them',
    amount: { denom: 'uyml', amount: '5' },
  });

  const unstake = beginUnbonding('yml1me', 'ymlvaloper1them', { denom: 'uyml', amount: '5' });
  assert.equal(unstake.typeUrl, '/cosmos.staking.v1beta1.MsgUndelegate');

  const rewards = claimRewards('yml1me', 'ymlvaloper1them');
  assert.equal(rewards.typeUrl, '/cosmos.distribution.v1beta1.MsgWithdrawDelegatorReward');
});

// The vote options are a protobuf enum, and their numbering is not
// alphabetical: getting no and abstain the wrong way round would cast the
// opposite of what somebody clicked, silently and irreversibly.
test('vote options map to the right enum values', () => {
  assert.equal((vote('yml1me', '1', 'yes').value as any).option, 1);
  assert.equal((vote('yml1me', '1', 'abstain').value as any).option, 2);
  assert.equal((vote('yml1me', '1', 'no').value as any).option, 3);
  assert.equal((vote('yml1me', '1', 'veto').value as any).option, 4);
});

test('a proposal id is carried as the integer the chain expects', () => {
  assert.equal((vote('yml1me', '42', 'yes').value as any).proposalId, 42n);
});

// A wallet has never heard of this chain, so the description it is given is the
// only thing standing between a user and a balance shown in raw base units.
test('the chain description gives a wallet everything it needs to render amounts', () => {
  const described = describeChain({
    chainId: 'yamale-testnet-1',
    chainName: 'Yamale',
    rpcUrl: 'http://localhost:26657',
    restUrl: 'http://localhost:1317',
    baseDenom: 'uyml',
    displayDenom: 'YML',
    exponent: 6,
    bech32Prefix: 'yml',
    gasPrice: 0.025,
  });

  assert.equal(described.stakeCurrency.coinDenom, 'YML');
  assert.equal(described.stakeCurrency.coinMinimalDenom, 'uyml');
  assert.equal(described.stakeCurrency.coinDecimals, 6);
  assert.equal(described.feeCurrencies[0]!.gasPriceStep.average, 0.025);

  // Every prefix a wallet derives, not just the account one — a missing valoper
  // prefix shows staking addresses as unparseable.
  assert.equal(described.bech32Config.bech32PrefixAccAddr, 'yml');
  assert.equal(described.bech32Config.bech32PrefixValAddr, 'ymlvaloper');
  assert.equal(described.bech32Config.bech32PrefixConsAddr, 'ymlvalcons');
});
