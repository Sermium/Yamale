/**
 * loyalty-api, as the app uses it. Every file is checked against its
 * fingerprint before anything is shown: a photo the API serves that does not
 * hash to what the chain recorded is reported as a mismatch, never displayed.
 */

import { sha256Hex, signedHeaders } from '../../sdk/src/loyaltyauth.ts';

// Not imported from config.ts: that module reads `window` when it loads, and
// record.ts, which Node's test runner loads, imports this one.
const API = '/api/loyalty';

export type Fetched =
  | { ok: true; bytes: Uint8Array; type: string }
  | { ok: false; reason: 'missing' | 'mismatch' | 'unreachable' };

export async function fetchVerified(hash: string): Promise<Fetched> {
  let res: Response;
  try {
    res = await fetch(`${API}/v1/files/${hash}`);
  } catch {
    return { ok: false, reason: 'unreachable' };
  }
  if (res.status === 404) return { ok: false, reason: 'missing' };
  if (!res.ok) return { ok: false, reason: 'unreachable' };
  const bytes = new Uint8Array(await res.arrayBuffer());
  if (sha256Hex(bytes) !== hash) return { ok: false, reason: 'mismatch' };
  return { ok: true, bytes, type: res.headers.get('Content-Type') ?? '' };
}

export interface JewelerProfile { logo: string; colour: string; accent: string }

export async function fetchProfile(jewelerId: string): Promise<JewelerProfile | null> {
  try {
    const res = await fetch(`${API}/v1/jewelers/${jewelerId}/profile`);
    if (!res.ok) return null;
    return (await res.json()) as JewelerProfile;
  } catch {
    return null;
  }
}

/** The platform's fee granter, or null when sponsoring is off or the API is down. */
export async function sponsorInfo(): Promise<string | null> {
  try {
    const res = await fetch(`${API}/v1/sponsor`);
    if (!res.ok) return null;
    return String((await res.json()).granter);
  } catch {
    return null;
  }
}

export async function requestSponsorship(key: Uint8Array, address: string, chainId: string):
  Promise<{ ok: true; granter: string } | { ok: false; status: number; error: string }> {
  try {
    const res = await fetch(`${API}/v1/sponsor`, {
      method: 'POST', headers: signedHeaders(key, 'sponsor', chainId, address),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, status: res.status, error: String(body.error ?? '') };
    return { ok: true, granter: String(body.granter) };
  } catch (err) {
    return { ok: false, status: 0, error: err instanceof Error ? err.message : String(err) };
  }
}

/**
 * Store a file under its fingerprint. The request is signed with the staff
 * member's key at once and the key is kept by nobody. A 503 means the API is
 * busy with other uploads and says nothing about this file: retry it.
 */
export async function uploadFile(key: Uint8Array, bytes: Uint8Array, chainId: string):
  Promise<{ ok: true; hash: string } | { ok: false; status: number; error: string }> {
  const hash = sha256Hex(bytes);
  try {
    const res = await fetch(`${API}/v1/files`, {
      method: 'POST', headers: signedHeaders(key, 'file', chainId, hash), body: bytes as Uint8Array<ArrayBuffer>,
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, status: res.status, error: String(body.error ?? '') };
    if (body.hash !== hash) return { ok: false, status: 500, error: 'the API stored different bytes' };
    return { ok: true, hash };
  } catch (err) {
    return { ok: false, status: 0, error: err instanceof Error ? err.message : String(err) };
  }
}
