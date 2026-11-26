/**
 * The parts of an email transfer that decide who can open the money: the
 * one-time key, from the link's secret and the code, and the link itself.
 * Kept apart from the rest of the app so they are tested on their own.
 */

import { secp256k1 } from '@noble/curves/secp256k1';
import { hkdf } from '@noble/hashes/hkdf.js';
import { scryptAsync } from '@noble/hashes/scrypt.js';
import { sha256 } from '@noble/hashes/sha2.js';

import { addressOfPublicKey, compressedPublicKey, toBase64Url } from '../../sdk/src/jewel.ts';

export const PAY_ORIGIN = 'https://pay.yamaletech.com';
const utf8 = new TextEncoder();

/**
 * A code is 6 to 8 digits: said over the phone, typed once. Six is the floor
 * because whoever holds the email can guess offline, a second a guess: four
 * digits fall in hours, six take weeks.
 */
export const validCode = (code: string) => /^[0-9]{6,8}$/.test(code);

/** The one-time account's key, from the link's secret and the code, if any. */
export async function claimKey(secret: Uint8Array, code: string): Promise<Uint8Array> {
  let k = code
    ? await scryptAsync(utf8.encode(code), secret, { N: 2 ** 16, r: 8, p: 1, dkLen: 32 })
    : hkdf(sha256, secret, undefined, utf8.encode('yamale-pay/email-transfer/v1'), 32);
  while (!secp256k1.utils.isValidPrivateKey(k)) k = sha256(k);
  return k;
}

export const addressOf = (key: Uint8Array) => addressOfPublicKey(compressedPublicKey(key));

export function linkFor(secret: Uint8Array, withCode: boolean): string {
  return `${PAY_ORIGIN}/app/#receive=${toBase64Url(secret)}${withCode ? '&code=1' : ''}`;
}
