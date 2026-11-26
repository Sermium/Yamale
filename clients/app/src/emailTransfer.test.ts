import assert from 'node:assert/strict';
import { test } from 'node:test';

import { addressOf, claimKey, linkFor, validCode } from './emailLink.ts';

const secret = new Uint8Array(32).fill(3);

test('the same link and code always open the same one-time account', async () => {
  const a = await claimKey(secret, '482913');
  const b = await claimKey(secret, '482913');
  assert.equal(addressOf(a), addressOf(b));
});

test('another code, or no code, opens a different account', async () => {
  const withCode = addressOf(await claimKey(secret, '482913'));
  assert.notEqual(addressOf(await claimKey(secret, '482914')), withCode);
  assert.notEqual(addressOf(await claimKey(secret, '')), withCode);
  assert.notEqual(addressOf(await claimKey(new Uint8Array(32).fill(4), '482913')), withCode);
});

test('the link carries the secret in the fragment, and says when a code is needed', () => {
  const plain = linkFor(secret, false);
  assert.match(plain, /^https:\/\/pay\.yamaletech\.com\/app\/#receive=[A-Za-z0-9_-]{43}$/);
  assert.match(linkFor(secret, true), /#receive=[A-Za-z0-9_-]{43}&code=1$/);
});

test('a code is 6 to 8 digits', () => {
  for (const ok of ['482913', '12345678']) assert.equal(validCode(ok), true);
  for (const bad of ['4821', '12345', '123456789', '12a456', '']) assert.equal(validCode(bad), false);
});
