/**
 * The encrypted form of a payment's ISO 20022 detail.
 *
 * This is the browser half of `x/paymsg/types/envelope.go`, and the two must
 * agree byte for byte: the payer seals in a wallet, the payee opens in another
 * wallet, and a regulator opens in Go tooling. They are held to that by
 * `testdata/vectors/confidentiality.json`, which both suites read — see
 * envelope.test.ts.
 *
 * The construction is the standard multi-recipient KEM/DEM composition. A fresh
 * content key encrypts the payload once under ChaCha20-Poly1305, and one X25519
 * agreement per recipient wraps a copy of that content key. It is what age does
 * and what RFC 9180 formalises as DHKEM(X25519, HKDF-SHA256). Nothing here is
 * novel, deliberately: the payload is the detail of somebody's payment, not the
 * place to find out whether a new scheme holds.
 *
 * @noble rather than WebCrypto because WebCrypto has no ChaCha20-Poly1305 at
 * all, and X25519 only in very recent browsers. A construction that silently
 * fell back to something else per browser would produce envelopes some readers
 * could not open.
 */
import { chacha20poly1305 } from '@noble/ciphers/chacha.js';
import { x25519 } from '@noble/curves/ed25519.js';
import { hkdf } from '@noble/hashes/hkdf.js';
import { sha256 } from '@noble/hashes/sha2.js';

import { PayloadEnvelope, RecipientBlock } from './generated/blockchain/paymsg/v1/payload_envelope.ts';
import { PaymentMetadata } from './generated/blockchain/paymsg/v1/payment_metadata.ts';
import { METADATA_SALT_BYTES, type PaymentMetadataPayload } from './metadata.ts';

/** The envelope format this build writes and reads. */
export const ENVELOPE_VERSION = 1;

/** X25519 public and private halves are both 32 bytes. */
export const VIEWING_KEY_BYTES = 32;

/** ChaCha20-Poly1305 key size, and therefore the content key size. */
const CONTENT_KEY_BYTES = 32;

/** ChaCha20-Poly1305 nonce size. */
const NONCE_BYTES = 12;

/**
 * How much of a recipient's key fingerprint travels in a block.
 *
 * Truncated so the block does not simply publish the recipient's public key to
 * whoever fetches the envelope. It is a lookup hint and never a commitment: two
 * keys can collide, so a reader whose fingerprint matches nothing must still
 * try every block.
 */
const KEY_ID_BYTES = 8;

/**
 * The plaintext is rounded up to a multiple of this before encryption.
 *
 * Without it the ciphertext length says how long the remittance line is, and
 * anyone who can see a response size can tell a four-character reference from a
 * name and address. That is a meaningful part of what moving the payload
 * off-chain was for, and it costs at most 256 bytes to close.
 */
const PADDING_BLOCK = 256;

/** Bounds what `openPayload` will allocate from a length prefix it was handed. */
const MAX_PLAINTEXT_BYTES = 64 * 1024;

/**
 * Separates this construction's key derivation from every other use of X25519
 * on this chain.
 *
 * A shared secret is just 32 bytes; what stops a wrapping key being usable
 * somewhere else is that this label went into the derivation. Its exact bytes
 * are part of the wire format — changing it makes every stored envelope
 * unreadable — which is why it is pinned by the shared vectors.
 */
const ENVELOPE_DOMAIN = 'yamale/paymsg/payload-envelope/v1';

/** Thrown when no block in an envelope opens with the key provided. */
export class EnvelopeUnreadableError extends Error {
  name = 'EnvelopeUnreadableError';
}

/**
 * Thrown for bytes that are not a well-formed envelope of a version this build
 * implements, and for one whose body failed to authenticate.
 *
 * Distinct from the above because they mean opposite things to the person
 * holding it: one says "this is not addressed to you", the other says "this is
 * not intact". A client that conflated them would tell a payee their key was
 * wrong when the store had handed them a truncated file.
 */
export class EnvelopeMalformedError extends Error {
  name = 'EnvelopeMalformedError';
}

const encoder = new TextEncoder();

function concat(...parts: Uint8Array[]): Uint8Array {
  let total = 0;
  for (const p of parts) total += p.length;
  const out = new Uint8Array(total);
  let at = 0;
  for (const p of parts) {
    out.set(p, at);
    at += p.length;
  }
  return out;
}

/**
 * The first 8 bytes of SHA-256 over a recipient's public key.
 *
 * Computed in one place because a writer emits it and a reader matches on it.
 * Two implementations that disagreed would produce envelopes whose blocks
 * nobody finds, which degrades to trying every block — so it would fail
 * silently rather than loudly.
 */
export function keyId(publicKey: Uint8Array): Uint8Array {
  return sha256(publicKey).slice(0, KEY_ID_BYTES);
}

/**
 * The associated data every envelope for a payment is bound to.
 *
 * It binds the ciphertext to one payment, so a store cannot serve the payload
 * of one payment under the key of another — a substitution that would otherwise
 * decrypt cleanly and fail only against the on-chain hash, if the reader
 * remembered to check it.
 */
export function paymentAad(instructingParticipant: string, endToEndId: string): Uint8Array {
  return concat(
    encoder.encode(ENVELOPE_DOMAIN),
    new Uint8Array([0]),
    encoder.encode(instructingParticipant),
    new Uint8Array([0]),
    encoder.encode(endToEndId),
  );
}

/**
 * Turns a raw X25519 secret into a wrapping key for one block.
 *
 * Both public keys go into the derivation, in a fixed order, alongside the
 * domain label. Hashing the shared secret alone would derive the same key for
 * two pairings that happened to agree on it; binding the transcript is what
 * makes each block's key specific to the ephemeral that produced it and the
 * recipient it was meant for.
 *
 * No salt, matching the Go side: the shared secret is already high-entropy and
 * the transcript rides in `info`, which is what HKDF's info parameter is for. A
 * random salt would have to travel in the block for the reader to reproduce it.
 */
function deriveWrappingKey(
  shared: Uint8Array,
  ephemeralPublic: Uint8Array,
  recipientPublic: Uint8Array,
): Uint8Array {
  const info = concat(
    encoder.encode(ENVELOPE_DOMAIN),
    new Uint8Array([0]),
    ephemeralPublic,
    recipientPublic,
  );
  return hkdf(sha256, shared, undefined, info, CONTENT_KEY_BYTES);
}

/**
 * Length-prefixed padding, rather than a trailing marker.
 *
 * The payload is protobuf and protobuf has no self-delimiting end, so a reader
 * that stripped trailing zeroes would also strip the last byte of any field
 * that legitimately ended in one — corrupting the payload for some inputs only.
 */
function pad(plaintext: Uint8Array): Uint8Array {
  if (plaintext.length > MAX_PLAINTEXT_BYTES) {
    throw new Error(`payload is ${plaintext.length} bytes, above the ${MAX_PLAINTEXT_BYTES}-byte limit`);
  }
  let total = plaintext.length + 4;
  const remainder = total % PADDING_BLOCK;
  if (remainder !== 0) total += PADDING_BLOCK - remainder;

  const padded = new Uint8Array(total);
  padded[0] = (plaintext.length >>> 24) & 0xff;
  padded[1] = (plaintext.length >>> 16) & 0xff;
  padded[2] = (plaintext.length >>> 8) & 0xff;
  padded[3] = plaintext.length & 0xff;
  padded.set(plaintext, 4);
  return padded;
}

function unpad(padded: Uint8Array): Uint8Array {
  if (padded.length < 4) {
    throw new EnvelopeMalformedError(`padded plaintext is ${padded.length} bytes`);
  }
  const n = ((padded[0]! << 24) | (padded[1]! << 16) | (padded[2]! << 8) | padded[3]!) >>> 0;
  // Checked against the buffer and the limit before it is used as a bound. The
  // AEAD tag already proves these bytes are the ones sealed, but a reader that
  // trusted the prefix on that basis would throw a range error on an envelope
  // its own sealer produced wrongly — a worse way to find out.
  if (n > MAX_PLAINTEXT_BYTES || 4 + n > padded.length) {
    throw new EnvelopeMalformedError(`padded plaintext claims a ${n}-byte payload`);
  }
  return padded.slice(4, 4 + n);
}

/**
 * Seals a copy of a 32-byte secret to one viewing key.
 *
 * The ephemeral key is fresh per block rather than shared across the envelope.
 * It costs 32 bytes each and buys two things: the blocks are unlinkable to each
 * other, and a party who later holds the content key can add a recipient — an
 * auditor appointed after the fact, a rotated regulator key — without the
 * original ephemeral secret, which nobody kept.
 */
function wrap(secret: Uint8Array, recipientPublic: Uint8Array): RecipientBlock {
  if (recipientPublic.length !== VIEWING_KEY_BYTES) {
    throw new Error(`viewing key must be ${VIEWING_KEY_BYTES} bytes, got ${recipientPublic.length}`);
  }
  const ephemeralPrivate = x25519.utils.randomSecretKey();
  const ephemeralPublic = x25519.getPublicKey(ephemeralPrivate);
  // Throws on a low-order point rather than returning an all-zero secret, which
  // is a "shared" secret every holder of the same degenerate value can also
  // reproduce.
  const shared = x25519.getSharedSecret(ephemeralPrivate, recipientPublic);

  const id = keyId(recipientPublic);
  const wrappingKey = deriveWrappingKey(shared, ephemeralPublic, recipientPublic);
  // A zero nonce is correct here and only here: the wrapping key comes from a
  // freshly generated ephemeral secret, so it encrypts exactly once and can
  // never repeat. A random nonce would add 12 bytes per block and protect
  // against nothing.
  const aead = chacha20poly1305(wrappingKey, new Uint8Array(NONCE_BYTES), id);
  return { keyId: id, ephemeralPublicKey: ephemeralPublic, wrappedKey: aead.encrypt(secret) };
}

/**
 * Recovers a secret from one block, or throws.
 *
 * The wrapping key is derived against the reader's own public key, not against
 * `block.keyId`. Taking the identity from the block would let a writer name
 * somebody else's fingerprint on a block only this reader can open, and the tag
 * would still verify — so the block's claim about who it is for could disagree
 * with who can actually read it.
 */
function unwrap(block: RecipientBlock, privateKey: Uint8Array, self: Uint8Array): Uint8Array {
  const shared = x25519.getSharedSecret(privateKey, block.ephemeralPublicKey);
  const wrappingKey = deriveWrappingKey(shared, block.ephemeralPublicKey, self);
  const aead = chacha20poly1305(wrappingKey, new Uint8Array(NONCE_BYTES), keyId(self));
  const secret = aead.decrypt(block.wrappedKey);
  if (secret.length !== CONTENT_KEY_BYTES) throw new Error('wrapped key is not a content key');
  return secret;
}

/** One party entitled to read a payload, as published in x/alias. */
export interface EnvelopeRecipient {
  publicKey: Uint8Array;
}

/**
 * Encrypts a payment payload to every entitled recipient.
 *
 * The payload's salt stays in the plaintext and never leaves it. The salt is
 * what makes the on-chain hash uninvertible, so it is also what makes deleting
 * the payload an erasure rather than a gesture: a four-character purpose code
 * hashed without one is a lookup table, and destroying a payload whose preimage
 * anybody can enumerate erases nothing at all.
 */
export function sealPayload(
  payload: PaymentMetadataPayload,
  recipients: EnvelopeRecipient[],
  aad: Uint8Array,
): PayloadEnvelope {
  if (recipients.length === 0) {
    // Refused rather than producing an envelope with no blocks. That envelope
    // encrypts, stores and serves perfectly and is readable by nobody — a
    // payment whose detail is gone from the moment it was sent, discovered
    // whenever somebody first tries to reconcile it.
    throw new Error('an envelope with no recipients is readable by nobody');
  }
  if (payload.salt.length !== METADATA_SALT_BYTES) {
    throw new Error(`payload salt must be ${METADATA_SALT_BYTES} bytes, got ${payload.salt.length}`);
  }

  const plaintext = PaymentMetadata.encode({
    salt: payload.salt,
    purposeCode: payload.purposeCode,
    remittanceInformation: payload.remittanceInformation,
  }).finish();
  // Copied out of protobufjs's pooled buffer before it is encrypted, or the
  // ciphertext would cover whatever else the pool holds.
  const padded = pad(Uint8Array.from(plaintext));

  const contentKey = x25519.utils.randomSecretKey();
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_BYTES));
  const ciphertext = chacha20poly1305(contentKey, nonce, aad).encrypt(padded);

  return {
    version: ENVELOPE_VERSION,
    recipients: recipients.map((r) => wrap(contentKey, r.publicKey)),
    nonce,
    ciphertext,
  };
}

/**
 * Decrypts an envelope with one recipient's private viewing key.
 *
 * `aad` must be the same payment identity the envelope was sealed under, so a
 * caller that fetched the wrong payment's envelope is told here rather than
 * discovering it against the on-chain hash later, or not at all.
 */
export function openPayload(
  envelope: PayloadEnvelope,
  privateKey: Uint8Array,
  aad: Uint8Array,
): PaymentMetadataPayload {
  if (envelope.version !== ENVELOPE_VERSION) {
    throw new EnvelopeMalformedError(`envelope version ${envelope.version}`);
  }
  if (envelope.nonce.length !== NONCE_BYTES) {
    throw new EnvelopeMalformedError(`nonce is ${envelope.nonce.length} bytes`);
  }
  if (privateKey.length !== VIEWING_KEY_BYTES) {
    throw new EnvelopeUnreadableError('that is not an X25519 private key');
  }

  const self = x25519.getPublicKey(privateKey);
  const want = keyId(self);

  const tryBlocks = (onlyMatching: boolean): Uint8Array | undefined => {
    for (const block of envelope.recipients) {
      if (onlyMatching && !bytesEqual(block.keyId, want)) continue;
      try {
        return unwrap(block, privateKey, self);
      } catch {
        // A block that does not open is a block for somebody else. Trying the
        // next one is the normal path, not an error worth surfacing.
      }
    }
    return undefined;
  };

  // Matching blocks first, then all of them. keyId is a hint and eight bytes is
  // not a commitment, so a reader that stopped at "no block matches my
  // fingerprint" would refuse an envelope it can actually open whenever two
  // keys collided — a failure that would be blamed on the store.
  const contentKey = tryBlocks(true) ?? tryBlocks(false);
  if (contentKey === undefined) {
    throw new EnvelopeUnreadableError('no recipient block in this envelope opens with the key provided');
  }

  let padded: Uint8Array;
  try {
    padded = chacha20poly1305(contentKey, envelope.nonce, aad).decrypt(envelope.ciphertext);
  } catch {
    // The content key opened a block, so the reader is entitled; the body still
    // failed. That is a tampered ciphertext or the wrong payment's associated
    // data, never a key problem — and saying so is the difference between an
    // afternoon spent on a store bug and one spent on a key.
    throw new EnvelopeMalformedError(
      'the content key was recovered but the body did not authenticate, so the ciphertext or the payment it names has been altered',
    );
  }

  const decoded = PaymentMetadata.decode(unpad(padded));
  return {
    salt: Uint8Array.from(decoded.salt),
    purposeCode: decoded.purposeCode,
    remittanceInformation: decoded.remittanceInformation,
  };
}

/**
 * Seals a 32-byte secret to one viewing key, producing a single block.
 *
 * The counterpart of openRecipientBlock, and the primitive the payload store's
 * challenge-response is built on: the store seals a nonce to the caller's
 * registered public half, and answering it proves possession of the private
 * one. Exported as a pair because half a primitive invites somebody to write
 * the other half again, and the second one is the one nobody reviews.
 *
 * This is `types.WrapTo` in Go, byte for byte.
 */
export function sealToViewingKey(secret: Uint8Array, recipientPublic: Uint8Array): RecipientBlock {
  if (secret.length !== CONTENT_KEY_BYTES) {
    throw new Error(`secret must be ${CONTENT_KEY_BYTES} bytes, got ${secret.length}`);
  }
  return wrap(secret, recipientPublic);
}

/**
 * Opens a single recipient block and returns the 32-byte secret inside it.
 *
 * Exported for the payload store's challenge-response, which seals a nonce to a
 * viewing key using exactly this block format. One wrapping primitive in this
 * SDK rather than two, because the second would be the one nobody reviewed.
 */
export function openRecipientBlock(block: RecipientBlock, privateKey: Uint8Array): Uint8Array {
  if (privateKey.length !== VIEWING_KEY_BYTES) {
    throw new EnvelopeUnreadableError('that is not an X25519 private key');
  }
  try {
    return unwrap(block, privateKey, x25519.getPublicKey(privateKey));
  } catch {
    throw new EnvelopeUnreadableError('this block does not open with the key provided');
  }
}

/** Encodes an envelope for storage or transport. */
export function encodeEnvelope(envelope: PayloadEnvelope): Uint8Array {
  return Uint8Array.from(PayloadEnvelope.encode(envelope).finish());
}

/** Decodes stored bytes, refusing anything that is not an envelope. */
export function decodeEnvelope(bytes: Uint8Array): PayloadEnvelope {
  try {
    return PayloadEnvelope.decode(bytes);
  } catch (caught) {
    throw new EnvelopeMalformedError(`these bytes are not a PayloadEnvelope: ${String(caught)}`);
  }
}

function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i += 1) diff |= a[i]! ^ b[i]!;
  return diff === 0;
}
