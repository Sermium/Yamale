/**
 * Registration, tested against the one thing it must never do.
 *
 * The register makes a single promise — an identifier means one animal — and
 * the way that promise dies is not fraud. It is an agent in a pen registering a
 * beast in good faith that somebody else registered last season. There are then
 * two records for one animal and nothing anywhere looks wrong.
 *
 * So most of this file is about refusals, and each one is written as the
 * failure it prevents rather than as the rule it enforces.
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { describe as describeMuzzle, type Enrolled } from './muzzle.ts';
import {
  captureAdvice,
  check,
  register,
  withParents,
  EMPTY_KNOWN,
  RegisterError,
  type Known,
} from './register.ts';

const HERD = '7';

function grain(seed: number, width = 96, height = 96): Uint8Array {
  const out = new Uint8Array(width * height);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const a = Math.sin((x * (2 + (seed % 3)) + y * (3 + (seed % 5))) * 0.21 + seed);
      const b = Math.cos((x * (5 + (seed % 4)) - y * (2 + (seed % 7))) * 0.13 - seed);
      out[y * width + x] = Math.max(0, Math.min(255, 128 + a * 50 + b * 40));
    }
  }
  return out;
}

const muzzleOf = (seed: number) => describeMuzzle(grain(seed), 96, 96, 'muzzle');

function known(over: Partial<Known> = {}): Known {
  return { ...EMPTY_KNOWN, ...over };
}

/* ------------------------------------------------------------ the happy path */

test('a new animal with a reference and an identifier registers', () => {
  const claimed = { reference: 'FORM-00412', nationalId: '221/682/001029/L' };
  const checked = check({ claimed, known: known() });
  assert.equal(checked.verdict, 'ready');

  const made = register({ claimed, known: known(), herdId: HERD }, checked);
  assert.equal(made.reference, 'FORM-00412');
  assert.equal(made.nationalId, '221/682/001029/L');
  assert.equal(made.herdId, HERD);
});

test('the identifier is stored canonically, not as it was typed', () => {
  // `221/682/1029/L` and `221/682/001029/L` are ONE animal. Storing what was
  // typed is precisely the bug x/herd/keeper/identifiers.go exists to fix, and
  // doing it here as well is not redundant: this device is in a pen with no
  // signal and cannot ask the chain anything.
  const claimed = { reference: 'FORM-1', nationalId: ' 221/682/1029/l ' };
  const made = register({ claimed, known: known(), herdId: HERD }, check({ claimed, known: known() }));
  assert.equal(made.nationalId, '221/682/001029/L');
});

test('a transponder is stored canonically too', () => {
  const claimed = { reference: 'FORM-2', tag: '982 000 000 000 003' };
  const made = register({ claimed, known: known(), herdId: HERD }, check({ claimed, known: known() }));
  assert.equal(made.tag, '982000000000003');
});

/* --------------------------------------------------------- the duplicates */

test('an identifier already on this device blocks the registration', () => {
  const k = known({ byNationalId: new Map([['221/682/001029/L', 'animal-12']]) });
  // Typed WITHOUT its leading zeros: the collision must still be seen, or the
  // check is a string comparison wearing a canonicalisation's clothes.
  const checked = check({ claimed: { reference: 'F', nationalId: '221/682/1029/L' }, known: k });
  assert.equal(checked.verdict, 'blocked');
  assert.equal(checked.verdict === 'blocked' ? checked.animalId : null, 'animal-12');
});

test('a transponder already on this device blocks the registration', () => {
  const k = known({ byTag: new Map([['982000000000003', 'animal-9']]) });
  const checked = check({ claimed: { reference: 'F', tag: '982.000.000.000.003' }, known: k });
  assert.equal(checked.verdict, 'blocked');
  assert.equal(checked.verdict === 'blocked' ? checked.animalId : null, 'animal-9');
});

test('one form cannot register two animals', () => {
  // The reference ties the record to a piece of paper. Two records against one
  // sheet means nobody can be asked which animal the sheet was about.
  const k = known({ usedReferences: new Set(['FORM-00412']) });
  const checked = check({ claimed: { reference: 'FORM-00412', nationalId: '221/682/000001/L' }, known: k });
  assert.equal(checked.verdict, 'blocked');
});

test('a muzzle that matches an existing animal asks rather than refuses', () => {
  // The asymmetry this file is built on. An identifier is issued and unique by
  // construction; a muzzle is a MEASUREMENT, taken with thresholds that have
  // never seen a real animal. Refusing outright would stop legitimate
  // registrations on the strength of a number nobody has calibrated.
  const enrolments: Enrolled[] = [
    { animalId: 'animal-3', enrolmentId: 'e1', capturedAt: 0, descriptor: muzzleOf(42) },
  ];
  const checked = check({
    claimed: { reference: 'F', nationalId: '221/682/000002/L' },
    known: known({ enrolments }),
    muzzle: muzzleOf(42),
  });
  assert.equal(checked.verdict, 'review');
  assert.deepEqual(checked.verdict === 'review' ? checked.animalIds : [], ['animal-3']);
});

test('a review can be overridden, but only deliberately', () => {
  const enrolments: Enrolled[] = [
    { animalId: 'animal-3', enrolmentId: 'e1', capturedAt: 0, descriptor: muzzleOf(42) },
  ];
  const input = {
    claimed: { reference: 'F', nationalId: '221/682/000002/L' },
    known: known({ enrolments }),
    muzzle: muzzleOf(42),
    herdId: HERD,
  };
  const checked = check(input);

  assert.throws(() => register(input, checked), RegisterError,
    'a review passed silently, so the matcher may as well not have run');
  const made = register(input, checked, true);
  assert.equal(made.nationalId, '221/682/000002/L');
});

test('a blocked check cannot be overridden at all', () => {
  // Overriding a measurement is a judgement. Overriding a collision is creating
  // the duplicate the whole file exists to prevent.
  const k = known({ byNationalId: new Map([['221/682/001029/L', 'animal-12']]) });
  const input = { claimed: { reference: 'F', nationalId: '221/682/001029/L' }, known: k, herdId: HERD };
  assert.throws(() => register(input, check(input), true), RegisterError);
});

test('a muzzle matching two different animals says so', () => {
  const enrolments: Enrolled[] = [
    { animalId: 'a', enrolmentId: 'e1', capturedAt: 0, descriptor: muzzleOf(42) },
    { animalId: 'b', enrolmentId: 'e2', capturedAt: 0, descriptor: muzzleOf(42) },
  ];
  const checked = check({
    claimed: { reference: 'F', nationalId: '221/682/000002/L' },
    known: known({ enrolments }),
    muzzle: muzzleOf(42),
  });
  assert.equal(checked.verdict, 'review');
  assert.equal(checked.verdict === 'review' ? checked.animalIds.length : 0, 2);
});

/* ------------------------------------------------------------- refusals */

test('a registration without a reference is refused', () => {
  const checked = check({ claimed: { reference: '  ', nationalId: '221/682/000001/L' }, known: known() });
  assert.equal(checked.verdict, 'blocked');
});

test('a registration with no identifier at all is refused', () => {
  // A record that can never be looked up again by anybody holding a
  // certificate or a reader is a row, not a registration.
  const claimed = { reference: 'FORM-9' };
  assert.throws(
    () => register({ claimed, known: known(), herdId: HERD }, check({ claimed, known: known() })),
    RegisterError,
  );
});

test('a malformed identifier is refused rather than stored', () => {
  for (const bad of ['221/682/0001029/L', '221/68A/001029/L', '221/682/001029']) {
    const checked = check({ claimed: { reference: 'F', nationalId: bad }, known: known() });
    assert.equal(checked.verdict, 'blocked', `${bad} was accepted`);
  }
});

test('a registration without a herd is refused', () => {
  const claimed = { reference: 'F', nationalId: '221/682/000001/L' };
  assert.throws(
    () => register({ claimed, known: known(), herdId: '  ' }, check({ claimed, known: known() })),
    RegisterError,
  );
});

/* ------------------------------------------------------------ the pedigree */

test('parents are a separate assertion carrying record ids', () => {
  const cert = withParents('animal-77', { dam: 'animal-12', sire: 'animal-30' });
  assert.equal(cert.animalId, 'animal-77');
  assert.equal(cert.pedigree.dam, 'animal-12');
  assert.equal(cert.pedigree.sire, 'animal-30');
});

test('one known parent is enough', () => {
  // An agent who knows the dam and not the sire can certify what they know.
  // Requiring both would mean recording nothing, which is worse.
  const cert = withParents('animal-77', { dam: 'animal-12', sire: '' });
  assert.equal(cert.pedigree.dam, 'animal-12');
  assert.equal(cert.pedigree.sire, '');
});

test('a certification asserting nothing is refused', () => {
  assert.throws(() => withParents('animal-77', { dam: '', sire: '' }), RegisterError);
});

test('an animal cannot be its own parent', () => {
  assert.throws(() => withParents('a', { dam: 'a', sire: '' }), RegisterError);
  assert.throws(() => withParents('a', { dam: '', sire: 'a' }), RegisterError);
});

test('the same record given as both parents is a typing error, not a pedigree', () => {
  // The one loop this device can see. It cannot walk the whole line offline,
  // and does not pretend to.
  assert.throws(() => withParents('a', { dam: 'b', sire: 'b' }), RegisterError);
});

/* -------------------------------------------------------- capture advice */

test('a poor capture is kept, and said to be poor', () => {
  // Registration is the best access to this animal the register will ever get.
  // Discarding a bad capture throws away the only one there may be.
  assert.equal(captureAdvice(0.2).keep, true);
  assert.match(captureAdvice(0.2).reason, /poor/);
  assert.equal(captureAdvice(0.9).keep, true);
  assert.match(captureAdvice(0.9).reason, /good/);
});

test('an unscored capture is not called poor', () => {
  // Null is not zero. An instrument that does not score its own captures has
  // not said the capture was bad.
  const advice = captureAdvice(null);
  assert.equal(advice.keep, true);
  assert.doesNotMatch(advice.reason, /poor/);
});
