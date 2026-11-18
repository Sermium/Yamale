/**
 * The app moved from loyalty.yamaletech.com to traceability.yamaletech.com.
 * A browser keeps an account under the site's own address, so the old address
 * serves a page (yamale-platform www/loyalty-move) that reads the sealed
 * account there and opens /move-in#move=…&to=… here. The fragment is never
 * sent to any server. The account is sealed under its owner's password
 * (device.ts), so what travels cannot be opened without it.
 */
import { readAccount, writeImportedAccount, type KeyStore, type StoredAccount } from './device.ts';

export type MoveOutcome =
  | { kind: 'stored'; address: string; to: string }
  | { kind: 'same'; address: string; to: string }
  | { kind: 'other'; here: string; incoming: string; to: string }
  | { kind: 'invalid'; to: string };

export function safeTarget(raw: string | null): string {
  if (!raw || !raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return '/';
  return raw;
}

function fromB64u(s: string | null): string | null {
  if (!s || !/^[A-Za-z0-9_-]+$/.test(s)) return null;
  try {
    const bin = atob(s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (s.length % 4)) % 4));
    return new TextDecoder('utf-8', { fatal: true }).decode(Uint8Array.from(bin, (c) => c.charCodeAt(0)));
  } catch { return null; }
}

function looksSealed(v: unknown): v is StoredAccount {
  const a = v as StoredAccount;
  const w = a?.sealed;
  return typeof a?.address === 'string' && /^yml1[02-9ac-hj-np-z]{38}$/.test(a.address)
    && typeof w?.salt === 'string' && typeof w?.nonce === 'string' && typeof w?.ciphertext === 'string'
    && Number.isInteger(w?.time) && Number.isInteger(w?.memory) && Number.isInteger(w?.threads);
}

export function acceptMove(fragment: string, store: KeyStore): MoveOutcome {
  const params = new URLSearchParams(fragment.replace(/^#/, ''));
  const to = safeTarget(fromB64u(params.get('to')));
  const text = fromB64u(params.get('move'));
  let incoming: unknown;
  try { incoming = text === null ? null : JSON.parse(text); } catch { incoming = null; }
  if (!looksSealed(incoming)) return { kind: 'invalid', to };
  const here = readAccount(store);
  if (here) {
    return here.address === incoming.address
      ? { kind: 'same', address: here.address, to }
      : { kind: 'other', here: here.address, incoming: incoming.address, to };
  }
  try { writeImportedAccount(incoming, store); } catch { return { kind: 'invalid', to }; }
  return { kind: 'stored', address: incoming.address, to };
}
