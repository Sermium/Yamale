import assert from 'node:assert/strict';
import { test } from 'node:test';

import { addressOfPublicKey, compressedPublicKey } from '../../../sdk/src/jewel.ts';
import { forgetLinks, isAccountCode, rememberLink, storedLink } from './links.ts';

function memory(): Storage {
  const m = new Map<string, string>();
  return {
    get length() { return m.size; },
    key: (i: number) => [...m.keys()][i] ?? null,
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
    removeItem: (k: string) => void m.delete(k),
    clear: () => m.clear(),
  };
}

test('an account code is a yml address of 20 bytes, and nothing else is', () => {
  const address = addressOfPublicKey(compressedPublicKey(new Uint8Array(32).fill(5)));
  assert.equal(isAccountCode(address), true);
  assert.equal(isAccountCode(`  ${address}\n`), true);
  assert.equal(isAccountCode(address.replace(/^yml/, 'cosmos')), false);
  assert.equal(isAccountCode('yml1notanaddress'), false);
  assert.equal(isAccountCode(''), false);
});

test('a claim link is kept per offer, and forgotten with the jewel', () => {
  const store = memory();
  rememberLink('4', 'aa', 'https://x/claim#4.k1', store);
  rememberLink('4', 'bb', 'https://x/claim#4.k2', store);
  rememberLink('5', 'cc', 'https://x/claim#5.k3', store);
  assert.equal(storedLink('4', 'bb', store), 'https://x/claim#4.k2');
  forgetLinks('4', store);
  assert.equal(storedLink('4', 'aa', store), null);
  assert.equal(storedLink('5', 'cc', store), 'https://x/claim#5.k3');
});
