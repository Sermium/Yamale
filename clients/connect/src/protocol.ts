/**
 * The wire protocol between an application and the Yamale wallet.
 *
 * There is no extension and no injected global. The wallet is a **web page on
 * its own origin**, and applications talk to it through `postMessage`. That
 * choice is the whole security model: the browser guarantees that a page on
 * origin A cannot read storage belonging to origin B, so the recovery phrase
 * stays inside the wallet's origin and an application only ever receives what
 * it asked for and the user approved.
 *
 * An extension would be stronger still, but it has to be installed. This works
 * in any browser, today, including on the phone somebody is testing from.
 *
 * Two rules hold everywhere below:
 *
 * 1. **Every message is origin-checked on both ends.** The wallet records which
 *    origin opened it and answers only that one; the application ignores any
 *    message that did not come from the wallet's origin. Skipping either check
 *    turns this into a signing oracle for whatever page is open in another tab.
 *
 * 2. **Binary crosses the boundary as base64, never as a typed array.** Structured
 *    clone would carry a `Uint8Array` intact, but sign documents also hold 64-bit
 *    integers, and those arrive as `Long`, `bigint` or `number` depending on who
 *    built them. Encoding everything explicitly means one representation rather
 *    than three that mostly work.
 */

/** Bumped only for a breaking change; the wallet refuses a version it predates. */
export const PROTOCOL_VERSION = 1;

export type RequestKind = 'connect' | 'accounts' | 'signDirect';

export interface Envelope<T = unknown> {
  /** Namespaced so a page using several postMessage protocols can tell them apart. */
  channel: 'yamale.connect';
  version: number;
  /** Correlates a response with its request; a reply with an unknown id is dropped. */
  id: string;
  payload: T;
}

export interface ConnectRequest {
  kind: 'connect';
  chainId: string;
  /** Shown to the user so they know who is asking. Advisory — the wallet trusts
   *  `event.origin`, never this. */
  appName: string;
}

export interface AccountsRequest {
  kind: 'accounts';
  chainId: string;
}

export interface SignDirectRequest {
  kind: 'signDirect';
  chainId: string;
  signerAddress: string;
  /** base64 of the protobuf TxBody */
  bodyBytes: string;
  /** base64 of the protobuf AuthInfo */
  authInfoBytes: string;
  /** decimal string; account numbers exceed Number.MAX_SAFE_INTEGER in principle */
  accountNumber: string;
}

export type AnyRequest = ConnectRequest | AccountsRequest | SignDirectRequest;

export interface AccountInfo {
  address: string;
  /** base64 of the compressed secp256k1 public key */
  pubkey: string;
  algo: 'secp256k1';
}

export interface ConnectResult {
  kind: 'connect';
  accounts: AccountInfo[];
}

export interface AccountsResult {
  kind: 'accounts';
  accounts: AccountInfo[];
}

export interface SignDirectResult {
  kind: 'signDirect';
  signature: string;
  /** Echoed back because the wallet is permitted to have changed them — a fee
   *  the user edited, for instance. The application must broadcast what was
   *  signed, not what it sent. */
  bodyBytes: string;
  authInfoBytes: string;
}

export interface ErrorResult {
  kind: 'error';
  /** `rejected` when the user said no, which callers should treat as ordinary
   *  rather than as a failure worth reporting as broken. */
  code: 'rejected' | 'locked' | 'unsupported' | 'wrong_chain' | 'internal';
  message: string;
}

export type AnyResult = ConnectResult | AccountsResult | SignDirectResult | ErrorResult;

export function envelope<T>(id: string, payload: T): Envelope<T> {
  return { channel: 'yamale.connect', version: PROTOCOL_VERSION, id, payload };
}

export function isEnvelope(value: unknown): value is Envelope {
  return (
    typeof value === 'object' &&
    value !== null &&
    (value as Envelope).channel === 'yamale.connect' &&
    typeof (value as Envelope).id === 'string'
  );
}

export function toBase64(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

export function fromBase64(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
  return bytes;
}
