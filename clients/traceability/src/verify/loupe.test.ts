import assert from 'node:assert/strict';
import { test } from 'node:test';

import { MAX_ZOOM, MIN_ZOOM, scrollAbout, wheelZoom } from './loupe.ts';

test('the wheel zooms in when rolled up, out when rolled down, within 1x and 4x', () => {
  assert.ok(wheelZoom(2, -100) > 2);
  assert.ok(wheelZoom(2, 100) < 2);
  assert.equal(wheelZoom(3.9, -1000), MAX_ZOOM);
  assert.equal(wheelZoom(1.1, 1000), MIN_ZOOM);
  assert.equal(wheelZoom(2, 0), 2);
});

test('zooming keeps the point under the cursor in place', () => {
  // View 400px wide, scrolled 100px, cursor 150px from its edge: content point 250 at zoom 2.
  // At zoom 4 that point is at 500, so the view scrolls to 500 - 150 = 350.
  assert.equal(scrollAbout(100, 150, 2, 4), 350);
  // And back: zooming out returns to where it was.
  assert.equal(scrollAbout(350, 150, 4, 2), 100);
  // Never a negative scroll at the left edge.
  assert.equal(scrollAbout(0, 10, 2, 1), 0);
});
