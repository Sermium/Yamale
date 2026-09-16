import test from 'node:test';
import assert from 'node:assert/strict';

import { judge, parseRequest } from './placement.js';

/** A request exactly as clients/sdk/src/placement.ts prints one. */
const REQUEST = [
  'PLACEMENT REQUEST',
  '',
  'account:      yml1holder',
  'country:      SN (Senegal)',
  'onboarded by: CBAO Senegal',
  'chain:        yamale-devnet-2',
  '',
  'I am asking the institution that onboarded me to record this account as',
  'being in SN.',
].join('\n');

const PARTICIPANTS = [{ name: 'CBAO Senegal', code: 'SN-CBAO', participant: 'yml1bank' }];

test('a request yields the three facts the institution has to act on', () => {
  assert.deepEqual(parseRequest(REQUEST), {
    address: 'yml1holder',
    country: 'SN',
    institution: 'CBAO Senegal',
  });
});

test('a paste that is not a placement request is refused, not half-read', () => {
  // Half-reading is the dangerous failure: a screen that recovered an address
  // from arbitrary text would offer to place an account nobody requested.
  const r = parseRequest('hello, please place my account in SN');
  assert.ok('problem' in r);
});

test('an unplaced account with an approved institution is ready to record', () => {
  const v = judge(parseRequest(REQUEST), {
    exists: true, jurisdiction: null, participants: PARTICIPANTS,
  });
  assert.equal(v.ok, true);
  assert.equal(v.action, 'record');
  assert.deepEqual(v.blocking, []);
});

test('an account that already has a jurisdiction is a correction, and not this institution’s to make', () => {
  const v = judge(parseRequest(REQUEST), {
    exists: true,
    jurisdiction: { country: 'NG', recorded_by: 'yml1someone' },
    participants: PARTICIPANTS,
  });
  assert.equal(v.ok, false);
  assert.equal(v.action, 'refuse');
  assert.ok(v.blocking.some((b) => /foundation administrator/i.test(b)),
    'a participant who could rewrite a recorded country could move a customer out from under an authority');
});

test('an institution that is not an approved participant is refused', () => {
  const v = judge(parseRequest(REQUEST), {
    exists: true, jurisdiction: null, participants: [],
  });
  assert.equal(v.ok, false);
  assert.ok(v.blocking.some((b) => /approved participant/i.test(b)));
});

test('a reserved country code is refused, naming the reason', () => {
  const zz = parseRequest(REQUEST.replace('SN (Senegal)', 'ZZ (reserved)').replace('being in SN', 'being in ZZ'));
  const v = judge(zz, { exists: true, jurisdiction: null, participants: PARTICIPANTS });
  assert.equal(v.ok, false);
  assert.ok(v.blocking.some((b) => /foundation/i.test(b)),
    'ZZ is the marker foundation administrators carry and must not be handed to an ordinary account');
});

test('an account the chain has never seen is an absence, reported as one', () => {
  const v = judge(parseRequest(REQUEST), {
    exists: false, jurisdiction: null, participants: PARTICIPANTS,
  });
  assert.equal(v.ok, false);
  assert.ok(v.blocking.some((b) => /never/i.test(b) || /no such account/i.test(b)));
});

test('judging a failed parse does not throw', () => {
  const v = judge({ problem: 'not a placement request' }, {
    exists: false, jurisdiction: null, participants: [],
  });
  assert.equal(v.ok, false);
});
