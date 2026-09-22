/**
 * The queue, and the one thing it must never do.
 *
 * §5: two devices that both recorded something offline are reconciled by
 * submitting both. So the tests that matter here are the ones that try to make
 * the queue lose an observation — by merging, by de-duplicating, by preferring
 * a later timestamp, by tidying away a refusal — and assert that it cannot.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  advance,
  disagreements,
  merge,
  newEntry,
  sendable,
  SUBMITTABLE,
  tally,
  type Entry,
} from './outbox.ts';

const BASE = {
  herdId: '1',
  deviceId: 'tablet-a',
  agent: 'yml1f3mx7yjkxml4aws8ehtekf6x6jweqc3z4vztkg',
};

/* ----------------------------------------------------------- minting an id */

describe('minting', () => {
  it('gives two entries different ids even in the same millisecond', () => {
    const a = newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 1_000 });
    const b = newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 1_000 });
    assert.notEqual(a.id, b.id);
  });

  it('does not derive the id from the content', () => {
    // A content hash would make two devices observing the same animal at the
    // same second mint the same id, and a merge keyed on it would drop one of
    // the two observations. That IS last-write-wins, arrived at by accident.
    const a = newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 1_000 });
    const b = newEntry({
      ...BASE, deviceId: 'tablet-b', kind: 'capture', animalId: '7', observedAt: 1_000,
    });
    assert.notEqual(a.id, b.id);
    assert.ok(a.id.startsWith('tablet-a:'));
    assert.ok(b.id.startsWith('tablet-b:'));
  });

  it('holds a capture rather than queueing it, because the chain has nowhere to put it', () => {
    // x/herd has no Enrolment and no IdentificationClaim message. §8 proposes
    // both; neither exists. A queue showing "waiting to send" would be a
    // progress bar with no end.
    assert.equal(newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 1 }).state, 'held');
    assert.equal(
      newEntry({ ...BASE, kind: 'identification-claim', animalId: '7', observedAt: 1 }).state,
      'held',
    );
    assert.equal(
      newEntry({ ...BASE, kind: 'attestation', animalId: '7', observedAt: 1 }).state,
      'queued',
    );
    assert.equal(SUBMITTABLE.capture, false);
    assert.equal(SUBMITTABLE.attestation, true);
  });

  it('keeps the device clock at the moment of observation', () => {
    const e = newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 1_700_000_000_000 });
    assert.equal(e.observedAt, 1_700_000_000_000);
    assert.equal(e.position, null);
  });
});

/* ---------------------------------------------------------------- merging */

describe('merging two lists', () => {
  const twoDevices = (): [Entry, Entry] => [
    { ...newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 5_000 }), id: 'a:1' },
    {
      ...newEntry({ ...BASE, deviceId: 'tablet-b', agent: 'yml1jxr50m8n8y7c2kljxq7w2s4wlrddmyvkta600k', kind: 'capture', animalId: '7', observedAt: 5_000 }),
      id: 'b:1',
    },
  ];

  it('keeps both observations of the same animal at the same instant', () => {
    const [a, b] = twoDevices();
    const got = merge([a], [b]);
    assert.equal(got.length, 2);
    assert.deepEqual(got.map((e) => e.id).sort(), ['a:1', 'b:1']);
  });

  it('does not let a later entry displace an earlier one', () => {
    const [a] = twoDevices();
    const later: Entry = { ...a, id: 'b:1', observedAt: 9_000, agent: 'somebody-else' };
    const got = merge([a], [later]);
    assert.equal(got.length, 2);
  });

  it('advances the same entry along its own state machine', () => {
    // The same id is the same entry on the same device seen twice, not two
    // observations. A clock comparison here would let a stale read resurrect
    // something already sent.
    const [a] = twoDevices();
    const sent: Entry = { ...a, state: 'sent', txHash: 'ABC' };
    assert.equal(merge([a], [sent])[0].state, 'sent');
    assert.equal(merge([sent], [a])[0].state, 'sent');
  });

  it('orders by when the thing happened', () => {
    const [a, b] = twoDevices();
    const got = merge([{ ...b, observedAt: 1_000 }], [a]);
    assert.deepEqual(got.map((e) => e.observedAt), [1_000, 5_000]);
  });
});

/* ------------------------------------------------------- visible conflict */

describe('disagreements', () => {
  it('reports two agents recording the same animal, and resolves nothing', () => {
    const a = { ...newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 1 }), id: 'a' };
    const b = {
      ...newEntry({ ...BASE, agent: 'yml1jxr50', deviceId: 'tablet-b', kind: 'capture', animalId: '7', observedAt: 2 }),
      id: 'b',
    };
    const got = disagreements([a, b]);
    assert.equal(got.length, 1);
    assert.equal(got[0].animalId, '7');
    assert.equal(got[0].differentAgents, true);
    assert.equal(got[0].entries.length, 2);
    // Both are still in the list they came from, untouched.
    assert.equal(a.state, 'held');
    assert.equal(b.state, 'held');
  });

  it('marks one agent recording twice as not a disagreement between people', () => {
    const a = { ...newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 1 }), id: 'a' };
    const b = { ...newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 2 }), id: 'b' };
    assert.equal(disagreements([a, b])[0].differentAgents, false);
  });

  it('does not pair different kinds about the same animal', () => {
    const a = { ...newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 1 }), id: 'a' };
    const b = { ...newEntry({ ...BASE, kind: 'attestation', animalId: '7', observedAt: 2 }), id: 'b' };
    assert.deepEqual(disagreements([a, b]), []);
  });

  it('stops reporting a pair once the chain holds it', () => {
    // Once it is on the chain, the chain shows the contradiction — with both
    // agents named — which is where §5 says it belongs.
    const a = { ...newEntry({ ...BASE, kind: 'attestation', animalId: '7', observedAt: 1 }), id: 'a', state: 'sent' as const };
    const b = { ...newEntry({ ...BASE, kind: 'attestation', animalId: '7', observedAt: 2 }), id: 'b' };
    assert.deepEqual(disagreements([a, b]), []);
  });

  it('does not apply a time window', () => {
    // A Tuesday capture and a Friday one are still a contradiction if both are
    // unsubmitted, and a window would hide the pair that sat in a bag.
    const a = { ...newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 0 }), id: 'a' };
    const b = {
      ...newEntry({ ...BASE, agent: 'other', kind: 'capture', animalId: '7', observedAt: 3 * 86_400_000 }),
      id: 'b',
    };
    assert.equal(disagreements([a, b]).length, 1);
  });
});

/* -------------------------------------------------------------- the tally */

describe('what the sync screen counts', () => {
  const entries = (): Entry[] => [
    { ...newEntry({ ...BASE, kind: 'capture', animalId: '7', observedAt: 300 }), id: 'a' },
    { ...newEntry({ ...BASE, kind: 'attestation', animalId: '7', observedAt: 100 }), id: 'b' },
    { ...newEntry({ ...BASE, kind: 'attestation', animalId: '9', observedAt: 200 }), id: 'c', state: 'sent' },
  ];

  it('separates held from queued', () => {
    const got = tally(entries());
    assert.equal(got.held, 1);
    assert.equal(got.queued, 1);
    assert.equal(got.sent, 1);
  });

  it('dates the oldest thing that has not reached the chain', () => {
    assert.equal(tally(entries()).oldestUnsent, 100);
  });

  it('reports nothing unsent as null, not as zero', () => {
    const all: Entry[] = entries().map((e) => ({ ...e, state: 'sent' }));
    assert.equal(tally(all).oldestUnsent, null);
  });

  it('offers only what the chain can accept, oldest first', () => {
    const got = sendable(entries());
    assert.deepEqual(got.map((e) => e.id), ['b']);
  });

  it('retries a refusal', () => {
    const list = entries().map((e) => (e.id === 'b' ? { ...e, state: 'refused' as const } : e));
    assert.deepEqual(sendable(list).map((e) => e.id), ['b']);
  });
});

/* ------------------------------------------------------------- advancing */

describe('advancing an entry', () => {
  const e = { ...newEntry({ ...BASE, kind: 'attestation', animalId: '7', observedAt: 1 }), id: 'a' };

  it('records the transaction that carried it', () => {
    const got = advance(e, { ok: true, txHash: 'D34DB33F' });
    assert.equal(got.state, 'sent');
    assert.equal(got.txHash, 'D34DB33F');
    assert.equal(got.attempts, 1);
  });

  it('keeps a refused entry, with the reason', () => {
    // A capture the chain refused is still the only record that the operator
    // stood in front of that animal. Dropping it to keep a queue tidy destroys
    // evidence to improve housekeeping.
    const got = advance(e, { ok: false, note: 'agent not accredited' });
    assert.equal(got.state, 'refused');
    assert.equal(got.note, 'agent not accredited');
    assert.equal(got.observedAt, 1);
    assert.equal(got.animalId, '7');
  });
});
