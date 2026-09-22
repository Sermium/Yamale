/**
 * Shamir secret sharing over GF(2^8) — the browser's half.
 *
 * A direct mirror of `recovery/shamir.go`. Not a reimplementation from the
 * paper: the two must agree byte for byte, so this follows the Go file line for
 * line, including the choice of polynomial and generator, and both are checked
 * against `recovery/testdata/vectors.json`.
 *
 * The reason to be this careful is the failure mode. A disagreement here would
 * not break enrolment — that only splits — it would break RECOVERY, months
 * later, for somebody who has already lost their phone and is being told their
 * password is wrong. There is no way to debug out of that after the fact, and
 * no way to notice it before.
 *
 * The comments that explain WHY each rule exists live in the Go file. Repeating
 * them here would give two places to update and one of them would go stale, so
 * this file states what it does and points there for the argument.
 */

/** One part of a split secret. `x` is never zero — that is where the secret is. */
export interface Share {
  x: number;
  y: Uint8Array;
}

export class ShamirError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ShamirError';
  }
}

/* --------------------------------------------------- GF(2^8) arithmetic */

// x^8 + x^4 + x^3 + x + 1, over generator 3. Same as recovery/shamir.go.
const POLYNOMIAL = 0x11b;

const LOG = new Uint8Array(256);
const EXP = new Uint8Array(512);

{
  let x = 1;
  for (let i = 0; i < 255; i++) {
    EXP[i] = x;
    EXP[i + 255] = x;
    LOG[x] = i;
    x ^= x << 1;
    if (x & 0x100) x ^= POLYNOMIAL;
  }
  LOG[0] = 0;
}

function mul(a: number, b: number): number {
  if (a === 0 || b === 0) return 0;
  return EXP[LOG[a]! + LOG[b]!]!;
}

function div(a: number, b: number): number {
  // Only reachable from a duplicate x, which combine() refuses first.
  if (b === 0) return 0;
  if (a === 0) return 0;
  return EXP[LOG[a]! - LOG[b]! + 255]!;
}

/* --------------------------------------------------------------- split */

/**
 * Split a secret so any `threshold` of `shares` rebuild it and fewer learn
 * nothing.
 *
 * `randomBytes` is injected so the tests can be deterministic. Production
 * passes nothing and gets `crypto.getRandomValues`, which — unlike
 * `crypto.subtle` — is available without a secure context. That distinction is
 * the whole reason this code can run on the devnet at all; see the note at the
 * top of ../vault.ts.
 */
export function split(
  secret: Uint8Array,
  shares: number,
  threshold: number,
  randomBytes: (n: number) => Uint8Array = defaultRandomBytes,
): Share[] {
  if (secret.length === 0) throw new ShamirError('there is nothing to split');
  if (threshold < 2 || threshold > shares || shares > 255) {
    throw new ShamirError(
      'the threshold must be at least 2 and no more than the number of shares',
    );
  }

  const out: Share[] = [];
  for (let i = 0; i < shares; i++) {
    out.push({ x: i + 1, y: new Uint8Array(secret.length) });
  }

  // One independent polynomial per byte, or two equal bytes of the secret would
  // produce equal share bytes at every x.
  for (let b = 0; b < secret.length; b++) {
    const coefficients = randomBytes(threshold - 1);
    if (coefficients.length !== threshold - 1) {
      throw new ShamirError('the randomness source returned the wrong number of bytes');
    }
    for (const share of out) {
      share.y[b] = evaluate(secret[b]!, coefficients, share.x);
    }
  }
  return out;
}

/** Horner's method from the top coefficient down. */
function evaluate(constant: number, coefficients: Uint8Array, x: number): number {
  let result = 0;
  for (let i = coefficients.length - 1; i >= 0; i--) {
    result = mul(result, x) ^ coefficients[i]!;
  }
  return mul(result, x) ^ constant;
}

/* ------------------------------------------------------------- combine */

/**
 * Rebuild the secret from any sufficient set of shares.
 *
 * Given too few it returns a value — the wrong one — with no error, and no
 * arithmetic could detect that. This is why the SEALED blob is what gets split:
 * its authentication tag is what tells the caller the reconstruction was wrong.
 * See seal.ts.
 */
export function combine(shares: Share[]): Uint8Array {
  if (shares.length < 2) throw new ShamirError('not enough shares to reconstruct');

  const length = shares[0]!.y.length;
  const seen = new Set<number>();
  for (const s of shares) {
    if (s.y.length !== length) {
      throw new ShamirError('the shares are not all the same length');
    }
    if (s.x === 0) {
      throw new ShamirError("a share's x is zero, which would be the secret itself");
    }
    if (seen.has(s.x)) {
      throw new ShamirError('two shares have the same x and carry the same information');
    }
    seen.add(s.x);
  }

  const secret = new Uint8Array(length);
  for (let b = 0; b < length; b++) {
    // Lagrange interpolation at x = 0, where the secret lives.
    let value = 0;
    for (let i = 0; i < shares.length; i++) {
      const si = shares[i]!;
      let basis = 1;
      for (let j = 0; j < shares.length; j++) {
        if (i === j) continue;
        const sj = shares[j]!;
        // (0 - xj) / (xi - xj); subtraction is xor here, so 0 - xj is xj.
        basis = mul(basis, div(sj.x, si.x ^ sj.x));
      }
      value ^= mul(si.y[b]!, basis);
    }
    secret[b] = value;
  }
  return secret;
}

/**
 * Constant-time comparison, for recovered key material.
 *
 * The obvious `every` returns on the first difference, and these callers are
 * comparing secrets.
 */
export function equal(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a[i]! ^ b[i]!;
  return diff === 0;
}

function defaultRandomBytes(n: number): Uint8Array {
  const out = new Uint8Array(n);
  crypto.getRandomValues(out);
  return out;
}
