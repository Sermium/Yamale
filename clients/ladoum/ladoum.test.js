// Tests for the register's reading logic.
//
// None of this could be tested before. `chain.js` computed the wallet URL from
// `window.location.origin` at import time, so the module graph was unloadable
// outside a browser, and the only way to reach a function in ladoum.js was to
// open a page and look at it. One line that touched `window` too early decided
// that everything behind it was untestable — and the bugs that shipped were
// exactly the ones a test catches: an import that did not resolve, a decoder
// reading the previous module's field order.
//
// What follows is the logic that turns records into what a buyer reads. It is
// all pure: no chain, no network, no browser.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  asTree,
  certificate,
  evidenceLevel,
  hasParent,
  pedigreeCoverage,
  sexLabel,
  speciesLabel,
} from './ladoum.js';

/** An animal record as the decoder produces one. */
const animal = (over = {}) => ({
  id: '3',
  herd_id: '1',
  national_id: '221/682/021029/L',
  tag: '982000000000003',
  level: 'LEVEL_CERTIFIED',
  name: 'Classico',
  sex: 'SEX_MALE',
  coat: 'blanc tacheté',
  born_on: '2026-08-25',
  birth_type: 'single',
  sire_id: '1',
  dam_id: '2',
  dna_hash: 'sha256:adn',
  photo_hash: 'sha256:photo',
  holder: 'yml1holder',
  authority: 'yml1office',
  status: 'STATUS_ACTIVE',
  registered_at: '94464',
  ...over,
});

/* ------------------------------------------------------- unknown parentage */

test('unknown parentage is the string zero, and hasParent knows it', () => {
  // The trap the whole u64-as-string decision exists for. `'0'` is truthy, so
  // a naive `if (animal.sire_id)` says every founder has a father.
  assert.equal(hasParent('0'), false);
  assert.equal(hasParent('1'), true);
  assert.equal(hasParent(undefined), false);
  assert.equal(hasParent(null), false);
  assert.ok('0', 'the string zero is truthy — which is why hasParent has to exist');
});

/* --------------------------------------------------------- evidence level */

test('the evidence level is read from the chain, not inferred here', () => {
  // It used to be deduced from the presence of two deeds, which meant the page
  // graded its own evidence. Now it is the field the chain writes and refuses
  // on: a record saying "filiation vérifiée" means consensus accepted it.
  const ev = evidenceLevel(animal());
  assert.equal(ev.level, 'verifiee');
  assert.equal(ev.chain, true);
  assert.match(ev.why, /Niveau 3/);
});

test('a tagged but uncertified animal is identified, not verified', () => {
  const ev = evidenceLevel(animal({ level: 'LEVEL_INDIVIDUAL', dna_hash: '' }));
  assert.equal(ev.level, 'pucee');
  assert.match(ev.label, /filiation déclarée/);
});

test('an untagged individual record says what it does not have', () => {
  const ev = evidenceLevel(animal({ level: 'LEVEL_INDIVIDUAL', tag: '', dna_hash: '' }));
  assert.equal(ev.level, 'individuelle');
  assert.match(ev.why, /rien ne la rattache/);
});

test('a herd-level record does not masquerade as an animal', () => {
  // Level 1 is the level that can cover a national herd, and saying so is the
  // point: a buyer must not read "registered" as "this animal is registered".
  const ev = evidenceLevel(animal({ level: 'LEVEL_HERD', tag: '', dna_hash: '' }));
  assert.equal(ev.level, 'troupeau');
  assert.match(ev.why, /cheptel national/);
});

test('an unspecified level falls back to the paper certificate’s own level', () => {
  const ev = evidenceLevel(animal({ level: 'LEVEL_UNSPECIFIED', tag: '', dna_hash: '' }));
  assert.equal(ev.level, 'declaree');
  assert.equal(ev.chain, false);
});

test('no animal is not an evidence level', () => {
  assert.equal(evidenceLevel(null), null);
});

/* ------------------------------------------------------------- certificate */

test('the certificate fields come from typed record fields', () => {
  // Previously these were `clé=valeur` pairs inside a deed's free-text
  // reference, which gave the impression the chain validated them when it was
  // storing a string. A sex is now an enum — the same enum that makes the
  // chain refuse a sire that is not male.
  const c = certificate(animal());
  assert.equal(c.idn, '221/682/021029/L');
  assert.equal(c.puce, '982000000000003');
  assert.equal(c.nom, 'Classico');
  assert.equal(c.sexe, 'Mâle');
  assert.equal(c.robe, 'blanc tacheté');
  assert.equal(c.portee, 'single');
  assert.equal(c.naissance, '2026-08-25');
  assert.equal(c.proprietaire, 'yml1holder');
});

test('missing optional fields render as a dash rather than as undefined', () => {
  const c = certificate(animal({ name: '', coat: '', born_on: '', tag: '', birth_type: '' }));
  assert.equal(c.nom, '—');
  assert.equal(c.robe, '—');
  assert.equal(c.naissance, '—');
  assert.equal(c.puce, '—');
});

test('labels cover every enum value and never emit undefined', () => {
  assert.equal(sexLabel('SEX_MALE'), 'Mâle');
  assert.equal(sexLabel('SEX_FEMALE'), 'Femelle');
  assert.equal(sexLabel('SEX_UNSPECIFIED'), '—');
  assert.equal(sexLabel('SEX_INVENTED'), '—');
  assert.equal(speciesLabel('SPECIES_SHEEP'), 'Ovin');
  assert.equal(speciesLabel(undefined), '—');
});

/* ----------------------------------------------------------- the pedigree */

/** The flat node list the chain's Pedigree query returns. */
const node = (a, generation, path) => ({ animal: a, generation, path });

test('the flat node list is rebuilt into the tree the chain described', () => {
  // The chain builds the paths on the way down, so rebuilding is presentation
  // only: this must not be able to invent a relation that was not returned.
  const root = animal({ id: '3', sire_id: '1', dam_id: '2' });
  const sire = animal({ id: '1', name: 'Atomique', sire_id: '5', dam_id: '0' });
  const dam = animal({ id: '2', name: 'Palmarin', sex: 'SEX_FEMALE', sire_id: '0', dam_id: '0' });
  const grandsire = animal({ id: '5', name: 'Nianing', sire_id: '0', dam_id: '0' });

  const tree = asTree([
    node(root, 0, ''),
    node(sire, 1, 'sire'),
    node(dam, 1, 'dam'),
    node(grandsire, 2, 'sire.sire'),
  ]);

  assert.equal(tree.animal.id, '3');
  assert.equal(tree.sire.animal.name, 'Atomique');
  assert.equal(tree.dam.animal.name, 'Palmarin');
  assert.equal(tree.sire.sire.animal.name, 'Nianing');
  assert.equal(tree.sire.dam, null);
  assert.equal(tree.dam.sire, null);
});

test('nodes arriving out of order still rebuild correctly', () => {
  // Wire order is the chain's business, not this function's assumption.
  const tree = asTree([
    node(animal({ id: '5', sire_id: '0', dam_id: '0' }), 2, 'sire.sire'),
    node(animal({ id: '1', sire_id: '5', dam_id: '0' }), 1, 'sire'),
    node(animal({ id: '3', sire_id: '1', dam_id: '0' }), 0, ''),
  ]);
  assert.equal(tree.animal.id, '3');
  assert.equal(tree.sire.sire.animal.id, '5');
});

test('an empty pedigree is a null tree, not a crash', () => {
  assert.equal(asTree([]), null);
});

/* ------------------------------------------------------------- coverage */

test('coverage counts ancestors held against ancestors cited', () => {
  const root = animal({ id: '3', sire_id: '1', dam_id: '2' });
  const sire = animal({ id: '1', sire_id: '5', dam_id: '0' });
  const dam = animal({ id: '2', sex: 'SEX_FEMALE', sire_id: '0', dam_id: '0' });
  const grandsire = animal({ id: '5', sire_id: '0', dam_id: '0' });

  const tree = asTree([
    node(root, 0, ''), node(sire, 1, 'sire'), node(dam, 1, 'dam'),
    node(grandsire, 2, 'sire.sire'),
  ]);

  const cov = pedigreeCoverage(tree);
  assert.equal(cov.cited, 3);
  assert.equal(cov.held, 3);
});

test('an ancestor cited but not registered is counted, not hidden', () => {
  // The honest case, and the one worth protecting. Lines predating the scheme
  // name ancestors that were never enrolled; a coverage figure that quietly
  // dropped them would report perfect coverage of a partial pedigree.
  const root = animal({ id: '3', sire_id: '1', dam_id: '9' });
  const sire = animal({ id: '1', sire_id: '0', dam_id: '0' });

  const tree = asTree([node(root, 0, ''), node(sire, 1, 'sire')]);
  const cov = pedigreeCoverage(tree);
  assert.equal(cov.cited, 2, 'both parents are cited on the record');
  assert.equal(cov.held, 1, 'only one came back from the register');
});

test('a founder cites nothing and that is not zero coverage of something', () => {
  const founder = animal({ id: '7', sire_id: '0', dam_id: '0' });
  const cov = pedigreeCoverage(asTree([node(founder, 0, '')]));
  assert.equal(cov.cited, 0);
  assert.equal(cov.held, 0);
});

test('coverage on a null tree is zero rather than an exception', () => {
  const cov = pedigreeCoverage(null);
  assert.equal(cov.cited, 0);
  assert.equal(cov.held, 0);
});
