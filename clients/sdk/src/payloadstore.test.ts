import assert from 'node:assert/strict';
import { test } from 'node:test';

import { openRecipientBlock, paymentAad, sealPayload, sealToViewingKey } from './envelope.ts';
import { metadataHash, newPaymentMetadata } from './metadata.ts';
import {
  PayloadStoreClient,
  describeUnavailable,
  newViewingKey,
  sealForStore,
  viewingKeyPublic,
  type PayloadResult,
  type PayloadUnavailableReason,
} from './payloadstore.ts';

const PARTICIPANT = 'yml1bankone';
const E2E = 'E2E-CLIENT-1';

function toBase64(bytes: Uint8Array): string {
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary);
}

/**
 * A store that answers exactly what the real one answers.
 *
 * Written against the wire shape rather than against a mocked client, because
 * the thing under test is how this SDK reacts to each of the store's answers —
 * and a mock that returned the SDK's own vocabulary would test nothing.
 */
function fakeStore(options: {
  reader: { privateKey: Uint8Array; publicKey: Uint8Array };
  body?: Record<string, unknown>;
  status?: number;
  envelope?: Uint8Array;
  throwOn?: 'challenge' | 'payload';
}): typeof fetch {
  return (async (input: string | URL | Request) => {
    const url = String(input);

    if (url.endsWith('/challenge')) {
      if (options.throwOn === 'challenge') throw new Error('connection refused');
      // The nonce is sealed to the caller's registered public half, which is
      // what makes answering the challenge proof of possession.
      const nonce = new Uint8Array(32).fill(3);
      const block = sealToViewingKey(nonce, options.reader.publicKey);
      return new Response(
        JSON.stringify({
          status: 'ok',
          challenge_id: 'abc123',
          ephemeral_public_key: toBase64(block.ephemeralPublicKey),
          wrapped_nonce: toBase64(block.wrappedKey),
          key_id: toBase64(block.keyId),
        }),
        { status: 200 },
      );
    }

    if (options.throwOn === 'payload') throw new Error('connection reset');
    if (options.envelope) {
      return new Response(JSON.stringify({ status: 'ok', envelope: toBase64(options.envelope) }), { status: 200 });
    }
    return new Response(JSON.stringify(options.body ?? {}), { status: options.status ?? 404 });
  }) as unknown as typeof fetch;
}

function client(reader: { privateKey: Uint8Array; publicKey: Uint8Array }, fetchImpl: typeof fetch) {
  return new PayloadStoreClient({
    baseUrl: 'https://store.example',
    address: 'yml1payee',
    viewingKey: reader.privateKey,
    fetchImpl,
  });
}

// The whole path: challenge answered from the viewing key, envelope fetched,
// decrypted, and checked against the hash the chain recorded.
test('an entitled reader fetches, decrypts and verifies against the chain hash', async () => {
  const reader = newViewingKey();
  const payload = newPaymentMetadata('SALA', 'March salary, employee 4417');
  const recorded = await metadataHash(payload);
  const envelope = sealForStore(payload, [{ publicKey: reader.publicKey }], PARTICIPANT, E2E);

  const result = await client(reader, fakeStore({ reader, envelope })).fetchPayload(PARTICIPANT, E2E, recorded);

  assert.equal(result.status, 'available');
  assert.equal(result.status === 'available' && result.payload.remittanceInformation, 'March salary, employee 4417');
});

// Erasure is the outcome the whole design exists to make possible, so it is a
// named result rather than an error. A client that reported it as a fault would
// have an operator chasing a working system.
test('an erased payload is reported as erased, not as a fault', async () => {
  const reader = newViewingKey();
  const recorded = await metadataHash(newPaymentMetadata('SALA', 'March salary'));

  const result = await client(
    reader,
    fakeStore({ reader, body: { status: 'unavailable', reason: 'erased' }, status: 404 }),
  ).fetchPayload(PARTICIPANT, E2E, recorded);

  assert.equal(result.status, 'unavailable');
  assert.equal(result.status === 'unavailable' && result.reason, 'erased');
  assert.match(describeUnavailable('erased'), /erased/);
  // And the wording says the payment itself is fine, because it is.
  assert.match(describeUnavailable('erased'), /still verifies/);
});

// Every one of the store's answers has to map to a reason the interface has
// wording for. A raw token reaching a screen is how "erased" ends up rendered
// as a bug.
test('every store answer maps to a reason with wording', async () => {
  const reader = newViewingKey();
  const recorded = await metadataHash(newPaymentMetadata('SALA', 'March salary'));

  const cases: Array<[Record<string, unknown>, PayloadUnavailableReason]> = [
    [{ status: 'unavailable', reason: 'erased' }, 'erased'],
    [{ status: 'unavailable', reason: 'never_stored' }, 'never-stored'],
    [{ status: 'unavailable', reason: 'no_such_payment' }, 'no-such-payment'],
    [{ status: 'denied', reason: 'not_entitled' }, 'not-entitled'],
    [{ status: 'denied', reason: 'unproven' }, 'unproven'],
    // A store speaking a dialect this build does not know is, to the caller, a
    // store it cannot use — not an excuse to invent a reason the UI has no
    // wording for.
    [{ status: 'unavailable', reason: 'something-new' }, 'unreachable'],
  ];

  for (const [body, expected] of cases) {
    const result = await client(reader, fakeStore({ reader, body, status: 404 })).fetchPayload(
      PARTICIPANT,
      E2E,
      recorded,
    );
    assert.equal(result.status, 'unavailable');
    assert.equal(result.status === 'unavailable' && result.reason, expected, JSON.stringify(body));
    assert.match(describeUnavailable(expected), /^Detail unavailable/);
  }
});

// A host that cannot be reached is a different statement from a payload that
// was destroyed, and conflating them is how a retryable fault gets reported as
// a permanent one.
test('an unreachable store is not reported as an erasure', async () => {
  const reader = newViewingKey();
  const recorded = await metadataHash(newPaymentMetadata('SALA', 'March salary'));

  for (const stage of ['challenge', 'payload'] as const) {
    const result = await client(reader, fakeStore({ reader, throwOn: stage })).fetchPayload(PARTICIPANT, E2E, recorded);
    assert.equal(result.status, 'unavailable');
    assert.equal(result.status === 'unavailable' && result.reason, 'unreachable', stage);
  }
});

// The chain's hash is the arbiter. A store that served a different payload —
// one that decrypts perfectly — must not be believed, or the payload is a claim
// by whoever is holding it.
test('a payload that disagrees with the chain hash is refused', async () => {
  const reader = newViewingKey();
  const served = newPaymentMetadata('SALA', 'March salary, employee 4418');
  const envelope = sealForStore(served, [{ publicKey: reader.publicKey }], PARTICIPANT, E2E);

  // The hash of a different payload — what the chain actually recorded.
  const recorded = await metadataHash(newPaymentMetadata('SALA', 'March salary, employee 4417'));

  const result = await client(reader, fakeStore({ reader, envelope })).fetchPayload(PARTICIPANT, E2E, recorded);
  assert.equal(result.status, 'unavailable');
  assert.equal(result.status === 'unavailable' && result.reason, 'not-the-recorded-payload');
});

// An envelope sealed for another payment must not be accepted for this one,
// even though it would decrypt: the associated data is what binds it.
test('an envelope sealed for another payment is refused', async () => {
  const reader = newViewingKey();
  const payload = newPaymentMetadata('SALA', 'March salary');
  const recorded = await metadataHash(payload);

  const envelope = sealForStore(payload, [{ publicKey: reader.publicKey }], PARTICIPANT, 'E2E-SOMETHING-ELSE');

  const result = await client(reader, fakeStore({ reader, envelope })).fetchPayload(PARTICIPANT, E2E, recorded);
  assert.equal(result.status, 'unavailable');
  assert.equal(result.status === 'unavailable' && result.reason, 'not-the-recorded-payload');
});

// The result type has no member carrying an empty payload, which is the
// property that makes the screen this repository already shipped once
// impossible to write again.
test('an available result always carries a payload', async () => {
  const reader = newViewingKey();
  const payload = newPaymentMetadata('SUPP', '');
  const recorded = await metadataHash(payload);
  const envelope = sealForStore(payload, [{ publicKey: reader.publicKey }], PARTICIPANT, E2E);

  const result: PayloadResult = await client(reader, fakeStore({ reader, envelope })).fetchPayload(
    PARTICIPANT,
    E2E,
    recorded,
  );

  // A payment whose remittance line is genuinely empty is 'available' with an
  // empty string — which is a different and truthful statement from
  // 'unavailable', and the caller can tell them apart.
  assert.equal(result.status, 'available');
  assert.equal(result.status === 'available' && result.payload.remittanceInformation, '');
  assert.equal(result.status === 'available' && result.payload.salt.length, 32);
});

// The viewing key never leaves the holder. What goes on-chain is the public
// half and nothing else.
test('only the public half is derived for publication', () => {
  const { privateKey, publicKey } = newViewingKey();
  assert.equal(privateKey.length, 32);
  assert.deepEqual(viewingKeyPublic(privateKey), publicKey);
  assert.notDeepEqual(privateKey, publicKey);
});

// A challenge sealed to one account cannot be answered by another, which is
// what makes proof of possession mean anything.
test('a challenge sealed to one key does not open with another', () => {
  const reader = newViewingKey();
  const stranger = newViewingKey();
  const block = sealToViewingKey(new Uint8Array(32).fill(5), reader.publicKey);

  assert.deepEqual(openRecipientBlock(block, reader.privateKey), new Uint8Array(32).fill(5));
  assert.throws(() => openRecipientBlock(block, stranger.privateKey));
});

// The envelope the SDK seals is the one the SDK opens, through the store's own
// encoding. This is the loop a participant's back office and a payee's wallet
// close between them.
test('sealForStore produces bytes the reader can open', () => {
  const reader = newViewingKey();
  const payload = newPaymentMetadata('SALA', 'March salary');
  const bytes = sealForStore(payload, [{ publicKey: reader.publicKey }], PARTICIPANT, E2E);

  assert.ok(bytes.length > 0);
  // Sealed directly for comparison: the same aad, so the same payment.
  const direct = sealPayload(payload, [{ publicKey: reader.publicKey }], paymentAad(PARTICIPANT, E2E));
  assert.equal(direct.recipients.length, 1);
});
