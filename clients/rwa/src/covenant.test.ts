import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  covenantTone,
  deadlineOf,
  fileNameOf,
  hasPeriod,
  isSuperseded,
  kindKey,
  obligationKey,
  openDeadlines,
  orderCovenants,
  shortDigest,
  toHex,
  verifyCommand,
  type Covenant,
} from './covenant.ts';

const DIGEST = 'a'.repeat(64);

function covenant(over: Partial<Covenant> = {}): Covenant {
  return {
    id: '1',
    assetId: '4',
    kind: 'COVENANT_KIND_FINANCIAL_STATEMENTS',
    title: 'Accounts',
    digest: DIGEST,
    uri: '',
    periodStart: 0,
    periodEnd: 0,
    actsBy: 0,
    publishedBy: 'yml1sponsor',
    publishedAt: 1_000,
    supersedes: '0',
    supersededBy: '0',
    ...over,
  };
}

/* ------------------------------------------------------------- the digest */

test('a digest is hex, and an absent one is empty rather than a throw', () => {
  assert.equal(toHex(new Uint8Array([0x00, 0x0f, 0xff])), '000fff');
  assert.equal(toHex(new Uint8Array(0)), '');
  assert.equal(toHex(undefined), '');
  assert.equal(toHex(null), '');
});

test('a short digest keeps both ends', () => {
  // A prefix-only truncation is the part a forger would attack, because the
  // first eight characters are the only ones anybody eyeballs.
  const short = shortDigest(DIGEST);
  assert.ok(short.startsWith('aaaaaaaa'));
  assert.ok(short.endsWith('aaaaaaaa'));
  assert.ok(short.length < DIGEST.length);
});

test('a digest short enough to show whole is not truncated', () => {
  assert.equal(shortDigest('abcd'), 'abcd');
});

test('the verify command carries the WHOLE digest, never the short one', () => {
  // The entire point: a truncated digest cannot be compared against anything.
  const cmd = verifyCommand(DIGEST, 'https://example.test/docs/accounts-2026.pdf');
  assert.ok(cmd.includes(DIGEST), 'the full 64 characters must be in the command');
  assert.ok(!cmd.includes('…'));
  assert.ok(cmd.includes('accounts-2026.pdf'));
  assert.ok(cmd.includes('--check'), 'a bare sha256sum leaves the comparison to the eye');
});

test('the verify command falls back to a placeholder filename', () => {
  assert.ok(verifyCommand(DIGEST, '').includes('document.pdf'));
  // A uri naming a directory names no file.
  assert.ok(verifyCommand(DIGEST, 'https://example.test/docs/').includes('document.pdf'));
});

test('a filename is taken from the uri only when it looks like one', () => {
  assert.equal(fileNameOf('ipfs://bafy.../accounts.pdf'), 'accounts.pdf');
  assert.equal(fileNameOf('https://x.test/a.pdf?v=2#page3'), 'a.pdf');
  assert.equal(fileNameOf(''), '');
  assert.equal(fileNameOf('https://x.test/a file.pdf'), '');
});

/* ----------------------------------------------------------- the deadline */

test('a filing with no acts_by asks nothing of a holder', () => {
  const d = deadlineOf(covenant(), 5_000);
  assert.equal(d.state, 'none');
  assert.equal(d.at, 0);
  assert.equal(obligationKey(d.state), 'rwa.cov.toRead');
});

test('an assembly notice in the future is a deadline that can be missed', () => {
  const d = deadlineOf(covenant({ actsBy: 6_000 }), 5_000);
  assert.equal(d.state, 'open');
  assert.equal(d.remainingSeconds, 1_000);
  assert.equal(obligationKey(d.state), 'rwa.cov.mustAct');
});

test('a deadline is inclusive of its final instant', () => {
  // Closing it a second early tells somebody they have missed a vote they have
  // not missed. Same rule as the challenge window in vehicle.ts.
  assert.equal(deadlineOf(covenant({ actsBy: 5_000 }), 5_000).state, 'open');
  assert.equal(deadlineOf(covenant({ actsBy: 5_000 }), 5_001).state, 'passed');
});

test('a passed deadline stays visible and reports no time remaining', () => {
  const d = deadlineOf(covenant({ actsBy: 4_000 }), 5_000);
  assert.equal(d.state, 'passed');
  assert.equal(d.remainingSeconds, 0);
  assert.equal(d.at, 4_000, 'the date itself is kept: it is why a holder lost a vote');
});

test('a deadline and a report do not read alike', () => {
  const notice = covenant({ kind: 'COVENANT_KIND_GENERAL_ASSEMBLY', actsBy: 6_000 });
  const accounts = covenant({ kind: 'COVENANT_KIND_FINANCIAL_STATEMENTS' });

  assert.notEqual(
    obligationKey(deadlineOf(notice, 5_000).state),
    obligationKey(deadlineOf(accounts, 5_000).state),
  );
  assert.equal(covenantTone(notice, deadlineOf(notice, 5_000)), 'warn');
  assert.equal(covenantTone(accounts, deadlineOf(accounts, 5_000)), 'ok');
});

test('a deadline is keyed on acts_by, not on the kind', () => {
  // A material event can carry a closing date; a resolution recorded after an
  // assembly carries none even though it came out of one.
  const material = covenant({ kind: 'COVENANT_KIND_MATERIAL_EVENT', actsBy: 6_000 });
  const resolution = covenant({ kind: 'COVENANT_KIND_RESOLUTION' });
  assert.equal(deadlineOf(material, 5_000).state, 'open');
  assert.equal(deadlineOf(resolution, 5_000).state, 'none');
});

/* --------------------------------------------------------- superseded ones */

test('a superseded filing is recognised, and zero is not a supersession', () => {
  assert.equal(isSuperseded(covenant({ supersededBy: '7' })), true);
  assert.equal(isSuperseded(covenant({ supersededBy: '0' })), false);
  assert.equal(isSuperseded(covenant({ supersededBy: '' })), false);
});

test('a corrected deadline stops being loud but does not disappear', () => {
  const c = covenant({ actsBy: 6_000, supersededBy: '9' });
  assert.equal(covenantTone(c, deadlineOf(c, 5_000)), 'mute');
  // Still in the register: orderCovenants keeps it, it is just ranked last.
  assert.equal(orderCovenants([c], 5_000).length, 1);
});

/* ---------------------------------------------------------------- the order */

test('a live deadline outranks every other filing, however old', () => {
  const assembly = covenant({ id: '1', actsBy: 9_000, publishedAt: 100 });
  const recent = covenant({ id: '2', publishedAt: 8_000 });

  const ordered = orderCovenants([recent, assembly], 5_000);
  assert.equal(ordered[0]!.id, '1',
    'a holder scrolling a chronology to find an assembly has been failed by the ordering');
});

test('live deadlines are soonest first', () => {
  const later = covenant({ id: '1', actsBy: 9_000 });
  const sooner = covenant({ id: '2', actsBy: 6_000 });
  const ordered = orderCovenants([later, sooner], 5_000);
  assert.deepEqual(ordered.map((c) => c.id), ['2', '1']);
});

test('ordinary filings are newest first, and superseded ones sink below them', () => {
  const old = covenant({ id: '1', publishedAt: 1_000 });
  const fresh = covenant({ id: '2', publishedAt: 4_000 });
  const corrected = covenant({ id: '3', publishedAt: 4_500, supersededBy: '2' });

  const ordered = orderCovenants([old, corrected, fresh], 5_000);
  assert.deepEqual(ordered.map((c) => c.id), ['2', '1', '3']);
});

test('ordering never drops a filing', () => {
  const all = [
    covenant({ id: '1', actsBy: 6_000 }),
    covenant({ id: '2', supersededBy: '1' }),
    covenant({ id: '3', actsBy: 4_000 }),
    covenant({ id: '4' }),
  ];
  assert.equal(orderCovenants(all, 5_000).length, 4,
    'current_only is deliberately unused: a dispute is about what was said at a date');
});

test('ordering does not mutate what it was given', () => {
  const all = [covenant({ id: '1' }), covenant({ id: '2', actsBy: 6_000 })];
  const before = all.map((c) => c.id);
  orderCovenants(all, 5_000);
  assert.deepEqual(all.map((c) => c.id), before);
});

test('open deadlines exclude passed and superseded ones', () => {
  const all = [
    covenant({ id: '1', actsBy: 6_000 }),
    covenant({ id: '2', actsBy: 4_000 }),
    covenant({ id: '3', actsBy: 7_000, supersededBy: '1' }),
    covenant({ id: '4' }),
  ];
  assert.deepEqual(openDeadlines(all, 5_000).map((c) => c.id), ['1']);
});

/* --------------------------------------------------------------- the words */

test('every kind has its own sentence, and an unknown one is not silently an accounts filing', () => {
  const kinds: Covenant['kind'][] = [
    'COVENANT_KIND_FINANCIAL_STATEMENTS',
    'COVENANT_KIND_STRUCTURE_CHANGE',
    'COVENANT_KIND_DIVIDEND_NOTICE',
    'COVENANT_KIND_GENERAL_ASSEMBLY',
    'COVENANT_KIND_RESOLUTION',
    'COVENANT_KIND_INDEPENDENT_REPORT',
    'COVENANT_KIND_MATERIAL_EVENT',
  ];
  const keys = kinds.map(kindKey);
  assert.equal(new Set(keys).size, 7, 'seven kinds, seven sentences');
  assert.equal(kindKey('COVENANT_KIND_UNSPECIFIED'), 'rwa.cov.kind.unknown');
});

test('a period is a span or it is nothing', () => {
  assert.equal(hasPeriod({ periodStart: 1, periodEnd: 2 }), true);
  assert.equal(hasPeriod({ periodStart: 0, periodEnd: 0 }), false);
  // Half a period describes no interval. The keeper refuses to write one.
  assert.equal(hasPeriod({ periodStart: 1, periodEnd: 0 }), false);
});
