/**
 * Unlocking with a fingerprint or a face, layered over the password.
 *
 * The important design point: **biometrics do not replace the password, they
 * release it.** The vault stays wrapped by the password (Argon2id +
 * XChaCha20-Poly1305, done by CosmJS). What this file adds is a way to keep
 * that password on the device such that it can only be handed back after the
 * platform authenticator says the right person is present.
 *
 * The naive version — "if fingerprint OK, skip the password" — is strictly
 * weaker than the password alone: the key sits unwrapped and anything that can
 * read local storage can spend the money. Here the password is encrypted with
 * an AES-GCM key held in IndexedDB as a **non-extractable** CryptoKey, and the
 * WebAuthn assertion is the gate in front of it. So an attacker with the disk
 * gets ciphertext and a key they cannot export; an attacker at the unlocked
 * phone still has to present the finger.
 *
 * This is honest about its limits. A platform authenticator proves *presence of
 * a registered user on this device*, not identity in a legal sense, and the
 * browser — not this code — decides how hard that is to spoof. It is a
 * convenience over the password, which remains the root of the account.
 */

const CRED_KEY = 'yamale.app.biometric.cred';
const WRAP_DB = 'yamale-biometric';
const WRAP_STORE = 'keys';
const WRAP_ID = 'wrap';
const SEALED_KEY = 'yamale.app.biometric.sealed';

/** True when this device can do platform (built-in) authentication at all. */
export async function available(): Promise<boolean> {
  try {
    if (!window.PublicKeyCredential || !window.isSecureContext) return false;
    return await PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable();
  } catch {
    return false;
  }
}

/** True when this account has already enrolled on this device. */
export function enrolled(): boolean {
  try {
    return !!localStorage.getItem(CRED_KEY) && !!localStorage.getItem(SEALED_KEY);
  } catch {
    return false;
  }
}

// --- the non-extractable wrapping key, kept in IndexedDB -------------------
// localStorage cannot hold a CryptoKey, and that is exactly why the key lives
// here: IndexedDB can store the key object itself, so the raw bytes never exist
// in a place a script or a disk image can read.

function idb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(WRAP_DB, 1);
    req.onupgradeneeded = () => req.result.createObjectStore(WRAP_STORE);
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function putKey(key: CryptoKey): Promise<void> {
  const db = await idb();
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(WRAP_STORE, 'readwrite');
    tx.objectStore(WRAP_STORE).put(key, WRAP_ID);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

async function getKey(): Promise<CryptoKey | null> {
  const db = await idb();
  return new Promise((resolve) => {
    const tx = db.transaction(WRAP_STORE, 'readonly');
    const req = tx.objectStore(WRAP_STORE).get(WRAP_ID);
    req.onsuccess = () => resolve((req.result as CryptoKey) ?? null);
    req.onerror = () => resolve(null);
  });
}

// Typed as ArrayBuffer rather than Uint8Array: WebAuthn's BufferSource and the
// newer lib.dom Uint8Array generic disagree, and the buffer is what both accept.
function randomBytes(n: number): ArrayBuffer {
  const b = new Uint8Array(n);
  crypto.getRandomValues(b);
  return b.buffer;
}

const b64 = {
  to: (b: Uint8Array | ArrayBuffer) =>
    btoa(String.fromCharCode(...new Uint8Array(b as ArrayBuffer))),
  from: (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0)).buffer,
};

/**
 * Enrol: register a platform credential and seal the password behind it.
 *
 * Called right after a successful password sign-in, because that is the only
 * moment the app legitimately holds the password in memory.
 */
export async function enrol(email: string, password: string): Promise<boolean> {
  try {
    if (!(await available())) return false;

    const userId = randomBytes(16);
    const cred = (await navigator.credentials.create({
      publicKey: {
        challenge: randomBytes(32),
        rp: { name: 'Yamale Pay', id: window.location.hostname },
        user: { id: userId, name: email, displayName: email },
        // ES256 then RS256 — the two every platform authenticator supports.
        pubKeyCredParams: [{ type: 'public-key', alg: -7 }, { type: 'public-key', alg: -257 }],
        authenticatorSelection: {
          authenticatorAttachment: 'platform',
          userVerification: 'required',
          residentKey: 'preferred',
        },
        timeout: 60000,
        attestation: 'none',
      },
    })) as PublicKeyCredential | null;
    if (!cred) return false;

    const wrapKey = await crypto.subtle.generateKey(
      { name: 'AES-GCM', length: 256 },
      false, // non-extractable: the whole point
      ['encrypt', 'decrypt'],
    );
    const iv = randomBytes(12);
    const sealed = await crypto.subtle.encrypt(
      { name: 'AES-GCM', iv },
      wrapKey,
      new TextEncoder().encode(password),
    );

    await putKey(wrapKey);
    localStorage.setItem(CRED_KEY, b64.to(cred.rawId));
    localStorage.setItem(SEALED_KEY, JSON.stringify({
      iv: b64.to(iv), data: b64.to(sealed), email,
    }));
    return true;
  } catch {
    // A refused prompt is a normal outcome, not an error worth surfacing.
    return false;
  }
}

/**
 * Unlock: prove presence, then hand back the password.
 *
 * Returns null on any failure so the caller falls through to the password form
 * — biometrics must never be able to lock somebody out of their own account.
 */
export async function unlock(): Promise<{ email: string; password: string } | null> {
  try {
    const rawId = localStorage.getItem(CRED_KEY);
    const blob = localStorage.getItem(SEALED_KEY);
    if (!rawId || !blob) return null;

    const assertion = await navigator.credentials.get({
      publicKey: {
        challenge: randomBytes(32),
        allowCredentials: [{ type: 'public-key', id: b64.from(rawId) }],
        userVerification: 'required',
        timeout: 60000,
      },
    });
    if (!assertion) return null;

    const wrapKey = await getKey();
    if (!wrapKey) return null;

    const { iv, data, email } = JSON.parse(blob);
    const plain = await crypto.subtle.decrypt(
      { name: 'AES-GCM', iv: b64.from(iv) },
      wrapKey,
      b64.from(data),
    );
    return { email, password: new TextDecoder().decode(plain) };
  } catch {
    return null;
  }
}

/** Forget the enrolment — on sign-out of the device, or if somebody asks. */
export async function forget(): Promise<void> {
  try {
    localStorage.removeItem(CRED_KEY);
    localStorage.removeItem(SEALED_KEY);
    const db = await idb();
    const tx = db.transaction(WRAP_STORE, 'readwrite');
    tx.objectStore(WRAP_STORE).delete(WRAP_ID);
  } catch {
    // Nothing enrolled, nothing to forget.
  }
}
