import assert from 'node:assert/strict';
import { test } from 'node:test';

import { errorKey } from './errors.ts';

const wrap = (msg: string) =>
  `failed to execute message; message index: 0: jewel 4: ${msg} [cosmos/cosmos-sdk@v0.53.6/baseapp/baseapp.go:1027] With gas wanted: '400000'`;

test('the refusals a person can hit map to their own sentence', () => {
  assert.equal(errorKey(wrap('this jewel already has an open offer; cancel it first')), 'offerOpen');
  assert.equal(errorKey(wrap('this offer has expired; ask the owner for a new one')), 'offerExpired');
  assert.equal(errorKey(wrap('the claim key or its signature does not match this offer')), 'badClaim');
  assert.equal(errorKey(wrap('this jewel is reported stolen')), 'stolen');
  assert.equal(errorKey(wrap('this voucher has already been used')), 'voucherUsed');
  assert.equal(errorKey(wrap("this benefit's allowance for the period is used up")), 'allowanceUsed');
  assert.equal(errorKey(wrap('this jeweler has already registered a jewel with this inscription')), 'inscriptionTaken');
  assert.equal(errorKey('fee-grant not found: not found'), 'fees');
  assert.equal(errorKey('message does not exist in allowed messages'), 'fees');
});

test('"not reported stolen" is not "reported stolen", and anything unknown is null', () => {
  assert.equal(errorKey(wrap('this jewel is not reported stolen')), null);
  assert.equal(errorKey('something nobody wrote a rule for'), null);
});
