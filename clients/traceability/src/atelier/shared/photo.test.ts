import assert from 'node:assert/strict';
import { test } from 'node:test';

import { fitWithin } from './photo.ts';

test('a photo is scaled to fit, keeping its shape, and never enlarged', () => {
  assert.deepEqual(fitWithin(4000, 3000, 2400), { width: 2400, height: 1800 });
  assert.deepEqual(fitWithin(3000, 4000, 2400), { width: 1800, height: 2400 });
  assert.deepEqual(fitWithin(800, 600, 2400), { width: 800, height: 600 });
});
