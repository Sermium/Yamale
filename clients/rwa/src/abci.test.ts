import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import { decoded, fromBase64, interpret, isAnswer, toHex, type Outcome } from './abci.ts';

test('request bytes go on the wire as 0x-prefixed lower-case hex', () => {
  assert.equal(toHex(new Uint8Array([])), '0x');
  assert.equal(toHex(new Uint8Array([0x08, 0x03])), '0x0803');
  // A byte below 0x10 must keep its leading zero, or every following byte is
  // read one nibble out and the request decodes as a different message.
  assert.equal(toHex(new Uint8Array([0x00, 0x0f, 0xff])), '0x000fff');
});

test('an empty response is a value, not a failure', () => {
  // A query whose answer is an empty message encodes to zero bytes. Several
  // base64 helpers treat the empty string as an error; this one must not.
  assert.deepEqual(fromBase64(''), new Uint8Array(0));
});

test('base64 comes back as the exact bytes', () => {
  // "EgA=" is what an empty Collections response actually returns.
  assert.deepEqual(fromBase64('EgA='), new Uint8Array([0x12, 0x00]));
});

test('a code of zero is an answer, and it carries the height it was read at', () => {
  const out = interpret({
    result: { response: { code: 0, value: 'EgA=', height: '94478' } },
  });
  assert.equal(out.ok, true);
  assert.ok(out.ok);
  assert.equal(out.height, 94478);
  assert.deepEqual(out.value, new Uint8Array([0x12, 0x00]));
});

test('a missing record is distinguished from a refused query', () => {
  // These need opposite sentences on screen. "The registry never permitted this
  // sale" and "we could not ask" must never render the same way.
  const missing = interpret({
    result: { response: { code: 22, log: 'this parcel has no fractionalisation authorisation: not found' } },
  });
  assert.equal(missing.ok, false);
  assert.ok(!missing.ok);
  assert.equal(missing.reason, 'not-found');

  const refused = interpret({
    result: { response: { code: 6, log: 'unknown query path: unknown request' } },
  });
  assert.ok(!refused.ok);
  assert.equal(refused.reason, 'refused');
});

test('a transport error is unreachable, never an empty answer', () => {
  const err = interpret({ error: { message: 'connection reset' } });
  assert.ok(!err.ok);
  assert.equal(err.reason, 'unreachable');
  assert.equal(err.detail, 'connection reset');

  const empty = interpret({});
  assert.ok(!empty.ok);
  assert.equal(empty.reason, 'unreachable');
});

test('only an unreachable node counts as the chain not having answered', () => {
  assert.equal(isAnswer({ ok: true, value: 1, height: 1 } as Outcome<number>), true);
  assert.equal(isAnswer({ ok: false, reason: 'not-found', detail: '' }), true);
  assert.equal(isAnswer({ ok: false, reason: 'refused', detail: '' }), true);
  assert.equal(isAnswer({ ok: false, reason: 'unreachable', detail: '' }), false);
});

test('decoding keeps the height, and a failed decode is a fault not an empty result', () => {
  const ok = decoded({ ok: true, value: new Uint8Array([1]), height: 42 }, (b) => b.length);
  assert.ok(ok.ok);
  assert.equal(ok.value, 1);
  assert.equal(ok.height, 42);

  const bad = decoded({ ok: true, value: new Uint8Array([1]), height: 42 }, () => {
    throw new Error('index out of range');
  });
  assert.ok(!bad.ok);
  assert.equal(bad.reason, 'refused');
  assert.match(bad.detail, /could not decode/);
});

test('a failure passes through decoding unchanged', () => {
  const out = decoded<number>({ ok: false, reason: 'not-found', detail: 'no such asset' }, () => 1);
  assert.ok(!out.ok);
  assert.equal(out.reason, 'not-found');
  assert.equal(out.detail, 'no such asset');
});
