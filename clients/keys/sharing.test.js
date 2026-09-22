// The page's arithmetic, held to the same vectors as the services.
//
// This is a third implementation of one piece of arithmetic — Go runs one, the
// wallet runs another — and the only thing that makes that acceptable is that
// none of them is trusted on its own authority. All three answer to
// recovery/testdata/vectors.json.
//
// The drift test at the bottom matters as much as the arithmetic. This
// directory ships as static files, so it carries its own copy of the vectors;
// a copy that quietly fell behind the source would let this page demonstrate
// values the services no longer produce, which is worse than not demonstrating
// anything.
import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import { combine, fromBase64, fromHex, same, split, toHex } from './sharing.js';

const here = dirname(fileURLToPath(import.meta.url));
const SOURCE = join(here, '../../recovery/testdata/vectors.json');
const SHIPPED = join(here, 'vectors.json');

const vectors = JSON.parse(readFileSync(SHIPPED, 'utf8'));

test('the shipped vectors are the ones the services were built against', () => {
  // Byte-for-byte. Regenerating the source and forgetting this copy is the
  // realistic way this page starts lying, and it would do so quietly.
  assert.equal(
    readFileSync(SHIPPED, 'utf8'),
    readFileSync(SOURCE, 'utf8'),
    'clients/keys/vectors.json has drifted from recovery/testdata/vectors.json — copy it again',
  );
});

test('combine reconstructs exactly what Go reconstructed', () => {
  assert.ok(vectors.combine.length > 0, 'no combine vectors, so this passes vacuously');
  for (const [i, v] of vectors.combine.entries()) {
    const shares = v.shares.map((s) => ({ x: s.x, y: fromHex(s.y) }));
    assert.equal(toHex(combine(shares)), v.expect, `combine[${i}] disagrees with Go`);
  }
});

test('every pair of parts rebuilds the secret', () => {
  // Not "some two" — every two. A Lagrange basis with the subtraction the wrong
  // way round rebuilds correctly from parts 1 and 2 and wrongly from 2 and 3.
  const secret = new TextEncoder().encode('a sealed blob, or something its length');
  const parts = split(secret, 3, 2);
  for (let i = 0; i < parts.length; i++) {
    for (let j = i + 1; j < parts.length; j++) {
      assert.ok(same(combine([parts[i], parts[j]]), secret), `parts ${i} and ${j} failed`);
    }
  }
});

test('one part alone is refused, and two of a three-of-three are wrong in silence', () => {
  const secret = new TextEncoder().encode('three of three');
  const parts = split(secret, 3, 3);
  assert.throws(() => combine([parts[0]]), /not enough/);

  // The property the page demonstrates: a well-formed wrong answer, with
  // nothing in the arithmetic able to flag it.
  const short = combine([parts[0], parts[1]]);
  assert.equal(short.length, secret.length, 'the wrong answer is not even the right shape');
  assert.ok(!same(short, secret), 'two of three rebuilt a three-of-three secret');
});

test('the demonstration the page runs: two service parts are not the key', () => {
  // The exact claim, exercised the way the page exercises it. The sealed
  // ciphertext is real — produced by the Go implementation — and what the two
  // service parts rebuild is that ciphertext and not the key inside it.
  const sealed = fromHex(vectors.seal[0].expect);
  const key = fromHex(vectors.seal[0].secret);

  const parts = split(sealed, 3, 2);
  const services = [parts[1], parts[2]];
  const rebuilt = combine(services);

  assert.ok(same(rebuilt, sealed), 'the two service parts did not rebuild the sealed blob');
  assert.ok(!same(rebuilt, key), 'the two service parts rebuilt the key itself');
  assert.ok(rebuilt.length > key.length, 'a sealed blob carries a tag, so it is longer than its key');
});

test('base64 decoding matches what the services emit', () => {
  // The wire form: Go marshals []byte as base64, and the page reads holdings
  // straight out of vectors.json in that form.
  const wire = vectors.wire;
  assert.ok(wire, 'no wire vector');
  assert.equal(fromBase64(wire.salt).length, 16);
  assert.equal(fromBase64(wire.nonce).length, 24);
  assert.ok(fromBase64(wire.share.y).length > 0);
});
