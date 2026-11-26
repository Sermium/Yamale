import assert from 'node:assert/strict';
import { test } from 'node:test';

import { holdsMoney } from './holdings.ts';

test('an account holding any money is not topped up; the fee token alone does not count', () => {
  assert.equal(holdsMoney(new Map()), false);
  assert.equal(holdsMoney(new Map([['uyml', '5000000']])), false);
  assert.equal(holdsMoney(new Map([['uusdc', '0'], ['uyml', '1']])), false);
  assert.equal(holdsMoney(new Map([['uusdc', '10000000000']])), true);
  assert.equal(holdsMoney(new Map([['uxof', '1'], ['uyml', '0']])), true);
});
