import assert from 'node:assert/strict';
import { test } from 'node:test';

import { formatAlias } from './jewelreads.ts';

test('an identifier reads as country, payload in fours, check character', () => {
  assert.equal(formatAlias('SNK3M97QRTB'), 'SN-K3M9-7QRT-B');
  assert.equal(formatAlias('sn-k3m9-7qrt-b'), 'SN-K3M9-7QRT-B');
  assert.equal(formatAlias('NGCAQ3C04ZM'), 'NG-CAQ3-C04Z-M');
});
