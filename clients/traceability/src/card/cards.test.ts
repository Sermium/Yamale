import assert from 'node:assert/strict';
import { test } from 'node:test';

import { compressedPublicKey, toHex } from '../../../sdk/src/jewel.ts';
import { cancels, cardFromLocation, forgetKeptCard, forgetScannedCard, issueCards, keepCard, keptCard, pages, selectPieces, type StockPiece } from './cards.ts';

class MemoryStore implements Storage {
  m = new Map<string, string>();
  get length() { return this.m.size; }
  clear() { this.m.clear(); }
  getItem(k: string) { return this.m.get(k) ?? null; }
  key(i: number) { return [...this.m.keys()][i] ?? null; }
  removeItem(k: string) { this.m.delete(k); }
  setItem(k: string, v: string) { this.m.set(k, v); }
}

const pieces = Array.from({ length: 7 }, (_, i) => ({ jewelId: String(i + 1), title: 'Ring', inscription: `R-${i + 1}` }));
let n = 0;
const keyN = () => new Uint8Array(32).fill(++n);

test('every batch that goes through is printed, with the key the chain was given', async () => {
  const sent: { jewelId: string; cardKey: string }[][] = [];
  const out = await issueCards(pieces, async (b) => { sent.push(b); return { ok: true }; }, 3, keyN);
  assert.equal(out.stopped, null);
  assert.deepEqual(sent.map((b) => b.length), [3, 3, 1]);
  assert.equal(out.issued.length, 7);
  for (const c of out.issued) {
    const onChain = sent.flat().find((s) => s.jewelId === c.jewelId)!;
    assert.equal(onChain.cardKey, toHex(compressedPublicKey(c.privateKey)));
  }
});

test('a refused batch prints nothing of its own, keeps what went before, and runs nothing after', async () => {
  let call = 0;
  const out = await issueCards(pieces, async () => (++call === 2 ? { ok: false, text: 'suspended' } : { ok: true }), 3, keyN);
  assert.equal(out.stopped, 'suspended');
  assert.deepEqual(out.issued.map((c) => c.jewelId), ['1', '2', '3']);
  assert.deepEqual([...out.outcomes.values()], ['issued', 'issued', 'issued', 'failed', 'failed', 'failed', 'notRun']);
});

test('a batch that threw is unknown, and its keys are kept: it may have reached the chain', async () => {
  const made: Uint8Array[] = [];
  const out = await issueCards(pieces.slice(0, 2), async () => { throw new Error('locked'); }, 3,
    () => { const k = keyN(); made.push(k); return k; });
  assert.equal(out.stopped, 'locked');
  assert.equal(out.issued.length, 0);
  assert.deepEqual([...out.outcomes.values()], ['unknown', 'unknown']);
  assert.ok(made.every((k) => k.some((b) => b !== 0)));
});

test('the scanned card survives a reload in the same tab, and is erased after filing', () => {
  const store = new MemoryStore();
  const secret = toHex(new Uint8Array(32).fill(9));
  const first = cardFromLocation(`#12.${secret}`, store);
  assert.equal(first?.jewelId, '12');
  const again = cardFromLocation('', store);
  assert.equal(again?.jewelId, '12');
  assert.deepEqual(again?.privateKey, new Uint8Array(32).fill(9));
  forgetScannedCard(store);
  assert.equal(cardFromLocation('', store), null);
});

test('a broken fragment is refused even when a card is kept', () => {
  const store = new MemoryStore();
  cardFromLocation(`#12.${toHex(new Uint8Array(32).fill(9))}`, store);
  assert.equal(cardFromLocation('#12.nope', store), null);
});

test('selections pick pieces without a card, one design, or one piece', () => {
  const stock: StockPiece[] = [
    { jewelId: '1', title: 'Ring', inscription: 'A', hasCard: true, design: 'ring' },
    { jewelId: '2', title: 'Ring', inscription: 'B', hasCard: false, design: 'ring' },
    { jewelId: '3', title: 'Bangle', inscription: 'C', hasCard: false, design: 'bangle' },
  ];
  assert.deepEqual(selectPieces(stock, { kind: 'without' }).map((p) => p.jewelId), ['2', '3']);
  assert.deepEqual(selectPieces(stock, { kind: 'design', design: 'ring' }).map((p) => p.jewelId), ['1', '2']);
  assert.deepEqual(selectPieces(stock, { kind: 'one', jewelId: '3' }).map((p) => p.jewelId), ['3']);
  assert.equal(cancels(selectPieces(stock, { kind: 'design', design: 'ring' })), 1);
});

test('cards fill A4 pages of eight', () => {
  assert.deepEqual(pages([1, 2, 3, 4, 5, 6, 7, 8, 9]).map((p) => p.length), [8, 1]);
  assert.deepEqual(pages([]), []);
});

test('the shop keeps the newest card of each jewel until it forgets it', () => {
  const store = new MemoryStore();
  assert.equal(keptCard('5', store), null);
  keepCard('5', new Uint8Array(32).fill(3), store);
  keepCard('5', new Uint8Array(32).fill(4), store);
  assert.deepEqual(keptCard('5', store), new Uint8Array(32).fill(4));
  forgetKeptCard('5', store);
  assert.equal(keptCard('5', store), null);
  store.setItem('yamale.traceability.card.6', 'not a key');
  assert.equal(keptCard('6', store), null);
});
