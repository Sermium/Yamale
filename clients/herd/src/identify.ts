/**
 * What an identification attempt is allowed to claim.
 *
 * This file is the whole point of the application. Everything else — the
 * screens, the replica, the queue — is plumbing around one rule:
 *
 *     A CONFIDENT WRONG ANSWER ABOUT WHICH BEAST THIS IS IS WORSE THAN NO
 *     ANSWER AT ALL.
 *
 * `iris.ts` already refuses to collapse a search to one identifier, and says
 * why: two animals both scoring 0.30 happens with twins, with a re-enrolment
 * recorded against the wrong animal, and with a deliberate substitution. This
 * turns its `Candidate[]` into a value a screen can render without anywhere to
 * put a guess. There is no member of `Identification` that names one animal
 * from a weak comparison, because the type is what stops a later screen doing
 * it.
 *
 * # The distinctions that are load-bearing
 *
 * 1. An IDENTIFIER resolved is not a biometric match. A tag read back or a
 *    national identifier typed in is exact — the chain either holds that record
 *    or it does not. §3 is blunt that a tag "can be defeated with a pair of
 *    pliers", so `identified` still carries what it was resolved by, and the
 *    screen asks for a second marker before anything is signed.
 *
 * 2. NOTHING TO COMPARE AGAINST is not NO MATCH. The chain holds no templates
 *    at all, by design (§2: a template "is not a secret that can be rotated").
 *    They live on the device and in the jurisdiction's service. So a herd of
 *    nine animals with no enrolments returns `nothing-enrolled`, and a herd
 *    where four of nine are enrolled returns a `no-candidate` that says five
 *    animals were never even considered. Reporting either as "this is not one
 *    of your animals" would be a false negative dressed as a finding.
 *
 * 3. AN OFFLINE SEARCH IS WITHIN ONE HERD. §5 scopes the replica to the
 *    keeper's own animals, so a local miss is a statement about forty beasts
 *    and not about twenty-one million. Every outcome carries its `scope`, and
 *    the screen's sentence changes with it.
 *
 * 4. AMBIGUOUS STOPS. Two different animals at `match` is `iris.ts`'s
 *    `ambiguous()`, and the only thing this application does with it is show
 *    both and ask for a person.
 */

import { ambiguous, type Candidate, type Enrolled, type IrisTemplate, search, THRESHOLDS } from './iris.ts';

/** Where the search could reach. */
export type Scope =
  /** The templates on this device, which are one keeper's animals. §5. */
  | 'herd'
  /** The jurisdiction's biometric service. §7 step 4 — does not exist yet. */
  | 'national';

/** How an identifier lookup was made. */
export type By = 'tag' | 'national-id';

/**
 * What an attempt produced.
 *
 * Every member either names an animal that an exact identifier resolved, or
 * names none at all. `biometric-match` is the one place a comparison names a
 * single animal, and it is reachable only when `iris.ts` returned at least one
 * candidate at `match` and `ambiguous()` was false — and even then it carries
 * `secondMarkerNeeded`, because §3's recommendation is two independent markers
 * and a disagreement between them being a finding rather than an error.
 */
export type Identification =
  /** An exact identifier resolved to a record. */
  | { kind: 'identified'; by: By; value: string; animalId: string }
  /** The chain answered that no record carries this identifier. */
  | { kind: 'not-found'; by: By; value: string; scope: Scope }
  /** One animal, and only one, at `match`. */
  | {
      kind: 'biometric-match';
      scope: Scope;
      best: Candidate;
      /** Every candidate for this same animal — an animal has several captures
       *  over its life and matching two of them is normal, not a conflict. */
      supporting: Candidate[];
      /** The best candidate belonging to a DIFFERENT animal, when there is one.
       *  Always shown: a near miss on a beast the operator believes they know
       *  is information, and hiding it is how a duplicate enters a registry. */
      runnerUp: Candidate | null;
      secondMarkerNeeded: true;
    }
  /** Two or more DIFFERENT animals at `match`. Stops, and asks for a person. */
  | { kind: 'ambiguous'; scope: Scope; candidates: Candidate[] }
  /**
   * Something came back, and none of it is an identification.
   *
   * `near-threshold` is the 0.32–0.38 band: a genuine capture degraded by dust,
   * motion or a partly closed lid lands here, and so does an impostor. Thirty
   * seconds of re-capture is the answer; a verdict is not.
   *
   * `not-comparable` is the masks overlapping too little for any number to
   * mean anything — `iris.ts` returns a null distance and this carries that
   * through rather than printing the zero a coercion would produce.
   */
  | { kind: 'inconclusive'; scope: Scope; why: 'near-threshold' | 'not-comparable'; candidates: Candidate[] }
  /** Compared against everything available, and nothing came close. */
  | { kind: 'no-candidate'; scope: Scope; compared: number; withoutTemplate: number }
  /** There was nothing to compare against. NOT a statement about the animal. */
  | { kind: 'nothing-enrolled'; scope: Scope; animalsInHerd: number }
  /** The capture itself failed. No comparison was made. */
  | { kind: 'no-capture'; refusal: string }
  /** Nothing was asked: no signal, or the node did not answer. */
  | { kind: 'unasked'; reason: 'offline' | 'unreachable'; detail: string };

/**
 * Turn a 1:N search into something a screen may render.
 *
 * `enrolledCount` and `herdSize` are both required and both used. The gap
 * between them is the number of animals that could not have matched because
 * nobody has ever enrolled them, and it is the difference between "this is not
 * your animal" and "five of your animals were not in the comparison".
 */
export function resolve(
  probe: IrisTemplate,
  enrolled: readonly Enrolled[],
  opts: { scope: Scope; herdSize: number; thresholds?: typeof THRESHOLDS; maxRotation?: number },
): Identification {
  const { scope, herdSize } = opts;

  const animalsWithTemplate = new Set(enrolled.map((e) => e.animalId));
  const withoutTemplate = Math.max(0, herdSize - animalsWithTemplate.size);

  if (enrolled.length === 0) {
    return { kind: 'nothing-enrolled', scope, animalsInHerd: herdSize };
  }

  const candidates = search(probe, enrolled, {
    thresholds: opts.thresholds,
    maxRotation: opts.maxRotation,
  });

  return fromCandidates(candidates, { scope, compared: enrolled.length, withoutTemplate });
}

/**
 * The classification itself, split out so it can be tested against candidate
 * lists that are laborious to provoke from real templates.
 *
 * The order of the branches is the safety property. `ambiguous` is checked
 * before any single animal is picked; the `match` branch is entered only after
 * that check has passed.
 */
export function fromCandidates(
  candidates: readonly Candidate[],
  ctx: { scope: Scope; compared: number; withoutTemplate: number },
): Identification {
  const { scope } = ctx;

  if (candidates.length === 0) {
    return {
      kind: 'no-candidate',
      scope,
      compared: ctx.compared,
      withoutTemplate: ctx.withoutTemplate,
    };
  }

  // Two different animals both at `match`. Nothing further is decided here.
  if (ambiguous(candidates)) {
    return { kind: 'ambiguous', scope, candidates: [...candidates] };
  }

  const matched = candidates.filter((c) => c.verdict === 'match');
  if (matched.length > 0) {
    // `ambiguous()` has already established these are all one animal.
    const best = matched.reduce((a, b) => (b.distance < a.distance ? b : a));
    const supporting = matched.filter((c) => c.enrolmentId !== best.enrolmentId);
    const others = candidates.filter((c) => c.animalId !== best.animalId);
    const runnerUp = others.length
      ? others.reduce((a, b) => (b.distance < a.distance ? b : a))
      : null;
    return { kind: 'biometric-match', scope, best, supporting, runnerUp, secondMarkerNeeded: true };
  }

  // Everything left is `inconclusive` — `search()` already dropped `no-match`.
  return {
    kind: 'inconclusive',
    scope,
    why: 'near-threshold',
    candidates: [...candidates],
  };
}

/**
 * The comparison could not be made at all.
 *
 * Reached when every enrolment's mask overlapped the probe's too little for a
 * distance to mean anything — `iris.ts` returns a null distance and `search()`
 * skips those, so they arrive here as an empty list that is NOT a no-match.
 * The caller distinguishes the two by counting comparable enrolments itself;
 * this is the constructor for the case where none were.
 */
export function notComparable(scope: Scope): Identification {
  return { kind: 'inconclusive', scope, why: 'not-comparable', candidates: [] };
}

/* --------------------------------------------------------- what to do next */

/**
 * The only three things a screen ever does with an identification.
 *
 * `stop` is not a failure state. It is the state in which the application is
 * working correctly and the answer is that a person has to look.
 */
export type NextStep =
  /** One record is on screen and may be opened and acted on. */
  | { step: 'proceed'; animalId: string; confirmed: boolean }
  /** Try again: a better capture, a different marker, or a connection. */
  | { step: 'retry'; because: 'quality' | 'near-threshold' | 'nothing-enrolled' | 'offline' }
  /** Do not proceed. An accredited person must decide. */
  | { step: 'stop'; because: 'ambiguous' | 'not-found' | 'no-candidate' };

export function nextStep(id: Identification): NextStep {
  switch (id.kind) {
    case 'identified':
      // Exact, and still not `confirmed`: §3's two independent markers. The
      // screen shows the record and asks for the second one.
      return { step: 'proceed', animalId: id.animalId, confirmed: false };
    case 'biometric-match':
      return { step: 'proceed', animalId: id.best.animalId, confirmed: false };
    case 'ambiguous':
      return { step: 'stop', because: 'ambiguous' };
    case 'not-found':
      return { step: 'stop', because: 'not-found' };
    case 'no-candidate':
      return { step: 'stop', because: 'no-candidate' };
    case 'inconclusive':
      return { step: 'retry', because: 'near-threshold' };
    case 'nothing-enrolled':
      return { step: 'retry', because: 'nothing-enrolled' };
    case 'no-capture':
      return { step: 'retry', because: 'quality' };
    case 'unasked':
      return { step: 'retry', because: 'offline' };
  }
}

/**
 * Every animal an identification points at, best first.
 *
 * Used by the screen to render the candidate list. For `ambiguous` this is
 * several animals and the screen must present them as peers — no ordering
 * emphasis, no default selection, no primary button. The order here is by
 * distance because a list has to have one, and the screen makes it visually
 * flat.
 */
export function animalsNamed(id: Identification): string[] {
  switch (id.kind) {
    case 'identified': return [id.animalId];
    case 'biometric-match': return [id.best.animalId];
    case 'ambiguous': case 'inconclusive': {
      const seen: string[] = [];
      for (const c of id.candidates) if (!seen.includes(c.animalId)) seen.push(c.animalId);
      return seen;
    }
    default: return [];
  }
}

/**
 * A distance, as a number a person may be shown — or null, meaning show none.
 *
 * There is no percentage here and there will not be one. A normalised Hamming
 * distance is not a probability, and 0.30 is not "70% confident": converting it
 * would manufacture a figure with no meaning, which somebody would then quote
 * in a dispute. The screen prints the distance itself, to two places, labelled
 * as a distance, with the operating threshold beside it so the reader can see
 * which side of it they are on.
 */
export function shownDistance(c: Candidate | null | undefined): string | null {
  if (!c || !Number.isFinite(c.distance)) return null;
  return c.distance.toFixed(2);
}

/**
 * How much of the two templates was actually comparable, as a fraction.
 *
 * Shown beside every distance. A 0.31 over a quarter of the bits and a 0.31
 * over all of them are not the same claim, and only one of them is worth
 * acting on — `iris.ts` already refuses comparisons below
 * `MIN_COMPARABLE_FRACTION`, but the reader is owed the number rather than
 * being told it passed a gate they cannot see.
 */
export function comparableFraction(c: Candidate, bits: number): number | null {
  if (!bits) return null;
  return c.comparedBits / bits;
}
