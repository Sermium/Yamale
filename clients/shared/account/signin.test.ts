import assert from 'node:assert/strict';
import { test } from 'node:test';

import { protect, ROLE_CUSTODIAN, ROLE_RECOVERY } from '../../wallet/src/account/protect.ts';
import { addressOfPublicKey, compressedPublicKey, toBase64 } from '../../sdk/src/jewel.ts';
import { base32, beginAuthenticator, deviceLabel, failureOf, signIn, startSignIn } from './signin.ts';

const services = { custodianUrl: 'http://custodian', recoveryUrl: 'http://recovery' };
const key = new Uint8Array(32).fill(7);
const address = addressOfPublicKey(compressedPublicKey(key));

function wire(h: any) {
  return { role: h.role, salt: toBase64(h.salt), nonce: toBase64(h.nonce), time: h.time, memory: h.memory,
    threads: h.threads, share: { x: h.share.x, y: toBase64(h.share.y) } };
}

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) => new Response(JSON.stringify(body), { status, headers });

test('the custodian says which code to ask for; an emailed one comes with its nonce', async () => {
  let sent: any;
  const fetchImpl = (async (url: string, init: RequestInit) => {
    sent = { url, body: JSON.parse(String(init.body)) };
    return json({ factor: 'email', nonce: 'n'.repeat(24), issued: 1795600000 });
  }) as unknown as typeof fetch;
  const out = await startSignIn(services, 'Awa@Example.sn', 'the long password', 'fr', fetchImpl);
  assert.deepEqual(out, { ok: true, factor: 'email', nonce: 'n'.repeat(24), issued: 1795600000 });
  assert.equal(sent.url, 'http://custodian/v1/signin/start');
  assert.equal(sent.body.email, 'awa@example.sn');
  assert.equal(sent.body.lang, 'fr');
});

test('both released copies and the password give the key back, and both services see the same code', async () => {
  const holdings = await protect(key, 'the long password');
  const byRole = new Map(holdings.map((h) => [h.role, h]));
  const sent: any[] = [];
  const fetchImpl = (async (url: string, init: RequestInit) => {
    sent.push({ url, body: JSON.parse(String(init.body)) });
    const role = url.startsWith('http://custodian') ? ROLE_CUSTODIAN : ROLE_RECOVERY;
    return json({ role, address, holding: wire(byRole.get(role)) });
  }) as unknown as typeof fetch;
  const out = await signIn(services, { email: 'awa@example.sn', password: 'the long password', factor: 'email',
    answer: ' 123456 ', nonce: 'n'.repeat(24), issued: 1795600000, device: 'Firefox on Linux' }, fetchImpl);
  assert.ok(out.ok);
  assert.deepEqual(out.privateKey, key);
  assert.equal(out.address, address);
  assert.deepEqual(sent.map((s) => s.url), ['http://custodian/v1/signin', 'http://recovery/v1/signin']);
  assert.ok(sent.every((s) => s.body.answer === '123456' && s.body.nonce === 'n'.repeat(24)));
});

test('a refusal says what to do: wait, ask for a new code, or that the account is held', () => {
  assert.equal(failureOf(401, 'that email and password do not match an account'), 'wrong');
  assert.equal(failureOf(401, 'that code has expired or was already used; ask for a new one'), 'expired');
  assert.equal(failureOf(429, 'too many'), 'wait');
  assert.equal(failureOf(409, 'a recovery is open'), 'recovery-open');
  assert.equal(failureOf(403, 'frozen'), 'frozen');
  assert.equal(failureOf(503, 'down'), 'unavailable');
  assert.equal(failureOf(0, 'network'), 'unavailable');
});

test('a wait carries its Retry-After', async () => {
  const fetchImpl = (async () => json({ error: 'too many wrong passwords' }, 429, { 'Retry-After': '120' })) as unknown as typeof fetch;
  const out = await startSignIn(services, 'a@b.c', 'x'.repeat(10), '', fetchImpl);
  assert.equal(out.ok, false);
  assert.equal((out as { reason: string }).reason, 'wait');
  assert.equal((out as { retryAfter?: number }).retryAfter, 120);
});

test('copies naming different accounts are not trusted', async () => {
  const holdings = await protect(key, 'the long password');
  const byRole = new Map(holdings.map((h) => [h.role, h]));
  const fetchImpl = (async (url: string) => {
    const role = url.startsWith('http://custodian') ? ROLE_CUSTODIAN : ROLE_RECOVERY;
    return json({ role, address: role === ROLE_CUSTODIAN ? address : 'yml1somebodyelse', holding: wire(byRole.get(role)) });
  }) as unknown as typeof fetch;
  const out = await signIn(services, { email: 'a@b.c', password: 'the long password', factor: 'totp', answer: '1' }, fetchImpl);
  assert.equal((out as { reason: string }).reason, 'mismatch');
});

test('one authenticator secret is chosen here and enrolled at both services', async () => {
  const seen: string[] = [];
  const fetchImpl = (async (url: string, init: RequestInit) => {
    const body = JSON.parse(String(init.body));
    seen.push(`${url}:${body.secret}`);
    return json({ secret: body.secret, uri: `otpauth://totp/Yamale:a?secret=${body.secret}` });
  }) as unknown as typeof fetch;
  const out = await beginAuthenticator(services, 'a@b.c', 'the long password', fetchImpl, (n) => new Uint8Array(n).fill(1));
  assert.ok(out.ok);
  assert.equal(out.secret, base32(new Uint8Array(20).fill(1)));
  assert.deepEqual(seen, [`http://custodian/v1/factors/totp/begin:${out.secret}`, `http://recovery/v1/factors/totp/begin:${out.secret}`]);
});

test('base32 matches RFC 4648', () => {
  assert.equal(base32(new TextEncoder().encode('foobar')), 'MZXW6YTBOI');
  assert.equal(base32(new TextEncoder().encode('f')), 'MY');
});

test('the device label names browser and system only', () => {
  assert.equal(deviceLabel('Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/131.0 Mobile Safari/537.36'), 'Chrome on Android');
  assert.equal(deviceLabel('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131.0 Safari/537.36 Edg/131.0'), 'Edge on Windows');
  assert.equal(deviceLabel('Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1'), 'Safari on iOS');
});
