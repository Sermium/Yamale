import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import { x25519 } from '@noble/curves/ed25519.js';

import {
  ENVELOPE_VERSION,
  EnvelopeMalformedError,
  EnvelopeUnreadableError,
  decodeEnvelope,
  encodeEnvelope,
  keyId,
  openPayload,
  paymentAad,
  sealPayload,
} from './envelope.ts';
import { metadataHash, newPaymentMetadata, verifyMetadata } from './metadata.ts';

/**
 * The one file both language implementations read.
 *
 * The Go suite reads exactly this, from x/paymsg/types. That is the whole point
 * of the file: this test used to reconstruct the expected bytes by hand while
 * the Go side pinned a constant, so the two agreed by construction and neither
 * could ever make the other fail. Now an implementation that moves goes red
 * against a fixture it cannot quietly rewrite.
 */
const vectors = JSON.parse(
  readFileSync(fileURLToPath(new URL('../../../testdata/vectors/confidentiality.json', import.meta.url)), 'utf8'),
) as {
  payment_metadata: Array<{
    name: string;
    salt_hex: string;
    purpose_code: string;
    remittance_information: string;
    wire_hex: string;
    hash_hex: string;
  }>;
  payload_envelope: Array<{
    name: string;
    instructing_participant: string;
    end_to_end_id: string;
    aad_hex: string;
    envelope_hex: string;
    salt_hex: string;
    purpose_code: string;
    remittance_information: string;
    hash_hex: string;
    readers: Array<{ role: string; private_key_hex: string; public_key_hex: string; key_id_hex: string }>;
    non_readers: Array<{ role: string; private_key_hex: string; public_key_hex: string; key_id_hex: string }>;
  }>;
};

function fromHex(hex: string): Uint8Array {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i += 1) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

function toHex(bytes: Uint8Array): string {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

function newKey(): { priv: Uint8Array; pub: Uint8Array } {
  const priv = x25519.utils.randomSecretKey();
  return { priv, pub: x25519.getPublicKey(priv) };
}

// The vectors have to actually contain something, or every case below passes by
// iterating over nothing — which is the vacuous green the shared file exists to
// eliminate.
test('the shared vectors are present and non-empty', () => {
  assert.ok(vectors.payment_metadata.length > 0, 'no metadata vectors');
  assert.ok(vectors.payload_envelope.length > 0, 'no envelope vectors');
});

// The hash the browser computes has to be the hash the chain recorded, and the
// encoding underneath it has to be the same encoding. The wire bytes are
// asserted as well as the digest: a digest alone says the two disagree, the
// encoding says where.
test('payment metadata matches the shared vectors', async () => {
  for (const v of vectors.payment_metadata) {
    const payload = {
      salt: fromHex(v.salt_hex),
      purposeCode: v.purpose_code,
      remittanceInformation: v.remittance_information,
    };
    assert.equal(toHex(await metadataHash(payload)), v.hash_hex, v.name);
    assert.equal(await verifyMetadata(payload, fromHex(v.hash_hex)), true, v.name);
  }
});

// The envelope this SDK reads must be the envelope the chain's Go writes. Both
// suites open the same recorded bytes with the same recorded keys, so a change
// to the HKDF label, the info ordering, the associated data framing or the
// padding shows up here rather than as a payment nobody can read.
test('the recorded envelopes open for every recorded reader', async () => {
  for (const v of vectors.payload_envelope) {
    const envelope = decodeEnvelope(fromHex(v.envelope_hex));
    assert.equal(envelope.version, ENVELOPE_VERSION, v.name);

    // Rebuilt from the payment's identity rather than read from the file, so
    // the vector pins paymentAad's framing too.
    const aad = paymentAad(v.instructing_participant, v.end_to_end_id);
    assert.equal(toHex(aad), v.aad_hex, `${v.name}: associated data framing has changed`);

    assert.ok(v.readers.length > 0);
    for (const reader of v.readers) {
      const priv = fromHex(reader.private_key_hex);
      assert.equal(toHex(x25519.getPublicKey(priv)), reader.public_key_hex);
      assert.equal(toHex(keyId(x25519.getPublicKey(priv))), reader.key_id_hex);

      const got = openPayload(envelope, priv, aad);
      assert.equal(got.purposeCode, v.purpose_code, `the ${reader.role} decrypted the wrong purpose code`);
      assert.equal(got.remittanceInformation, v.remittance_information, `the ${reader.role} decrypted the wrong remittance line`);
      assert.equal(toHex(got.salt), v.salt_hex);

      // And what came out is what the chain recorded.
      assert.equal(await verifyMetadata(got, fromHex(v.hash_hex)), true);
    }

    assert.ok(v.non_readers.length > 0);
    for (const stranger of v.non_readers) {
      assert.throws(
        () => openPayload(envelope, fromHex(stranger.private_key_hex), aad),
        EnvelopeUnreadableError,
        `the ${stranger.role} opened an envelope it was never sealed to`,
      );
    }
  }
});

// The three parties the design names all read the same payload, and a fourth
// does not. Sealed here rather than read from the file, so the writing half is
// exercised too.
test('every entitled party opens and a stranger does not', async () => {
  const payer = newKey();
  const payee = newKey();
  const regulator = newKey();
  const stranger = newKey();

  const payload = newPaymentMetadata('SALA', 'March salary, employee 4417');
  const recorded = await metadataHash(payload);
  const aad = paymentAad('yml1bankone', 'E2E-1');

  const envelope = sealPayload(
    payload,
    [{ publicKey: payer.pub }, { publicKey: payee.pub }, { publicKey: regulator.pub }],
    aad,
  );

  for (const [role, key] of [['payer', payer], ['payee', payee], ['regulator', regulator]] as const) {
    const got = openPayload(envelope, key.priv, aad);
    assert.equal(got.remittanceInformation, 'March salary, employee 4417', role);
    assert.equal(await verifyMetadata(got, recorded), true, role);
  }

  assert.throws(() => openPayload(envelope, stranger.priv, aad), EnvelopeUnreadableError);
});

// A store that edited the remittance line is caught by the cipher, not only by
// the on-chain hash. The hash is the backstop; the AEAD tag fires whether or not
// the reader remembered to look the payment up.
test('a tampered ciphertext does not open', () => {
  const reader = newKey();
  const aad = paymentAad('yml1bankone', 'E2E-3');
  const envelope = sealPayload(newPaymentMetadata('SALA', 'March salary'), [{ publicKey: reader.pub }], aad);

  envelope.ciphertext[Math.floor(envelope.ciphertext.length / 2)] ^= 0x01;
  assert.throws(() => openPayload(envelope, reader.priv, aad), EnvelopeMalformedError);
});

// The associated data binds an envelope to one payment, so a store cannot serve
// payment A's envelope in answer to a request for payment B.
test('an envelope does not open under another payment identity', () => {
  const reader = newKey();
  const right = paymentAad('yml1bankone', 'E2E-A');
  const envelope = sealPayload(newPaymentMetadata('SALA', 'March salary'), [{ publicKey: reader.pub }], right);

  // The positive case first. Without it this test passes when the binding is
  // removed entirely: every open fails, including the correct one, and a test
  // that only asserts failure cannot tell "correctly refused" from "nothing
  // works".
  assert.equal(openPayload(envelope, reader.priv, right).remittanceInformation, 'March salary');

  assert.throws(
    () => openPayload(envelope, reader.priv, paymentAad('yml1bankone', 'E2E-B')),
    EnvelopeMalformedError,
  );
  // The same reference under a different instructing participant, which is the
  // collision the payment record's composite key exists for.
  assert.throws(
    () => openPayload(envelope, reader.priv, paymentAad('yml1banktwo', 'E2E-A')),
    EnvelopeMalformedError,
  );
});

// Rotation is forward-only: the key rotated away from still opens what was
// sealed to it, and the new key does not. The alternative failure is invisible
// — an operator rotates, and payment detail from before silently stops opening.
test('a rotated key still opens the envelopes sealed to it', () => {
  const oldKey = newKey();
  const newer = newKey();
  const aad = paymentAad('yml1bankone', 'E2E-ROT');

  const before = sealPayload(newPaymentMetadata('SALA', 'before the rotation'), [{ publicKey: oldKey.pub }], aad);
  assert.equal(openPayload(before, oldKey.priv, aad).remittanceInformation, 'before the rotation');
  assert.throws(() => openPayload(before, newer.priv, aad), EnvelopeUnreadableError);

  const after = sealPayload(newPaymentMetadata('SALA', 'after the rotation'), [{ publicKey: newer.pub }], aad);
  assert.equal(openPayload(after, newer.priv, aad).remittanceInformation, 'after the rotation');
  assert.throws(() => openPayload(after, oldKey.priv, aad), EnvelopeUnreadableError);
});

// An envelope with no recipients encrypts, stores and serves perfectly and is
// readable by nobody, so it is refused where it is built.
test('sealing refuses an envelope nobody can read', () => {
  assert.throws(
    () => sealPayload(newPaymentMetadata('SALA', 'March salary'), [], paymentAad('yml1bankone', 'E2E-4')),
    /readable by nobody/,
  );
});

// A viewing key that is not a viewing key is refused at the moment of sealing,
// rather than becoming an envelope that looks addressed to somebody and opens
// for nobody.
test('sealing refuses a malformed recipient key', () => {
  const payload = newPaymentMetadata('SALA', 'March salary');
  const aad = paymentAad('yml1bankone', 'E2E-5');

  assert.throws(() => sealPayload(payload, [{ publicKey: new Uint8Array(31) }], aad));
  // Thirty-two zero bytes is the case that matters most: it is what an
  // uninitialised buffer looks like, and it is a low-order point whose
  // agreement every holder of the same value can reproduce.
  assert.throws(() => sealPayload(payload, [{ publicKey: new Uint8Array(32) }], aad));
});

// The ciphertext length must not track the length of the remittance line, or
// anyone who can see a response size can tell a reference from a name and
// address.
test('ciphertext length does not leak the remittance length', () => {
  const reader = newKey();
  const aad = paymentAad('yml1bankone', 'E2E-6');
  const lengths = new Set<number>();

  for (const remittance of ['', 'x', 'invoice 1', 'Mrs Adaeze Okonkwo, 14 Marina Road, Lagos, invoice 88213']) {
    const envelope = sealPayload(newPaymentMetadata('SALA', remittance), [{ publicKey: reader.pub }], aad);
    lengths.add(envelope.ciphertext.length);
  }
  assert.equal(lengths.size, 1, 'payloads of very different lengths produced ciphertexts of different sizes');
});

// Two sealings of one payload must not produce the same bytes, or an observer
// learns that two payments carry identical detail without ever reading it.
test('sealing twice produces different bytes', () => {
  const reader = newKey();
  const payload = newPaymentMetadata('SALA', 'March salary');
  const aad = paymentAad('yml1bankone', 'E2E-7');

  const first = sealPayload(payload, [{ publicKey: reader.pub }], aad);
  const second = sealPayload(payload, [{ publicKey: reader.pub }], aad);

  assert.notDeepEqual(first.ciphertext, second.ciphertext);
  assert.notDeepEqual(first.nonce, second.nonce);
  assert.notDeepEqual(first.recipients[0]!.ephemeralPublicKey, second.recipients[0]!.ephemeralPublicKey);
});

// The content key must be fresh per envelope, and differing ciphertexts do not
// prove that: a fixed content key with a fresh nonce still produces different
// bytes every time. What a shared content key would mean is that one recovered
// key opens every envelope ever sealed, so that is what is tested — a recipient
// block lifted from one envelope must not open another's body.
test('a block from one envelope does not open another', () => {
  const reader = newKey();
  const aad = paymentAad('yml1bankone', 'E2E-SHARED');

  const envA = sealPayload(newPaymentMetadata('SALA', 'first payment'), [{ publicKey: reader.pub }], aad);
  const envB = sealPayload(newPaymentMetadata('SALA', 'second payment'), [{ publicKey: reader.pub }], aad);

  const spliced = { ...envB, recipients: envA.recipients };
  assert.throws(
    () => openPayload(spliced, reader.priv, aad),
    EnvelopeMalformedError,
    "a block from one envelope opened another's body, so the content key is not fresh per envelope",
  );
});

// keyId is a hint and eight bytes is not a commitment, so a reader whose
// fingerprint matches no block must still try the rest.
test('a reader opens a block whose hint is wrong', () => {
  const reader = newKey();
  const aad = paymentAad('yml1bankone', 'E2E-8');
  const envelope = sealPayload(newPaymentMetadata('SALA', 'March salary'), [{ publicKey: reader.pub }], aad);

  envelope.recipients[0]!.keyId = new Uint8Array([9, 9, 9, 9, 9, 9, 9, 9]);
  assert.equal(openPayload(envelope, reader.priv, aad).remittanceInformation, 'March salary');
});

// An envelope survives the round trip through its stored form, which is the
// only form the payee ever sees.
test('an envelope round-trips through its encoded form', () => {
  const reader = newKey();
  const aad = paymentAad('yml1bankone', 'E2E-9');
  const envelope = sealPayload(newPaymentMetadata('SUPP', 'invoice 88213'), [{ publicKey: reader.pub }], aad);

  const reopened = decodeEnvelope(encodeEnvelope(envelope));
  assert.equal(openPayload(reopened, reader.priv, aad).remittanceInformation, 'invoice 88213');

  assert.throws(() => decodeEnvelope(new Uint8Array([0xff, 0xff, 0xff, 0xff])), EnvelopeMalformedError);
});
