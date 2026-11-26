/**
 * The accounts API: what follows an account across every Yamale app.
 *
 * Every app reaches it on its own origin, at /api/accounts (the gate routes it
 * to one service on yamale-us), so the jewellery app, pay and the others read
 * and write the same record. Nothing here is on the chain:
 *
 *   - the profile: first and last name, readable by any account of the apps
 *     next to the identifier, and a date of birth returned to its owner alone;
 *   - onboarding: on devnet, the platform's participant records the country
 *     the person chose, which x/alias needs before it issues the identifier.
 *
 * Every request is signed with the account's own key (sdk traceabilityauth),
 * so a record can only be written by the account it belongs to.
 */

import { sha256Hex, signedHeaders } from '../../sdk/src/traceabilityauth.ts';

export const ACCOUNTS_API = '/api/accounts';

export interface Person { first_name: string; last_name: string; birth_date?: string }
export type Result<T> = { ok: true; value: T } | { ok: false; status: number; error: string };

const utf8 = new TextEncoder();

async function call<T>(path: string, init: RequestInit, fetchImpl: typeof fetch): Promise<Result<T>> {
  try {
    const res = await fetchImpl(`${ACCOUNTS_API}${path}`, init);
    const body = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, status: res.status, error: String((body as { error?: unknown }).error ?? '') };
    return { ok: true, value: body as T };
  } catch (err) {
    return { ok: false, status: 0, error: err instanceof Error ? err.message : String(err) };
  }
}

/** This account's own record, date of birth included; null when it has none yet. */
export async function readMyPerson(key: Uint8Array, address: string, chainId: string,
  fetchImpl: typeof fetch = fetch): Promise<Result<Person | null>> {
  const r = await call<Person>('/v1/people/me', { headers: signedHeaders(key, 'person-read', chainId, address) }, fetchImpl);
  return !r.ok && r.status === 404 ? { ok: true, value: null } : r;
}

/** Save this account's name and date of birth, signed over the address and the exact body. */
export async function savePerson(key: Uint8Array, address: string, p: Person, chainId: string,
  fetchImpl: typeof fetch = fetch): Promise<Result<Person>> {
  const bytes = utf8.encode(JSON.stringify(p));
  return call<Person>('/v1/people/me', {
    method: 'PUT',
    headers: { ...signedHeaders(key, 'person-save', chainId, `${address}.${sha256Hex(bytes)}`), 'Content-Type': 'application/json' },
    body: bytes as Uint8Array<ArrayBuffer>,
  }, fetchImpl);
}

/** The name behind another account; null when it has none or cannot be read. */
export async function lookupPerson(key: Uint8Array, target: string, chainId: string,
  fetchImpl: typeof fetch = fetch): Promise<{ first_name: string; last_name: string } | null> {
  const r = await call<{ first_name: string; last_name: string }>(`/v1/people/${target}`,
    { headers: signedHeaders(key, 'person-lookup', chainId, target) }, fetchImpl);
  return r.ok ? r.value : null;
}

/** "Awa Diop", or null. */
export const fullName = (p: { first_name: string; last_name: string } | null): string | null =>
  p ? `${p.first_name} ${p.last_name}`.trim() || null : null;

/**
 * Ask the onboarding participant to record this account's country, so the
 * chain can issue its identifier. Devnet only; signed over "<address>:<country>".
 */
export async function requestOnboarding(key: Uint8Array, address: string, country: string, chainId: string,
  fetchImpl: typeof fetch = fetch): Promise<{ ok: true; participant: string } | { ok: false; status: number; error: string }> {
  const r = await call<{ participant: string }>('/v1/onboard', {
    method: 'POST',
    headers: { ...signedHeaders(key, 'onboard', chainId, `${address}:${country}`), 'Content-Type': 'application/json' },
    body: JSON.stringify({ country }),
  }, fetchImpl);
  return r.ok ? { ok: true, participant: String(r.value.participant) } : r;
}
