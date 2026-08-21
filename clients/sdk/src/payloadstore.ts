/**
 * Retrieval of a payment's off-chain detail, and the vocabulary for when there
 * is none.
 *
 * The shape that matters here is `PayloadResult`. Every way this can end —
 * decrypted, erased, never stored, no store registered, not entitled, host
 * unreachable — is a named outcome the caller has to handle, and none of them
 * is an empty payload. This repository has already shipped a screen that
 * rendered empty because a route 404'd, and the failure was invisible: the page
 * looked like a record with nothing in it rather than a record nobody could
 * read. A union with no "empty" member makes that particular mistake
 * unwriteable.
 *
 * Erasure is a first-class outcome rather than an error for the same reason.
 * Deleting a payload is the point of holding it off-chain — an append-only
 * ledger has no deletion path — so a payment whose detail has been destroyed is
 * a correct, expected state, and a client that reported it as a fault would
 * have an operator chasing a working system.
 */
import { x25519 } from '@noble/curves/ed25519.js';

import {
  decodeEnvelope,
  encodeEnvelope,
  openPayload,
  openRecipientBlock,
  paymentAad,
  sealPayload,
  type EnvelopeRecipient,
} from './envelope.ts';
import { verifyMetadata, type PaymentMetadataPayload } from './metadata.ts';

/** Why a payload is not being shown. */
export type PayloadUnavailableReason =
  /** The controller destroyed it, on request or on a retention schedule. */
  | 'erased'
  /** The store serves this participant but holds nothing for this payment. */
  | 'never-stored'
  /** The chain records no such payment, so nothing could legitimately be held. */
  | 'no-such-payment'
  /** The instructing participant has registered no store. */
  | 'no-store-registered'
  /** The chain does not record the caller as entitled to read this one. */
  | 'not-entitled'
  /** The caller could not prove possession of a registered viewing key. */
  | 'unproven'
  /** The store could not be reached, or answered with something unusable. */
  | 'unreachable'
  /**
   * The store returned bytes that did not decrypt, or that decrypted to a
   * payload disagreeing with the hash on the chain.
   *
   * Separate from 'unreachable' because it is an accusation rather than a
   * network condition: the store handed back something that is not the payload
   * this payment recorded.
   */
  | 'not-the-recorded-payload';

/**
 * The outcome of asking for a payment's detail.
 *
 * There is deliberately no member carrying an empty payload. "Available with
 * nothing in it" and "unavailable" are different statements about somebody's
 * money, and a type that could express the first by accident is how they get
 * confused.
 */
export type PayloadResult =
  | { readonly status: 'available'; readonly payload: PaymentMetadataPayload }
  | { readonly status: 'unavailable'; readonly reason: PayloadUnavailableReason; readonly detail?: string };

/**
 * What a person should be shown for an unavailable payload.
 *
 * Here rather than in each screen, because four clients writing their own
 * wording is four chances to render an erasure as a bug — and because the
 * distinction between "destroyed" and "we could not reach the server" is the
 * whole reason the store answers with reasons instead of a bare 404.
 */
export function describeUnavailable(reason: PayloadUnavailableReason): string {
  switch (reason) {
    case 'erased':
      return 'Detail unavailable — the payment detail was erased. The payment itself is unchanged and still verifies.';
    case 'never-stored':
      return 'Detail unavailable — no detail was stored for this payment.';
    case 'no-such-payment':
      return 'Detail unavailable — the chain records no payment with this reference.';
    case 'no-store-registered':
      return 'Detail unavailable — the instructing institution publishes no payload store.';
    case 'not-entitled':
      return 'Detail unavailable — you are not recorded as a party to this payment.';
    case 'unproven':
      return 'Detail unavailable — your viewing key could not be verified.';
    case 'unreachable':
      return 'Detail unavailable — the institution’s payload store could not be reached.';
    case 'not-the-recorded-payload':
      return 'Detail unavailable — the stored payload is not the one this payment recorded.';
  }
}

interface StoreResponse {
  status?: string;
  reason?: string;
  detail?: string;
  envelope?: string;
  challenge_id?: string;
  ephemeral_public_key?: string;
  wrapped_nonce?: string;
  key_id?: string;
}

function toBase64(bytes: Uint8Array): string {
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary);
}

function fromBase64(encoded: string): Uint8Array {
  const binary = atob(encoded);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) out[i] = binary.charCodeAt(i);
  return out;
}

/**
 * Maps the store's machine-readable reason onto this module's vocabulary.
 *
 * An unrecognised reason becomes 'unreachable' rather than being passed
 * through. A store speaking a dialect this build does not know is, from the
 * caller's point of view, a store it cannot use — and inventing a reason string
 * the UI has no wording for is how a screen ends up rendering a raw token.
 */
function reasonOf(body: StoreResponse): PayloadUnavailableReason {
  switch (body.reason) {
    case 'erased':
      return 'erased';
    case 'never_stored':
      return 'never-stored';
    case 'no_such_payment':
      return 'no-such-payment';
    case 'not_entitled':
      return 'not-entitled';
    case 'unproven':
      return 'unproven';
    default:
      return 'unreachable';
  }
}

export interface PayloadStoreOptions {
  /** The base URL registered on-chain by the instructing participant. */
  baseUrl: string;
  /** The account asking, which must match the viewing key below. */
  address: string;
  /** The private half of a viewing key this account has registered on-chain. */
  viewingKey: Uint8Array;
  fetchImpl?: typeof fetch;
}

/**
 * Reads one participant's payload store on behalf of one entitled account.
 *
 * It never sends the viewing key anywhere. The store issues a challenge sealed
 * to the account's registered public half; this class decrypts it locally and
 * returns the answer. That is the same credential that will decrypt the
 * payload, so authenticating costs nothing extra and grants nothing extra.
 */
export class PayloadStoreClient {
  private readonly baseUrl: string;
  private readonly address: string;
  private readonly viewingKey: Uint8Array;
  private readonly fetchImpl: typeof fetch;

  constructor(options: PayloadStoreOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, '');
    this.address = options.address;
    this.viewingKey = options.viewingKey;
    this.fetchImpl = options.fetchImpl ?? fetch;
  }

  /**
   * Builds the retrieval URL, matching `types.PayloadStoreEndpoint` in Go.
   *
   * Both ends escape the two path segments, because an end-to-end id is ISO
   * 20022 free text and may carry a slash — which would otherwise open a path
   * segment the store never meant to serve.
   */
  private endpoint(instructingParticipant: string, endToEndId: string): string {
    return `${this.baseUrl}/payloads/${encodeURIComponent(instructingParticipant)}/${encodeURIComponent(endToEndId)}`;
  }

  /** Answers a challenge, proving possession of the registered viewing key. */
  private async authorise(): Promise<string> {
    const issued = await this.fetchImpl(`${this.baseUrl}/challenge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ address: this.address }),
    });
    const body = (await issued.json()) as StoreResponse;
    if (!issued.ok || !body.challenge_id || !body.ephemeral_public_key || !body.wrapped_nonce || !body.key_id) {
      throw new Error(body.detail ?? 'the store would not issue a challenge');
    }

    // Decrypted here and nowhere else. The viewing key never leaves this
    // object, so proving who we are costs the store nothing it could replay and
    // tells it nothing it could not already look up on the chain.
    const nonce = openRecipientBlock(
      {
        keyId: fromBase64(body.key_id),
        ephemeralPublicKey: fromBase64(body.ephemeral_public_key),
        wrappedKey: fromBase64(body.wrapped_nonce),
      },
      this.viewingKey,
    );
    return `Yamale ${body.challenge_id}:${toBase64(nonce)}`;
  }

  /**
   * Fetches and decrypts one payment's detail.
   *
   * `recordedHash` is the hash from the chain's payment record. It is required
   * rather than optional, because verifying against it is the only thing that
   * gives the store any authority at all: without it the payload is a claim by
   * whoever is holding it, and a participant in a dispute could produce
   * whichever version of the remittance line suited them.
   */
  async fetchPayload(
    instructingParticipant: string,
    endToEndId: string,
    recordedHash: Uint8Array,
  ): Promise<PayloadResult> {
    let response: Response;
    let body: StoreResponse;
    try {
      response = await this.fetchImpl(this.endpoint(instructingParticipant, endToEndId), {
        headers: { Authorization: await this.authorise() },
      });
      body = (await response.json()) as StoreResponse;
    } catch (caught) {
      return { status: 'unavailable', reason: 'unreachable', detail: String(caught) };
    }

    if (!response.ok || body.status !== 'ok' || !body.envelope) {
      return { status: 'unavailable', reason: reasonOf(body), detail: body.detail };
    }

    try {
      const payload = openPayload(
        decodeEnvelope(fromBase64(body.envelope)),
        this.viewingKey,
        paymentAad(instructingParticipant, endToEndId),
      );
      // The chain's hash is the arbiter. A store that edited the remittance
      // line is caught here even if the ciphertext somehow authenticated, and a
      // payload that disagrees with the block is not this payment's payload
      // whatever else it is.
      if (!(await verifyMetadata(payload, recordedHash))) {
        return { status: 'unavailable', reason: 'not-the-recorded-payload' };
      }
      return { status: 'available', payload };
    } catch (caught) {
      return { status: 'unavailable', reason: 'not-the-recorded-payload', detail: String(caught) };
    }
  }
}

/**
 * Seals a payload for storage.
 *
 * Exposed so a participant's back office can build the envelope with the same
 * code the payee will open it with, which is the only way to be sure the two
 * agree.
 */
export function sealForStore(
  payload: PaymentMetadataPayload,
  recipients: EnvelopeRecipient[],
  instructingParticipant: string,
  endToEndId: string,
): Uint8Array {
  return encodeEnvelope(sealPayload(payload, recipients, paymentAad(instructingParticipant, endToEndId)));
}

/** The public half of a viewing key, for publishing with MsgRegisterViewingKey. */
export function viewingKeyPublic(privateKey: Uint8Array): Uint8Array {
  return x25519.getPublicKey(privateKey);
}

/**
 * Generates a viewing key pair.
 *
 * The private half never leaves the holder and is never sent to the chain: a
 * private key on an append-only ledger is a private key published to everyone
 * forever, and there is no erasure path that takes it back.
 */
export function newViewingKey(): { privateKey: Uint8Array; publicKey: Uint8Array } {
  const privateKey = x25519.utils.randomSecretKey();
  return { privateKey, publicKey: x25519.getPublicKey(privateKey) };
}
