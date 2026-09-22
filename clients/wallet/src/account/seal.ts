/**
 * Sealing a key under a password — the browser's half.
 *
 * Mirrors `recovery/seal.go`, and the Go file is where the argument for the
 * construction lives. What belongs here is the part that is specific to running
 * in a browser, because that is what constrained the design on both sides.
 *
 * # crypto.subtle does not exist here
 *
 * The devnet is served over plain HTTP from `http://10.0.0.188:8092`, so
 * `crypto.subtle` is `undefined` — it is gated on a secure context. An earlier
 * version of ../vault.ts failed exactly this way, with "Cannot read properties
 * of undefined", and the failure looks arbitrary because
 * `crypto.getRandomValues` is NOT gated and keeps working.
 *
 * So everything cryptographic comes from @cosmjs/crypto, which ships its own
 * libsodium and needs no secure context. That is also what the existing wallet
 * vault uses, so it is a proven path in this exact deployment rather than a
 * hopeful one.
 *
 * # Two limits libsodium imposes, which shaped the Go side
 *
 *   - Argon2id runs at PARALLELISM 1 and offers no way to set it. The Go seal
 *     was written with 4 and had to be changed: a blob the browser cannot
 *     reproduce is a blob nobody can ever open.
 *   - the salt must be EXACTLY 16 bytes. The domain separator is therefore
 *     hashed in rather than prepended — see argonSalt, which mirrors the Go
 *     function of the same name.
 *
 * And one the AEAD imposes: `Xchacha20poly1305Ietf` takes no associated data,
 * so the seal uses none. Nothing is unbound by that — the domain is inside the
 * Argon2 salt and the parameters are inputs to the derivation — but it is the
 * reason the Go side passes nil where an AAD would naturally go.
 */

import { Argon2id, Sha256, Xchacha20poly1305Ietf, xchacha20NonceLength } from '@cosmjs/crypto';

/**
 * Argon2id parameters.
 *
 * Measured with the same WASM this runs on: 0.65s on a desktop, so roughly two
 * to four seconds on a phone. 256 MiB was rejected on MEMORY rather than time —
 * a quarter-gigabyte WASM allocation on a low-end Android browser is a failure
 * cliff. See the table in recovery/seal.go.
 *
 * These must equal the Go constants. The vector test is what enforces that;
 * changing one side alone makes it fail.
 */
export const SEAL_TIME = 3;
export const SEAL_MEMORY_KIB = 128 * 1024;
export const SEAL_THREADS = 1;
export const SEAL_KEY_LEN = 32;
export const SEAL_SALT_LEN = 16;

const SEAL_DOMAIN = 'yamale/recovery/seal/v1';

/** A key encrypted under a password. Field names match the Go JSON exactly. */
export interface Sealed {
  salt: Uint8Array;
  nonce: Uint8Array;
  ciphertext: Uint8Array;
  time: number;
  memory: number;
  threads: number;
}

export class SealError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'SealError';
  }
}

/** Thrown when a blob does not open. Deliberately does not say why. */
export class WrongSeal extends SealError {
  constructor() {
    super(
      'The password is wrong, or the account was not put back together correctly. ' +
        'There is no way to tell which from here.',
    );
    this.name = 'WrongSeal';
  }
}

/**
 * The 16 bytes handed to Argon2id.
 *
 * Length-prefixed, mirroring the Go `argonSalt`. Both inputs are fixed-length
 * today so nothing is ambiguous yet, which is precisely when a prefix-free
 * encoding is free to adopt.
 */
export function argonSalt(salt: Uint8Array): Uint8Array {
  const parts = [utf8(SEAL_DOMAIN), salt];
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
  return new Sha256(buf).digest().slice(0, 16);
}

async function sealKey(password: string, sealed: Pick<Sealed, 'salt' | 'time' | 'memory'>) {
  return Argon2id.execute(password, argonSalt(sealed.salt), {
    outputLength: SEAL_KEY_LEN,
    opsLimit: sealed.time,
    memLimitKib: sealed.memory,
  });
}

/** Encrypt a secret under a password. */
export async function seal(
  secret: Uint8Array,
  password: string,
  randomBytes: (n: number) => Uint8Array = defaultRandomBytes,
): Promise<Sealed> {
  if (secret.length === 0) throw new SealError('there is nothing to seal');
  if (password === '') throw new SealError('a seal needs a password');

  const out: Sealed = {
    salt: randomBytes(SEAL_SALT_LEN),
    // 24 bytes is wide enough that a random nonce will not repeat. A 12-byte
    // one would need a counter, and a counter needs somewhere to persist that
    // a browser tab does not have.
    nonce: randomBytes(xchacha20NonceLength),
    ciphertext: new Uint8Array(),
    time: SEAL_TIME,
    memory: SEAL_MEMORY_KIB,
    threads: SEAL_THREADS,
  };

  const key = await sealKey(password, out);
  out.ciphertext = await Xchacha20poly1305Ietf.encrypt(secret, key, out.nonce);
  key.fill(0);
  return out;
}

/**
 * Decrypt a sealed secret.
 *
 * A wrong password and a badly reconstructed ciphertext raise the same error,
 * on purpose: telling them apart would tell somebody holding two of three
 * shares that their password was right.
 */
export async function open(sealed: Sealed, password: string): Promise<Uint8Array> {
  if (!sealed?.salt?.length || !sealed?.nonce?.length || !sealed?.ciphertext?.length) {
    throw new SealError('the sealed key is not in a form this can open');
  }
  if (password === '') throw new SealError('a seal needs a password');
  if (sealed.time < 1 || sealed.memory < 64 * 1024) {
    throw new SealError('the sealed key names Argon2 parameters weaker than this build accepts');
  }
  if (sealed.threads !== 1) {
    // Written by something that did not know libsodium cannot do it. Reported
    // clearly rather than as a decryption failure, because the two have
    // completely different remedies.
    throw new SealError(
      'this account was sealed with an Argon2 parallelism this browser cannot reproduce; ' +
        'it has to be opened by the service that wrote it',
    );
  }
  if (sealed.nonce.length !== xchacha20NonceLength) {
    throw new SealError('the sealed key is not in a form this can open');
  }

  const key = await sealKey(password, sealed);
  try {
    return await Xchacha20poly1305Ietf.decrypt(sealed.ciphertext, key, sealed.nonce);
  } catch {
    throw new WrongSeal();
  } finally {
    key.fill(0);
  }
}

function utf8(s: string): Uint8Array {
  return new TextEncoder().encode(s);
}

function defaultRandomBytes(n: number): Uint8Array {
  const out = new Uint8Array(n);
  crypto.getRandomValues(out);
  return out;
}
