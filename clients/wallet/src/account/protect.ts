/**
 * Protect and restore — the browser's half of `recovery/account.go`.
 *
 * Two calls. `protect` turns a key into three holdings, one for each party;
 * `restore` turns any two of them plus the password back into the key.
 *
 * SEAL, THEN SPLIT. Split the raw key and the custodian plus the recovery
 * service — both run by the platform — reconstruct it between them, and "the
 * operator cannot move your money" is false while sounding true. The Go file
 * carries the full argument and the test that fails if anybody reorders it.
 */

import { open, seal, type Sealed } from './seal.ts';
import { combine, split, type Share } from './shamir.ts';
import type { Holding } from './deposit.ts';

export const ROLE_DEVICE = 'device';
export const ROLE_CUSTODIAN = 'custodian';
export const ROLE_RECOVERY = 'recovery';

/** Two of three. One would make a single breached service the end of it; three
 *  would mean losing the phone — the ordinary reason anybody recovers
 *  anything — made recovery impossible. */
export const THRESHOLD = 2;

// Fixed so a holding's role says which share it is, matching roleX in Go.
const ROLE_X: Record<string, number> = {
  [ROLE_DEVICE]: 1,
  [ROLE_CUSTODIAN]: 2,
  [ROLE_RECOVERY]: 3,
};

export class ProtectError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ProtectError';
  }
}

/** Seal a key under a password and split the result three ways. */
export async function protect(
  secret: Uint8Array,
  password: string,
  randomBytes?: (n: number) => Uint8Array,
): Promise<Holding[]> {
  const sealed = await seal(secret, password, randomBytes);
  // The CIPHERTEXT is what gets split.
  const shares = split(sealed.ciphertext, 3, THRESHOLD, randomBytes);

  return [ROLE_DEVICE, ROLE_CUSTODIAN, ROLE_RECOVERY].map((role) => {
    const share = shares.find((s) => s.x === ROLE_X[role]);
    if (!share) throw new ProtectError(`no share at x=${ROLE_X[role]} for the ${role} role`);
    return holdingOf(role, sealed, share);
  });
}

function holdingOf(role: string, sealed: Sealed, share: Share): Holding {
  return {
    role,
    // The header is NOT secret and is repeated in every holding. It has to be:
    // a recovery is assembled from whichever two parties are reachable, and a
    // header living with only one of them would make that party's loss fatal
    // while its share was still theoretically unnecessary.
    salt: sealed.salt,
    nonce: sealed.nonce,
    time: sealed.time,
    memory: sealed.memory,
    threads: sealed.threads,
    share,
  };
}

/**
 * Rebuild the key from any two holdings and the password.
 *
 * A wrong password and a wrong pair both surface as WrongSeal, because the
 * authentication tag is the only thing that can tell either from success and it
 * cannot say which.
 */
export async function restore(holdings: Holding[], password: string): Promise<Uint8Array> {
  if (holdings.length < THRESHOLD) {
    throw new ProtectError('a recovery needs at least two holdings');
  }

  const head = holdings[0]!;
  for (const h of holdings) {
    if (!(h.role in ROLE_X)) throw new ProtectError('that is not one of the three roles');
    // Checked rather than trusted. Two holdings from different enrolments would
    // combine into a ciphertext that fails its tag, and the account would be
    // reported unrecoverable when in fact the wrong pair was fetched — a
    // diagnosis nobody would reach from the seal error alone.
    if (
      !sameBytes(h.salt, head.salt) ||
      !sameBytes(h.nonce, head.nonce) ||
      h.time !== head.time ||
      h.memory !== head.memory ||
      h.threads !== head.threads
    ) {
      throw new ProtectError('these holdings are not from the same sealed key');
    }
  }

  const ciphertext = combine(holdings.map((h) => h.share));
  return open(
    {
      salt: head.salt,
      nonce: head.nonce,
      ciphertext,
      time: head.time,
      memory: head.memory,
      threads: head.threads,
    },
    password,
  );
}

/** Plain comparison: these are public header fields, not secrets. */
function sameBytes(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}
