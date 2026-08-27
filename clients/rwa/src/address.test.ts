import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { addressOf, canSign, looksLikeAddress } from './address.ts';

// A real address off yamale-devnet-2, used as the positive case so the length
// and alphabet are checked against something the chain actually issued rather
// than against a shape somebody imagined.
const REAL = 'yml12urcg48rnzetfd0645p4d7mcw0g369uursqaqm';

test('a real address is recognised', () => {
  assert.equal(looksLikeAddress(REAL), true);
  assert.equal(looksLikeAddress(`  ${REAL}  `), true, 'a pasted address carries whitespace');
});

test('the things people actually paste into an address box are refused', () => {
  assert.equal(looksLikeAddress(''), false);
  assert.equal(looksLikeAddress('my wallet'), false);
  // A transaction hash. Refusing it here is the difference between an error and
  // an empty account that looks like a real answer.
  assert.equal(looksLikeAddress('9F3C1A2B4D5E6F708192A3B4C5D6E7F8'), false);
  assert.equal(looksLikeAddress('ymlvaloper1x2y3z'), false, 'a validator operator address');
});

test('bech32 leaves out the lookalike characters, and so does the check', () => {
  // 1, b, i and o are absent from the alphabet on purpose. Accepting them
  // would let a transcription error through to a screen that then shows an
  // empty account, which reads as "this address holds nothing".
  assert.equal(looksLikeAddress(REAL.replace('u', 'b')), false);
  assert.equal(looksLikeAddress(REAL.replace('u', 'i')), false);
  assert.equal(looksLikeAddress(REAL.replace('u', 'o')), false);
});

test('length is part of the shape', () => {
  assert.equal(looksLikeAddress(`${REAL}z`), false);
  assert.equal(looksLikeAddress(REAL.slice(0, -1)), false);
});

test('only a connected account can sign, and watching is not a degraded one', () => {
  assert.equal(canSign({ mode: 'none' }), false);
  assert.equal(canSign({ mode: 'watching', address: REAL }), false);

  assert.equal(addressOf({ mode: 'none' }), '');
  assert.equal(addressOf({ mode: 'watching', address: REAL }), REAL,
    'a watched address is a real address to read against');
});
