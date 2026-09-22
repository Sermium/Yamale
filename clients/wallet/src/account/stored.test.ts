/**
 * The local record of an enrolment.
 *
 * Small surface, and one property worth being careful about: a stored holding
 * has to come back as the SAME bytes, because the moment it is used is a
 * recovery, and a recovery that produces a plausible wrong answer is
 * indistinguishable from a forgotten password.
 */
import { strict as assert } from 'node:assert';
import { test, beforeEach } from 'node:test';

import { enrolmentFor, forgetEnrolment, rememberEnrolment, type Enrolment } from './stored.ts';
import { protect, ROLE_DEVICE } from './protect.ts';
import { restore } from './protect.ts';
import type { Holding } from './deposit.ts';

// node:test has no DOM. A minimal localStorage, which also exercises the
// JSON-in-a-string path the browser actually takes.
function installStorage() {
  const map = new Map<string, string>();
  (globalThis as any).localStorage = {
    getItem: (k: string) => map.get(k) ?? null,
    setItem: (k: string, v: string) => void map.set(k, v),
    removeItem: (k: string) => void map.delete(k),
    clear: () => map.clear(),
  };
  return map;
}

let storage = installStorage();
beforeEach(() => {
  storage = installStorage();
});

const PASSWORD = 'seven horses eating apples slowly';

async function holdings(): Promise<{ device: Holding; others: Holding[]; secret: Uint8Array }> {
  const secret = new Uint8Array(32);
  crypto.getRandomValues(secret);
  secret[0] = secret[0]! | 1;
  const all = await protect(secret, PASSWORD);
  return {
    device: all.find((h) => h.role === ROLE_DEVICE)!,
    others: all.filter((h) => h.role !== ROLE_DEVICE),
    secret,
  };
}

function record(address: string, holding: Holding): Enrolment {
  return { address, email: 'amara@example.org', holding, enrolledAt: '2026-09-22T12:00:00Z' };
}

test('a stored holding comes back as the same bytes, and still recovers the key', async () => {
  // The assertion that matters. Round-tripping the fields is not enough: the
  // proof is that the holding still rebuilds the key when paired with a
  // service's, which is the only thing it will ever be asked to do.
  const { device, others, secret } = await holdings();
  rememberEnrolment(record('yml1one', device));

  const back = enrolmentFor('yml1one');
  assert.ok(back, 'the enrolment was not found');
  assert.deepEqual(back.holding.share.y, device.share.y);
  assert.deepEqual(back.holding.salt, device.salt);
  assert.equal(back.holding.threads, device.threads);

  const restored = await restore([back.holding, others[0]!], PASSWORD);
  assert.deepEqual(restored, secret, 'the stored holding no longer recovers the key');
});

test('records are kept per address, not one slot for the browser', async () => {
  // A single slot would tell somebody their account is protected while the
  // record belonged to a different account they enrolled earlier.
  const a = await holdings();
  const b = await holdings();
  rememberEnrolment(record('yml1one', a.device));
  rememberEnrolment(record('yml1two', b.device));

  assert.deepEqual(enrolmentFor('yml1one')!.holding.share.y, a.device.share.y);
  assert.deepEqual(enrolmentFor('yml1two')!.holding.share.y, b.device.share.y);
  assert.equal(enrolmentFor('yml1three'), null);
});

test('re-enrolling one address replaces its record rather than adding a second', async () => {
  const first = await holdings();
  const second = await holdings();
  rememberEnrolment(record('yml1one', first.device));
  rememberEnrolment(record('yml1one', second.device));

  assert.deepEqual(enrolmentFor('yml1one')!.holding.share.y, second.device.share.y);
  assert.equal(JSON.parse(storage.get('yamale.wallet.enrolment.v1')!).length, 1);
});

test('forgetting one address leaves the others alone', async () => {
  const a = await holdings();
  const b = await holdings();
  rememberEnrolment(record('yml1one', a.device));
  rememberEnrolment(record('yml1two', b.device));

  forgetEnrolment('yml1one');
  assert.equal(enrolmentFor('yml1one'), null);
  assert.ok(enrolmentFor('yml1two'), 'forgetting one address dropped another');
});

test('a truncated record reads as absent rather than as a broken holding', () => {
  // The screen then offers to enrol, which is recoverable. Returning a holding
  // that is missing its share would claim a protection that fails at the one
  // moment it is needed.
  storage.set(
    'yamale.wallet.enrolment.v1',
    JSON.stringify([
      {
        address: 'yml1one',
        email: 'a@b.test',
        enrolledAt: '2026-09-22T12:00:00Z',
        holding: { role: 'device', salt: '', nonce: '', time: 3, memory: 131072, threads: 1, share: { x: 1, y: '' } },
      },
    ]),
  );
  assert.equal(enrolmentFor('yml1one'), null);
});

test('unreadable storage is absent, not a crash', () => {
  storage.set('yamale.wallet.enrolment.v1', 'not json at all');
  assert.equal(enrolmentFor('yml1one'), null);

  // And writing over it recovers, rather than leaving the wallet stuck.
  assert.doesNotThrow(() =>
    rememberEnrolment(
      record('yml1one', {
        role: 'device',
        salt: Uint8Array.of(1),
        nonce: Uint8Array.of(2),
        time: 3,
        memory: 131072,
        threads: 1,
        share: { x: 1, y: Uint8Array.of(3) },
      }),
    ),
  );
  assert.ok(enrolmentFor('yml1one'));
});

test('storage that refuses to write does not fail the caller', () => {
  // The enrolment itself has already succeeded at that point and the services
  // hold what they need. Losing the local copy costs a recovery one extra step,
  // not the account, so it must not surface as a failure.
  (globalThis as any).localStorage.setItem = () => {
    throw new Error('quota');
  };
  assert.doesNotThrow(() =>
    rememberEnrolment(
      record('yml1one', {
        role: 'device',
        salt: Uint8Array.of(1),
        nonce: Uint8Array.of(2),
        time: 3,
        memory: 131072,
        threads: 1,
        share: { x: 1, y: Uint8Array.of(3) },
      }),
    ),
  );
});
