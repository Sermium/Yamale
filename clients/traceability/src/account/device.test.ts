import assert from 'node:assert/strict';
import { test } from 'node:test';

import { createAccount, forgetAccount, readAccount, unlockAccount, updateAccount, WrongPassword, type KeyStore } from './device.ts';

function memory(): KeyStore {
  const m = new Map<string, string>();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => void m.set(k, v), removeItem: (k) => void m.delete(k) };
}
const fixedKey = () => new Uint8Array(32).fill(7);

test('an account is made on the device, and opens with its password only', async () => {
  const store = memory();
  const { account, privateKey } = await createAccount('Awa@Example.sn', 'correct horse', store, fixedKey);
  assert.match(account.address, /^yml1/);
  assert.equal(account.email, 'awa@example.sn');
  assert.deepEqual(readAccount(store), account);
  assert.deepEqual(await unlockAccount(account, 'correct horse'), privateKey);
  await assert.rejects(unlockAccount(account, 'wrong horse!'), WrongPassword);
});

test('one account per device, and a short password is refused', async () => {
  const store = memory();
  await assert.rejects(createAccount('a@b.c', 'short', store, fixedKey), /at least 10/);
  await createAccount('a@b.c', 'long enough!', store, fixedKey);
  await assert.rejects(createAccount('a@b.c', 'long enough!', store, fixedKey), /already/);
});

test('flags update in place, and forgetting removes everything', async () => {
  const store = memory();
  await createAccount('a@b.c', 'long enough!', store, fixedKey);
  assert.equal(updateAccount({ sponsored: true }, store).sponsored, true);
  assert.equal(readAccount(store)?.sponsored, true);
  forgetAccount(store);
  assert.equal(readAccount(store), null);
});
