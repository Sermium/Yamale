import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { decodeBalances, stringField, toHex, varint } from './threshold.js';

// The console has no build step, so it carries its own protobuf. A codec
// nobody tests is a codec that reports a balance nobody holds — which is worse
// than reporting none, because it looks like an answer.

test('varint encodes the boundary cases protobuf actually hits', () => {
  assert.deepEqual(varint(0), [0]);
  assert.deepEqual(varint(1), [1]);
  // 127 is the last single byte; 128 is the first that continues.
  assert.deepEqual(varint(127), [127]);
  assert.deepEqual(varint(128), [128, 1]);
  assert.deepEqual(varint(300), [172, 2]);
});

test('a string field carries its tag, its length and its bytes', () => {
  const out = stringField(1, 'yml1abc');
  // field 1, wire type 2 -> tag 0x0a; then the length; then the text.
  assert.equal(out[0], 0x0a);
  assert.equal(out[1], 7);
  assert.equal(new TextDecoder().decode(Uint8Array.from(out.slice(2))), 'yml1abc');
});

test('toHex pads, because a dropped leading zero is a different address', () => {
  assert.equal(toHex(Uint8Array.from([0, 10, 255])), '000aff');
});

/** Builds an AllBalances response the way the chain would. */
function encodeCoins(coins) {
  const out = [];
  for (const { denom, amount } of coins) {
    const inner = [...stringField(1, denom), ...stringField(2, amount)];
    out.push(...varint((1 << 3) | 2), ...varint(inner.length), ...inner);
  }
  return Uint8Array.from(out);
}

test('balances round-trip', () => {
  const coins = [
    { denom: 'uyml', amount: '2984000' },
    { denom: 'uxof', amount: '12500' },
  ];
  assert.deepEqual(decodeBalances(encodeCoins(coins)), coins);
});

test('an empty response is no coins rather than a crash', () => {
  assert.deepEqual(decodeBalances(new Uint8Array()), []);
});

test('an unknown field is skipped rather than guessed at', () => {
  // A pagination field (2) sitting after the coins, which the chain does send.
  const coins = encodeCoins([{ denom: 'uyml', amount: '5' }]);
  const withExtra = Uint8Array.from([...coins, (2 << 3) | 2, 2, 0x08, 0x00]);
  assert.deepEqual(decodeBalances(withExtra), [{ denom: 'uyml', amount: '5' }]);
});
