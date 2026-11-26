/**
 * Opening the account on this device with a fingerprint or a face instead of
 * the password, in every Yamale app.
 *
 * The account on a device is its key sealed under the password (device.ts).
 * Biometrics add a second sealed copy of that key, opened by a passkey of this
 * device instead of the password. The password itself is never stored, and it
 * stays the way in on a new device and on a device without biometrics.
 *
 * Two strengths, and the screen says which one is in use:
 *
 *   - 'prf' (recent Chrome on Android and Windows, Safari on iOS 18 and macOS):
 *     the passkey itself derives the secret that opens the copy (the WebAuthn
 *     PRF extension). Without the finger or the face there is no secret, and
 *     what is stored on the device opens nothing.
 *   - 'device' (older authenticators): the copy is sealed by a browser key that
 *     cannot be exported, and the passkey check is the gate in front of it.
 *     Somebody with the unlocked browser could use that key without the check,
 *     so this is a convenience, weaker than the password.
 *
 * Each app has its own passkey (named after the app) and its own copy, since a
 * browser keeps storage per origin.
 */

import { chacha20poly1305 } from '@noble/ciphers/chacha.js';
import { hkdf } from '@noble/hashes/hkdf.js';
import { sha256 } from '@noble/hashes/sha2.js';

import { addressOfPublicKey, compressedPublicKey, fromBase64Url, toBase64Url } from '../../sdk/src/jewel.ts';

const STORE = 'yamale.biometric';
const DB = 'yamale-biometric';
const INFO = new TextEncoder().encode('yamale/biometric/v1');

export type BiometricMode = 'prf' | 'device';

interface Saved {
  address: string;
  credentialId: string;
  salt: string;
  mode: BiometricMode;
  nonce: string;
  sealed: string;
}

const random = (n: number) => crypto.getRandomValues(new Uint8Array(n));

function saved(): Saved | null {
  try { return JSON.parse(localStorage.getItem(STORE) ?? 'null') as Saved | null; } catch { return null; }
}

/** Whether this device has a fingerprint or face unlock the browser can use. */
export async function biometricsAvailable(): Promise<boolean> {
  try {
    return typeof PublicKeyCredential !== 'undefined'
      && await PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable();
  } catch {
    return false;
  }
}

/** The mode in use for this account on this device, or null when biometrics are off. */
export function biometricsFor(address: string): BiometricMode | null {
  const s = saved();
  return s && s.address === address ? s.mode : null;
}

/* ------------------------------------------------ the browser key, for 'device' */

function idb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const open = indexedDB.open(DB, 1);
    open.onupgradeneeded = () => open.result.createObjectStore('keys');
    open.onsuccess = () => resolve(open.result);
    open.onerror = () => reject(open.error);
  });
}

async function idbDo<T>(mode: IDBTransactionMode, run: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const db = await idb();
  return new Promise((resolve, reject) => {
    const req = run(db.transaction('keys', mode).objectStore('keys'));
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

/* ------------------------------------------------ the passkey */

function prfOf(credential: Credential | null): Uint8Array | null {
  const ext = (credential as PublicKeyCredential | null)?.getClientExtensionResults() as
    { prf?: { results?: { first?: ArrayBuffer } } } | undefined;
  const first = ext?.prf?.results?.first;
  return first ? new Uint8Array(first) : null;
}

async function assert(credentialId: Uint8Array, salt: Uint8Array): Promise<Credential | null> {
  return navigator.credentials.get({
    publicKey: {
      challenge: random(32),
      allowCredentials: [{ type: 'public-key', id: credentialId as Uint8Array<ArrayBuffer>, transports: ['internal'] }],
      userVerification: 'required',
      timeout: 60_000,
      extensions: { prf: { eval: { first: salt as Uint8Array<ArrayBuffer> } } } as AuthenticationExtensionsClientInputs,
    },
  });
}

export type EnableOutcome = { ok: true; mode: BiometricMode } | { ok: false; reason: 'unavailable' | 'cancelled' };

/**
 * Turn biometrics on for this account on this device. `key` is the account's
 * open key (the session is unlocked); a copy of it is sealed to the passkey.
 */
export async function enableBiometrics(address: string, email: string, appName: string, key: Uint8Array): Promise<EnableOutcome> {
  if (!(await biometricsAvailable())) return { ok: false, reason: 'unavailable' };
  const salt = random(32);
  let credential: Credential | null;
  try {
    credential = await navigator.credentials.create({
      publicKey: {
        challenge: random(32),
        rp: { id: location.hostname, name: appName },
        user: { id: random(16), name: email, displayName: email },
        pubKeyCredParams: [{ type: 'public-key', alg: -7 }, { type: 'public-key', alg: -257 }],
        authenticatorSelection: { authenticatorAttachment: 'platform', userVerification: 'required', residentKey: 'discouraged' },
        timeout: 60_000,
        extensions: { prf: { eval: { first: salt } } } as AuthenticationExtensionsClientInputs,
      },
    });
  } catch {
    return { ok: false, reason: 'cancelled' };
  }
  if (!credential) return { ok: false, reason: 'cancelled' };
  const credentialId = new Uint8Array((credential as PublicKeyCredential).rawId);

  // Some authenticators give the PRF output only on an assertion, not at creation.
  let secret = prfOf(credential);
  const ext = (credential as PublicKeyCredential).getClientExtensionResults() as { prf?: { enabled?: boolean } };
  if (!secret && ext.prf?.enabled) {
    try { secret = prfOf(await assert(credentialId, salt)); } catch { return { ok: false, reason: 'cancelled' }; }
  }

  const nonce = random(12);
  let mode: BiometricMode;
  let sealed: Uint8Array;
  if (secret) {
    mode = 'prf';
    sealed = chacha20poly1305(hkdf(sha256, secret, salt, INFO, 32), nonce).encrypt(key);
    secret.fill(0);
  } else {
    mode = 'device';
    const browserKey = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt']);
    await idbDo('readwrite', (s) => s.put(browserKey, address));
    sealed = new Uint8Array(await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce }, browserKey, key as Uint8Array<ArrayBuffer>));
  }
  const record: Saved = {
    address, credentialId: toBase64Url(credentialId), salt: toBase64Url(salt), mode,
    nonce: toBase64Url(nonce), sealed: toBase64Url(sealed),
  };
  try { localStorage.setItem(STORE, JSON.stringify(record)); } catch { return { ok: false, reason: 'unavailable' }; }
  return { ok: true, mode };
}

/** The account's open key, after the fingerprint or the face; null when it does not open. */
export async function unlockWithBiometrics(address: string): Promise<Uint8Array | null> {
  const s = saved();
  if (!s || s.address !== address) return null;
  let credential: Credential | null;
  try { credential = await assert(fromBase64Url(s.credentialId), fromBase64Url(s.salt)); } catch { return null; }
  if (!credential) return null;
  let key: Uint8Array;
  try {
    if (s.mode === 'prf') {
      const secret = prfOf(credential);
      if (!secret) return null;
      key = chacha20poly1305(hkdf(sha256, secret, fromBase64Url(s.salt), INFO, 32), fromBase64Url(s.nonce)).decrypt(fromBase64Url(s.sealed));
      secret.fill(0);
    } else {
      const browserKey = await idbDo<CryptoKey | undefined>('readonly', (st) => st.get(address) as IDBRequest<CryptoKey | undefined>);
      if (!browserKey) return null;
      key = new Uint8Array(await crypto.subtle.decrypt({ name: 'AES-GCM', iv: fromBase64Url(s.nonce) as Uint8Array<ArrayBuffer> }, browserKey, fromBase64Url(s.sealed) as Uint8Array<ArrayBuffer>));
    }
  } catch {
    return null;
  }
  if (addressOfPublicKey(compressedPublicKey(key)) !== address) { key.fill(0); return null; }
  return key;
}

/** Turn biometrics off on this device. The passkey stays in the device's keychain, unused. */
export async function disableBiometrics(): Promise<void> {
  const s = saved();
  try { localStorage.removeItem(STORE); } catch { /* nothing kept */ }
  if (s?.mode === 'device') await idbDo('readwrite', (st) => st.delete(s.address)).catch(() => undefined);
}
