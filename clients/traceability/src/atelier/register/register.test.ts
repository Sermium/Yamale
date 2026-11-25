import assert from 'node:assert/strict';
import { test } from 'node:test';

import { paced, registerPieces, type Piece, type RegisterDeps } from './register.ts';

/** A register in memory: inscriptions to ids, and a switch to refuse one batch. */
function fakeChain(refuseBatch = -1) {
  const jewels = new Map<string, string>();
  const stored = new Set<string>();
  let batches = 0;
  let next = 100;
  const deps: RegisterDeps = {
    async taken(ins) { return { ok: true, value: new Map(ins.filter((i) => jewels.has(i)).map((i) => [i, jewels.get(i)!])) }; },
    async upload(record) { const h = String(record.length) + ':' + new TextDecoder().decode(record); stored.add(h); return { ok: true, value: h }; },
    async submit(batch) {
      if (batches++ === refuseBatch) return { ok: false, text: 'refused', raw: 'out of gas' };
      return { ok: true, value: batch.map((b) => { const id = String(next++); jewels.set(b.inscription, id); return id; }) };
    },
    progress() {},
  };
  return { deps, jewels, stored, batchesSent: () => batches };
}

const pieces = (n: number): Piece[] => Array.from({ length: n }, (_, i) => ({
  id: i, inscription: `P${String(i).padStart(4, '0')}`, record: new TextEncoder().encode(`record ${i}`),
}));

test('pieces are registered in batches, each record stored just before its batch', async () => {
  const c = fakeChain();
  const out = await registerPieces(pieces(12), c.deps, 5);
  assert.equal(out.stopped, null);
  assert.equal(c.batchesSent(), 3);
  assert.equal(c.jewels.size, 12);
  assert.deepEqual([...out.outcomes.values()].map((o) => o.status), Array(12).fill('registered'));
  assert.equal(out.outcomes.get(0)!.jewelId, '100');
});

test('a refused batch stops the run there; the earlier ones stay registered', async () => {
  const c = fakeChain(1);
  const out = await registerPieces(pieces(12), c.deps, 5);
  assert.deepEqual(out.stopped, { text: 'refused', raw: 'out of gas' });
  assert.equal(c.jewels.size, 5);
  const statuses = [...out.outcomes.values()].map((o) => o.status);
  assert.deepEqual(statuses, [...Array(5).fill('registered'), ...Array(5).fill('failed'), 'notRun', 'notRun']);
});

test('running the same file again registers only the rest, and nothing twice', async () => {
  const c = fakeChain(1);
  await registerPieces(pieces(12), c.deps, 5);
  let next = 500; // the next run's batches are accepted
  c.deps.submit = async (batch) => ({ ok: true, value: batch.map((b) => { const id = String(next++); c.jewels.set(b.inscription, id); return id; }) });
  const again = await registerPieces(pieces(12), c.deps, 5);
  assert.equal(again.stopped, null);
  assert.equal(c.jewels.size, 12);
  const statuses = [...again.outcomes.values()].map((o) => o.status);
  assert.deepEqual(statuses, [...Array(5).fill('alreadyRegistered'), ...Array(7).fill('registered')]);
});

test('a failed upload stops before its batch is sent', async () => {
  const c = fakeChain();
  let n = 0;
  c.deps.upload = async (record) => (++n === 7 ? { ok: false, text: 'busy', raw: '503' } : { ok: true, value: String(record.length) });
  const out = await registerPieces(pieces(12), c.deps, 5);
  assert.equal(c.batchesSent(), 1);
  assert.deepEqual(out.stopped, { text: 'busy', raw: '503' });
  assert.deepEqual([...out.outcomes.values()].map((o) => o.status),
    [...Array(5).fill('registered'), 'notRun', 'failed', ...Array(5).fill('notRun')]);
});

test('an upload that throws stops the run, keeps what was registered and marks only its own piece failed', async () => {
  const c = fakeChain();
  let n = 0;
  c.deps.upload = async (record) => { if (++n === 7) throw new Error('locked'); return { ok: true, value: String(record.length) }; };
  const out = await registerPieces(pieces(12), c.deps, 5);
  assert.equal(out.interrupted, true);
  assert.deepEqual(out.stopped, { text: '', raw: 'locked' });
  assert.deepEqual([...out.outcomes.values()].map((o) => o.status),
    [...Array(5).fill('registered'), 'notRun', 'failed', ...Array(5).fill('notRun')]);
});

test('a submit that throws leaves its batch unknown, the earlier batch registered and the rest not run', async () => {
  const c = fakeChain();
  const real = c.deps.submit;
  let n = 0;
  c.deps.submit = async (batch) => { if (++n === 2) throw new Error('locked'); return real(batch); };
  const out = await registerPieces(pieces(12), c.deps, 5);
  assert.equal(out.interrupted, true);
  assert.deepEqual([...out.outcomes.values()].map((o) => o.status),
    [...Array(5).fill('registered'), ...Array(5).fill('unknown'), 'notRun', 'notRun']);
});

test('progress counts settled pieces', async () => {
  const c = fakeChain();
  const seen: [number, number][] = [];
  c.deps.progress = (done, of) => seen.push([done, of]);
  await registerPieces(pieces(7), c.deps, 5);
  assert.deepEqual(seen, [[0, 7], [5, 7], [7, 7]]);
});

test('a busy answer is retried after a pause; another answer is returned at once', async () => {
  const waits: number[] = [];
  let calls = 0;
  const r = await paced(async () => (++calls < 3 ? { status: 429 } : { status: 201 }), (x) => x.status === 429,
    async (ms) => { waits.push(ms); }, [10, 20, 30]);
  assert.deepEqual(r, { status: 201 });
  assert.deepEqual(waits, [10, 20]);
  const never = await paced(async () => ({ status: 503 }), (x) => x.status === 503, async () => {}, [1, 1]);
  assert.deepEqual(never, { status: 503 });
});
