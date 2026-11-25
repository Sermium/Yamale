import assert from 'node:assert/strict';
import { test } from 'node:test';

import { checkRedeem, type RedeemInput } from './redeem.ts';

const NOW = 2_000_000;
const base: RedeemInput = {
  benefit: { jewelId: '7', jewelerId: '1', allowance: 2, period: '31536000', expiresAt: '0', uses: [] },
  voucherJewelId: '7', jewelerId: '1', signer: 'yml1owner', owner: 'yml1owner', codeExpiresAt: NOW + 200, stolen: false, now: NOW,
};
const key = (over: Partial<RedeemInput>) => {
  const r = checkRedeem({ ...base, ...over });
  return r.ok ? 'ok' : r.key;
};

test('a fresh code from the owner for our own benefit is accepted, with the uses left', () => {
  assert.deepEqual(checkRedeem(base), { ok: true, left: 2 });
});

test('a missing benefit or another jeweler\'s is not ours', () => {
  assert.equal(key({ benefit: undefined }), 'traceability.voucherNotOurs');
  assert.equal(key({ jewelerId: '2' }), 'traceability.voucherNotOurs');
});

test('a code signed by anyone but the current owner is refused, as is a bad signature', () => {
  assert.equal(key({ signer: 'yml1previous' }), 'traceability.voucherNotOwner');
  assert.equal(key({ signer: null }), 'traceability.voucherNotOwner');
});

test('freshness is by the chain clock: lapsed at or before the head is stale', () => {
  assert.equal(key({ codeExpiresAt: NOW }), 'traceability.voucherStale');
  assert.equal(key({ codeExpiresAt: NOW - 1 }), 'traceability.voucherStale');
  assert.equal(key({ codeExpiresAt: NOW + 1 }), 'ok');
});

test('a stolen jewel is refused', () => {
  assert.equal(key({ stolen: true }), 'traceability.err.stolen');
});

test('an ended benefit and a used-up one are told apart', () => {
  assert.equal(key({ benefit: { ...base.benefit!, expiresAt: String(NOW - 10) } }), 'traceability.err.benefitExpired');
  assert.equal(key({ benefit: { ...base.benefit!, uses: [String(NOW - 100), String(NOW - 50)] } }), 'traceability.err.allowanceUsed');
});

test('uses older than the period no longer count', () => {
  const old = String(NOW - 31_536_000 - 5);
  assert.deepEqual(checkRedeem({ ...base, benefit: { ...base.benefit!, uses: [old, String(NOW - 50)] } }), { ok: true, left: 1 });
});

test("a code that outlives the chain's ceiling is refused; exactly at it is accepted", () => {
  assert.equal(key({ codeExpiresAt: NOW + 300 }), 'ok');
  assert.equal(key({ codeExpiresAt: NOW + 301 }), 'traceability.voucherTooFar');
});

test('a code naming another jewel than the benefit belongs to has its own message', () => {
  assert.equal(key({ voucherJewelId: '8' }), 'traceability.voucherWrongJewel');
});
