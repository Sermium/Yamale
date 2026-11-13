import assert from 'node:assert/strict';
import { test } from 'node:test';

import { markOf, voucherPhase } from './voucher.ts';

const issued = markOf(['100', '200']);

test('a code is showing until it lapses', () => {
  assert.equal(voucherPhase(issued, issued, 1000, 5000), 'showing');
  assert.equal(voucherPhase(issued, null, 1000, 5000), 'showing');
  assert.equal(voucherPhase(issued, issued, 5000, 5000), 'lapsed');
});

test('a use recorded after issue closes the code, even before it lapses', () => {
  assert.equal(voucherPhase(issued, markOf(['100', '200', '300']), 1000, 5000), 'used');
  // A periodic benefit may drop old uses: a newer date is still a use.
  assert.equal(voucherPhase(issued, markOf(['200', '300']), 1000, 5000), 'used');
});

test('used outranks lapsed', () => {
  assert.equal(voucherPhase(issued, markOf(['100', '200', '300']), 9000, 5000), 'used');
});
