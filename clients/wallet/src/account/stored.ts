/**
 * What this device remembers about an enrolment.
 *
 * # Why this is not encrypted, and why that is not an oversight
 *
 * The device's holding is ONE share of a three-way split. On its own it is
 * information-theoretically nothing: every possible ciphertext remains exactly
 * as likely as it was before, which is the property that makes Shamir sharing
 * different from an encoding. So storing it in the clear reveals nothing that
 * encrypting it would hide.
 *
 * That is a genuinely different situation from ../vault.ts, which holds the key
 * itself and therefore has to be sealed. Encrypting this one too would look
 * more careful and would cost the thing that matters: the device holding has to
 * be readable when the vault CANNOT be opened, because pairing it with a
 * service holding is one of the ways a recovery works.
 *
 * What is stored alongside it is the email and the address, neither secret —
 * the address is public by nature and the email is already on this device in
 * every other form.
 *
 * # Keyed by address
 *
 * Not by email, and not a single slot. One browser can hold more than one
 * account over its life, and a record that quietly belonged to whichever
 * account was enrolled last is a record that tells somebody their account is
 * protected when a different one is.
 */

import type { Holding } from './deposit.ts';

const KEY = 'yamale.wallet.enrolment.v1';

export interface Enrolment {
  address: string;
  /** The address the services were told to file this under. */
  email: string;
  /** This device's share. One of three. */
  holding: Holding;
  /** ISO 8601, UTC. For the screen to say when, not for any decision. */
  enrolledAt: string;
}

/** The stored shape: byte arrays as base64, because JSON has no bytes. */
interface StoredHolding {
  role: string;
  salt: string;
  nonce: string;
  time: number;
  memory: number;
  threads: number;
  share: { x: number; y: string };
}

interface StoredEnrolment {
  address: string;
  email: string;
  holding: StoredHolding;
  enrolledAt: string;
}

export function enrolmentFor(address: string): Enrolment | null {
  for (const stored of readAll()) {
    if (stored.address === address) {
      try {
        return { ...stored, holding: decodeHolding(stored.holding) };
      } catch {
        // A record written by something else, or truncated. Treated as absent:
        // the screen then offers to enrol, which is recoverable, rather than
        // claiming a protection that cannot be used.
        return null;
      }
    }
  }
  return null;
}

/** Records an enrolment, replacing any earlier one for the same address. */
export function rememberEnrolment(enrolment: Enrolment): void {
  const kept = readAll().filter((e) => e.address !== enrolment.address);
  kept.push({
    address: enrolment.address,
    email: enrolment.email,
    holding: encodeHolding(enrolment.holding),
    enrolledAt: enrolment.enrolledAt,
  });
  write(kept);
}

/**
 * Forgets this device's record.
 *
 * It does NOT un-enrol anything: the two services still hold their holdings and
 * the account is still recoverable through them. This only drops the local
 * copy, which is why it is safe to call when an account is detached from this
 * browser — and why the screen must not describe it as cancelling anything.
 */
export function forgetEnrolment(address: string): void {
  write(readAll().filter((e) => e.address !== address));
}

function readAll(): StoredEnrolment[] {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as StoredEnrolment[]) : [];
  } catch {
    // Private browsing, blocked storage, or a corrupt value. An empty list is
    // the honest answer and the screen offers to enrol.
    return [];
  }
}

function write(all: StoredEnrolment[]): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(all));
  } catch {
    // Nothing to do and nothing to say: the enrolment itself succeeded, and the
    // services hold what they need. This device simply will not remember, which
    // costs a recovery one extra step rather than the account.
  }
}

function encodeHolding(h: Holding): StoredHolding {
  return {
    role: h.role,
    salt: base64(h.salt),
    nonce: base64(h.nonce),
    time: h.time,
    memory: h.memory,
    threads: h.threads,
    share: { x: h.share.x, y: base64(h.share.y) },
  };
}

function decodeHolding(s: StoredHolding): Holding {
  const holding: Holding = {
    role: s.role,
    salt: bytes(s.salt),
    nonce: bytes(s.nonce),
    time: s.time,
    memory: s.memory,
    threads: s.threads,
    share: { x: s.share.x, y: bytes(s.share.y) },
  };
  // Checked rather than trusted. A truncated record that still parses would
  // produce a holding that fails only at the moment of a recovery.
  if (!holding.role || holding.salt.length === 0 || holding.nonce.length === 0) {
    throw new Error('incomplete');
  }
  if (!holding.share.x || holding.share.y.length === 0) throw new Error('incomplete');
  return holding;
}

function base64(b: Uint8Array): string {
  let binary = '';
  for (const byte of b) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function bytes(s: string): Uint8Array {
  return Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
}
