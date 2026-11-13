import assert from 'node:assert/strict';
import { test } from 'node:test';

import { protect, ROLE_CUSTODIAN, ROLE_RECOVERY } from '../../../wallet/src/account/protect.ts';
import { addressOfPublicKey, compressedPublicKey, toBase64 } from '../../../sdk/src/jewel.ts';
import { claimRecovered } from './recover.ts';

const services = { custodianUrl: 'http://custodian', recoveryUrl: 'http://recovery' };
const key = new Uint8Array(32).fill(9);
const address = addressOfPublicKey(compressedPublicKey(key));

function wire(h: any) {
  return { role: h.role, salt: toBase64(h.salt), nonce: toBase64(h.nonce), time: h.time, memory: h.memory,
    threads: h.threads, share: { x: h.share.x, y: toBase64(h.share.y) } };
}

test('two released holdings and the password give the key back', async () => {
  const holdings = await protect(key, 'the long password');
  const byRole = new Map(holdings.map((h) => [h.role, h]));
  const sent: any[] = [];
  const fetchImpl = (async (url: string, init: RequestInit) => {
    sent.push(JSON.parse(String(init.body)));
    const role = url.startsWith('http://custodian') ? ROLE_CUSTODIAN : ROLE_RECOVERY;
    return new Response(JSON.stringify({ role, address, holding: wire(byRole.get(role)) }), { status: 200 });
  }) as unknown as typeof fetch;

  const out = await claimRecovered({ recoveryId: 'r-1', email: 'Awa@Example.sn', password: 'the long password',
    factor: 'email', answer: '123456' }, services, fetchImpl);
  assert.ok(out.ok);
  assert.deepEqual(out.privateKey, key);
  assert.equal(out.address, address);
  assert.equal(sent[0].id, 'r-1');
  assert.equal(sent[0].factor, 'email');
});

test('a recovery still inside its hold says so', async () => {
  const fetchImpl = (async () => new Response(JSON.stringify({ error: 'released after 24h' }),
    { status: 409, headers: { 'Retry-After': '3600' } })) as unknown as typeof fetch;
  const out = await claimRecovered({ recoveryId: 'r', email: 'a@b.c', password: 'x'.repeat(10), factor: 'email', answer: '1' },
    services, fetchImpl);
  assert.equal(out.ok, false);
  assert.equal((out as { reason: string }).reason, 'held');
});

test('services that disagree about the account are not trusted', async () => {
  const holdings = await protect(key, 'the long password');
  const byRole = new Map(holdings.map((h) => [h.role, h]));
  const fetchImpl = (async (url: string) => {
    const role = url.startsWith('http://custodian') ? ROLE_CUSTODIAN : ROLE_RECOVERY;
    const claimed = role === ROLE_CUSTODIAN ? address : 'yml1somebodyelse';
    return new Response(JSON.stringify({ role, address: claimed, holding: wire(byRole.get(role)) }), { status: 200 });
  }) as unknown as typeof fetch;
  const out = await claimRecovered({ recoveryId: 'r', email: 'a@b.c', password: 'the long password', factor: 'email', answer: '1' },
    services, fetchImpl);
  assert.equal((out as { reason: string }).reason, 'mismatch');
});
