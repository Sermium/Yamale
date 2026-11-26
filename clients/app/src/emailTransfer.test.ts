import assert from 'node:assert/strict';
import { test } from 'node:test';

import { addressOf, claimKey, linkFor, validCode } from './emailLink.ts';

const secret = new Uint8Array(32).fill(3);

test('the same link and code always open the same one-time account', async () => {
  const a = await claimKey(secret, '4821');
  const b = await claimKey(secret, '4821');
  assert.equal(addressOf(a), addressOf(b));
});

test('another code, or no code, opens a different account', async () => {
  const withCode = addressOf(await claimKey(secret, '4821'));
  assert.notEqual(addressOf(await claimKey(secret, '4822')), withCode);
  assert.notEqual(addressOf(await claimKey(secret, '')), withCode);
  assert.notEqual(addressOf(await claimKey(new Uint8Array(32).fill(4), '4821')), withCode);
});

test('the link carries the secret in the fragment, and says when a code is needed', () => {
  const plain = linkFor(secret, false);
  assert.match(plain, /^https:\/\/pay\.yamaletech\.com\/app\/#receive=[A-Za-z0-9_-]{43}$/);
  assert.match(linkFor(secret, true), /#receive=[A-Za-z0-9_-]{43}&code=1$/);
});

test('a code is 4 to 8 digits', () => {
  for (const ok of ['4821', '12345678']) assert.equal(validCode(ok), true);
  for (const bad of ['123', '123456789', '12a4', '']) assert.equal(validCode(bad), false);
});
