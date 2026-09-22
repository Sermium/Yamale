/**
 * The transport's two decisions, neither of which needs a network.
 *
 * Whether a reply is an answer, and what the bytes look like on the way out.
 * Both are provoked here from replies that are awkward to get out of a live
 * node — a pruned height, a path the module does not serve — because those are
 * exactly the ones a field device meets and nobody can reproduce on a desk.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { fromBase64, interpret, isAnswer, isSilence, toHex } from './abci.ts';

describe('the bytes', () => {
  it('encodes a request the way the data parameter wants it', () => {
    assert.equal(toHex(new Uint8Array([8, 1])), '0x0801');
    assert.equal(toHex(new Uint8Array([0, 255, 16])), '0x00ff10');
    assert.equal(toHex(new Uint8Array(0)), '0x');
  });

  it('decodes an empty response without calling it an error', () => {
    // A query whose answer is an empty message encodes to zero bytes, and
    // several base64 helpers treat the empty string as a failure.
    assert.deepEqual(fromBase64(''), new Uint8Array(0));
  });
});

describe('interpreting a reply', () => {
  it('reads a success with its height', () => {
    const got = interpret({ result: { response: { code: 0, value: '', height: '462701' } } });
    assert.equal(got.ok, true);
    if (!got.ok) return;
    assert.equal(got.height, 462701);
  });

  it('calls the module\'s own not-found an answer', () => {
    // Read off devnet-2 by asking for herd 3, which does not exist.
    const got = interpret({
      result: { response: { code: 22, log: 'rpc error: code = NotFound desc = no herd 3: key not found' } },
    });
    assert.equal(got.ok, false);
    if (got.ok) return;
    assert.equal(got.reason, 'not-found');
    assert.equal(isAnswer(got), true);
    assert.equal(isSilence(got), false);
  });

  it('calls an unserved path a refusal, which is not the same as absent', () => {
    // A node that has stopped serving a path, or a pruned height. The record
    // may well exist; this reply says nothing about whether it does.
    const got = interpret({ result: { response: { code: 1, log: 'unknown query path' } } });
    assert.equal(got.ok, false);
    if (got.ok) return;
    assert.equal(got.reason, 'refused');
    assert.equal(isAnswer(got), true);
  });

  it('calls a transport error silence rather than a finding', () => {
    const got = interpret({ error: { message: 'connection reset' } });
    assert.equal(got.ok, false);
    if (got.ok) return;
    assert.equal(got.reason, 'unreachable');
    assert.equal(isAnswer(got), false);
    assert.equal(isSilence(got), true);
  });

  it('treats a reply with no response at all as silence', () => {
    const got = interpret({});
    assert.equal(got.ok, false);
    if (got.ok) return;
    assert.equal(got.reason, 'unreachable');
  });

  it('separates being offline from a broken node', () => {
    // A field device with no signal is Tuesday, not a fault, and a screen that
    // reported it with the same red panel as a broken node would train the
    // operator to ignore the red panel.
    const offline = { ok: false as const, reason: 'offline' as const, detail: '' };
    assert.equal(isSilence(offline), true);
    assert.equal(isAnswer(offline), false);
  });
});
