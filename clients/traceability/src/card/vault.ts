/**
 * The card vault, the browser's half: every member of a shop's staff can find
 * its ownership cards again, on any device, without the server ever being able
 * to read one.
 *
 * A card's secret is sealed to each member's public key: an ephemeral
 * secp256k1 key agrees a secret with the member's key, HKDF-SHA256 turns it
 * into a ChaCha20-Poly1305 key bound to the piece and the member, and the box
 * is the ephemeral public key, the nonce and the ciphertext (93 bytes). Only
 * the member's own account key opens it.
 *
 * Three checks keep a dishonest or confused server from doing harm:
 *   - a member's key is used only if it hashes to the member's address;
 *   - a box is accepted only if it opens to the secret of the card key
 *     recorded beside it;
 *   - a card this browser holds is never replaced by one from the vault unless
 *     that one opens and verifies; a newer card in the vault wins over an older
 *     one kept here, and a card just issued here wins over the vault.
 */

import { secp256k1 } from '@noble/curves/secp256k1';
import { chacha20poly1305 } from '@noble/ciphers/chacha.js';
import { hkdf } from '@noble/hashes/hkdf.js';
import { sha256 } from '@noble/hashes/sha2.js';

import { addressOfPublicKey, compressedPublicKey, toHex } from '../../../sdk/src/jewel.ts';

export interface VaultMember { address: string; key: string }
export interface VaultEntry { card_key: string; box?: string; holders: string[] }
export interface VaultRead { members: VaultMember[]; cards: Record<string, VaultEntry> }
export interface VaultUpload { jewelId: string; body: { card_key: string; boxes: Record<string, string> } }

const utf8 = new TextEncoder();
const info = (jewelId: string, recipient: string) => utf8.encode(`yamale-traceability/cardvault/v1:${jewelId}:${recipient}`);

function b64(bytes: Uint8Array): string {
  let s = '';
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}

function unb64(s: string): Uint8Array | null {
  try {
    const raw = atob(s);
    return Uint8Array.from(raw, (c) => c.charCodeAt(0));
  } catch {
    return null;
  }
}

/** The card key the chain holds for a card secret: its compressed public key in hex. */
export const cardKeyOf = (secret: Uint8Array): string => toHex(compressedPublicKey(secret));

/** A member's key, if it really is the key of the address it is listed for. */
export function memberKey(m: VaultMember): Uint8Array | null {
  const key = unb64(m.key);
  if (!key || key.length !== 33) return null;
  try {
    return addressOfPublicKey(key) === m.address ? key : null;
  } catch {
    return null;
  }
}

export function sealCard(secret: Uint8Array, recipientKey: Uint8Array, jewelId: string, recipient: string,
  random: (n: number) => Uint8Array = (n) => crypto.getRandomValues(new Uint8Array(n))): string {
  const ephemeral = random(32);
  const epk = secp256k1.getPublicKey(ephemeral, true);
  const shared = secp256k1.getSharedSecret(ephemeral, recipientKey, true).slice(1);
  const key = hkdf(sha256, shared, epk, info(jewelId, recipient), 32);
  const nonce = random(12);
  const ct = chacha20poly1305(key, nonce, epk).encrypt(secret);
  const out = new Uint8Array(33 + 12 + ct.length);
  out.set(epk, 0);
  out.set(nonce, 33);
  out.set(ct, 45);
  return b64(out);
}

/** The card secret in a box, or null when it does not open with this key. */
export function openCard(box: string, accountKey: Uint8Array, jewelId: string, me: string): Uint8Array | null {
  const raw = unb64(box);
  if (!raw || raw.length !== 93) return null;
  try {
    const epk = raw.slice(0, 33);
    const shared = secp256k1.getSharedSecret(accountKey, epk, true).slice(1);
    const key = hkdf(sha256, shared, epk, info(jewelId, me), 32);
    return chacha20poly1305(key, raw.slice(33, 45), epk).decrypt(raw.slice(45));
  } catch {
    return null;
  }
}

/**
 * Which card each piece should use, from what this browser keeps and what the
 * vault holds. `open` opens this member's box for a piece. A vault card is used
 * only if it opens and matches its recorded key; it replaces a kept one that
 * differs (the card was issued again elsewhere) unless the kept one was just
 * issued here.
 */
export function reconcile(read: VaultRead, jewelIds: string[], kept: (id: string) => Uint8Array | null,
  open: (box: string, jewelId: string) => Uint8Array | null, fresh: ReadonlySet<string> = new Set()): Map<string, Uint8Array> {
  const out = new Map<string, Uint8Array>();
  for (const id of jewelIds) {
    const local = kept(id);
    if (local && fresh.has(id)) { out.set(id, local); continue; }
    const entry = read.cards[id];
    const opened = entry?.box ? open(entry.box, id) : null;
    if (opened && cardKeyOf(opened) === entry!.card_key) { out.set(id, opened); continue; }
    // Kept here and not contradicted by the vault: either the vault has no
    // card for the piece, or it holds this same one.
    if (local && (!entry || entry.card_key === cardKeyOf(local))) out.set(id, local);
  }
  return out;
}

/** The boxes to upload: for each card known here, one per member who lacks it and whose key checks out. */
export function plan(read: VaultRead, cards: Map<string, Uint8Array>,
  seal: (secret: Uint8Array, key: Uint8Array, jewelId: string, recipient: string) => string): VaultUpload[] {
  const members = read.members.flatMap((m) => {
    const key = memberKey(m);
    return key ? [{ address: m.address, key }] : [];
  });
  const uploads: VaultUpload[] = [];
  for (const [jewelId, secret] of cards) {
    const card_key = cardKeyOf(secret);
    const entry = read.cards[jewelId];
    // A different key in the vault is replaced by this one (reconcile kept it, so it is the newer).
    const holders = new Set(entry && entry.card_key === card_key ? entry.holders : []);
    const boxes: Record<string, string> = {};
    for (const m of members) {
      if (!holders.has(m.address)) boxes[m.address] = seal(secret, m.key, jewelId, m.address);
    }
    // Sent when a member lacks the card, or when the vault does not yet record
    // this card key at all (it then replaces whatever it held for the piece).
    if (Object.keys(boxes).length > 0 || !entry || entry.card_key !== card_key) {
      uploads.push({ jewelId, body: { card_key, boxes } });
    }
  }
  return uploads;
}
