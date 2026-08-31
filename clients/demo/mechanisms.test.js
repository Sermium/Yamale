// Tests for the catalogue itself.
//
// Nothing here talks to a chain. What is tested is the set of claims the page
// makes before it reads anything: that every mechanism links somewhere that
// exists, that every one states a refusal rather than only a feature, and —
// the one that matters most in a room — that no `read` can return a number when
// the chain does not answer.
//
// That last property is tested by handing every mechanism a fetch that fails in
// each of the ways the deployment actually fails: a timeout, the gateway's 401,
// the 503 the proxy returns when a node is halted for an upgrade, and an HTML
// error page where JSON was expected. All four have been observed against
// pay.yamalelegal.com. If any mechanism comes back `proven` from one of them,
// this page is capable of showing a room a figure it did not read, and the test
// fails.

import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { TUNING } from './chain.js';
import { ACTS, MECHANISMS, NOT_BUILT, SURFACES, THRESHOLD_ACCOUNT } from './mechanisms.js';

/**
 * The real pacing is measured against a gateway that rate-limits RPC, and it
 * makes the failure tests below spend two minutes asleep proving a point they
 * prove just as well at zero. The pacing itself has its own test, which puts
 * the real numbers back for the duration.
 */
const REAL_TUNING = { rpcGapMs: TUNING.rpcGapMs, backoffMs: TUNING.backoffMs };
TUNING.rpcGapMs = 0;
TUNING.backoffMs = [0];

/* ========================================================================= */
/*  Shape                                                                    */
/* ========================================================================= */

test('every mechanism has an id, and no two share one', () => {
  const ids = MECHANISMS.map((m) => m.id);
  assert.equal(new Set(ids).size, ids.length, 'duplicate mechanism id');
  ids.forEach((id) => assert.match(id, /^[a-z][a-z0-9-]*$/, `${id} is not usable as a fragment`));
});

test('every mechanism belongs to an act that exists, and every act has mechanisms', () => {
  const acts = new Set(ACTS.map((a) => a.id));
  MECHANISMS.forEach((m) => assert.ok(acts.has(m.act), `${m.id} is in unknown act "${m.act}"`));
  ACTS.forEach((a) => assert.ok(MECHANISMS.some((m) => m.act === a.id), `act "${a.id}" is empty`));
});

test('every mechanism links to a surface that exists', () => {
  // A typo here is a dead click in front of a central bank. It is a failing
  // test instead.
  MECHANISMS.forEach((m) => {
    assert.ok(SURFACES[m.surface], `${m.id} links to unknown surface "${m.surface}"`);
  });
});

test('every surface href is an absolute path on this origin', () => {
  for (const [key, s] of Object.entries(SURFACES)) {
    assert.match(s.href, /^\//, `${key} is not an absolute path`);
    assert.doesNotMatch(s.href, /^https?:/, `${key} names a host; moving origin would break it`);
    assert.ok(s.label && s.blurb, `${key} is missing a label or a blurb`);
    assert.ok(['live', 'building'].includes(s.status), `${key} has status "${s.status}"`);
  }
});

test('any surface not yet deployed is disclosed rather than only marked', () => {
  // Deliberately not a hard-coded list of which ones are unbuilt. Oversight and
  // markets were 'building' for most of the time this page was written and both
  // went live before it was finished; a test naming them would now be failing
  // for the wrong reason. What must hold is the invariant: if a surface is
  // marked unbuilt, the "what is not built" section says so, because a reader
  // scanning that section should not then be surprised by a 404.
  const building = Object.entries(SURFACES).filter(([, s]) => s.status === 'building');
  const disclosure = NOT_BUILT.join(' ').toLowerCase();
  for (const [key, s] of building) {
    assert.ok(disclosure.includes(key) || disclosure.includes(s.label.toLowerCase()),
      `${key} is marked as not deployed but is not mentioned in NOT_BUILT`);
  }
});

test('every surface is reachable from at least one mechanism or the grid', () => {
  // The grid draws all of SURFACES, so nothing is orphaned; this pins the
  // reverse direction — that the two surfaces built while this page was being
  // written are still each the destination of a mechanism, rather than only
  // appearing as a tile.
  for (const key of ['oversight', 'markets']) {
    assert.ok(MECHANISMS.some((m) => m.surface === key),
      `no mechanism sends anybody to ${key}`);
  }
});

test('every surface named in the deployment is present', () => {
  // Fetched against the live deployment on 2026-08-31: all thirteen answer 200.
  for (const key of ['site', 'app', 'wallet', 'safe', 'explorer', 'land', 'rwa',
    'governance', 'foundation', 'validator', 'docs', 'oversight', 'markets']) {
    assert.ok(SURFACES[key], `${key} is missing from the surface list`);
  }
});

/* ========================================================================= */
/*  The claims                                                               */
/* ========================================================================= */

test('every mechanism states what it does and what it refuses, at length', () => {
  MECHANISMS.forEach((m) => {
    assert.ok(m.name && m.module && m.watch, `${m.id} is missing a name, module or watch line`);
    assert.ok(m.does.length > 40, `${m.id}: "does" is too short to be a sentence`);
    // The refusal is the headline. A one-clause refusal is a feature list entry
    // wearing a different label, and the room will treat it as one.
    assert.ok(m.refuses.length > 60, `${m.id}: "refuses" is too short to be an argument`);
    assert.match(m.does, /[.]$/, `${m.id}: "does" does not end in a full stop`);
    assert.match(m.refuses, /[.]$/, `${m.id}: "refuses" does not end in a full stop`);
  });
});

test('a refusal names something that cannot happen, not something that can', () => {
  // Crude, and deliberately so: the failure it catches is a "refusal" written
  // as "allows an authority to freeze an account", which is a feature.
  MECHANISMS.forEach((m) => {
    assert.doesNotMatch(m.refuses, /^(Allows|Lets|Enables|Supports|Provides)\b/,
      `${m.id}: the refusal describes a capability, not a refusal`);
  });
});

test('the module of every mechanism names an x/ module or the mpc component', () => {
  MECHANISMS.forEach((m) => {
    assert.match(m.module, /^(x\/[a-z]+|mpc)$/, `${m.id} names module "${m.module}"`);
  });
});

test('the threshold account is a bech32 address on this chain', () => {
  assert.match(THRESHOLD_ACCOUNT, /^yml1[a-z0-9]{38}$/);
});

test('what is not built is stated, and includes the four things a room would find', () => {
  assert.ok(NOT_BUILT.length >= 5);
  const all = NOT_BUILT.join(' ').toLowerCase();
  for (const missing of ['audit', 'account service', 'two validators', 'paymsg']) {
    assert.ok(all.includes(missing), `"${missing}" is not disclosed`);
  }
  NOT_BUILT.forEach((line) => assert.match(line, /[.]$/, `not a sentence: ${line}`));
});

/* ========================================================================= */
/*  No read may invent a number                                              */
/* ========================================================================= */

/**
 * The four ways this deployment actually fails, each observed.
 *
 * `halted` is the one that matters most: the chain is scheduled to halt for an
 * upgrade during the demonstration this page is for, and the proxy answers 503
 * while it is down.
 */
const BREAKAGES = {
  timeout: () => Promise.reject(Object.assign(new Error('The operation was aborted'), { name: 'TimeoutError' })),
  refused: () => Promise.reject(new TypeError('fetch failed')),
  halted: () => Promise.resolve(new Response('upstream unavailable', { status: 503 })),
  gated: () => Promise.resolve(new Response('', {
    status: 401,
    headers: { 'www-authenticate': 'Basic realm="Yamale — supervisor access"' },
  })),
  htmlErrorPage: () => Promise.resolve(new Response('<html><h1>502 Bad Gateway</h1></html>', {
    status: 200, headers: { 'content-type': 'text/html' },
  })),
  emptyBody: () => Promise.resolve(new Response('', { status: 200 })),
};

for (const [name, broken] of Object.entries(BREAKAGES)) {
  test(`no mechanism reports a proven figure when the chain ${name}`, async () => {
    const real = globalThis.fetch;
    globalThis.fetch = broken;
    try {
      for (const m of MECHANISMS) {
        const proof = await m.read({ secondsPerBlock: 5.4 });
        assert.equal(proof.state, 'unread',
          `${m.id} claimed a proof from a ${name} chain: ${JSON.stringify(proof).slice(0, 200)}`);
        assert.ok(['unreachable', 'denied', 'absent'].includes(proof.reason),
          `${m.id} gave reason "${proof.reason}"`);
        assert.ok(proof.detail.length > 12, `${m.id} gave no explanation`);
        // The whole point: nothing numeric escapes.
        assert.equal(proof.rows, undefined, `${m.id} produced rows from a ${name} chain`);
        assert.doesNotMatch(proof.detail, /\b0\b/, `${m.id} put a nought in its failure text`);
      }
    } finally {
      globalThis.fetch = real;
    }
  });
}

test('a read never throws — it returns an unread proof instead', async () => {
  // The page draws seventeen proofs concurrently. One that throws rather than
  // returning would take its own panel down and, before this was arranged,
  // could leave the panel showing "reading…" for the rest of the session.
  const real = globalThis.fetch;
  globalThis.fetch = () => { throw new Error('synchronous explosion'); };
  try {
    for (const m of MECHANISMS) {
      const proof = await m.read({});
      assert.equal(proof.state, 'unread', `${m.id} did not degrade`);
    }
  } finally {
    globalThis.fetch = real;
  }
});

test('a read survives a chain that answers with well-formed but empty JSON', async () => {
  // The subtle one: the node is up, the gateway is up, and every query returns
  // {} because a module was removed in an upgrade. Nothing throws, so this is
  // the case most likely to render as confident noughts.
  const real = globalThis.fetch;
  globalThis.fetch = () => Promise.resolve(new Response('{}', {
    status: 200, headers: { 'content-type': 'application/json' },
  }));
  try {
    for (const m of MECHANISMS) {
      const proof = await m.read({ secondsPerBlock: 5.4 });
      if (proof.state !== 'proven') continue;
      // Where a mechanism does report a proof from an empty answer, it must not
      // dress absence up as measurement: every row needs a value, and a note
      // has to be attached saying what was actually found.
      proof.rows.forEach((row) => {
        assert.ok(row.label, `${m.id} produced a row with no label`);
        assert.ok(row.value !== undefined && row.value !== null && row.value !== '',
          `${m.id} produced an empty value for "${row.label}"`);
        assert.doesNotMatch(String(row.value), /undefined|NaN|\[object/,
          `${m.id} rendered "${row.label}" as ${row.value}`);
      });
    }
  } finally {
    globalThis.fetch = real;
  }
});

/* ========================================================================= */
/*  The page must not overwhelm the node it is reporting on                  */
/* ========================================================================= */

test('RPC calls are paced and serialised; REST calls are not', async () => {
  // The gateway rate-limits /api/rpc/ and does not rate-limit /api/rest/.
  // Measured: RPC returns 503 for every request in a burst of twelve and stays
  // limited for sequential requests afterwards; REST passes twelve at once.
  // The full table is in the block comment at the top of chain.js.
  //
  // This test is the reason the tour does not blame the chain for a limit the
  // page tripped itself. Before the pacing existed, six of seventeen mechanisms
  // showed "cannot reach the chain" on a chain that was answering perfectly.
  TUNING.rpcGapMs = REAL_TUNING.rpcGapMs;

  const seen = [];
  let concurrentRpc = 0; let peakRpc = 0;
  let concurrentRest = 0; let peakRest = 0;

  const real = globalThis.fetch;
  globalThis.fetch = async (url) => {
    const rpc = String(url).startsWith('/api/rpc');
    if (rpc) { concurrentRpc += 1; peakRpc = Math.max(peakRpc, concurrentRpc); }
    else { concurrentRest += 1; peakRest = Math.max(peakRest, concurrentRest); }
    seen.push({ rpc, at: Date.now() });
    await new Promise((r) => setTimeout(r, 5));
    if (rpc) concurrentRpc -= 1; else concurrentRest -= 1;
    return new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } });
  };

  try {
    await Promise.all(MECHANISMS.map((m) => m.read({ secondsPerBlock: 5.4 })));
  } finally {
    globalThis.fetch = real;
    TUNING.rpcGapMs = 0;
  }

  const rpcAt = seen.filter((s) => s.rpc).map((s) => s.at);
  assert.ok(rpcAt.length >= 8, `expected the ABCI-only modules to be queried, saw ${rpcAt.length}`);

  // One at a time. Two RPC requests in flight together is how the burst that
  // tripped the limiter happened in the first place.
  assert.equal(peakRpc, 1, `${peakRpc} RPC requests were in flight at once`);

  const gaps = rpcAt.slice(1).map((t, i) => t - rpcAt[i]);
  const tooClose = gaps.filter((g) => g < REAL_TUNING.rpcGapMs - 40);
  assert.equal(tooClose.length, 0,
    `${tooClose.length} RPC calls came faster than ${REAL_TUNING.rpcGapMs}ms: ${tooClose}`);

  // REST is deliberately NOT held to one at a time. It is not rate-limited, and
  // pacing it as well would make the page slower for no reason at all.
  assert.ok(peakRest > 1, 'REST was serialised, which the gateway does not require');
});

test('a failure in one RPC call does not stop the queue behind it', async () => {
  // The pacing chains promises. A rejected link that was not caught would
  // poison the tail, and every mechanism queued behind the first failure would
  // silently never ask — leaving those panels on "reading…" for ever, which is
  // worse than either a figure or an honest error.
  TUNING.rpcGapMs = 1;
  let calls = 0;
  const real = globalThis.fetch;
  globalThis.fetch = async () => {
    calls += 1;
    if (calls <= 2) throw new TypeError('fetch failed');
    return new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } });
  };
  try {
    const proofs = await Promise.all(MECHANISMS.map((m) => m.read({ secondsPerBlock: 5.4 })));
    proofs.forEach((p, i) => assert.ok(['proven', 'unread'].includes(p.state),
      `${MECHANISMS[i].id} never settled`));
    assert.ok(calls > 3, 'the queue stopped after the first failure');
  } finally {
    globalThis.fetch = real;
    TUNING.rpcGapMs = 0;
  }
});
