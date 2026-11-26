import assert from 'node:assert/strict';
import { test } from 'node:test';

import { appOrigin, handoffUrl, receiveHandoff, sealForApp } from './handoff.ts';
import type { KeyStore, StoredAccount } from './device.ts';

function memory(): KeyStore & { map: Map<string, string> } {
  const map = new Map<string, string>();
  return { map, getItem: (k) => map.get(k) ?? null, setItem: (k, v) => { map.set(k, v); }, removeItem: (k) => { map.delete(k); } };
}

const account: StoredAccount = {
  address: 'yml1dzwpk73ntdyve2vx49flv0dxxguy6a5ygp2zv7', email: 'awa@example.test',
  sealed: { salt: 'c2FsdA', nonce: 'bm9uY2U', ciphertext: 'Y2lwaGVy', time: 3, memory: 65536, threads: 1 },
  createdAt: '2026-11-26T00:00:00Z', protection: 'enrolled', sponsored: false,
};

const PAY = 'https://pay.yamaletech.com';

/** The app's side up to the redirect: the one-time key, and the query the accounts page reads. */
function start(session: KeyStore) {
  const url = new URL(handoffUrl(`${PAY}/app/`, session));
  return { pk: url.searchParams.get('pk')!, state: url.searchParams.get('state')!, ret: url.searchParams.get('return')! };
}

test('only a Yamale app over https may receive an account', () => {
  assert.equal(appOrigin('https://pay.yamaletech.com/app/?x=1'), PAY);
  assert.equal(appOrigin('https://traceability.yamaletech.com/account'), 'https://traceability.yamaletech.com');
  assert.equal(appOrigin('https://evil.example/app/'), null);
  assert.equal(appOrigin('http://pay.yamaletech.com/app/'), null);
  assert.equal(appOrigin('https://pay.yamaletech.com.evil.example/'), null);
  assert.equal(appOrigin('not a url'), null);
});

test('an account sealed for the app opens in the tab that asked, and is stored there', () => {
  const session = memory();
  const store = memory();
  const { pk, state, ret } = start(session);
  assert.equal(appOrigin(ret), PAY);
  const fragment = sealForApp(account, pk, PAY, state);
  assert.ok(!fragment.includes(account.email), 'the fragment carries no readable account');
  const out = receiveHandoff('#' + fragment, PAY, session, store);
  assert.equal(out.outcome, 'imported');
  assert.equal(out.account?.address, account.address);
  assert.equal(JSON.parse(store.map.get('yamale.account')!).address, account.address);
  assert.equal(session.map.size, 0, 'the one-time key is gone');
});

test('the one-time key opens one hand-off only', () => {
  const session = memory();
  const { pk, state } = start(session);
  const fragment = sealForApp(account, pk, PAY, state);
  assert.equal(receiveHandoff('#' + fragment, PAY, session, memory()).outcome, 'imported');
  assert.equal(receiveHandoff('#' + fragment, PAY, session, memory()).outcome, 'failed');
});

test('a box sealed for another app, or altered, does not open', () => {
  const session = memory();
  const { pk, state } = start(session);
  const forWallet = sealForApp(account, pk, 'https://wallet.yamaletech.com', state);
  assert.equal(receiveHandoff('#' + forWallet, PAY, session, memory()).outcome, 'failed');

  const again = memory();
  const s2 = start(again);
  const good = sealForApp(account, s2.pk, PAY, s2.state);
  const params = new URLSearchParams(good);
  const box = params.get('yamale-handoff')!;
  params.set('yamale-handoff', box.slice(0, -2) + (box.endsWith('A') ? 'BB' : 'AA'));
  assert.equal(receiveHandoff('#' + params.toString(), PAY, again, memory()).outcome, 'failed');
});

test('an app already holding another account is told so, and keeps its own', () => {
  const session = memory();
  const store = memory();
  store.setItem('yamale.account', JSON.stringify({ ...account, address: 'yml1someoneelse' }));
  const { pk, state } = start(session);
  const out = receiveHandoff('#' + sealForApp(account, pk, PAY, state), PAY, session, store);
  assert.equal(out.outcome, 'exists');
  assert.equal(JSON.parse(store.map.get('yamale.account')!).address, 'yml1someoneelse');
});

test('a page with no hand-off in its fragment is left alone', () => {
  assert.equal(receiveHandoff('', PAY, memory(), memory()).outcome, 'none');
  assert.equal(receiveHandoff('#/j/1/2', PAY, memory(), memory()).outcome, 'none');
});

test('the open key travels only when it is the account’s own, and opens the account at once', async () => {
  const { secp256k1 } = await import('@noble/curves/secp256k1');
  const { addressOfPublicKey } = await import('../../sdk/src/jewel.ts');
  const key = new Uint8Array(32).fill(7);
  const mine = { ...account, address: addressOfPublicKey(secp256k1.getPublicKey(key, true)) };
  const session = memory();
  const { pk, state } = start(session);
  const out = receiveHandoff('#' + sealForApp(mine, pk, PAY, state, undefined, key), PAY, session, memory());
  assert.equal(out.outcome, 'imported');
  assert.deepEqual(out.account?.address, mine.address);
  assert.deepEqual(out.key, key);

  // A key that is not the account's is dropped: the app asks for the password instead.
  const s2 = memory();
  const r2 = start(s2);
  const wrong = receiveHandoff('#' + sealForApp(mine, r2.pk, PAY, r2.state, undefined, new Uint8Array(32).fill(9)), PAY, s2, memory());
  assert.equal(wrong.outcome, 'imported');
  assert.equal(wrong.key, undefined);
});
