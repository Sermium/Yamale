import assert from 'node:assert/strict';
import { test } from 'node:test';

import { benefitState, ownershipSteps } from './history.ts';

test('the history is the stock date, then one date per owner', () => {
  assert.deepEqual(ownershipSteps({ registeredAt: '100', ownerSince: ['200', '300'] }), [
    { kind: 'stock', ordinal: 0, at: 100 },
    { kind: 'owner', ordinal: 1, at: 200 },
    { kind: 'owner', ordinal: 2, at: 300 },
  ]);
});

test('a periodic benefit counts only the uses inside its window, as the chain does', () => {
  const b = { allowance: 2, period: '1000', expiresAt: '0', uses: ['100', '1500'] };
  // At 2000 the window is (1000, 2000]: only the use at 1500 counts.
  assert.deepEqual(benefitState(b, true, 2000), { state: 'active', left: 1, resetsAt: null, expiresAt: null });
  // At 1099 both count and the next one frees at 100 + 1000.
  assert.deepEqual(benefitState({ ...b, uses: ['100', '1050'] }, true, 1099),
    { state: 'active', left: 0, resetsAt: 1100, expiresAt: null });
});

test('a lifetime benefit counts every use, expiry and suspension end it', () => {
  const b = { allowance: 3, period: '0', expiresAt: '5000', uses: ['1', '2'] };
  assert.deepEqual(benefitState(b, true, 10), { state: 'active', left: 1, resetsAt: null, expiresAt: 5000 });
  assert.deepEqual(benefitState(b, true, 5000), { state: 'expired', expiredAt: 5000 });
  assert.deepEqual(benefitState(b, false, 10), { state: 'withdrawn' });
});
