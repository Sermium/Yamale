/**
 * Matching a beast by the print on its muzzle, and by the shape of its face.
 *
 * # Why the muzzle and not the face
 *
 * The bovine muzzle carries a pattern of ridges and beads — the *grain* — that
 * is laid down before birth, does not change with age, and differs between
 * identical twins. It is the one cattle biometric with a long literature behind
 * it: inked muzzle prints have been used as proof of identity in herd books
 * since the 1920s, and the pattern is what a photograph of it captures.
 *
 * The FACE is a much weaker signal and this file treats it as one. Two animals
 * of the same breed, age and sex have close to the same face, and the same
 * animal photographed from ten degrees further round does not. So the face is
 * here to NARROW a search — it is worth a great deal when it says "not these
 * four hundred" and almost nothing when it says "this one" — and the code says
 * so where it would otherwise be tempting to believe it.
 *
 * That asymmetry is the whole design: `compareMuzzle` can support an identity
 * claim, and `compareFace` may only ever reject one.
 *
 * # What this is, mechanically
 *
 * A local binary pattern histogram over a grid. Each pixel is described by
 * whether its neighbours are brighter or darker than it, which is a statement
 * about local TEXTURE — the grain — and is unchanged by any lighting that
 * affects a neighbourhood uniformly. That last property is why it is used
 * instead of comparing pixels: a muzzle photographed in sun and in shade has
 * the same grain and completely different pixel values.
 *
 * Histograms are compared per cell and the cell scores combined, so the
 * descriptor keeps a coarse idea of WHERE each texture was. A single histogram
 * over the whole image would match a muzzle against a photograph of gravel.
 *
 * # Read the warning on the thresholds
 *
 * Exactly as in iris.ts: these numbers have never seen an animal. They exist so
 * the screens have three answers to render and the arithmetic has a shape. They
 * must be re-derived from field captures before anybody relies on them, and
 * until then this file can say "these two are different" far more safely than
 * it can say "these two are the same".
 */

/* ------------------------------------------------------------ descriptors */

/** A grid of local binary pattern histograms. */
export interface Descriptor {
  /** Which part of the animal this describes. */
  kind: 'muzzle' | 'face';
  /** Cells across and down. */
  cols: number;
  rows: number;
  /** cols * rows * BINS, normalised so each cell's histogram sums to 1. */
  bins: Float32Array;
}

/**
 * Uniform LBP over 8 neighbours, folded to 59 bins.
 *
 * "Uniform" means patterns with at most two transitions between 0 and 1 around
 * the circle. There are 58 of those and they account for the overwhelming
 * majority of what appears in a natural texture; everything else goes in one
 * shared bin. The point is not compression — it is that the non-uniform
 * patterns are mostly noise, and giving each of them its own bin lets noise
 * vote as loudly as structure.
 */
const BINS = 59;

const UNIFORM_BIN = (() => {
  const table = new Uint8Array(256).fill(58);
  let next = 0;
  for (let code = 0; code < 256; code++) {
    let transitions = 0;
    for (let i = 0; i < 8; i++) {
      const a = (code >> i) & 1;
      const b = (code >> ((i + 1) % 8)) & 1;
      if (a !== b) transitions++;
    }
    if (transitions <= 2) table[code] = next++;
  }
  return table;
})();

export class MuzzleError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'MuzzleError';
  }
}

/**
 * Describe an image.
 *
 * `luma` is one byte per pixel, which is what `device.ts` hands out in a
 * `Frame`. The grid is deliberately coarse: a finer one is more discriminating
 * and far less tolerant of the animal turning its head, and an animal in a
 * crush turns its head.
 */
export function describe(
  luma: Uint8Array,
  width: number,
  height: number,
  kind: Descriptor['kind'],
  cols = 8,
  rows = 8,
): Descriptor {
  if (luma.length !== width * height) {
    throw new MuzzleError('the image is not width * height bytes');
  }
  if (width < cols * 3 || height < rows * 3) {
    throw new MuzzleError('the image is too small for the grid');
  }

  const bins = new Float32Array(cols * rows * BINS);
  const counts = new Uint32Array(cols * rows);

  // The border is skipped: a pixel on the edge has no ring of neighbours, and
  // wrapping or clamping would invent a pattern that was never photographed.
  for (let y = 1; y < height - 1; y++) {
    const cellY = Math.min(rows - 1, Math.floor((y * rows) / height));
    for (let x = 1; x < width - 1; x++) {
      const centre = luma[y * width + x]!;
      let code = 0;
      code |= (luma[(y - 1) * width + x - 1]! >= centre ? 1 : 0) << 0;
      code |= (luma[(y - 1) * width + x]! >= centre ? 1 : 0) << 1;
      code |= (luma[(y - 1) * width + x + 1]! >= centre ? 1 : 0) << 2;
      code |= (luma[y * width + x + 1]! >= centre ? 1 : 0) << 3;
      code |= (luma[(y + 1) * width + x + 1]! >= centre ? 1 : 0) << 4;
      code |= (luma[(y + 1) * width + x]! >= centre ? 1 : 0) << 5;
      code |= (luma[(y + 1) * width + x - 1]! >= centre ? 1 : 0) << 6;
      code |= (luma[y * width + x - 1]! >= centre ? 1 : 0) << 7;

      const cellX = Math.min(cols - 1, Math.floor((x * cols) / width));
      const cell = cellY * cols + cellX;
      bins[cell * BINS + UNIFORM_BIN[code]!]! += 1;
      counts[cell]!++;
    }
  }

  for (let cell = 0; cell < counts.length; cell++) {
    const n = counts[cell]!;
    if (n === 0) continue;
    for (let b = 0; b < BINS; b++) bins[cell * BINS + b]! /= n;
  }
  return { kind, cols, rows, bins };
}

/* ------------------------------------------------------------ comparison */

/**
 * Chi-squared distance between two descriptors, 0 (identical) upward.
 *
 * Chi-squared rather than Euclidean because these are histograms: it weights a
 * difference by how rare the bin is, so a discrepancy in a bin that is usually
 * empty counts for more than the same absolute discrepancy in a bin that is
 * always full. That is the right behaviour for texture, where the distinctive
 * part of a pattern is by definition the uncommon part.
 */
export function distance(a: Descriptor, b: Descriptor): number {
  if (a.kind !== b.kind) {
    throw new MuzzleError(`cannot compare a ${a.kind} with a ${b.kind}`);
  }
  if (a.cols !== b.cols || a.rows !== b.rows || a.bins.length !== b.bins.length) {
    throw new MuzzleError('these descriptors were made with different grids');
  }

  let total = 0;
  for (let i = 0; i < a.bins.length; i++) {
    const x = a.bins[i]!;
    const y = b.bins[i]!;
    const sum = x + y;
    if (sum > 0) total += ((x - y) * (x - y)) / sum;
  }
  // Averaged per cell so the number does not depend on the grid size, which
  // would otherwise make every threshold here a statement about `cols` as well.
  return total / (a.cols * a.rows);
}

/**
 * Thresholds.
 *
 * WRITTEN TO BE CALIBRATED, NOT BELIEVED. Nothing here has been measured
 * against a real animal. See the note at the top of this file, and iris.ts,
 * which says the same thing about its own for the same reason.
 *
 * The muzzle pair is tighter than the face pair because the muzzle is being
 * asked a harder question: the face may only ever reject.
 */
export const MUZZLE_THRESHOLDS = { match: 0.18, inconclusive: 0.30 };
export const FACE_THRESHOLDS = { reject: 0.55 };

export type Verdict = 'match' | 'inconclusive' | 'no-match';

/**
 * What a muzzle distance means.
 *
 * Three answers, and the middle one is the useful one — an operator has to be
 * able to tell "this is a different animal" from "photograph it again".
 */
export function muzzleVerdict(d: number | null): Verdict {
  if (d === null) return 'inconclusive';
  if (d <= MUZZLE_THRESHOLDS.match) return 'match';
  if (d <= MUZZLE_THRESHOLDS.inconclusive) return 'inconclusive';
  return 'no-match';
}

/**
 * What a face distance means, and it is never 'match'.
 *
 * This is the file's one real opinion. A face can exclude an animal and cannot
 * identify one: cattle of a breed look alike, and the same animal at a
 * different angle does not look like itself. Returning 'match' here would put a
 * claim on a screen that the measurement cannot support, so the type says the
 * only two answers are "not this one" and "cannot tell".
 */
export function faceVerdict(d: number | null): 'no-match' | 'inconclusive' {
  if (d === null) return 'inconclusive';
  return d > FACE_THRESHOLDS.reject ? 'no-match' : 'inconclusive';
}

/* -------------------------------------------------------------- searching */

export interface Enrolled {
  animalId: string;
  enrolmentId: string;
  capturedAt: number;
  descriptor: Descriptor;
}

export interface Candidate {
  animalId: string;
  enrolmentId: string;
  distance: number;
  verdict: Verdict;
}

/**
 * Compare a muzzle against a set of enrolments.
 *
 * Returns a LIST, best first, and never picks. The same reasoning as
 * `iris.ts.search`: an identification that silently chose between two close
 * candidates is how a registry acquires a duplicate, and the caller has to see
 * that there were two.
 */
export function search(probe: Descriptor, herd: readonly Enrolled[]): Candidate[] {
  const out: Candidate[] = [];
  for (const e of herd) {
    if (e.descriptor.kind !== probe.kind) continue;
    let d: number;
    try {
      d = distance(probe, e.descriptor);
    } catch {
      // A descriptor made with a different grid is not comparable. Skipped
      // rather than scored, because any number here would be meaningless and
      // would sort among numbers that are not.
      continue;
    }
    const verdict = muzzleVerdict(d);
    if (verdict === 'no-match') continue;
    out.push({ animalId: e.animalId, enrolmentId: e.enrolmentId, distance: d, verdict });
  }
  return out.sort((a, b) => a.distance - b.distance);
}

/**
 * Whether a result set names more than one ANIMAL.
 *
 * Several enrolments of one animal are normal and expected — it is photographed
 * again every time it is handled. Two different animals both matching is not,
 * and it must stop the flow and fetch a person: it is twins, a re-enrolment
 * filed against the wrong beast, or a substitution.
 */
export function ambiguous(candidates: readonly Candidate[]): boolean {
  const animals = new Set<string>();
  for (const c of candidates) if (c.verdict === 'match') animals.add(c.animalId);
  return animals.size > 1;
}

/**
 * Narrow a herd by face before doing the expensive thing.
 *
 * The one job a face is good for. Returns what could NOT be excluded, which is
 * deliberately how it is phrased: this function never says an animal matches,
 * only that it has not been ruled out.
 */
export function narrowByFace(probe: Descriptor, herd: readonly Enrolled[]): Enrolled[] {
  if (probe.kind !== 'face') throw new MuzzleError('narrowByFace needs a face descriptor');
  return herd.filter((e) => {
    if (e.descriptor.kind !== 'face') return true; // nothing to exclude it by
    try {
      return faceVerdict(distance(probe, e.descriptor)) !== 'no-match';
    } catch {
      return true;
    }
  });
}
