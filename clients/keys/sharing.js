// Secret sharing over GF(2^8), for the demonstration on this page.
//
// # Why there is a third copy of this
//
// `recovery/shamir.go` is what the services run and
// `clients/wallet/src/account/shamir.ts` is what the wallet runs. This is a
// third implementation, and a third implementation of one piece of arithmetic
// is normally a bug waiting for a quiet afternoon.
//
// It exists because this page has no bundler — it loads plain ES modules from a
// static directory — so it cannot import the wallet's TypeScript, and a page
// that DESCRIBED the arithmetic instead of running it would be exactly the kind
// of claim this console exists to stop making.
//
// What makes it acceptable is that it is not trusted on its own authority.
// `sharing.test.js` checks it against `recovery/testdata/vectors.json`, the same
// file the Go and TypeScript implementations answer to. Three implementations
// pinned to one set of vectors is a different thing from three implementations.
//
// # What this deliberately does NOT do
//
// It does not seal. Sealing is Argon2id at 128 MiB, which takes seconds and
// would mean shipping libsodium to every visitor — and the 19 MB WebAssembly
// module this page used to load is precisely the mistake not to repeat. The
// sealed values shown on the page are real, produced by the Go implementation,
// and read from vectors.json rather than computed here.
//
// The half that IS run in front of the reader is the half they are entitled to
// disbelieve: that two of the three parts, held by two services run by the same
// operator, reconstruct something that is not the key.

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

function mul(a, b) {
  if (a === 0 || b === 0) return 0;
  return EXP[LOG[a] + LOG[b]];
}

function div(a, b) {
  if (b === 0 || a === 0) return 0;
  return EXP[LOG[a] - LOG[b] + 255];
}

/**
 * Split a secret into `shares` parts, any `threshold` of which rebuild it.
 *
 * `randomBytes` is injectable so the test can be deterministic. The page passes
 * nothing and gets crypto.getRandomValues, which — unlike crypto.subtle — needs
 * no secure context, and this site is served over plain HTTP on the devnet.
 */
export function split(secret, shares, threshold, randomBytes = defaultRandomBytes) {
  if (secret.length === 0) throw new Error('there is nothing to split');
  if (threshold < 2 || threshold > shares || shares > 255) {
    throw new Error('the threshold must be at least 2 and no more than the number of shares');
  }

  const out = [];
  for (let i = 0; i < shares; i++) out.push({ x: i + 1, y: new Uint8Array(secret.length) });

  // One independent polynomial per byte, or two equal bytes of the secret
  // produce equal share bytes at every x and the shape of the secret leaks.
  for (let b = 0; b < secret.length; b++) {
    const coefficients = randomBytes(threshold - 1);
    for (const share of out) {
      let result = 0;
      for (let i = coefficients.length - 1; i >= 0; i--) {
        result = mul(result, share.x) ^ coefficients[i];
      }
      share.y[b] = mul(result, share.x) ^ secret[b];
    }
  }
  return out;
}

/**
 * Rebuild from any sufficient set of parts.
 *
 * Given too few it returns a value — the wrong one — and says nothing, because
 * no arithmetic here could tell. That is the property the page demonstrates,
 * and it is why the thing being split is a SEALED blob: the authentication tag
 * is what distinguishes a real reconstruction from a plausible wrong one.
 */
export function combine(shares) {
  if (shares.length < 2) throw new Error('not enough parts to reconstruct');

  const length = shares[0].y.length;
  const seen = new Set();
  for (const s of shares) {
    if (s.y.length !== length) throw new Error('the parts are not all the same length');
    if (s.x === 0) throw new Error("a part's x is zero, which would be the secret itself");
    if (seen.has(s.x)) throw new Error('two parts have the same x');
    seen.add(s.x);
  }

  const secret = new Uint8Array(length);
  for (let b = 0; b < length; b++) {
    let value = 0;
    for (let i = 0; i < shares.length; i++) {
      let basis = 1;
      for (let j = 0; j < shares.length; j++) {
        if (i === j) continue;
        basis = mul(basis, div(shares[j].x, shares[i].x ^ shares[j].x));
      }
      value ^= mul(shares[i].y[b], basis);
    }
    secret[b] = value;
  }
  return secret;
}

/** Constant-time enough for a page that is comparing public demo values. */
export function same(a, b) {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a[i] ^ b[i];
  return diff === 0;
}

export function toHex(bytes) {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export function fromHex(hex) {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

export function fromBase64(b64) {
  const binary = atob(b64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

function defaultRandomBytes(n) {
  const out = new Uint8Array(n);
  crypto.getRandomValues(out);
  return out;
}
