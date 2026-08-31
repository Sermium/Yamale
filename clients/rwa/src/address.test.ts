import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { addressOf, canSign, looksLikeAddress } from './address.ts';

// A 20-byte account, the ordinary secp256k1 shape.
const ACCOUNT = 'yml1p52pkg3fxqmnu32v2ddxz6r0we7cfzujsn4udc';
// A 32-byte account: module accounts and x/group accounts are this shape, and
// so is every registry office this app names on a land vehicle. Taken from
// yamale-devnet-2's land authorities.
const OFFICE = 'yml1c799jddmlz7segvg6jrw6w2k6svwafganjdznard3tc74n7td7rq4axc2s';

test('an ordinary account is recognised', () => {
  assert.equal(looksLikeAddress(ACCOUNT), true);
  assert.equal(looksLikeAddress(`  ${ACCOUNT}  `), true, 'a pasted address carries whitespace');
});

test('a 32-byte account is recognised, because half this app is about them', () => {
  // The rule this replaces was fixed at 42 characters, which would have refused
  // every module account, every x/group account, and every registry office on
  // the chain — the accounts a land vehicle names most often.
  assert.equal(looksLikeAddress(OFFICE), true);
  assert.equal(OFFICE.length, 62);
});

test('a transposed character is caught by the checksum, not waved through', () => {
  // This is what bech32's checksum exists for. Without it the address reaches
  // the node, comes back a 400, and the screen honestly reports that the chain
  // did not answer — which is true of nothing and useless to the person who
  // mistyped.
  const swapped = ACCOUNT.slice(0, 10) + ACCOUNT[11] + ACCOUNT[10] + ACCOUNT.slice(12);
  assert.notEqual(swapped, ACCOUNT);
  assert.equal(looksLikeAddress(swapped), false);
});

test('the things people actually paste into an address box are refused', () => {
  assert.equal(looksLikeAddress(''), false);
  assert.equal(looksLikeAddress('my wallet'), false);
  // A transaction hash. Refusing it here is the difference between an error and
  // an empty account that looks like a real answer.
  assert.equal(looksLikeAddress('9F3C1A2B4D5E6F708192A3B4C5D6E7F8'), false);
});

test('a validator operator address is not an account that can hold shares', () => {
  // It decodes cleanly; the prefix is what disqualifies it.
  assert.equal(looksLikeAddress('ymlvaloper1p52pkg3fxqmnu32v2ddxz6r0we7cfzujxdedql'), false);
});

test('only a connected account can sign, and watching is not a degraded one', () => {
  assert.equal(canSign({ mode: 'none' }), false);
  assert.equal(canSign({ mode: 'watching', address: ACCOUNT }), false);

  assert.equal(addressOf({ mode: 'none' }), '');
  assert.equal(addressOf({ mode: 'watching', address: ACCOUNT }), ACCOUNT,
    'a watched address is a real address to read against');
});
