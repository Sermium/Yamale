/**
 * The rule the whole application exists to enforce, tested directly.
 *
 * Every test here is a way the interface could name an animal it does not know.
 * They are written as the failure, not as the feature: "two animals both match
 * and it picks one" is the defect, and the assertion is that it cannot.
 *
 * The candidate lists are hand-built rather than produced from real templates,
 * on purpose. A comparison that happens to land at 0.31 is hard to arrange and
 * impossible to arrange twice; the classification has to be right for every
 * arrangement, so it is driven directly. `iris.test.ts` already proves the
 * distances themselves.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  animalsNamed,
  comparableFraction,
  fromCandidates,
  nextStep,
  notComparable,
  resolve,
  shownDistance,
} from './identify.ts';
import type { Candidate, Enrolled, IrisTemplate } from './iris.ts';

/* ------------------------------------------------------------- fixtures */

function candidate(
  animalId: string,
  distance: number,
  verdict: Candidate['verdict'],
  enrolmentId = `${animalId}-a`,
): Candidate {
  return { animalId, enrolmentId, distance, rotation: 0, comparedBits: 1800, verdict };
}

const CTX = { scope: 'herd' as const, compared: 9, withoutTemplate: 0 };

/* ------------------------------------------------------- the dangerous one */

describe('two different animals both matching', () => {
  const both = [candidate('4', 0.29, 'match'), candidate('5', 0.30, 'match')];

  it('stops, and names neither', () => {
    const got = fromCandidates(both, CTX);
    assert.equal(got.kind, 'ambiguous');
    // Twins. devnet-2 has a real pair — animals 4 and 5, both born 2020-04-18
    // to dam 2, both birth_type twin — which is exactly the case iris.ts warns
    // about, and exactly the case a registry acquires a duplicate through.
    assert.deepEqual(nextStep(got), { step: 'stop', because: 'ambiguous' });
  });

  it('shows both, as peers', () => {
    assert.deepEqual(animalsNamed(fromCandidates(both, CTX)), ['4', '5']);
  });

  it('is ambiguous even when one is clearly better than the other', () => {
    // A tenth of the scale apart and both still below the operating point. A
    // "best" here would be a guess with a decimal place on it.
    const got = fromCandidates(
      [candidate('4', 0.10, 'match'), candidate('5', 0.31, 'match')],
      CTX,
    );
    assert.equal(got.kind, 'ambiguous');
  });
});

describe('one animal matching on several of its own captures', () => {
  it('is not ambiguous — an animal has several enrolments over its life', () => {
    const got = fromCandidates(
      [candidate('7', 0.11, 'match', '7-a'), candidate('7', 0.14, 'match', '7-b')],
      CTX,
    );
    assert.equal(got.kind, 'biometric-match');
    if (got.kind !== 'biometric-match') return;
    assert.equal(got.best.enrolmentId, '7-a');
    assert.equal(got.supporting.length, 1);
    assert.equal(got.supporting[0].enrolmentId, '7-b');
    assert.equal(got.runnerUp, null);
  });

  it('still demands a second marker', () => {
    // §3: two independent markers, and a disagreement between them is a
    // finding rather than an error. A biometric match alone never closes.
    const got = fromCandidates([candidate('7', 0.05, 'match')], CTX);
    assert.equal(got.kind, 'biometric-match');
    if (got.kind !== 'biometric-match') return;
    assert.equal(got.secondMarkerNeeded, true);
    assert.deepEqual(nextStep(got), { step: 'proceed', animalId: '7', confirmed: false });
  });

  it('carries the nearest miss on a different animal', () => {
    const got = fromCandidates(
      [candidate('7', 0.11, 'match'), candidate('3', 0.35, 'inconclusive'), candidate('6', 0.37, 'inconclusive')],
      CTX,
    );
    assert.equal(got.kind, 'biometric-match');
    if (got.kind !== 'biometric-match') return;
    // A near miss on a beast the operator believes they know is information.
    assert.equal(got.runnerUp?.animalId, '3');
  });
});

/* --------------------------------------------------------- the 0.35 case */

describe('a candidate at 0.35', () => {
  it('is inconclusive, and the next step is to capture it again', () => {
    // The brief's own example. 0.35 sits between the 0.32 operating point and
    // the 0.38 upper bound: a genuine capture degraded by dust or a partly
    // closed lid lands here, and so does an impostor.
    const got = fromCandidates([candidate('4', 0.35, 'inconclusive')], CTX);
    assert.equal(got.kind, 'inconclusive');
    if (got.kind !== 'inconclusive') return;
    assert.equal(got.why, 'near-threshold');
    assert.deepEqual(nextStep(got), { step: 'retry', because: 'near-threshold' });
  });

  it('does not become a match by being the only one', () => {
    const got = fromCandidates([candidate('4', 0.379, 'inconclusive')], CTX);
    assert.equal(got.kind, 'inconclusive');
  });

  it('still lists the animal, because a near miss is worth seeing', () => {
    assert.deepEqual(animalsNamed(fromCandidates([candidate('4', 0.35, 'inconclusive')], CTX)), ['4']);
  });
});

describe('a comparison that could not be made', () => {
  it('is inconclusive, and says so differently', () => {
    // The masks overlapped too little for any number to mean anything.
    // "This capture cannot be compared" is a different statement from "this is
    // not the same animal", and iris.ts makes the same point in its own words.
    const got = notComparable('herd');
    assert.equal(got.kind, 'inconclusive');
    if (got.kind !== 'inconclusive') return;
    assert.equal(got.why, 'not-comparable');
    assert.deepEqual(got.candidates, []);
  });
});

/* -------------------------------------------------- nothing to compare to */

describe('an empty result', () => {
  it('carries how many animals were never in the comparison', () => {
    // Four of nine enrolled. The five with no template were not ruled out —
    // they were not looked at. A screen that said "not one of your animals"
    // would be wrong about five of them.
    const got = fromCandidates([], { scope: 'herd', compared: 4, withoutTemplate: 5 });
    assert.equal(got.kind, 'no-candidate');
    if (got.kind !== 'no-candidate') return;
    assert.equal(got.compared, 4);
    assert.equal(got.withoutTemplate, 5);
    assert.deepEqual(nextStep(got), { step: 'stop', because: 'no-candidate' });
  });
});

describe('no templates at all', () => {
  const probe: IrisTemplate = {
    code: new Uint8Array(256), mask: new Uint8Array(256).fill(0xff), bits: 2048,
  };

  it('is not a statement about the animal', () => {
    const got = resolve(probe, [], { scope: 'herd', herdSize: 9 });
    assert.equal(got.kind, 'nothing-enrolled');
    if (got.kind !== 'nothing-enrolled') return;
    assert.equal(got.animalsInHerd, 9);
    // Retry, not stop: the answer is "enrol some animals", not "this beast is
    // not yours". devnet-2 is exactly here — nine animals, no templates.
    assert.deepEqual(nextStep(got), { step: 'retry', because: 'nothing-enrolled' });
  });

  it('counts uncovered animals from the enrolments, not from the list length', () => {
    // Two enrolments of ONE animal is one animal covered, not two.
    const enrolled: Enrolled[] = [
      { animalId: '7', enrolmentId: '7-a', capturedAt: 1, template: probe },
      { animalId: '7', enrolmentId: '7-b', capturedAt: 2, template: probe },
    ];
    const got = resolve(probe, enrolled, { scope: 'herd', herdSize: 9 });
    // Identical template, so this matches at distance 0 — the point of the test
    // is the arithmetic that would have been reported had it not.
    assert.equal(got.kind, 'biometric-match');

    const miss: IrisTemplate = {
      code: new Uint8Array(256).fill(0xff), mask: new Uint8Array(256).fill(0xff), bits: 2048,
    };
    const away = resolve(miss, enrolled, { scope: 'herd', herdSize: 9 });
    assert.equal(away.kind, 'no-candidate');
    if (away.kind !== 'no-candidate') return;
    assert.equal(away.withoutTemplate, 8);
  });
});

/* ------------------------------------------------------------- the scope */

describe('scope', () => {
  it('travels with every biometric outcome', () => {
    // §5: an offline search is 1:N within one herd. A local miss is a statement
    // about forty beasts and not about twenty-one million, and the screen's
    // sentence changes with this field.
    for (const outcome of [
      fromCandidates([], CTX),
      fromCandidates([candidate('4', 0.35, 'inconclusive')], CTX),
      fromCandidates([candidate('4', 0.1, 'match'), candidate('5', 0.1, 'match')], CTX),
      fromCandidates([candidate('4', 0.1, 'match')], CTX),
    ]) {
      assert.equal('scope' in outcome ? outcome.scope : null, 'herd');
    }
  });
});

/* ------------------------------------------------------ numbers on screen */

describe('what a number is allowed to say', () => {
  it('prints a distance as a distance, never as a confidence', () => {
    // 0.30 is not "70% confident". A normalised Hamming distance is not a
    // probability, and converting it would manufacture a figure somebody would
    // later quote in a dispute.
    assert.equal(shownDistance(candidate('4', 0.301, 'match')), '0.30');
    assert.equal(shownDistance(null), null);
  });

  it('refuses to print anything for a candidate with no finite distance', () => {
    assert.equal(shownDistance({ ...candidate('4', NaN, 'inconclusive') }), null);
  });

  it('reports how much of the templates was comparable', () => {
    // 0.31 over a quarter of the bits and 0.31 over all of them are not the
    // same claim. The reader is owed the fraction, not a gate they cannot see.
    assert.equal(comparableFraction(candidate('4', 0.31, 'match'), 2048), 1800 / 2048);
    assert.equal(comparableFraction(candidate('4', 0.31, 'match'), 0), null);
  });
});

/* --------------------------------------------------- an exact identifier */

describe('an identifier lookup', () => {
  it('proceeds, unconfirmed', () => {
    // A tag can be defeated with a pair of pliers — the module says so. Exact
    // is not the same as confirmed, and the screen asks for a second marker.
    const got = nextStep({ kind: 'identified', by: 'tag', value: '982000000021029', animalId: '7' });
    assert.deepEqual(got, { step: 'proceed', animalId: '7', confirmed: false });
  });

  it('treats a miss as a stop, not a retry', () => {
    // The chain answered. Typing it again will not change the answer; a person
    // has to work out whether the beast is unregistered or the tag was swapped.
    const got = nextStep({ kind: 'not-found', by: 'tag', value: '982000000099999', scope: 'national' });
    assert.deepEqual(got, { step: 'stop', because: 'not-found' });
  });

  it('treats silence as a retry', () => {
    const got = nextStep({ kind: 'unasked', reason: 'offline', detail: 'no signal' });
    assert.deepEqual(got, { step: 'retry', because: 'offline' });
  });
});
