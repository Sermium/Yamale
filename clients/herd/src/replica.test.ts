/**
 * The copy on the device, and what an answer from it is allowed to mean.
 *
 * The fixtures are devnet-2's herd 1 — nine animals, two of them twins, one
 * three-generation line — because a lookup table is only interesting against
 * identifiers that share prefixes, and real ones do.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { asHeight, type Animal } from './herd.ts';
import type { Enrolled, IrisTemplate } from './iris.ts';
import {
  biometricScope,
  freshness,
  index,
  lookup,
  nearestInTime,
  reEnrolmentDue,
  type ReplicaMeta,
} from './replica.ts';
import { memoryStore } from './store.ts';

/* ------------------------------------------------------------- fixtures */

function animal(id: string, nationalId: string, tag: string): Animal {
  return {
    id, herdId: '1', nationalId, tag, level: 'individual', name: '', sex: 'unspecified',
    coat: '', bornOn: '', birthType: '', sireId: '0', damId: '0', dnaHash: '', photoHash: '',
    holder: '', authority: '', status: 'active', registeredAt: asHeight(320898),
  };
}

// The real nine, off the chain.
const HERD: Animal[] = [
  animal('1', '221/682/000102/L', '982000000000102'),
  animal('2', '221/682/000103/L', '982000000000103'),
  animal('3', '221/682/000201/L', '982000000000201'),
  animal('4', '221/682/000202/L', '982000000000202'),
  animal('5', '221/682/000203/L', '982000000000203'),
  animal('6', '221/682/000204/L', '982000000000204'),
  animal('7', '221/682/021029/L', '982000000021029'),
  animal('8', '221/682/021030/L', '982000000021030'),
  animal('9', '221/682/031044/L', '982000000031044'),
];

const template = (fill: number): IrisTemplate => ({
  code: new Uint8Array(256).fill(fill),
  mask: new Uint8Array(256).fill(0xff),
  bits: 2048,
});

/* ---------------------------------------------------------------- index */

describe('indexing a herd', () => {
  const idx = index(HERD);

  it('resolves the three ways a person arrives at an animal', () => {
    assert.equal(idx.byId.get('7')?.name, HERD[6].name);
    assert.equal(idx.byNationalId.get('221/682/021029/L')?.id, '7');
    assert.equal(idx.byTag.get('982000000021029')?.id, '7');
  });

  it('is empty of collisions for a healthy download', () => {
    assert.deepEqual(idx.collisions, []);
  });

  it('collects a collision instead of letting the last writer win', () => {
    // The chain enforces uniqueness on both identifiers, so a duplicate here
    // means the download is corrupt or two herds are in one store. A Map that
    // kept the last writer would resolve a certificate to whichever animal
    // happened to come second in a page.
    const doubled = [...HERD, animal('99', '221/682/021029/L', '982000000099999')];
    const bad = index(doubled);
    assert.equal(bad.collisions.length, 1);
    assert.equal(bad.collisions[0].kind, 'national-id');
    assert.deepEqual(bad.collisions[0].animalIds, ['7', '99']);
  });

  it('ignores an empty tag rather than indexing every level-1 animal under it', () => {
    const untagged = [animal('20', '221/682/040001/L', ''), animal('21', '221/682/040002/L', '')];
    const got = index(untagged);
    assert.equal(got.byTag.size, 0);
    assert.deepEqual(got.collisions, []);
  });
});

/* --------------------------------------------------------------- lookup */

describe('looking up against the replica', () => {
  const idx = index(HERD);

  it('finds by either identifier, however it was typed', () => {
    assert.deepEqual(lookup(idx, '982 000 000 021 029'), { found: true, animal: HERD[6] });
    assert.deepEqual(lookup(idx, '221-682-021029-l'), { found: true, animal: HERD[6] });
  });

  it('says "not in this replica", which is not "not in the registry"', () => {
    const got = lookup(idx, '982000000099999');
    assert.deepEqual(got, { found: false, why: 'not-in-replica' });
  });

  it('never falls back to the other identifier kind', () => {
    // A fifteen-digit tag that is absent is absent. Trying it as a national
    // identifier too and reporting whichever answered would leave the operator
    // unable to say which marker the record was found by, which is exactly the
    // distinction §3's two-marker rule turns on.
    const idOnly = index([animal('30', '982000000012345', '')]);
    assert.equal(lookup(idOnly, '982000000012345').found, false);
  });

  it('refuses a lookup that would be a guess between two animals', () => {
    const bad = index([...HERD, animal('99', '221/682/021029/L', '')]);
    const got = lookup(bad, '221/682/021029/L');
    assert.equal(got.found, false);
    if (got.found) return;
    assert.equal(got.why, 'collision');
  });

  it('distinguishes having no replica from finding nothing in one', () => {
    assert.deepEqual(lookup(null, '221/682/021029/L'), { found: false, why: 'no-replica' });
  });

  it('reports unrecognised input as unrecognised', () => {
    assert.deepEqual(lookup(idx, 'Classico'), { found: false, why: 'unrecognised' });
    assert.deepEqual(lookup(idx, ''), { found: false, why: 'unrecognised' });
  });
});

/* ------------------------------------------------------------ freshness */

describe('freshness', () => {
  const meta: ReplicaMeta = {
    herdId: '1', chainId: 'yamale-devnet-2', height: asHeight(320_950),
    downloadedAt: 1_000_000, animals: 9,
  };

  it('measures the gap in blocks and in time', () => {
    const got = freshness(meta, 1_000_000 + 3 * 86_400_000, 462_701);
    assert.equal(got.blocksBehind, 141_751);
    assert.equal(got.ageMs, 3 * 86_400_000);
    assert.equal(got.chainUnknown, false);
  });

  it('says it cannot say, rather than saying zero', () => {
    const got = freshness(meta, 1_000_000, null);
    assert.equal(got.blocksBehind, null);
    assert.equal(got.chainUnknown, true);
  });

  it('never reports a negative gap', () => {
    // A replica read at a height ahead of the head happens when the node being
    // asked is behind the one the download came from. Not a fault of the copy.
    assert.equal(freshness(meta, 1_000_000, 320_000).blocksBehind, 0);
  });
});

/* ------------------------------------------------- what can be compared */

describe('the biometric scope of a replica', () => {
  const enrolled: Enrolled[] = [
    { animalId: '7', enrolmentId: '7-a', capturedAt: 10, template: template(0x0f) },
    { animalId: '7', enrolmentId: '7-b', capturedAt: 90, template: template(0x0e) },
    { animalId: '4', enrolmentId: '4-a', capturedAt: 50, template: template(0xf0) },
  ];

  it('counts animals, not enrolments', () => {
    const got = biometricScope(HERD, enrolled);
    assert.equal(got.withTemplate, 2);
    assert.equal(got.withoutTemplate, 7);
    assert.equal(got.enrolled.length, 3);
  });

  it('drops a template for an animal that is not in this herd', () => {
    // "Which of MY animals is this" must not be answered with somebody else's
    // beast, however good the comparison.
    const stray: Enrolled = {
      animalId: '10', enrolmentId: '10-a', capturedAt: 1, template: template(0xaa),
    };
    const got = biometricScope(HERD, [...enrolled, stray]);
    assert.equal(got.enrolled.length, 3);
    assert.equal(got.withTemplate, 2);
  });

  it('reports nothing enrolled as nine animals uncovered', () => {
    const got = biometricScope(HERD, []);
    assert.equal(got.withTemplate, 0);
    assert.equal(got.withoutTemplate, 9);
  });
});

describe('choosing which capture to compare against', () => {
  const enrolled: Enrolled[] = [
    { animalId: '7', enrolmentId: 'old', capturedAt: 0, template: template(1) },
    { animalId: '7', enrolmentId: 'mid', capturedAt: 100, template: template(2) },
    { animalId: '7', enrolmentId: 'new', capturedAt: 300, template: template(3) },
  ];

  it('takes the one nearest in time, not the newest', () => {
    // §4: a calf's muzzle is not an adult's. The correct reference is the one
    // closest to the animal's state, which for an archived probe is not the
    // most recent enrolment.
    assert.equal(nearestInTime(enrolled, 110)?.enrolmentId, 'mid');
    assert.equal(nearestInTime(enrolled, 400)?.enrolmentId, 'new');
    assert.equal(nearestInTime(enrolled, -50)?.enrolmentId, 'old');
  });

  it('returns null for nothing rather than a placeholder', () => {
    assert.equal(nearestInTime([], 0), null);
  });
});

describe('when to enrol again', () => {
  const NOW = 1_000_000_000_000;
  const MONTH = 30 * 24 * 3600 * 1000;

  it('asks sooner for a growing animal', () => {
    assert.equal(reEnrolmentDue(6, NOW - 4 * MONTH, NOW).due, true);
    assert.equal(reEnrolmentDue(36, NOW - 4 * MONTH, NOW).due, false);
  });

  it('treats never enrolled as due', () => {
    const got = reEnrolmentDue(36, null, NOW);
    assert.equal(got.due, true);
    assert.equal(got.sinceMs, null);
  });

  it('uses the longer interval when the age is unknown', () => {
    assert.equal(reEnrolmentDue(null, NOW - 4 * MONTH, NOW).intervalMs, 12 * MONTH);
  });
});

/* ------------------------------------------------------------- the store */

describe('the session-only store', () => {
  it('says it will not survive being closed', async () => {
    const store = memoryStore('no IndexedDB here');
    assert.equal(store.durable, false);
    assert.equal(store.why, 'no IndexedDB here');
  });

  it('round-trips a replica and keeps the outbox out of forgetReplica', async () => {
    const store = memoryStore();
    const meta: ReplicaMeta = {
      herdId: '1', chainId: 'yamale-devnet-2', height: asHeight(320_950),
      downloadedAt: 1, animals: 9,
    };
    await store.putReplica({
      meta,
      herd: {
        id: '1', reference: 'SN-RF-H-000147', keeper: 'yml1zu5', authority: 'yml1424',
        species: 'sheep', declaredSize: 40, locality: 'Niacoulrab, Rufisque',
        locationHash: '', jurisdiction: 'SN', status: 'active', registeredAt: asHeight(320_894),
      },
      animals: HERD,
      healthEvents: [],
    });
    await store.putEntries([
      { id: 'a', kind: 'capture', animalId: '7', herdId: '1', observedAt: 1, deviceId: 'd',
        agent: 'x', position: null, detail: {}, state: 'held', attempts: 0, txHash: null, note: null },
    ]);

    assert.equal((await store.animals()).length, 9);
    assert.equal((await store.meta())?.herdId, '1');

    await store.forgetReplica();
    assert.equal(await store.meta(), null);
    assert.equal((await store.animals()).length, 0);
    // Discarding a replica is housekeeping. Discarding unsent observations
    // destroys evidence, and nobody asked for that by pressing "download".
    assert.equal((await store.outbox()).length, 1);
  });
});
