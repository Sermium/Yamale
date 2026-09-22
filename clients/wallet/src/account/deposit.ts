/**
 * The deposit proof — the browser's half.
 *
 * Mirrors `recovery/deposit.go`. The device signs this to prove it holds the
 * key for the address it claims, and that it meant THIS holding to go to THIS
 * service for THIS account.
 *
 * What it proves and what it does not is argued in the Go file. The short
 * version: it closes substitution by anything between the device and a service,
 * and it leaves open a broken client depositing a well-formed holding of the
 * wrong key — which loses that person their own account and is not a route to
 * anybody else's.
 */

import { Secp256k1, Sha256 } from '@cosmjs/crypto';

import type { Share } from './shamir.ts';

const DEPOSIT_DOMAIN = 'yamale/recovery/deposit/v1';

/**
 * The one spelling of an address this system uses.
 *
 * ASCII case folding only, and that is not laziness. `toLowerCase()` and Go's
 * `strings.ToLower` disagree on a handful of characters — the Turkish dotted I
 * is the famous one — so a device folding one way and a service folding the
 * other would sign a digest the service could not reproduce. Domains are ASCII
 * or punycode and local parts are case-sensitive per RFC 5321 anyway, so
 * folding only ASCII is both portable and more correct.
 *
 * Mirrors `recovery.CanonicalEmail`. A vector pins the two together.
 */
export function canonicalEmail(email: string): string {
  let out = '';
  for (const ch of email.trim()) {
    const c = ch.charCodeAt(0);
    out += c >= 0x41 && c <= 0x5a ? String.fromCharCode(c + 32) : ch;
  }
  return out;
}

/** Fixed r||s, 32 bytes each — not DER. */
export const SIGNATURE_LEN = 64;

/** What one party stores. Field names match the Go JSON exactly. */
export interface Holding {
  role: string;
  salt: Uint8Array;
  nonce: Uint8Array;
  time: number;
  memory: number;
  threads: number;
  share: Share;
}

/**
 * What the device signs.
 *
 * Every field is length-prefixed. Plain concatenation would let one deposit's
 * fields be re-cut into another's — an index of "ab" with a role of "c" hashing
 * the same as an index of "a" with a role of "bc" — and binding a specific set
 * of values together is the entire job of this digest.
 */
export function depositDigest(
  email: string,
  role: string,
  pubKey: Uint8Array,
  holding: Holding,
): Uint8Array {
  const params = new Uint8Array(9);
  const pv = new DataView(params.buffer);
  pv.setUint32(0, holding.time, false);
  pv.setUint32(4, holding.memory, false);
  params[8] = holding.threads;

  const parts: Uint8Array[] = [
    utf8(DEPOSIT_DOMAIN),
    utf8(canonicalEmail(email)),
    utf8(role),
    pubKey,
    holding.salt,
    holding.nonce,
    params,
    Uint8Array.of(holding.share.x),
    holding.share.y,
  ];

  let total = 0;
  for (const p of parts) total += 4 + p.length;
  const buf = new Uint8Array(total);
  const view = new DataView(buf.buffer);
  let at = 0;
  for (const p of parts) {
    view.setUint32(at, p.length, false);
    at += 4;
    buf.set(p, at);
    at += p.length;
  }
  return new Sha256(buf).digest();
}

/**
 * Sign a deposit.
 *
 * `Secp256k1.createSignature` returns a signature in the low-S normalised form
 * the chain also requires, and `toFixedLength()` gives the 64 bytes the Go side
 * parses. DER is deliberately not used: its length prefixes are a well-worn
 * source of parsers that accept two encodings of one signature.
 */
export async function signDeposit(
  privateKey: Uint8Array,
  email: string,
  role: string,
  holding: Holding,
): Promise<Uint8Array> {
  const { pubkey } = await Secp256k1.makeKeypair(privateKey);
  const compressed = Secp256k1.compressPubkey(pubkey);
  const digest = depositDigest(email, role, compressed, holding);
  const signature = await Secp256k1.createSignature(digest, privateKey);
  // r and s, 32 bytes each, no recovery byte — which toFixedLength appends.
  return signature.toFixedLength().slice(0, SIGNATURE_LEN);
}

/** The compressed public key for a private key, which is what a deposit carries. */
export async function publicKeyOf(privateKey: Uint8Array): Promise<Uint8Array> {
  const { pubkey } = await Secp256k1.makeKeypair(privateKey);
  return Secp256k1.compressPubkey(pubkey);
}

function utf8(s: string): Uint8Array {
  return new TextEncoder().encode(s);
}
