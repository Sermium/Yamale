/**
 * Payment metadata: the ISO 20022 detail that no longer goes on the chain.
 *
 * `purpose_code` and `remittance_information` are free-text fields, and in
 * practice they are where an operator puts a customer's name. Written to this
 * chain they were public, permanent and unerasable — the exposure under
 * Nigeria's NDPA, Ghana's DPA, POPIA and the GDPR. They now travel as a payload
 * held here, and the chain records only a hash of it.
 *
 * The hash is what makes the payload worth anything. Without it the payload is
 * a claim by whoever happens to be storing it, and a party in a dispute could
 * produce whichever version of the remittance line suited them. With it, a
 * payload either hashes to what the block says or it is not the payload.
 *
 * Two things this deliberately does not do, stated here because assuming
 * otherwise is how a pilot goes wrong:
 *
 * - **It does not encrypt.** A hash proves integrity and reveals nothing, but
 *   it also lets nobody read. The viewing keys that let the payer, the payee
 *   and the regulator decrypt are the next workstream; see
 *   docs/scope/confidentiality.md.
 * - **It does not deliver.** This store is `localStorage`, so it is one browser
 *   on one origin. The payer's device holds the payload the payer wrote; the
 *   payee's device does not. Until a shared encrypted store exists, the payload
 *   has to reach the other side by whatever channel already carries the
 *   remittance advice, and only the hash makes it checkable when it arrives.
 */

import { PaymentMetadata } from './generated/blockchain/paymsg/v1/payment_metadata.ts';

/** Length of the salt, matching MetadataSaltLength in x/paymsg/types. */
export const METADATA_SALT_BYTES = 32;

/** Length of the recorded hash, matching MetadataHashLength in x/paymsg/types. */
export const METADATA_HASH_BYTES = 32;

/** ISO 20022 limits, the same ones the chain applies to the plaintext fields. */
const MAX_PURPOSE_CODE = 4;
const MAX_REMITTANCE = 140;

export interface PaymentMetadataPayload {
  /** 32 random bytes, fresh for every payment. */
  salt: Uint8Array;
  purposeCode: string;
  remittanceInformation: string;
}

/**
 * Build a payload with a fresh salt.
 *
 * The salt is not decoration. A purpose code is four characters from a
 * published list, so the hash of an unsalted one is not a fingerprint but a
 * lookup table anybody can build in a second — and the ledger is public and
 * permanent, so they have unlimited time to try. It is regenerated per payment
 * rather than per account because a reused salt makes two payments carrying the
 * same detail hash identically, which tells an observer they match without ever
 * revealing what they say.
 */
export function newPaymentMetadata(
  purposeCode: string,
  remittanceInformation: string,
): PaymentMetadataPayload {
  if (purposeCode.length > MAX_PURPOSE_CODE) {
    throw new Error(`purposeCode must be at most ${MAX_PURPOSE_CODE} characters`);
  }
  if (remittanceInformation.length > MAX_REMITTANCE) {
    throw new Error(`remittanceInformation must be at most ${MAX_REMITTANCE} characters`);
  }

  const salt = new Uint8Array(METADATA_SALT_BYTES);
  crypto.getRandomValues(salt);
  return { salt, purposeCode, remittanceInformation };
}

/**
 * Hash a payload into the value that goes on-chain.
 *
 * The bytes hashed are the generated protobuf encoding, not a string this file
 * assembles. That is the only reason the chain and this package agree: a
 * hand-written serialiser here and a protobuf one in the keeper would produce
 * the same answer right up until somebody added a field, and a hash that
 * disagrees proves nothing while looking exactly like a hash that works.
 */
export async function metadataHash(payload: PaymentMetadataPayload): Promise<Uint8Array> {
  if (payload.salt.length !== METADATA_SALT_BYTES) {
    throw new Error(
      `salt must be ${METADATA_SALT_BYTES} bytes, got ${payload.salt.length}; an unsalted hash publishes the field it was meant to hide`,
    );
  }

  const encoded = PaymentMetadata.encode({
    salt: payload.salt,
    purposeCode: payload.purposeCode,
    remittanceInformation: payload.remittanceInformation,
  }).finish();

  // Copied into a standalone buffer rather than handed over as-is. protobufjs
  // writes into a pooled backing array, so the encoder's view can sit inside a
  // larger buffer shared with whatever was encoded before it — digesting that
  // would hash the neighbours too, and only sometimes.
  const bytes = new Uint8Array(encoded.length);
  bytes.set(encoded);

  const digest = await crypto.subtle.digest('SHA-256', bytes.buffer);
  return new Uint8Array(digest);
}

/**
 * Check a payload against the hash a payment recorded.
 *
 * Anyone who receives a payload from anywhere — the other participant, a
 * backup, this store after a browser upgrade — should run this before believing
 * it. The hash comes from the block; the payload does not.
 */
export async function verifyMetadata(
  payload: PaymentMetadataPayload,
  recorded: Uint8Array,
): Promise<boolean> {
  if (recorded.length !== METADATA_HASH_BYTES) return false;

  const computed = await metadataHash(payload);
  let differing = 0;
  for (let i = 0; i < METADATA_HASH_BYTES; i++) differing |= computed[i]! ^ recorded[i]!;
  return differing === 0;
}

// ------------------------------------------------------------------- storage

const STORAGE_KEY = 'yamale.paymsg.metadata.v1';

/**
 * Payloads are filed under the chain's own key for a payment — (instructing
 * participant, end-to-end id) — rather than a local id, so a record fetched
 * from the node leads straight to its payload without a second index to keep
 * in step.
 */
function storageKey(instructingParticipant: string, endToEndId: string): string {
  return `${instructingParticipant}/${endToEndId}`;
}

/** The salt is bytes and localStorage holds strings, so it is stored base64. */
interface StoredPayload {
  salt: string;
  purposeCode: string;
  remittanceInformation: string;
  savedAt: string;
}

type Store = Record<string, StoredPayload>;

function read(): Store {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Store) : {};
  } catch {
    // A corrupt store must not take the page down. Returning empty loses the
    // detail, not the payment: the chain still holds the hash, so a payload
    // recovered from anywhere else can still be proven to be the right one.
    return {};
  }
}

function write(store: Store): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(store));
  window.dispatchEvent(new CustomEvent('yamale:paymsg-metadata'));
}

function toBase64(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function fromBase64(encoded: string): Uint8Array {
  const binary = atob(encoded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

/** File a payload against the payment that recorded its hash. */
export function savePaymentMetadata(
  instructingParticipant: string,
  endToEndId: string,
  payload: PaymentMetadataPayload,
): void {
  const store = read();
  store[storageKey(instructingParticipant, endToEndId)] = {
    salt: toBase64(payload.salt),
    purposeCode: payload.purposeCode,
    remittanceInformation: payload.remittanceInformation,
    savedAt: new Date().toISOString(),
  };
  write(store);
}

/** Retrieve a payload. Verify it against the chain's hash before trusting it. */
export function loadPaymentMetadata(
  instructingParticipant: string,
  endToEndId: string,
): PaymentMetadataPayload | undefined {
  const stored = read()[storageKey(instructingParticipant, endToEndId)];
  if (!stored) return undefined;

  return {
    salt: fromBase64(stored.salt),
    purposeCode: stored.purposeCode,
    remittanceInformation: stored.remittanceInformation,
  };
}
