/**
 * traceability-api, as the app uses it. Every file is checked against its
 * fingerprint before anything is shown: a photo the API serves that does not
 * hash to what the chain recorded is reported as a mismatch, never displayed.
 */

import { sha256Hex, signedHeaders, type TraceabilityPurpose } from '../../sdk/src/traceabilityauth.ts';
import type { Stone } from './verify/record.ts';

// Not imported from config.ts: that module reads `window` when it loads, and
// record.ts, which Node's test runner loads, imports this one.
const API = '/api/traceability';

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

const HEX_COLOUR = /^#[0-9a-f]{6}$/;
function safeColour(v: unknown): string { return typeof v === 'string' && HEX_COLOUR.test(v) ? v : ''; }

export interface JewelerProfile { logo: string; colour: string; accent: string }

export async function fetchProfile(jewelerId: string): Promise<JewelerProfile | null> {
  try {
    const res = await fetch(`${API}/v1/jewelers/${jewelerId}/profile`);
    if (!res.ok) return null;
    const p = (await res.json()) as JewelerProfile;
    // Colours go into CSS: only a plain hex colour is used, anything else is ignored.
    return { ...p, colour: safeColour(p.colour), accent: safeColour(p.accent) };
  } catch {
    return null;
  }
}

export type ProfileRead =
  | { state: 'loaded'; profile: JewelerProfile }
  | { state: 'none' }
  | { state: 'failed'; detail: string };

/** Like fetchProfile, but a missing profile (404) is told apart from a failure to read one. */
export async function readProfile(jewelerId: string): Promise<ProfileRead> {
  try {
    const res = await fetch(`${API}/v1/jewelers/${jewelerId}/profile`);
    if (res.status === 404) return { state: 'none' };
    if (!res.ok) return { state: 'failed', detail: `${res.status}` };
    const p = (await res.json()) as JewelerProfile;
    if (typeof p.colour !== 'string' || typeof p.accent !== 'string') return { state: 'failed', detail: 'unreadable profile' };
    return { state: 'loaded', profile: { logo: String(p.logo ?? ''), colour: p.colour, accent: p.accent } };
  } catch (err) {
    return { state: 'failed', detail: err instanceof Error ? err.message : String(err) };
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

async function signedSend(method: 'PUT' | 'POST', path: string, key: Uint8Array, purpose: TraceabilityPurpose,
  subject: string, body: Uint8Array, chainId: string): Promise<{ ok: true } | { ok: false; status: number; error: string }> {
  try {
    const res = await fetch(`${API}${path}`, {
      method, headers: { ...signedHeaders(key, purpose, chainId, subject), 'Content-Type': 'application/json' },
      body: body as Uint8Array<ArrayBuffer>,
    });
    if (res.ok) return { ok: true };
    const b = await res.json().catch(() => ({}));
    return { ok: false, status: res.status, error: String(b.error ?? '') };
  } catch (err) {
    return { ok: false, status: 0, error: err instanceof Error ? err.message : String(err) };
  }
}

/** The jeweler's look, signed by its business account. The logo must already be stored. */
export function saveProfile(key: Uint8Array, jewelerId: string, profile: JewelerProfile, chainId: string) {
  const body = new TextEncoder().encode(JSON.stringify({ logo: profile.logo, colour: profile.colour, accent: profile.accent }));
  return signedSend('PUT', `/v1/jewelers/${jewelerId}/profile`, key, 'profile', `${jewelerId}.${sha256Hex(body)}`, body, chainId);
}

export interface Application {
  business: string; name: string; contact: string; email: string; phone: string; country: string; message: string;
}

/** An application, signed by the account that will be the business. It admits nobody. */
export function submitApplication(key: Uint8Array, a: Application, chainId: string) {
  const body = new TextEncoder().encode(JSON.stringify({
    business: a.business, name: a.name, contact: a.contact, email: a.email, phone: a.phone, country: a.country, message: a.message,
  }));
  return signedSend('POST', '/v1/applications', key, 'application', sha256Hex(body), body, chainId);
}

export type ApiResult<T> = { ok: true; value: T } | { ok: false; status: number; error: string; code: string };

export interface DesignFields { metal: string; weightGrams: string; stones: Stone[] }
export interface Design {
  id: string; key: string; title: string; description: string; type: string; tags: string[];
  productUrl: string; listPrice: { amount: string; currency: string } | null; imageUrls: string[];
  draft: DesignFields | null; draftFor: string; draftNote: '' | 'unavailable' | 'paused';
  fields: DesignFields; sourceHash: string;
  status: 'imported' | 'drafted' | 'changed' | 'confirmed' | 'gone';
  confirmed: { title: string; fields: DesignFields; photos: string[]; sourceHash: string; by: string; at: string } | null;
}
export interface ImportJob { state: 'idle' | 'reading' | 'drafting' | 'done' | 'failed'; done: number; of: number; error: string }
export interface Catalogue { origin: string; importedAt: string; designs: Design[]; job: ImportJob }

async function apiCall<T>(path: string, init: RequestInit, read: (res: Response) => Promise<T>): Promise<ApiResult<T>> {
  try {
    const res = await fetch(`${API}${path}`, init);
    if (res.ok) return { ok: true, value: await read(res) };
    const b = await res.json().catch(() => ({}));
    return { ok: false, status: res.status, error: String(b.error ?? ''), code: String(b.code ?? '') };
  } catch (err) {
    return { ok: false, status: 0, error: err instanceof Error ? err.message : String(err), code: '' };
  }
}

const utf8 = new TextEncoder();
const json = (res: Response) => res.json();

export function readCatalogue(key: Uint8Array, jewelerId: string, chainId: string): Promise<ApiResult<Catalogue>> {
  return apiCall('/v1/catalogue', { headers: signedHeaders(key, 'catalogue-read', chainId, jewelerId) }, json);
}

export function startImport(key: Uint8Array, origin: string, chainId: string): Promise<ApiResult<ImportJob>> {
  const body = utf8.encode(JSON.stringify({ origin }));
  return apiCall('/v1/catalogue/import', {
    method: 'POST', headers: { ...signedHeaders(key, 'catalogue-import', chainId, sha256Hex(utf8.encode(origin))), 'Content-Type': 'application/json' },
    body: body as Uint8Array<ArrayBuffer>,
  }, json);
}

export function saveDesign(key: Uint8Array, d: Design, title: string, fields: DesignFields, chainId: string): Promise<ApiResult<Design>> {
  const body = utf8.encode(JSON.stringify({ title, fields: { metal: fields.metal, weightGrams: fields.weightGrams, stones: fields.stones } }));
  return apiCall(`/v1/catalogue/designs/${d.id}`, {
    method: 'PUT', headers: { ...signedHeaders(key, 'design-save', chainId, sha256Hex(new Uint8Array([...utf8.encode(`${d.key}:`), ...body]))), 'Content-Type': 'application/json' },
    body: body as Uint8Array<ArrayBuffer>,
  }, json);
}

/** The subject binds the key, the website text that was checked, and every byte of the confirmation. */
export function confirmDesign(key: Uint8Array, d: Design, c: { title: string; fields: DesignFields; photos: string[] }, chainId: string): Promise<ApiResult<Design>> {
  const body = utf8.encode(JSON.stringify({
    sourceHash: d.sourceHash, title: c.title,
    fields: { metal: c.fields.metal, weightGrams: c.fields.weightGrams, stones: c.fields.stones },
    photos: c.photos, rightsConfirmed: true,
  }));
  const prefix = utf8.encode(`${d.key}:${d.sourceHash}:`);
  const signed = new Uint8Array(prefix.length + body.length);
  signed.set(prefix);
  signed.set(body, prefix.length);
  return apiCall(`/v1/catalogue/designs/${d.id}/confirm`, {
    method: 'POST', headers: { ...signedHeaders(key, 'design-confirm', chainId, sha256Hex(signed)), 'Content-Type': 'application/json' },
    body: body as Uint8Array<ArrayBuffer>,
  }, json);
}

export function fetchDesignImage(key: Uint8Array, jewelerId: string, id: string, n: number, chainId: string): Promise<ApiResult<Blob>> {
  return apiCall(`/v1/catalogue/designs/${id}/images/${n}`, { headers: signedHeaders(key, 'catalogue-read', chainId, jewelerId) }, (res) => res.blob());
}
