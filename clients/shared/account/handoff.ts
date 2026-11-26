/**
 * "Sign in with Yamale": one place, accounts.yamaletech.com, where a person
 * signs in once per device, and every other Yamale app receives the account
 * from it instead of asking for a code again.
 *
 * Each app keeps its own copy of the account, because a browser keeps storage
 * per origin; what travels between them is the account in its SEALED form (the
 * key under the person's password), never an open key and never the password.
 * The receiving app still asks for the password, which is what keeps the key
 * safe on a device somebody else picks up.
 *
 * The trip:
 *   1. The app makes a one-time key, keeps its private half in this tab's
 *      session storage, and sends the person to the accounts page with the
 *      public half, a state value and where to come back to.
 *   2. The accounts page checks that the return address belongs to a Yamale
 *      app, asks the person to confirm, seals the account to the one-time key
 *      and sends them back with it in the fragment, which no server receives.
 *   3. The app opens it with the private half, stores the account and forgets
 *      the one-time key. A copied URL or the browser history yields a box only
 *      that tab could open, holding a sealed account.
 *
 * Box: ephemeral secp256k1 key (33 bytes), nonce (12), ChaCha20-Poly1305 of
 * the JSON account, keyed by HKDF-SHA256 over the ECDH secret and bound to the
 * returning origin and the state.
 */

import { secp256k1 } from '@noble/curves/secp256k1';
import { chacha20poly1305 } from '@noble/ciphers/chacha.js';
import { hkdf } from '@noble/hashes/hkdf.js';
import { sha256 } from '@noble/hashes/sha2.js';

import { fromBase64Url, toBase64Url, toHex, fromHex } from '../../sdk/src/jewel.ts';
import { readAccount, writeImportedAccount, type KeyStore, type StoredAccount } from './device.ts';

export const ACCOUNTS_ORIGIN = 'https://accounts.yamaletech.com';
export const ACCOUNTS_PATH = '/accounts/';

/** The apps that may receive an account, by origin, with the name the person is shown. */
export const APPS: Record<string, string> = {
  'https://pay.yamaletech.com': 'Yamale Pay',
  'https://traceability.yamaletech.com': 'Yamale Jewels',
  'https://wallet.yamaletech.com': 'Yamale Wallet',
};

const SESSION_PREFIX = 'yamale.handoff.';
const utf8 = new TextEncoder();
const info = (origin: string, state: string) => utf8.encode(`yamale-accounts/handoff/v1:${origin}:${state}`);

/** The origin of a return address, if it is a Yamale app; null otherwise. */
export function appOrigin(returnUrl: string, apps: Record<string, string> = APPS): string | null {
  try {
    const u = new URL(returnUrl);
    return u.protocol === 'https:' && apps[u.origin] ? u.origin : null;
  } catch {
    return null;
  }
}

function seal(plain: Uint8Array, recipient: Uint8Array, origin: string, state: string,
  random: (n: number) => Uint8Array): Uint8Array {
  const eph = random(32);
  const epk = secp256k1.getPublicKey(eph, true);
  const key = hkdf(sha256, secp256k1.getSharedSecret(eph, recipient, true).slice(1), epk, info(origin, state), 32);
  const nonce = random(12);
  const ct = chacha20poly1305(key, nonce, epk).encrypt(plain);
  const out = new Uint8Array(45 + ct.length);
  out.set(epk, 0);
  out.set(nonce, 33);
  out.set(ct, 45);
  return out;
}

function open(box: Uint8Array, privateKey: Uint8Array, origin: string, state: string): Uint8Array | null {
  if (box.length < 45 + 16) return null;
  try {
    const epk = box.slice(0, 33);
    const key = hkdf(sha256, secp256k1.getSharedSecret(privateKey, epk, true).slice(1), epk, info(origin, state), 32);
    return chacha20poly1305(key, box.slice(33, 45), epk).decrypt(box.slice(45));
  } catch {
    return null;
  }
}

const defaultRandom = (n: number) => crypto.getRandomValues(new Uint8Array(n));

/** On the accounts page: the account sealed for the app that asked, as a fragment. */
export function sealForApp(account: StoredAccount, appPublicKey: string, origin: string, state: string,
  random: (n: number) => Uint8Array = defaultRandom): string {
  const box = seal(utf8.encode(JSON.stringify({ v: 1, account })), fromBase64Url(appPublicKey), origin, state, random);
  return `yamale-handoff=${toBase64Url(box)}&state=${encodeURIComponent(state)}`;
}

/** In an app: where to send the person to sign in with Yamale, remembering the one-time key. */
export function handoffUrl(returnUrl: string, session: KeyStore = globalThis.sessionStorage,
  random: (n: number) => Uint8Array = defaultRandom): string {
  const priv = random(32);
  const state = toHex(random(16));
  session.setItem(SESSION_PREFIX + state, toHex(priv));
  const q = new URLSearchParams({
    pk: toBase64Url(secp256k1.getPublicKey(priv, true)),
    state,
    return: returnUrl,
  });
  return `${ACCOUNTS_ORIGIN}${ACCOUNTS_PATH}?${q}`;
}

export type Received = 'none' | 'imported' | 'exists' | 'failed';

/**
 * In an app, on load: take an account handed over in the fragment, if there is
 * one, store it, and clear the fragment and the one-time key either way.
 */
export function receiveHandoff(hash: string, origin: string, session: KeyStore = globalThis.sessionStorage,
  store?: KeyStore): { outcome: Received; account?: StoredAccount } {
  const params = new URLSearchParams(hash.replace(/^#/, ''));
  const boxed = params.get('yamale-handoff');
  const state = params.get('state');
  if (!boxed || !state) return { outcome: 'none' };
  const privHex = session.getItem(SESSION_PREFIX + state);
  session.removeItem(SESSION_PREFIX + state);
  if (!privHex) return { outcome: 'failed' };
  const plain = open(fromBase64Url(boxed), fromHex(privHex), origin, state);
  if (!plain) return { outcome: 'failed' };
  let account: StoredAccount;
  try {
    const parsed = JSON.parse(new TextDecoder().decode(plain)) as { v: number; account: StoredAccount };
    if (parsed.v !== 1 || !parsed.account?.address || !parsed.account?.sealed) return { outcome: 'failed' };
    account = parsed.account;
  } catch {
    return { outcome: 'failed' };
  }
  const here = readAccount(store);
  if (here) return { outcome: here.address === account.address ? 'imported' : 'exists', account };
  writeImportedAccount(account, store);
  return { outcome: 'imported', account };
}

/* ------------------------------------------------ in an app, at start-up */

let outcome: Received = 'none';

/**
 * Called once before the app renders, so the account is stored before the
 * session reads it. The fragment is cleared, so a reload or a shared link does
 * not carry the box any further.
 */
export function receiveHandoffHere(): Received {
  if (typeof location === 'undefined' || !location.hash.includes('yamale-handoff=')) return outcome;
  try {
    outcome = receiveHandoff(location.hash, location.origin).outcome;
  } catch {
    outcome = 'failed';
  }
  history.replaceState(null, '', location.pathname + location.search);
  return outcome;
}

/** How the last "Sign in with Yamale" ended, for the sign-in screen to say so. */
export const handoffOutcome = (): Received => outcome;

/** Whether this page is a Yamale app the accounts page will send an account to. */
export const canSignInWithYamale = (): boolean =>
  typeof location !== 'undefined' && appOrigin(location.href) !== null;

/** Send the person to sign in with Yamale, and back to this page afterwards. */
export function signInWithYamale(returnPath: string = location.pathname): void {
  location.assign(handoffUrl(location.origin + returnPath));
}
