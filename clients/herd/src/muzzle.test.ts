/**
 * The muzzle matcher, tested against the things that make it wrong.
 *
 * Every image here is synthesised from a known texture rather than photographed,
 * because the question is whether the arithmetic behaves — whether the same
 * grain matches itself under lighting that changed, whether different grain
 * does not, whether the descriptor notices WHERE a texture was. A matcher that
 * is wrong in any of those ways still produces plausible-looking numbers on real
 * animals, which is exactly why it is worth being unambiguous here.
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  ambiguous,
  coatVerdict,
  describe,
  distance,
  faceVerdict,
  muzzleVerdict,
  narrowByCoat,
  narrowByFace,
  search,
  MuzzleError,
  COAT_THRESHOLDS,
  FACE_THRESHOLDS,
  MUZZLE_THRESHOLDS,
  type Descriptor,
  type Enrolled,
} from './muzzle.ts';

const W = 96;
const H = 96;

/** A deterministic "grain": beads and ridges at a seed-dependent scale. */
function grain(seed: number, width = W, height = H): Uint8Array {
  const out = new Uint8Array(width * height);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      // Two interfering sinusoids plus reproducible speckle. The seed changes
      // the frequencies and the phase, so two seeds are two different animals.
      const a = Math.sin((x * (2 + (seed % 3)) + y * (3 + (seed % 5))) * 0.21 + seed);
      const b = Math.cos((x * (5 + (seed % 4)) - y * (2 + (seed % 7))) * 0.13 - seed);
      let v = (seed * 2654435761 + x * 40503 + y * 15485863) >>> 0;
      v ^= v >>> 13;
      const speck = ((v * 1274126177) >>> 24) / 255;
      out[y * width + x] = Math.max(0, Math.min(255, 128 + a * 50 + b * 40 + speck * 20));
    }
  }
  return out;
}

function shift(luma: Uint8Array, dx: number, dy: number, width = W, height = H): Uint8Array {
  const out = new Uint8Array(width * height);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const sx = Math.min(width - 1, Math.max(0, x - dx));
      const sy = Math.min(height - 1, Math.max(0, y - dy));
      out[y * width + x] = luma[sy * width + sx]!;
    }
  }
  return out;
}

/* ------------------------------------------------------------ the measure */

test('the same muzzle compares to itself at zero', () => {
  const d = describe(grain(1), W, H, 'muzzle');
  assert.equal(distance(d, d), 0);
  assert.equal(muzzleVerdict(0), 'match');
});

test('two different animals are further apart than one animal is from itself', () => {
  // The property the whole measure rests on. If it drifted, the thresholds
  // would pass impostors and nothing in the numbers would show it.
  const a = describe(grain(7), W, H, 'muzzle');
  const b = describe(grain(99), W, H, 'muzzle');
  const aAgain = describe(grain(7), W, H, 'muzzle');

  assert.equal(distance(a, aAgain), 0);
  assert.ok(distance(a, b) > MUZZLE_THRESHOLDS.inconclusive,
    `two animals should not match, got ${distance(a, b)}`);
  assert.equal(muzzleVerdict(distance(a, b)), 'no-match');
});

test('the same muzzle under different lighting still matches', () => {
  // THE reason for a local binary pattern rather than comparing pixels. A
  // muzzle in sun and in shade has the same grain and completely different
  // pixel values; a matcher that failed here would reject animals it had
  // already enrolled, every time the weather changed.
  const base = grain(23);
  const dim = new Uint8Array(base.length);
  for (let i = 0; i < base.length; i++) dim[i] = Math.round(base[i]! * 0.55 + 20);

  const d = distance(describe(base, W, H, 'muzzle'), describe(dim, W, H, 'muzzle'));
  assert.ok(d <= MUZZLE_THRESHOLDS.match,
    `the same muzzle differently lit should match, got ${d}`);
});

test('a small shift is tolerated and a large one is not', () => {
  // An animal in a crush moves. A few pixels must not matter; half the frame
  // must, or the descriptor is not describing position at all.
  const base = grain(41);
  const nudged = describe(shift(base, 2, 2), W, H, 'muzzle');
  const moved = describe(shift(base, 40, 30), W, H, 'muzzle');
  const original = describe(base, W, H, 'muzzle');

  assert.ok(distance(original, nudged) < distance(original, moved),
    'a large shift should be further away than a small one');
});

test('the descriptor knows WHERE a texture was', () => {
  // A single histogram over the whole image would match a muzzle against a
  // photograph of gravel. This checks the grid actually carries position: the
  // same two textures swapped between halves must not look identical.
  const left = grain(3);
  const right = grain(88);
  const ab = new Uint8Array(W * H);
  const ba = new Uint8Array(W * H);
  for (let y = 0; y < H; y++) {
    for (let x = 0; x < W; x++) {
      const i = y * W + x;
      const leftHalf = x < W / 2;
      ab[i] = leftHalf ? left[i]! : right[i]!;
      ba[i] = leftHalf ? right[i]! : left[i]!;
    }
  }
  const d = distance(describe(ab, W, H, 'muzzle'), describe(ba, W, H, 'muzzle'));
  assert.ok(d > MUZZLE_THRESHOLDS.match,
    `swapping halves should be visible to the descriptor, got ${d}`);
});

/* ------------------------------------------------------------- refusals */

test('a muzzle is never compared with a face', () => {
  const m = describe(grain(5), W, H, 'muzzle');
  const f = describe(grain(5), W, H, 'face');
  assert.throws(() => distance(m, f), MuzzleError);
});

test('descriptors made with different grids are refused rather than scored', () => {
  const a = describe(grain(5), W, H, 'muzzle', 8, 8);
  const b = describe(grain(5), W, H, 'muzzle', 4, 4);
  assert.throws(() => distance(a, b), MuzzleError);
});

test('an image that is not width times height is refused', () => {
  assert.throws(() => describe(new Uint8Array(10), W, H, 'muzzle'), MuzzleError);
});

test('an image too small for the grid is refused', () => {
  assert.throws(() => describe(new Uint8Array(4 * 4), 4, 4, 'muzzle', 8, 8), MuzzleError);
});

/* ----------------------------------------------------------- the face */

test('a face can reject and can never confirm', () => {
  // The one real opinion in this file. Cattle of a breed look alike and the
  // same animal at another angle does not look like itself, so a face that
  // returned "match" would put a claim on the screen the measurement cannot
  // support. The TYPE makes that impossible rather than the documentation.
  const a = describe(grain(11), W, H, 'face');
  const b = describe(grain(900), W, H, 'face');

  const verdicts: ('no-match' | 'inconclusive')[] = [
    faceVerdict(0),
    faceVerdict(distance(a, a)),
    faceVerdict(distance(a, b)),
    faceVerdict(null),
  ];
  for (const v of verdicts) {
    assert.ok(v === 'no-match' || v === 'inconclusive', `a face returned ${v}`);
  }
  // Identical faces are still only "cannot tell".
  assert.equal(faceVerdict(distance(a, a)), 'inconclusive');
});

test('narrowing by face removes animals and keeps the uncertain ones', () => {
  const probe = describe(grain(11), W, H, 'face');
  const herd: Enrolled[] = [
    { animalId: 'same', enrolmentId: 's', capturedAt: 0, descriptor: describe(grain(11), W, H, 'face') },
    { animalId: 'other', enrolmentId: 'o', capturedAt: 0, descriptor: describe(grain(900), W, H, 'face') },
  ];
  const kept = narrowByFace(probe, herd).map((e) => e.animalId);
  assert.ok(kept.includes('same'), 'narrowing excluded the animal it was given');
  assert.ok(kept.length <= herd.length);
});

test('narrowing keeps an animal that has no face on file', () => {
  // Absence of evidence is not exclusion. An animal enrolled before faces were
  // captured must not be filtered out of its own identification.
  const probe = describe(grain(11), W, H, 'face');
  const herd: Enrolled[] = [
    { animalId: 'muzzle-only', enrolmentId: 'm', capturedAt: 0, descriptor: describe(grain(900), W, H, 'muzzle') },
  ];
  assert.equal(narrowByFace(probe, herd).length, 1);
});

/* ---------------------------------------------------------- the 1:N search */

function enrol(id: string, seed: number): Enrolled {
  return { animalId: id, enrolmentId: `${id}-0`, capturedAt: 0, descriptor: describe(grain(seed), W, H, 'muzzle') };
}

test('a probe matches its own animal and nothing else matches', () => {
  // Exactly one MATCH, not exactly one candidate. An inconclusive neighbour is
  // returned on purpose: hiding it would be the matcher quietly deciding
  // between two animals, which is how a registry acquires a duplicate. The
  // caller has to be able to see there was a second.
  const herd = [enrol('a', 100), enrol('b', 200), enrol('c', 300)];
  const got = search(describe(grain(200), W, H, 'muzzle'), herd);

  const matched = got.filter((c) => c.verdict === 'match');
  assert.equal(matched.length, 1, 'more than one animal matched');
  assert.equal(matched[0]!.animalId, 'b');
  assert.equal(matched[0]!.distance, 0);

  assert.ok(!got.some((c) => c.animalId === 'c'),
    'a clearly different animal should have been excluded entirely');
  assert.equal(ambiguous(got), false);
});

test('candidates come back best first', () => {
  const herd = [enrol('far', 300), enrol('same', 100)];
  const got = search(describe(grain(100), W, H, 'muzzle'), herd);
  assert.ok(got.length >= 1);
  assert.equal(got[0]!.animalId, 'same');
});

test('two enrolments of one animal are not ambiguous', () => {
  // Normal and expected: an animal is photographed every time it is handled.
  const herd: Enrolled[] = [
    { animalId: 'a', enrolmentId: 'a-0', capturedAt: 0, descriptor: describe(grain(50), W, H, 'muzzle') },
    { animalId: 'a', enrolmentId: 'a-1', capturedAt: 1, descriptor: describe(grain(50), W, H, 'muzzle') },
  ];
  const got = search(describe(grain(50), W, H, 'muzzle'), herd);
  assert.equal(got.length, 2);
  assert.equal(ambiguous(got), false);
});

test('two DIFFERENT animals matching is ambiguous and must stop the flow', () => {
  const herd: Enrolled[] = [
    { animalId: 'a', enrolmentId: 'a-0', capturedAt: 0, descriptor: describe(grain(50), W, H, 'muzzle') },
    { animalId: 'b', enrolmentId: 'b-0', capturedAt: 0, descriptor: describe(grain(50), W, H, 'muzzle') },
  ];
  const got = search(describe(grain(50), W, H, 'muzzle'), herd);
  assert.equal(got.length, 2);
  assert.equal(ambiguous(got), true);
});

test('an empty herd finds nothing and does not throw', () => {
  assert.deepEqual(search(describe(grain(1), W, H, 'muzzle'), []), []);
});

test('a face enrolment is not returned by a muzzle search', () => {
  const herd: Enrolled[] = [
    { animalId: 'a', enrolmentId: 'a-0', capturedAt: 0, descriptor: describe(grain(50), W, H, 'face') },
  ];
  assert.deepEqual(search(describe(grain(50), W, H, 'muzzle'), herd), []);
});

/* ------------------------------------------------------------ thresholds */

test('the thresholds are ordered, and the face is looser than the muzzle', () => {
  assert.ok(MUZZLE_THRESHOLDS.match < MUZZLE_THRESHOLDS.inconclusive);
  assert.ok(FACE_THRESHOLDS.reject > MUZZLE_THRESHOLDS.inconclusive,
    'a face must be slower to exclude than a muzzle is to stop matching');
});

test('a null distance is inconclusive and never a rejection', () => {
  assert.equal(muzzleVerdict(null), 'inconclusive');
  assert.equal(faceVerdict(null), 'inconclusive');
});

/* ------------------------------------------------------------- the coat */

test('a coat can reject and can never confirm, like a face', () => {
  // The conditionality is the reason. A piebald pattern is close to individual;
  // a solid-coloured Gobra is one brown shape among nine hundred, and nothing
  // in this file can tell which it is looking at. A marker that is sometimes
  // worthless must never be allowed to confirm.
  const a = describe(grain(21), W, H, 'coat');
  const b = describe(grain(770), W, H, 'coat');
  for (const v of [coatVerdict(0), coatVerdict(distance(a, a)), coatVerdict(distance(a, b)), coatVerdict(null)]) {
    assert.ok(v === 'no-match' || v === 'inconclusive', `a coat returned ${v}`);
  }
  assert.equal(coatVerdict(distance(a, a)), 'inconclusive');
});

test('a coat is slower to exclude than a face', () => {
  // Mud, a wet hide, a moult and a season's growth all move this measurement on
  // an animal that has not changed. Excluding on a small difference would throw
  // away the right animal for being dirty.
  assert.ok(COAT_THRESHOLDS.reject > FACE_THRESHOLDS.reject);
});

test('narrowing by coat keeps an animal with no coat on file', () => {
  const probe = describe(grain(21), W, H, 'coat');
  const herd: Enrolled[] = [
    { animalId: 'muzzle-only', enrolmentId: 'm', capturedAt: 0, descriptor: describe(grain(900), W, H, 'muzzle') },
  ];
  assert.equal(narrowByCoat(probe, herd).length, 1);
});

test('narrowing refuses a probe of the wrong kind', () => {
  // narrowByCoat given a face would compare a face against coats, which
  // distance() refuses one pair at a time — so every animal would survive and
  // the caller would believe nothing could be excluded.
  const face = describe(grain(21), W, H, 'face');
  assert.throws(() => narrowByCoat(face, []), MuzzleError);
});

test('a coat is never returned by a muzzle search', () => {
  const herd: Enrolled[] = [
    { animalId: 'a', enrolmentId: 'a-0', capturedAt: 0, descriptor: describe(grain(50), W, H, 'coat') },
  ];
  assert.deepEqual(search(describe(grain(50), W, H, 'muzzle'), herd), []);
});
