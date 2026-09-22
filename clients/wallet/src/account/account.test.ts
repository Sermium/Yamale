/**
 * The randomised half, which vectors cannot pin.
 *
 * vectors.test.ts proves this implementation agrees with Go on every
 * deterministic value. It cannot prove that `split` produces shares that
 * reconstruct, or that `protect` puts the seal and the split in the right
 * order, because both choose fresh randomness every time.
 *
 * So these are property tests, and each one is aimed at a specific way the code
 * can be wrong while still returning plausible-looking bytes.
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { combine, equal, split, ShamirError } from './shamir.ts';
import { open, seal, WrongSeal } from './seal.ts';
import {
  protect,
  restore,
  ROLE_CUSTODIAN,
  ROLE_DEVICE,
  ROLE_RECOVERY,
  ProtectError,
} from './protect.ts';
import { publicKeyOf, signDeposit } from './deposit.ts';

const PASSWORD = 'seven horses eating apples slowly';

function bytes(...values: number[]): Uint8Array {
  return Uint8Array.from(values);
}

function randomKey(): Uint8Array {
  const k = new Uint8Array(32);
  crypto.getRandomValues(k);
  // secp256k1 rejects zero and values above the order; a random 32 bytes is
  // overwhelmingly valid, and the one in 2^128 case is not worth a loop here.
  k[0] = k[0]! | 1;
  return k;
}

/* --------------------------------------------------------------- sharing */

test('every pair of shares rebuilds the secret', () => {
  // Not "some two shares" — every two. A Lagrange basis with the subtraction
  // the wrong way round rebuilds correctly from shares 1 and 2 and wrongly from
  // 2 and 3, and a test that tries one pair never sees it.
  const secret = new TextEncoder().encode('the device key, or something its length');
  const shares = split(secret, 5, 2);
  for (let i = 0; i < shares.length; i++) {
    for (let j = i + 1; j < shares.length; j++) {
      assert.deepEqual(
        combine([shares[i]!, shares[j]!]),
        secret,
        `shares ${i} and ${j} did not rebuild the secret`,
      );
    }
  }
});

test('too few shares rebuild something else entirely, and say nothing about it', () => {
  // The property that makes this secret sharing rather than an encoding, and
  // the reason combine cannot report an error: two shares of a 3-of-5 split
  // produce a perfectly well-formed wrong answer.
  const secret = new TextEncoder().encode('three of five');
  const shares = split(secret, 5, 3);
  const got = combine(shares.slice(0, 2));
  assert.equal(got.length, secret.length, 'the wrong answer is not even the right shape');
  assert.ok(!equal(got, secret), 'two shares of a 3-of-5 split rebuilt the secret');
});

test('a secret of any byte value survives', () => {
  // Zero and 0xff are where a field implementation that special-cases badly
  // goes wrong, and a key is uniform random so it contains both.
  const secret = new Uint8Array(256);
  for (let i = 0; i < 256; i++) secret[i] = i;
  const shares = split(secret, 3, 2);
  assert.deepEqual(combine([shares[0]!, shares[2]!]), secret);
});

test('no share is the secret, and none sits at x=0', () => {
  const secret = new TextEncoder().encode('0123456789abcdef0123456789abcdef');
  for (const s of split(secret, 3, 2)) {
    assert.notEqual(s.x, 0, 'a share at x=0 is the secret under another name');
    assert.ok(!equal(s.y, secret), 'a share IS the secret');
  }
});

test('two splits of one secret differ, so the polynomial is really random', () => {
  const secret = new TextEncoder().encode('same secret, twice');
  const a = split(secret, 2, 2);
  const b = split(secret, 2, 2);
  assert.ok(!equal(a[0]!.y, b[0]!.y), 'two splits produced the same share');
  assert.ok(!equal(a[0]!.y, a[1]!.y), 'two shares of one split are identical');
});

test('combine refuses shares that cannot belong together', () => {
  const shares = split(bytes(1, 2, 3, 4), 3, 2);
  assert.throws(() => combine([shares[0]!]), ShamirError, 'one share was accepted');
  assert.throws(
    () => combine([shares[0]!, { x: 9, y: bytes(1, 2) }]),
    ShamirError,
    'mismatched lengths were accepted',
  );
  // The dangerous one: two copies of one share look like two shares, and the
  // interpolation would divide by zero and return plausible rubbish.
  assert.throws(() => combine([shares[0]!, shares[0]!]), ShamirError, 'a duplicate x was accepted');
  assert.throws(
    () => combine([shares[0]!, { x: 0, y: new Uint8Array(4) }]),
    ShamirError,
    'x=0 was accepted',
  );
});

test('split refuses a threshold it cannot honour', () => {
  for (const [shares, threshold] of [
    [3, 4],
    [3, 1],
    [3, 0],
    [300, 2],
  ] as const) {
    assert.throws(
      () => split(bytes(1), shares, threshold),
      ShamirError,
      `split(${shares}, ${threshold}) was accepted`,
    );
  }
});

/* ------------------------------------------------------------ the seal */

test('a sealed secret comes back, and a wrong password does not', async () => {
  const secret = randomKey();
  const sealed = await seal(secret, PASSWORD);
  assert.deepEqual(await open(sealed, PASSWORD), secret);
  await assert.rejects(() => open(sealed, PASSWORD + ' '), WrongSeal);
});

test('two seals of one secret differ', async () => {
  const secret = bytes(1, 2, 3, 4);
  const a = await seal(secret, PASSWORD);
  const b = await seal(secret, PASSWORD);
  assert.ok(!equal(a.salt, b.salt), 'the salt is not fresh');
  assert.ok(!equal(a.nonce, b.nonce), 'the nonce is not fresh');
  assert.ok(!equal(a.ciphertext, b.ciphertext), 'two seals are identical');
});

test('a tampered ciphertext is detected', async () => {
  const sealed = await seal(bytes(9, 9, 9, 9), PASSWORD);
  sealed.ciphertext[0] = sealed.ciphertext[0]! ^ 1;
  await assert.rejects(() => open(sealed, PASSWORD), WrongSeal);
});

/* -------------------------------------------------------- the holdings */

test('any two holdings restore the key, and the services alone cannot', async () => {
  const secret = randomKey();
  const holdings = await protect(secret, PASSWORD);
  assert.equal(holdings.length, 3);

  const byRole = Object.fromEntries(holdings.map((h) => [h.role, h]));
  const pairs: [string, string][] = [
    [ROLE_DEVICE, ROLE_CUSTODIAN],
    [ROLE_DEVICE, ROLE_RECOVERY],
    [ROLE_CUSTODIAN, ROLE_RECOVERY],
  ];
  for (const [a, b] of pairs) {
    assert.deepEqual(
      await restore([byRole[a]!, byRole[b]!], PASSWORD),
      secret,
      `${a} + ${b} did not restore the key`,
    );
  }

  // THE POINT, and the test that fails if anybody ever splits before sealing:
  // what the two platform-run services hold between them, with no password,
  // must not be the key.
  const services = [byRole[ROLE_CUSTODIAN]!, byRole[ROLE_RECOVERY]!];
  const ciphertext = combine(services.map((h) => h.share));
  assert.ok(!equal(ciphertext, secret), 'the two services hold the key between them');
  await assert.rejects(
    () => restore(services, 'not the password at all'),
    WrongSeal,
    'a wrong password restored the key from the two service holdings',
  );
});

test('holdings from different enrolments are refused rather than combined', async () => {
  // The operational failure: a store returning the right role from the wrong
  // account. Combined blindly they produce a ciphertext that fails its tag, and
  // the operator reads "wrong password" and tells the customer they forgot it.
  const a = await protect(randomKey(), PASSWORD);
  const b = await protect(randomKey(), PASSWORD);
  await assert.rejects(() => restore([a[0]!, b[1]!], PASSWORD), ProtectError);
});

test('one holding is not a recovery', async () => {
  const holdings = await protect(randomKey(), PASSWORD);
  await assert.rejects(() => restore(holdings.slice(0, 1), PASSWORD), ProtectError);
});

test('each role gets a different share', async () => {
  const holdings = await protect(randomKey(), PASSWORD);
  const xs = new Set(holdings.map((h) => h.share.x));
  assert.equal(xs.size, 3, 'two roles hold the same share');
});

/* ------------------------------------------------------- the deposit */

test('a deposit signature is 64 bytes over the key it names', async () => {
  const key = randomKey();
  const holdings = await protect(key, PASSWORD);
  const custodian = holdings.find((h) => h.role === ROLE_CUSTODIAN)!;

  const signature = await signDeposit(key, 'amara@example.org', ROLE_CUSTODIAN, custodian);
  assert.equal(signature.length, 64, 'the Go side parses exactly r||s');

  const pub = await publicKeyOf(key);
  assert.equal(pub.length, 33, 'the deposit carries a COMPRESSED public key');
  assert.ok(pub[0] === 2 || pub[0] === 3, 'that is not a compressed point');
});
