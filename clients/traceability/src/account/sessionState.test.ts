import assert from 'node:assert/strict';
import { test } from 'node:test';

import { sessionAccountState } from './sessionState.ts';
import { writeImportedAccount, type KeyStore, type StoredAccount } from '../../../shared/account/device.ts';

const memory = (): KeyStore => {
  const data: Record<string, string> = {};
  return { getItem: (k) => data[k] ?? null, setItem: (k, v) => { data[k] = v; }, removeItem: (k) => { delete data[k]; } };
};
const A = 'yml1dzwpk73ntdyve2vx49flv0dxxguy6a5ygp2zv7';
const acct: StoredAccount = {
  address: A, email: 'a@example.com', createdAt: '2026-11-16T00:00:00.000Z', protection: 'none', sponsored: true,
  sealed: { salt: 'c2FsdA', nonce: 'bm9uY2U', ciphertext: 'Y2lwaGVy', time: 3, memory: 65536, threads: 1 },
};

test('the session state sees an account written after it first read, and starts locked', () => {
  const s = memory();
  const mount = sessionAccountState(s);
  assert.equal(mount.account, null);
  writeImportedAccount(acct, s);
  const refreshed = sessionAccountState(s);
  assert.equal(refreshed.account?.address, A);
  assert.equal(refreshed.unlocked, false);
});
