/**
 * loyalty-api, as the app uses it. Every file is checked against its
 * fingerprint before anything is shown: a photo the API serves that does not
 * hash to what the chain recorded is reported as a mismatch, never displayed.
 */

import { sha256Hex } from '../../sdk/src/loyaltyauth.ts';

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
