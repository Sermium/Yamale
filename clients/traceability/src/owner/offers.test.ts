import assert from 'node:assert/strict';
import { test } from 'node:test';

import { liveClaimKey, offerIsOpen, stockOfferBlocked } from './offers.ts';

const offer = { expiresAt: '1000', claimKey: 'abc' };

test('an offer is open strictly before it expires, and never when there is none', () => {
  assert.equal(offerIsOpen(offer, 999), true);
  assert.equal(offerIsOpen(offer, 1000), false);
  assert.equal(offerIsOpen(offer, 5000), false);
  assert.equal(offerIsOpen(undefined, 1), false);
  assert.equal(offerIsOpen(null, 1), false);
});

test('only a live link offer has a claim key worth keeping', () => {
  assert.equal(liveClaimKey(offer, 999), 'abc');
  assert.equal(liveClaimKey(offer, 1000), null);
  assert.equal(liveClaimKey({ expiresAt: '2000', claimKey: '' }, 1), null);
  assert.equal(liveClaimKey(undefined, 1), null);
});

test('an offer from a suspended jeweler\'s own stock is blocked, and nothing else is', () => {
  assert.equal(stockOfferBlocked({ from: 'yml1shop' }, 'yml1shop', false), true);
  assert.equal(stockOfferBlocked({ from: 'yml1shop' }, 'yml1shop', true), false);
  assert.equal(stockOfferBlocked({ from: 'yml1owner' }, 'yml1shop', false), false);
  assert.equal(stockOfferBlocked(undefined, 'yml1shop', false), false);
  assert.equal(stockOfferBlocked({ from: 'yml1shop' }, null, false), false);
});
