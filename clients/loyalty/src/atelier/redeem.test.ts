import assert from 'node:assert/strict';
import { test } from 'node:test';

import { checkRedeem, type RedeemInput } from './redeem.ts';

const NOW = 2_000_000;
const base: RedeemInput = {
  benefit: { jewelerId: '1', allowance: 2, period: '31536000', expiresAt: '0', uses: [] },
  jewelerId: '1', signer: 'yml1owner', owner: 'yml1owner', codeExpiresAt: NOW + 200, stolen: false, now: NOW,
};
const key = (over: Partial<RedeemInput>) => {
  const r = checkRedeem({ ...base, ...over });
  return r.ok ? 'ok' : r.key;
};

test('a fresh code from the owner for our own benefit is accepted, with the uses left', () => {
  assert.deepEqual(checkRedeem(base), { ok: true, left: 2 });
});

test('a missing benefit or another jeweler\'s is not ours', () => {
  assert.equal(key({ benefit: undefined }), 'loyalty.voucherNotOurs');
  assert.equal(key({ jewelerId: '2' }), 'loyalty.voucherNotOurs');
});

test('a code signed by anyone but the current owner is refused, as is a bad signature', () => {
  assert.equal(key({ signer: 'yml1previous' }), 'loyalty.voucherNotOwner');
  assert.equal(key({ signer: null }), 'loyalty.voucherNotOwner');
});

test('freshness is by the chain clock: lapsed at or before the head is stale', () => {
  assert.equal(key({ codeExpiresAt: NOW }), 'loyalty.voucherStale');
  assert.equal(key({ codeExpiresAt: NOW - 1 }), 'loyalty.voucherStale');
  assert.equal(key({ codeExpiresAt: NOW + 1 }), 'ok');
});

test('a stolen jewel is refused', () => {
  assert.equal(key({ stolen: true }), 'loyalty.err.stolen');
});

test('an ended benefit and a used-up one are told apart', () => {
  assert.equal(key({ benefit: { ...base.benefit!, expiresAt: String(NOW - 10) } }), 'loyalty.err.benefitExpired');
  assert.equal(key({ benefit: { ...base.benefit!, uses: [String(NOW - 100), String(NOW - 50)] } }), 'loyalty.err.allowanceUsed');
});

test('uses older than the period no longer count', () => {
  const old = String(NOW - 31_536_000 - 5);
  assert.deepEqual(checkRedeem({ ...base, benefit: { ...base.benefit!, uses: [old, String(NOW - 50)] } }), { ok: true, left: 1 });
});
