/**
 * Comparing two irises.
 *
 * This is the part of an identification system that has to be right, and it is
 * also the part that can be written and tested without an animal, a camera or a
 * device. Everything above it — the capture, the hardware, the screens — either
 * works or visibly does not. This can be subtly wrong and look fine.
 *
 * # The measure
 *
 * An iris code is a bit string: the phase of a bank of Gabor filters sampled
 * around the normalised iris, two bits per sample. Two codes from the same eye
 * agree on far more bits than two codes from different eyes, and the measure is
 * the fraction on which they disagree — the normalised Hamming distance.
 *
 * Identical eye, perfect capture: 0. Different eyes: about 0.5, because
 * unrelated iris codes are essentially independent coin flips.
 *
 * # The mask is not optional
 *
 * Eyelids, eyelashes and specular highlights cover part of every real iris, and
 * the bits under them are noise. Every template carries a mask saying which of
 * its bits are worth comparing, and the distance is computed over the bits BOTH
 * templates consider valid:
 *
 *     HD = popcount((A xor B) and maskA and maskB) / popcount(maskA and maskB)
 *
 * Ignoring the masks does not make matching slightly worse. It compares noise
 * against noise and pushes every score toward 0.5, which is the same as
 * refusing every genuine match.
 *
 * # Rotation
 *
 * A head tilts. In the normalised polar image a tilt is a horizontal shift, so
 * the comparison is repeated over a range of shifts and the BEST score is
 * taken. This is standard and it is also why the threshold cannot be borrowed
 * from a paper that did not rotate: taking the minimum of many comparisons
 * makes an impostor's best score better than a single comparison would, so a
 * rotated match needs a tighter threshold than an unrotated one.
 *
 * # What this file does NOT do
 *
 * It does not extract a template from an image. Segmenting an iris, unwrapping
 * it and running the filter bank is a research problem, and getting it wrong
 * produces templates that compare beautifully to each other and mean nothing.
 * A dedicated device does that in firmware, which is why the hardware is worth
 * having. See device.ts.
 *
 * AND THE CODE IT RETURNS IS NOT A STANDARD. An earlier version of this comment
 * said the device hands back "a finished ISO/IEC 19794-6 template". It does
 * not, and the distinction decides a purchase: **19794-6 standardises iris
 * IMAGES, not iris CODES.** The Gabor-phase code and mask this file compares
 * are a vendor's own representation, and no interchange format for them exists.
 *
 * So templates from one manufacturer cannot be compared with another's, ever.
 * The images can be re-processed if they were kept; the codes cannot be
 * translated. Choosing a device is choosing a matcher for the life of every
 * animal enrolled with it, and the mitigation — if the operation can afford the
 * storage and the privacy exposure — is to retain the IMAGE alongside the code,
 * because that is the only artefact a second vendor could ever re-enrol from.
 */

/**
 * An iris template: the code, the mask, and where it came from.
 *
 * `bits` is the code's length in bits. Both arrays are bit-packed, so a 2048-bit
 * code is 256 bytes. Kept as bytes rather than a string because this is compared
 * thousands of times in a 1:N search and a string would be re-parsed each time.
 */
export interface IrisTemplate {
  /** The iris code, bit-packed, most significant bit first within each byte. */
  code: Uint8Array;
  /** Which bits are valid. Same length as `code`. A 1 means "comparable". */
  mask: Uint8Array;
  /** Length of the code in bits. */
  bits: number;
}

/** What a comparison produced. */
export interface Comparison {
  /**
   * Normalised Hamming distance over the bits both masks admit, 0 to 1.
   * Lower is more alike. `null` when the masks left nothing to compare.
   */
  distance: number | null;
  /** The rotation, in bit positions, at which the best score was found. */
  rotation: number;
  /** How many bits the two masks agreed were valid. */
  comparedBits: number;
}

/**
 * How much of a template has to be usable before a comparison means anything.
 *
 * Two templates that overlap on a tenth of their bits will produce a distance,
 * and it will be noise. Rejecting the comparison is the honest answer: "this
 * capture cannot be compared" is a different statement from "this is not the
 * same animal", and an operator needs to be told which one they have.
 */
export const MIN_COMPARABLE_FRACTION = 0.4;

/**
 * Compare two templates at one rotation.
 *
 * THE SIGN CONVENTION, stated because it is invisible and gets flipped:
 * `shiftBits` advances the position read in B. Bit `i` of A is compared with
 * bit `i + shiftBits` of B. So if B is A rotated LATER — B(i) = A(i - k) — then
 * the alignment is found at `+k`, not at `-k`.
 *
 * Exported for the tests, which need to check a single alignment without the
 * minimum-over-rotations doing the work.
 */
export function distanceAt(
  a: IrisTemplate,
  b: IrisTemplate,
  shiftBits: number,
): { distance: number | null; comparedBits: number } {
  if (a.bits !== b.bits) return { distance: null, comparedBits: 0 };

  let differing = 0;
  let comparable = 0;

  // Walked a bit at a time rather than a byte at a time, because a rotation is
  // not a whole number of bytes. Slower and correct; the fast version packs the
  // shift into word operations and is worth writing only once this is proven.
  for (let i = 0; i < a.bits; i++) {
    const j = (((i + shiftBits) % a.bits) + a.bits) % a.bits;
    if (!bitAt(a.mask, i) || !bitAt(b.mask, j)) continue;
    comparable++;
    if (bitAt(a.code, i) !== bitAt(b.code, j)) differing++;
  }

  if (comparable === 0) return { distance: null, comparedBits: 0 };
  return { distance: differing / comparable, comparedBits: comparable };
}

function bitAt(bytes: Uint8Array, bit: number): boolean {
  const byte = bytes[bit >> 3];
  if (byte === undefined) return false;
  return (byte & (0x80 >> (bit & 7))) !== 0;
}

/**
 * Compare two templates, taking the best alignment within ±`maxRotation` bits.
 *
 * Returns the lowest distance found and the rotation that produced it. A
 * comparison whose overlapping mask is too small for the result to mean
 * anything comes back with a null distance rather than a number nobody should
 * act on.
 */
export function compare(
  a: IrisTemplate,
  b: IrisTemplate,
  maxRotation = 16,
): Comparison {
  let best: Comparison = { distance: null, rotation: 0, comparedBits: 0 };

  for (let shift = -maxRotation; shift <= maxRotation; shift++) {
    const got = distanceAt(a, b, shift);
    if (got.distance === null) continue;
    if (got.comparedBits < a.bits * MIN_COMPARABLE_FRACTION) continue;
    if (best.distance === null || got.distance < best.distance) {
      best = { distance: got.distance, rotation: shift, comparedBits: got.comparedBits };
    }
  }
  return best;
}

/**
 * What a distance means, in the only three words an operator should act on.
 *
 * Deliberately three and not two. A system that answers only yes or no makes
 * the decision at a threshold nobody can see, and the interesting captures are
 * exactly the ones near it — where the honest answer is "capture it again",
 * which costs thirty seconds, rather than a verdict that costs an argument.
 */
export type Verdict = 'match' | 'inconclusive' | 'no-match';

/**
 * Thresholds.
 *
 * WRITTEN TO BE CALIBRATED, NOT BELIEVED. These are the values the iris
 * literature uses for human irises captured under near-infrared with a
 * cooperative subject, and they are a starting point rather than an answer:
 *
 *   - 0.32 is the classical operating point for a rotated comparison of
 *     2048-bit codes, chosen because the impostor distribution of independent
 *     codes is tight around 0.5 and essentially nothing crosses down to 0.32 by
 *     chance.
 *   - The gap up to 0.38 is the region where a genuine capture degraded by
 *     dust, motion or a partly closed lid lands. Calling those no-match loses
 *     real animals; calling them match is how a registry gets a wrong beast in
 *     it. They are sent back for another capture.
 *
 * These MUST be re-derived from field captures of the species in question
 * before anybody relies on them. The procedure is in the design document:
 * capture a few hundred genuine pairs and a few thousand impostor pairs, plot
 * both distributions, and put the thresholds where they actually separate. A
 * threshold carried over from human irises in a laboratory is a guess wearing a
 * number's clothes.
 */
export const THRESHOLDS = {
  match: 0.32,
  inconclusive: 0.38,
} as const;

export function verdictOf(
  distance: number | null,
  thresholds: { match: number; inconclusive: number } = THRESHOLDS,
): Verdict {
  // A comparison that could not be made is not a rejection. The caller is
  // expected to distinguish these, and the type makes that awkward to skip.
  if (distance === null) return 'inconclusive';
  if (distance <= thresholds.match) return 'match';
  if (distance <= thresholds.inconclusive) return 'inconclusive';
  return 'no-match';
}

/* ------------------------------------------------------------ the 1:N search */

/** One enrolled template, and the animal it belongs to. */
export interface Enrolled {
  animalId: string;
  /** Which capture of that animal this is. An animal has several over its life. */
  enrolmentId: string;
  capturedAt: number;
  template: IrisTemplate;
}

/** A candidate the search turned up. */
export interface Candidate {
  animalId: string;
  enrolmentId: string;
  distance: number;
  rotation: number;
  comparedBits: number;
  verdict: Verdict;
}

/**
 * Search a probe against a set of enrolments.
 *
 * Returns candidates sorted best first, and only those that are not outright
 * rejections — an operator wants to see the near misses, because a near miss on
 * a beast they believe they know is information.
 *
 * # Why this returns a list rather than an answer
 *
 * A 1:N search that returns "it is animal 412" hides the case that matters: two
 * animals both scoring 0.30. That happens with twins, with a re-enrolment
 * recorded against the wrong animal, and with a deliberate substitution — and
 * each of those is something a person must look at. Collapsing it to one
 * identifier is how a registry quietly acquires a duplicate.
 *
 * The caller decides what to do with several strong candidates. `ambiguous`
 * below says when there are.
 */
export function search(
  probe: IrisTemplate,
  enrolled: readonly Enrolled[],
  options: { maxRotation?: number; thresholds?: typeof THRESHOLDS; limit?: number } = {},
): Candidate[] {
  const { maxRotation = 16, thresholds = THRESHOLDS, limit = 10 } = options;

  const out: Candidate[] = [];
  for (const e of enrolled) {
    const got = compare(probe, e.template, maxRotation);
    if (got.distance === null) continue;
    const verdict = verdictOf(got.distance, thresholds);
    if (verdict === 'no-match') continue;
    out.push({
      animalId: e.animalId,
      enrolmentId: e.enrolmentId,
      distance: got.distance,
      rotation: got.rotation,
      comparedBits: got.comparedBits,
      verdict,
    });
  }

  out.sort((x, y) => x.distance - y.distance);
  return out.slice(0, limit);
}

/**
 * True when the result should not be acted on without a person looking.
 *
 * Two DIFFERENT animals both matching is the dangerous case, and it is not the
 * same as one animal matching on two of its own enrolments — which is normal
 * and expected, because an animal has several captures over its life.
 */
export function ambiguous(candidates: readonly Candidate[]): boolean {
  const matched = candidates.filter((c) => c.verdict === 'match');
  const animals = new Set(matched.map((c) => c.animalId));
  return animals.size > 1;
}
