import assert from 'node:assert/strict';
import { test } from 'node:test';

import { acceptMove, safeTarget } from './movein.ts';
import type { KeyStore, StoredAccount } from '../../../shared/account/device.ts';

const KEY = 'yamale.account';
const memory = (init: Record<string, string> = {}): KeyStore & { data: Record<string, string> } => {
  const data = { ...init };
  return { data, getItem: (k) => data[k] ?? null, setItem: (k, v) => { data[k] = v; }, removeItem: (k) => { delete data[k]; } };
};
const b64u = (s: string) => Buffer.from(s, 'utf8').toString('base64url');
const acct = (address: string): StoredAccount => ({
  address, email: 'a@example.com', createdAt: '2026-11-16T00:00:00.000Z', protection: 'none', sponsored: true,
  sealed: { salt: 'c2FsdA', nonce: 'bm9uY2U', ciphertext: 'Y2lwaGVy', time: 3, memory: 65536, threads: 1 },
});
const A = 'yml1dzwpk73ntdyve2vx49flv0dxxguy6a5ygp2zv7';
const B = 'yml1rjgw9548jhz9e87me4zja37sym0nrt3n3huu2q';

test('acceptMove stores the incoming account when this origin holds none, and keeps the target', () => {
  const s = memory();
  const out = acceptMove(`#move=${b64u(JSON.stringify(acct(A)))}&to=${b64u('/claim?x=1#k=abc')}`, s);
  assert.deepEqual(out, { kind: 'stored', address: A, to: '/claim?x=1#k=abc' });
  assert.equal(JSON.parse(s.data[KEY]!).address, A);
});

test('acceptMove does nothing when the same account is already here', () => {
  const s = memory({ [KEY]: JSON.stringify(acct(A)) });
  const before = s.data[KEY];
  assert.deepEqual(acceptMove(`#move=${b64u(JSON.stringify(acct(A)))}&to=${b64u('/')}`, s), { kind: 'same', address: A, to: '/' });
  assert.equal(s.data[KEY], before);
});

test('acceptMove never overwrites a different account', () => {
  const s = memory({ [KEY]: JSON.stringify(acct(B)) });
  assert.deepEqual(acceptMove(`#move=${b64u(JSON.stringify(acct(A)))}&to=${b64u('/mine')}`, s),
    { kind: 'other', here: B, incoming: A, to: '/mine' });
  assert.equal(JSON.parse(s.data[KEY]!).address, B);
});

test('acceptMove refuses a payload that is not a sealed account', () => {
  const s = memory();
  for (const bad of ['not-base64!', b64u('{}'), b64u('{"address":"cosmos1x","sealed":{}}'), b64u(JSON.stringify({ ...acct(A), sealed: { salt: 'x' } }))]) {
    assert.equal(acceptMove(`#move=${bad}&to=${b64u('/')}`, s).kind, 'invalid');
  }
  assert.equal(s.data[KEY], undefined);
});

test('acceptMove refuses KDF parameters beyond what the seal produces, and malformed metadata', () => {
  const s = memory();
  const w = acct(A).sealed;
  const bad: unknown[] = [
    { ...acct(A), sealed: { ...w, time: 1e9 } },
    { ...acct(A), sealed: { ...w, memory: 4194304 } },
    { ...acct(A), sealed: { ...w, threads: 64 } },
    { ...acct(A), sealed: { ...w, time: 0 } },
    { ...acct(A), email: undefined },
    { ...acct(A), createdAt: 5 },
    { ...acct(A), protection: 'weird' },
  ];
  for (const b of bad) assert.equal(acceptMove(`#move=${b64u(JSON.stringify(b))}&to=${b64u('/')}`, s).kind, 'invalid');
  assert.equal(s.data[KEY], undefined);
  assert.equal(acceptMove(`#move=${b64u(JSON.stringify(acct(A)))}&to=${b64u('/')}`, s).kind, 'stored');
});

test('acceptMove: an unsafe target becomes /', () => {
  const s = memory();
  assert.equal(acceptMove(`#move=${b64u(JSON.stringify(acct(A)))}&to=${b64u('//evil.example/x')}`, s).to, '/');
});

const rows: Array<[string | null, string]> = [
  ['/j/1/1', '/j/1/1'], ['/claim#k=1', '/claim#k=1'], ['//x.example', '/'], ['/\\x.example', '/'],
  ['https://x.example', '/'], [null, '/'], ['', '/'],
];
for (const [raw, want] of rows) {
  test(`safeTarget ${JSON.stringify(raw)} -> ${JSON.stringify(want)}`, () => assert.equal(safeTarget(raw), want));
}
