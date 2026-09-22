/**
 * The comparison, tested against the things that make it wrong.
 *
 * Every test here builds its templates from known bits rather than from
 * captures, because the question is whether the arithmetic is right, not
 * whether a camera works. A matcher that is wrong in these ways still produces
 * plausible-looking numbers on real data, which is why it is worth being
 * unambiguous here.
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  ambiguous,
  compare,
  distanceAt,
  search,
  verdictOf,
  MIN_COMPARABLE_FRACTION,
  THRESHOLDS,
  type Enrolled,
  type IrisTemplate,
} from './iris.ts';

const BITS = 256;

/** A template from a bit-producing function, with an all-valid mask. */
function make(bitAt: (i: number) => boolean, maskAt: (i: number) => boolean = () => true): IrisTemplate {
  const bytes = BITS / 8;
  const code = new Uint8Array(bytes);
  const mask = new Uint8Array(bytes);
  for (let i = 0; i < BITS; i++) {
    if (bitAt(i)) code[i >> 3]! |= 0x80 >> (i & 7);
    if (maskAt(i)) mask[i >> 3]! |= 0x80 >> (i & 7);
  }
  return { code, mask, bits: BITS };
}

/** Deterministic pseudo-random bits, so a failure is reproducible. */
function noise(seed: number): (i: number) => boolean {
  return (i) => {
    let x = (seed * 2654435761 + i * 40503) >>> 0;
    x ^= x >>> 13; x = (x * 1274126177) >>> 0; x ^= x >>> 16;
    return (x & 1) === 1;
  };
}

/* --------------------------------------------------------------- the measure */

test('the same template compares to itself at zero', () => {
  const t = make(noise(1));
  const got = compare(t, t);
  assert.equal(got.distance, 0);
  assert.equal(got.rotation, 0);
  assert.equal(got.comparedBits, BITS);
});

test('two unrelated irises land near a half, not near zero', () => {
  // The property the whole measure rests on: independent codes disagree on
  // about half their bits. If this drifted toward 0 the thresholds would pass
  // impostors, and there would be nothing in the numbers to show it.
  const a = make(noise(7));
  const b = make(noise(99));
  const got = compare(a, b);
  assert.ok(got.distance !== null);
  assert.ok(got.distance > 0.35 && got.distance < 0.65,
    `unrelated codes should sit near 0.5, got ${got.distance}`);
});

test('a template with some bits flipped is still recognisably itself', () => {
  // A degraded capture of the same eye: one bit in eight corrupted.
  const a = make(noise(11));
  const b = make((i) => (i % 8 === 0 ? !noise(11)(i) : noise(11)(i)));
  const got = compare(a, b);
  assert.ok(got.distance !== null);
  assert.ok(Math.abs(got.distance - 0.125) < 0.02,
    `one bit in eight should give about 0.125, got ${got.distance}`);
  assert.equal(verdictOf(got.distance), 'match');
});

/* -------------------------------------------------------------- the rotation */

test('a rotated capture of the same eye still matches', () => {
  // A tilted head is a horizontal shift in the normalised image. Without the
  // rotation search this is the case that fails, and it fails on genuine
  // animals rather than on impostors — so the symptom is a registry that keeps
  // rejecting beasts it has already enrolled.
  const src = noise(23);
  const a = make(src);
  const shift = 5;
  const b = make((i) => src((i - shift + BITS) % BITS));

  assert.ok(distanceAt(a, b, 0).distance! > 0.3, 'unaligned, this should look wrong');

  const got = compare(a, b, 16);
  assert.equal(got.distance, 0);
  // Positive, per the sign convention on distanceAt: shiftBits advances the
  // position read in B, and B here is A rotated later.
  assert.equal(got.rotation, shift);
});

test('a rotation beyond the search range is not found', () => {
  // Stated as a test because it is a real limit, not a bug: a beast
  // photographed at ninety degrees will not match, and the answer is to capture
  // it again rather than to widen the search until impostors get in.
  const src = noise(31);
  const a = make(src);
  const b = make((i) => src((i - 40 + BITS) % BITS));
  const got = compare(a, b, 16);
  assert.ok(got.distance === null || got.distance > THRESHOLDS.inconclusive);
});

/* ------------------------------------------------------------------ the mask */

test('bits under an eyelid are not compared', () => {
  // The same eye, but the second capture had the top quarter occluded and the
  // bits there are garbage. Masked correctly, the comparison ignores them.
  const src = noise(41);
  const a = make(src);
  const b = make(
    (i) => (i < BITS / 4 ? !src(i) : src(i)),
    (i) => i >= BITS / 4,
  );
  const got = compare(a, b);
  assert.equal(got.distance, 0, 'the occluded quarter should not count against it');
  assert.equal(got.comparedBits, BITS * 0.75);
});

test('ignoring the mask would turn a genuine match into a rejection', () => {
  // The failure this guards, made explicit: the same eye compared with and
  // without its mask. A drooping lid and lashes over 40% of the iris is an
  // ordinary field capture, not a bad one.
  //
  // An earlier version of this test occluded a quarter and asserted a
  // rejection. A quarter scores 0.25, which is still inside the match
  // threshold — so it proved nothing, and it would have passed against a
  // matcher that ignored masks entirely. The occlusion has to be large enough
  // to actually cross the line, which is exactly the case that happens in a
  // pen at dusk.
  const src = noise(41);
  const cover = 0.4;
  const a = make(src);
  const occluded = make(
    (i) => (i < BITS * cover ? !src(i) : src(i)),
    (i) => i >= BITS * cover,
  );

  const masked = compare(a, occluded);
  assert.equal(masked.distance, 0, 'with the mask it is the same eye');
  assert.equal(verdictOf(masked.distance), 'match');

  const ignoringMask = make((i) => (i < BITS * cover ? !src(i) : src(i)));
  const unmasked = compare(a, ignoringMask);
  assert.ok(unmasked.distance! > THRESHOLDS.inconclusive,
    `without the mask the same eye scores ${unmasked.distance}`);
  assert.equal(verdictOf(unmasked.distance), 'no-match',
    'a matcher that drops the mask rejects animals it has already enrolled');
});

test('too little overlap is refused rather than scored', () => {
  // "This capture cannot be compared" and "this is a different animal" are
  // different sentences and an operator must be told which one they have.
  const src = noise(53);
  const a = make(src, (i) => i < BITS * 0.3);
  const b = make(src, (i) => i >= BITS * 0.8);
  const got = compare(a, b);
  assert.equal(got.distance, null);
  assert.equal(verdictOf(got.distance), 'inconclusive');
});

test('the comparable fraction is a real floor', () => {
  assert.ok(MIN_COMPARABLE_FRACTION > 0.2 && MIN_COMPARABLE_FRACTION < 0.8);
});

/* --------------------------------------------------------------- the verdict */

test('there are three answers, and the middle one is the useful one', () => {
  assert.equal(verdictOf(0.10), 'match');
  assert.equal(verdictOf(THRESHOLDS.match), 'match', 'at the threshold is a match');
  assert.equal(verdictOf(0.35), 'inconclusive');
  assert.equal(verdictOf(THRESHOLDS.inconclusive), 'inconclusive');
  assert.equal(verdictOf(0.45), 'no-match');
  assert.equal(verdictOf(null), 'inconclusive', 'uncomparable is never a rejection');
});

test('the thresholds are separated, and in the right order', () => {
  assert.ok(THRESHOLDS.match < THRESHOLDS.inconclusive,
    'a match must be stricter than merely inconclusive');
  assert.ok(THRESHOLDS.inconclusive < 0.5,
    'anything at 0.5 is indistinguishable from an unrelated eye');
});

/* -------------------------------------------------------------- the 1:N search */

function enrol(id: string, seed: number, at = 0): Enrolled {
  return { animalId: id, enrolmentId: `${id}-${at}`, capturedAt: at, template: make(noise(seed)) };
}

test('a probe finds its own animal and not the rest of the herd', () => {
  const herd = [enrol('a', 100), enrol('b', 200), enrol('c', 300)];
  const probe = make(noise(200));

  const got = search(probe, herd);
  assert.equal(got.length, 1, 'only the genuine animal should survive');
  assert.equal(got[0]!.animalId, 'b');
  assert.equal(got[0]!.verdict, 'match');
});

test('candidates come back best first', () => {
  const src = noise(500);
  const near = make((i) => (i % 16 === 0 ? !src(i) : src(i)));   // ~0.06
  const mid = make((i) => (i % 3 === 0 ? !src(i) : src(i)));     // ~0.33
  const herd: Enrolled[] = [
    { animalId: 'mid', enrolmentId: 'mid-0', capturedAt: 0, template: mid },
    { animalId: 'near', enrolmentId: 'near-0', capturedAt: 0, template: near },
  ];
  const got = search(make(src), herd);
  assert.ok(got.length >= 1);
  assert.equal(got[0]!.animalId, 'near', 'the closest must be first');
});

test('an animal matching on two of its own captures is not ambiguous', () => {
  // Normal and expected: an animal has several enrolments over its life, and
  // they should all match it. Treating that as a conflict would make every
  // properly re-enrolled animal unusable.
  const src = noise(600);
  const herd: Enrolled[] = [
    { animalId: 'a', enrolmentId: 'a-0', capturedAt: 0, template: make(src) },
    { animalId: 'a', enrolmentId: 'a-1', capturedAt: 1, template: make((i) => (i % 20 === 0 ? !src(i) : src(i))) },
  ];
  const got = search(make(src), herd);
  assert.equal(got.length, 2);
  assert.equal(ambiguous(got), false);
});

test('two DIFFERENT animals matching is ambiguous and must stop the flow', () => {
  // Twins, a re-enrolment filed against the wrong beast, or a substitution.
  // Each needs a person, and collapsing this to a single identifier is how a
  // registry quietly acquires a duplicate.
  const src = noise(700);
  const herd: Enrolled[] = [
    { animalId: 'a', enrolmentId: 'a-0', capturedAt: 0, template: make(src) },
    { animalId: 'b', enrolmentId: 'b-0', capturedAt: 0, template: make((i) => (i % 20 === 0 ? !src(i) : src(i))) },
  ];
  const got = search(make(src), herd);
  assert.equal(got.length, 2);
  assert.equal(ambiguous(got), true);
});

test('an empty herd finds nothing and does not throw', () => {
  assert.deepEqual(search(make(noise(1)), []), []);
});

test('templates of different lengths are not comparable', () => {
  // Two devices with different code sizes. Silently returning a distance here
  // would compare unrelated things and occasionally produce a low one.
  const a = make(noise(1));
  const b: IrisTemplate = { code: new Uint8Array(16), mask: new Uint8Array(16), bits: 128 };
  assert.equal(compare(a, b).distance, null);
});
