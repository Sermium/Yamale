/**
 * Signing requests to traceability-api: the browser's half of
 * tools/traceabilityapi/auth.go, pinned by tools/traceabilityapi/testdata/vectors.json.
 *
 *   yamale-traceability/v1:<purpose>:<chain id>:<subject>:<issued, unix seconds>
 *
 * The subject binds the signature to the request's content: the file's
 * fingerprint, "<jeweler id>.<fingerprint>" for a profile, the signer's own
 * address for sponsorship.
 */

import { sha256 } from '@noble/hashes/sha2';

import { compressedPublicKey, signArbitrary, toBase64, toHex } from './jewel.ts';

export type TraceabilityPurpose = 'file' | 'profile' | 'sponsor' | 'application' | 'catalogue-import' | 'catalogue-read' | 'design-save' | 'design-confirm' | 'cardvault-read' | 'cardvault-put' | 'onboard' | 'person-save' | 'person-read' | 'person-lookup' | 'transfer-notify' | 'request-create' | 'requests-read' | 'request-update';

const utf8 = new TextEncoder();

export function traceabilitySignBytes(purpose: string, chainId: string, subject: string, issued: number): Uint8Array {
  return utf8.encode(`yamale-traceability/v1:${purpose}:${chainId}:${subject}:${Math.floor(issued)}`);
}

export function sha256Hex(bytes: Uint8Array): string {
  return toHex(sha256(bytes));
}

export function signedHeaders(
  privateKey: Uint8Array, purpose: TraceabilityPurpose, chainId: string, subject: string,
  issued: number = Math.floor(Date.now() / 1000),
): Record<string, string> {
  const signature = signArbitrary(privateKey, traceabilitySignBytes(purpose, chainId, subject, issued));
  return {
    'X-Yamale-Key': toBase64(compressedPublicKey(privateKey)),
    'X-Yamale-Signature': toBase64(signature),
    'X-Yamale-Issued': String(Math.floor(issued)),
  };
}
