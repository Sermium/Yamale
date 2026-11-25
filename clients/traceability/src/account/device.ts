/**
 * The account on this device.
 *
 * One secp256k1 key, made here, sealed under the person's password with the
 * wallet's own seal (Argon2id, then XChaCha20-Poly1305), and kept in this
 * origin's storage. The key never leaves the device unsealed. Opening it
 * takes the password, every time the tab is reopened.
 *
 * No seed phrase. If the person also enrols the account with the custodian
 * and recovery services (session.tsx, protect), a lost phone can be recovered
 * through the platform's operator-run recovery. A forgotten password cannot:
 * nobody holds it.
 */

import { secp256k1 } from '@noble/curves/secp256k1';

import { open, seal, WrongSeal, type Sealed } from '../../../wallet/src/account/seal.ts';
import { canonicalEmail } from '../../../wallet/src/account/deposit.ts';
import { addressOfPublicKey, compressedPublicKey, fromBase64Url, toBase64Url } from '../../../sdk/src/jewel.ts';

export interface KeyStore {
  getItem(k: string): string | null;
  setItem(k: string, v: string): void;
  removeItem(k: string): void;
}

export interface WireSealed { salt: string; nonce: string; ciphertext: string; time: number; memory: number; threads: number }

export interface StoredAccount {
  address: string;
  email: string;
  sealed: WireSealed;
  createdAt: string;
  protection: 'none' | 'enrolled';
  sponsored: boolean;
  /**
   * The version of the sponsor's message list this account was granted. The
   * API re-grants an older list once, so an account sponsored before claims
   * existed asks again. Absent in accounts stored before it existed.
   */
  sponsorList?: number;
}

/** The sponsor's current message list version (traceabilityapi ownerList). */
export const SPONSOR_LIST = 2;

export const MIN_PASSWORD = 10;
export const ACCOUNT_EXISTS = 'this device already holds an account; forget it first';
const KEY = 'yamale.traceability.account';

export class WrongPassword extends Error {
  constructor() { super('That password does not open this account.'); this.name = 'WrongPassword'; }
}

function browserStore(): KeyStore {
  return globalThis.localStorage;
}

const toWire = (s: Sealed): WireSealed => ({
  salt: toBase64Url(s.salt), nonce: toBase64Url(s.nonce), ciphertext: toBase64Url(s.ciphertext),
  time: s.time, memory: s.memory, threads: s.threads,
});
const fromWire = (w: WireSealed): Sealed => ({
  salt: fromBase64Url(w.salt), nonce: fromBase64Url(w.nonce), ciphertext: fromBase64Url(w.ciphertext),
  time: w.time, memory: w.memory, threads: w.threads,
});

export function readAccount(store: KeyStore = browserStore()): StoredAccount | null {
  try {
    const raw = store.getItem(KEY);
    return raw ? (JSON.parse(raw) as StoredAccount) : null;
  } catch {
    return null;
  }
}

/**
 * Stores an account carried over from the app's previous address
 * (loyalty.yamaletech.com, see movein.ts). It is the sealed form only: the
 * password never travelled. Refuses to replace an account already here.
 */
export function writeImportedAccount(acct: StoredAccount, store: KeyStore = browserStore()): void {
  if (store.getItem(KEY) !== null) throw new Error(ACCOUNT_EXISTS);
  store.setItem(KEY, JSON.stringify(acct));
  if (readAccount(store)?.address !== acct.address) {
    store.removeItem(KEY);
    throw new Error('the carried account does not read back');
  }
}

function write(account: StoredAccount, store: KeyStore): void {
  store.setItem(KEY, JSON.stringify(account));
}

export async function adoptKey(privateKey: Uint8Array, email: string, password: string,
  store: KeyStore = browserStore()): Promise<StoredAccount> {
  if (password.length < MIN_PASSWORD) throw new Error(`a password needs at least ${MIN_PASSWORD} characters`);
  const account: StoredAccount = {
    address: addressOfPublicKey(compressedPublicKey(privateKey)),
    email: canonicalEmail(email),
    sealed: toWire(await seal(privateKey, password)),
    createdAt: new Date().toISOString(),
    protection: 'none',
    sponsored: false,
  };
  write(account, store);
  return account;
}

export async function createAccount(email: string, password: string, store: KeyStore = browserStore(),
  newKey: () => Uint8Array = () => secp256k1.utils.randomPrivateKey()): Promise<{ account: StoredAccount; privateKey: Uint8Array }> {
  if (readAccount(store)) throw new Error(ACCOUNT_EXISTS);
  if (password.length < MIN_PASSWORD) throw new Error(`a password needs at least ${MIN_PASSWORD} characters`);
  const privateKey = newKey();
  return { account: await adoptKey(privateKey, email, password, store), privateKey };
}

export async function unlockAccount(account: StoredAccount, password: string): Promise<Uint8Array> {
  let key: Uint8Array;
  try {
    key = await open(fromWire(account.sealed), password);
  } catch (err) {
    if (err instanceof WrongSeal) throw new WrongPassword();
    throw err;
  }
  if (addressOfPublicKey(compressedPublicKey(key)) !== account.address) {
    key.fill(0);
    throw new WrongPassword();
  }
  return key;
}

export function updateAccount(patch: Partial<StoredAccount>, store: KeyStore = browserStore()): StoredAccount {
  const current = readAccount(store);
  if (!current) throw new Error('no account on this device');
  const next = { ...current, ...patch, address: current.address };
  write(next, store);
  return next;
}

export function forgetAccount(store: KeyStore = browserStore()): void {
  store.removeItem(KEY);
}
