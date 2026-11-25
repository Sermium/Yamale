/**
 * Signing in on a new device, and setting up the second factor that makes it possible.
 *
 * Email, password and a code: the custodian checks the password and says which
 * code to ask for (emailing one when the account's factor is the email), then
 * each service checks the code itself and releases its sealed copy. The two
 * copies and the password open the key here, and nowhere else. See
 * tools/custodian/signin.go.
 *
 * An authenticator is enrolled with ONE secret, chosen here, at both services,
 * so the person keeps a single entry in their authenticator app.
 */

import { restore, ROLE_CUSTODIAN, ROLE_RECOVERY } from '../../../wallet/src/account/protect.ts';
import { canonicalEmail, type Holding } from '../../../wallet/src/account/deposit.ts';
import type { Services } from '../../../wallet/src/account/enrol.ts';
import { addressOfPublicKey, compressedPublicKey } from '../../../sdk/src/jewel.ts';

export type SignInFailure =
  | 'wrong'          // the email, password or code did not match
  | 'wait'           // too many wrong tries: the service asks to wait
  | 'recovery-open'  // a slow recovery is running for this account
  | 'frozen'         // the account was reported stolen
  | 'expired'        // the emailed code expired or was used: ask for another
  | 'unavailable'    // a service is down, or cannot check this factor
  | 'mismatch'       // the services disagree, or the copies open another key
  | 'refused';

export type Started =
  | { ok: true; factor: 'totp' }
  | { ok: true; factor: 'email'; nonce: string; issued: number }
  | { ok: false; reason: SignInFailure; retryAfter?: number; detail: string };

export type SignedIn =
  | { ok: true; privateKey: Uint8Array; address: string }
  | { ok: false; reason: SignInFailure; retryAfter?: number; detail: string };

export interface SignInAnswer {
  email: string;
  password: string;
  factor: 'totp' | 'email';
  answer: string;
  nonce?: string;
  issued?: number;
  device?: string;
  lang?: string;
}

function bytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function holdingFromWire(w: any): Holding {
  return {
    role: String(w.role), salt: bytes(w.salt), nonce: bytes(w.nonce),
    time: Number(w.time), memory: Number(w.memory), threads: Number(w.threads),
    share: { x: Number(w.share.x), y: bytes(w.share.y) },
  };
}

/** What a refusal means for the person, from its status and words. */
export function failureOf(status: number, error: string): SignInFailure {
  if (status === 0 || status === 503 || status >= 500) return 'unavailable';
  if (status === 429) return 'wait';
  if (status === 409) return 'recovery-open';
  if (status === 403) return 'frozen';
  if (status === 401 && /expired|already used/.test(error)) return 'expired';
  if (status === 401) return 'wrong';
  return 'refused';
}

async function post(fetchImpl: typeof fetch, url: string, body: unknown):
  Promise<{ status: number; body: any; retryAfter?: number }> {
  try {
    const res = await fetchImpl(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    const parsed = await res.json().catch(() => ({}));
    const ra = Number(res.headers.get('Retry-After'));
    return { status: res.status, body: parsed, ...(ra > 0 ? { retryAfter: ra } : {}) };
  } catch (err) {
    return { status: 0, body: { error: err instanceof Error ? err.message : String(err) } };
  }
}

function failed(r: { status: number; body: any; retryAfter?: number }) {
  const detail = String(r.body?.error ?? `http ${r.status}`);
  return { ok: false as const, reason: failureOf(r.status, detail), detail, ...(r.retryAfter ? { retryAfter: r.retryAfter } : {}) };
}

/** Checks the password at the custodian and learns which code to ask for. */
export async function startSignIn(services: Services, email: string, password: string, lang = '',
  fetchImpl: typeof fetch = fetch): Promise<Started> {
  const r = await post(fetchImpl, `${services.custodianUrl}/v1/signin/start`, { email: canonicalEmail(email), password, lang });
  if (r.status !== 200) return failed(r);
  if (r.body.factor === 'totp') return { ok: true, factor: 'totp' };
  if (r.body.factor === 'email' && typeof r.body.nonce === 'string' && Number.isInteger(r.body.issued)) {
    return { ok: true, factor: 'email', nonce: r.body.nonce, issued: r.body.issued };
  }
  return { ok: false, reason: 'refused', detail: 'the custodian named no factor this app knows' };
}

async function releaseOne(url: string, role: string, a: SignInAnswer, fetchImpl: typeof fetch):
  Promise<{ ok: true; address: string; holding: Holding } | Extract<SignedIn, { ok: false }>> {
  const r = await post(fetchImpl, `${url}/v1/signin`, {
    email: canonicalEmail(a.email), password: a.password, factor: a.factor, answer: a.answer.trim(),
    nonce: a.nonce ?? '', issued: a.issued ?? 0, device: a.device ?? '', lang: a.lang ?? '',
  });
  if (r.status !== 200) return failed(r);
  if (r.body.role !== role) return { ok: false, reason: 'mismatch', detail: 'a service answered as the wrong role' };
  try {
    return { ok: true, address: String(r.body.address), holding: holdingFromWire(r.body.holding) };
  } catch {
    return { ok: false, reason: 'refused', detail: 'the service sent a copy this app cannot read' };
  }
}

/** Both sealed copies, opened here with the password. */
export async function signIn(services: Services, a: SignInAnswer, fetchImpl: typeof fetch = fetch): Promise<SignedIn> {
  const custodian = await releaseOne(services.custodianUrl, ROLE_CUSTODIAN, a, fetchImpl);
  if (!custodian.ok) return custodian;
  const recovery = await releaseOne(services.recoveryUrl, ROLE_RECOVERY, a, fetchImpl);
  if (!recovery.ok) return recovery;
  if (custodian.address !== recovery.address) {
    return { ok: false, reason: 'mismatch', detail: 'the two services named different accounts' };
  }
  let privateKey: Uint8Array;
  try {
    privateKey = await restore([custodian.holding, recovery.holding], a.password);
  } catch (err) {
    return { ok: false, reason: 'refused', detail: err instanceof Error ? err.message : String(err) };
  }
  const address = addressOfPublicKey(compressedPublicKey(privateKey));
  if (address !== custodian.address) {
    privateKey.fill(0);
    return { ok: false, reason: 'mismatch', detail: 'the copies open a different key' };
  }
  return { ok: true, privateKey, address };
}

/* ------------------------------------------------------------ authenticator */

const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

/** RFC 4648 base32 without padding, the form authenticator apps take. */
export function base32(bytes: Uint8Array): string {
  let out = '', bits = 0, value = 0;
  for (const b of bytes) {
    value = (value << 8) | b; bits += 8;
    while (bits >= 5) { out += B32[(value >>> (bits - 5)) & 31]; bits -= 5; }
  }
  if (bits > 0) out += B32[(value << (5 - bits)) & 31];
  return out;
}

export type Begun = { ok: true; secret: string; uri: string } | { ok: false; reason: SignInFailure; detail: string };

/** Starts an authenticator at both services with one secret chosen here. Nothing counts until confirmed. */
export async function beginAuthenticator(services: Services, email: string, password: string,
  fetchImpl: typeof fetch = fetch, random: (n: number) => Uint8Array = (n) => crypto.getRandomValues(new Uint8Array(n))): Promise<Begun> {
  const secret = base32(random(20));
  let uri = '';
  for (const url of [services.custodianUrl, services.recoveryUrl]) {
    const r = await post(fetchImpl, `${url}/v1/factors/totp/begin`, { email: canonicalEmail(email), password, secret });
    if (r.status !== 200) return failed(r);
    if (r.body.secret !== secret) return { ok: false, reason: 'mismatch', detail: 'a service kept a different secret' };
    uri ||= String(r.body.uri);
  }
  return { ok: true, secret, uri };
}

/** Arms the authenticator at both services with one code read off it. */
export async function confirmAuthenticator(services: Services, email: string, password: string, code: string,
  fetchImpl: typeof fetch = fetch): Promise<{ ok: true } | { ok: false; reason: SignInFailure; detail: string }> {
  for (const url of [services.custodianUrl, services.recoveryUrl]) {
    const r = await post(fetchImpl, `${url}/v1/factors/totp/confirm`, { email: canonicalEmail(email), password, code: code.trim() });
    if (r.status !== 200) return failed(r);
  }
  return { ok: true };
}

/** A short description of this browser for the new-sign-in email, so its owner recognises it. */
export function deviceLabel(ua: string): string {
  const browser = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : 'a browser';
  const os = /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS' : /Windows/.test(ua) ? 'Windows' : /Mac OS X/.test(ua) ? 'macOS' : /Linux/.test(ua) ? 'Linux' : '';
  return os ? `${browser} on ${os}` : browser;
}
