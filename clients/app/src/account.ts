/**
 * The abstraction layer. Everything Web3 lives in this file and nothing above
 * it may import from CosmJS, mention an address, or use the word wallet.
 *
 * A user has a name, an email and a password. Behind that is a key they never
 * see and never write down. Since 2026-11-26 it is the SAME key every Yamale
 * app uses: the account in clients/shared/account, made on a device, sealed
 * under the password, and enrolled with the custodian and recovery services.
 * Signing in here with the email and password used in the jewellery app (or
 * any other) opens the same account, the same balance and the same identifier.
 * A new device asks for a code as well; a lost phone is recovered through the
 * services, and a forgotten password cannot be, because nobody holds it.
 *
 * Before that, this app made its own wallet from a 24-word phrase, kept only in
 * this browser. Such an account is carried over the first time its owner signs
 * in: enrolled with the services under the same key, so its address and money
 * do not move; or, when the email already belongs to an account made elsewhere,
 * opened alongside it so its balance can be moved across.
 */

import { DirectSecp256k1HdWallet, DirectSecp256k1Wallet, type OfflineDirectSigner } from '@cosmjs/proto-signing';

import {
  adoptKey, createAccount, forgetAccount, readAccount, unlockAccount, updateAccount, WrongPassword,
} from '../../shared/account/device.ts';
import { protectionOutcome } from '../../shared/account/protection.ts';
import { deviceLabel, signIn as servicesSignIn, startSignIn, type SignInFailure } from '../../shared/account/signin.ts';
import { canonicalEmail } from '../../wallet/src/account/deposit.ts';
import { enrol, privateKeyFromMnemonic, servicesAvailable, type Services } from '../../wallet/src/account/enrol.ts';
import { rememberEnrolment } from '../../wallet/src/account/stored.ts';
import { fullName, readMyPerson, requestOnboarding, savePerson } from '../../shared/account/people.ts';
import { signRequest, signRequestWithKey, type Signed } from './signing.ts';
import { signedHeaders, type TraceabilityPurpose } from '../../sdk/src/traceabilityauth.ts';

/** The account services, reached through this host like every Yamale app. */
const SERVICES: Services = { custodianUrl: '/api/custodian', recoveryUrl: '/api/recovery' };

/** The chain the account services and the accounts API sign for. */
const CHAIN_ID = 'yamale-devnet-2';

/** Where this app kept its own accounts before they were shared. */
const STORE = 'yamale.app.account';
/** The name this app shows, per account; the profile itself is shared later. */
const NAMES = 'yamale.app.names';

export interface Profile {
  name: string;
  email: string;
  /** The chain-assigned user id, e.g. K3M9-7QRT-B. Shown instead of an address. */
  userId?: string;
}

const lang = () => (typeof document !== 'undefined' ? document.documentElement.lang : '') || 'en';

function rememberName(address: string, name: string): void {
  try {
    const all = JSON.parse(localStorage.getItem(NAMES) ?? '{}') as Record<string, string>;
    all[address] = name;
    localStorage.setItem(NAMES, JSON.stringify(all));
  } catch { /* private browsing: the name falls back to the email */ }
}

function nameOf(address: string, email: string): string {
  try {
    const all = JSON.parse(localStorage.getItem(NAMES) ?? '{}') as Record<string, string>;
    if (all[address]) return all[address];
  } catch { /* no storage */ }
  return email.split('@')[0];
}

/* ------------------------------------------------ the old, device-only accounts */

interface Stored { profile: Profile; vault: string }

/** The key an old account was stored under: the email AND the password, stretched. */
async function accountKey(email: string, password: string): Promise<string> {
  const normalised = email.trim().toLowerCase();
  const material = await crypto.subtle.importKey('raw', new TextEncoder().encode(password), 'PBKDF2', false, ['deriveBits']);
  const bits = await crypto.subtle.deriveBits(
    { name: 'PBKDF2', hash: 'SHA-256', salt: new TextEncoder().encode('yamale.account.v2:' + normalised), iterations: 210_000 },
    material, 256);
  return Array.from(new Uint8Array(bits)).map((b) => b.toString(16).padStart(2, '0')).join('');
}

/** The older bare hash of the email, from before the key above. */
async function hashKey(email: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(email.trim().toLowerCase()));
  return Array.from(new Uint8Array(digest)).map((b) => b.toString(16).padStart(2, '0')).join('');
}

interface Legacy { profile: Profile; wallet: DirectSecp256k1HdWallet; slots: string[] }

/** An old account this password opens, if this browser holds one. */
async function openLegacy(email: string, password: string): Promise<Legacy | null> {
  const slots = [STORE + '.' + (await accountKey(email, password)), STORE + '.' + (await hashKey(email))];
  for (const slot of slots) {
    const raw = localStorage.getItem(slot);
    if (!raw) continue;
    try {
      const stored = JSON.parse(raw) as Stored;
      const wallet = await DirectSecp256k1HdWallet.deserialize(stored.vault, password);
      return { profile: stored.profile, wallet, slots };
    } catch {
      return null; // the password does not open it
    }
  }
  return null;
}

function dropLegacy(legacy: Legacy): void {
  for (const slot of legacy.slots) localStorage.removeItem(slot);
}

/* ------------------------------------------------ signing up and in */

/** An old account of this app, kept open so its balance can be moved to the shared one. */
export interface Previous { signer: Signer; forget: () => void }

export type SignInStep =
  /** Signed in. `previous` is an old account of this app whose balance can be moved across. */
  | { kind: 'done'; signer: Signer; previous?: Previous }
  /** The services want a code (a new device): from the authenticator app, or emailed. */
  | { kind: 'code'; factor: 'totp' | 'email' | 'pin'; email: string; password: string; nonce?: string; issued?: number; previous?: Previous }
  | { kind: 'failed'; reason: SignInFailure | 'no-account' | 'email-in-use' | 'device-taken' };

/** What a new account says about the person, kept in the shared profile. */
export interface Details { firstName: string; lastName: string; country: string; birthDate?: string }

/**
 * The name to show, from the profile every Yamale app shares; the name this
 * device remembers, or the email, when the profile cannot be read.
 */
async function nameFor(privateKey: Uint8Array, address: string, email: string): Promise<string> {
  const r = await readMyPerson(privateKey, address, CHAIN_ID).catch(() => null);
  const name = r && r.ok ? fullName(r.value) : null;
  if (name) { rememberName(address, name); return name; }
  return nameOf(address, email);
}

async function fromKey(privateKey: Uint8Array, profile: Profile): Promise<Signer> {
  const wallet = await DirectSecp256k1Wallet.fromKey(privateKey, 'yml');
  return new Signer(profile, wallet, privateKey);
}

function previousOf(legacy: Legacy): Previous {
  return { signer: new Signer(legacy.profile, legacy.wallet, null), forget: () => dropLegacy(legacy) };
}

/**
 * A new account, enrolled with the services so it opens in every Yamale app,
 * with its profile saved where every app reads it and its country recorded so
 * the chain can issue its identifier. The last two are best effort: an account
 * without them still works, and the profile can be completed later.
 */
export async function signUp(email: string, details: Details, password: string): Promise<Signer> {
  if (readAccount()) throw new Error('device-taken');
  const made = await createAccount(email, password);
  const name = `${details.firstName.trim()} ${details.lastName.trim()}`.trim();
  const profile: Profile = { name, email: made.account.email };
  rememberName(made.account.address, name);
  if (await servicesAvailable(SERVICES)) {
    const done = await enrol(made.privateKey, made.account.email, password, SERVICES);
    if (protectionOutcome(done) === 'email-in-use') {
      // The email already opens an account made elsewhere: sign in to that one instead.
      forgetAccount();
      throw new Error('email-in-use');
    }
    rememberEnrolment({ address: done.address, email: made.account.email, holding: done.deviceHolding, enrolledAt: new Date().toISOString() });
    updateAccount({ protection: 'enrolled' });
  }
  const address = made.account.address;
  await savePerson(made.privateKey, address, {
    first_name: details.firstName.trim(), last_name: details.lastName.trim(), birth_date: details.birthDate ?? '',
  }, CHAIN_ID).catch(() => undefined);
  if (details.country) await requestOnboarding(made.privateKey, address, details.country, CHAIN_ID).catch(() => undefined);
  return fromKey(made.privateKey, profile);
}

/**
 * Sign in with the email and password. On a device that already holds the
 * account, the password opens it here. Elsewhere the services are asked, and
 * they want a code: finishSignIn completes it.
 */
export async function signIn(email: string, password: string): Promise<SignInStep> {
  const canonical = canonicalEmail(email);
  const local = readAccount();
  if (local && local.email === canonical) {
    try {
      const key = await unlockAccount(local, password);
      return { kind: 'done', signer: await fromKey(key, { name: await nameFor(key, local.address, local.email), email: local.email }) };
    } catch (err) {
      if (err instanceof WrongPassword) return { kind: 'failed', reason: 'wrong' };
      throw err;
    }
  }

  const legacy = await openLegacy(email, password);
  if (legacy && !local) {
    // An account of this app from before: keep its key, and so its address and
    // money, by enrolling it, unless the email already opens another account.
    const mnemonic = (legacy.wallet as unknown as { mnemonic: string }).mnemonic;
    const key = await privateKeyFromMnemonic(mnemonic);
    const reachable = await servicesAvailable(SERVICES);
    const done = reachable ? await enrol(key, canonical, password, SERVICES) : null;
    if (!done || protectionOutcome(done) === 'enrolled') {
      const adopted = await adoptKey(key, canonical, password);
      if (done) {
        rememberEnrolment({ address: done.address, email: canonical, holding: done.deviceHolding, enrolledAt: new Date().toISOString() });
        updateAccount({ protection: 'enrolled' });
      }
      rememberName(adopted.address, legacy.profile.name);
      dropLegacy(legacy);
      return { kind: 'done', signer: await fromKey(key, { ...legacy.profile, email: canonical }) };
    }
    key.fill(0);
    // The email is taken by an account made in another app: that one is the
    // account now, and the old one's balance is offered to move across.
  }

  if (local && local.email !== canonical && local.protection !== 'enrolled') {
    // Another person's account lives only on this device: replacing it would lose it.
    return { kind: 'failed', reason: 'device-taken' };
  }
  const started = await startSignIn(SERVICES, canonical, password, lang());
  if (!started.ok) return { kind: 'failed', reason: started.reason === 'wrong' && !legacy ? 'no-account' : started.reason };
  return {
    kind: 'code', factor: started.factor, email: canonical, password,
    ...(started.factor === 'email' ? { nonce: started.nonce, issued: started.issued } : {}),
    ...(legacy ? { previous: previousOf(legacy) } : {}),
  };
}

/** The code step of a sign-in on a new device. */
export async function finishSignIn(step: Extract<SignInStep, { kind: 'code' }>, code: string): Promise<SignInStep> {
  const out = await servicesSignIn(SERVICES, {
    email: step.email, password: step.password, factor: step.factor, answer: code.trim(),
    nonce: step.nonce, issued: step.issued, device: deviceLabel(navigator.userAgent), lang: lang(),
  });
  if (!out.ok) return { kind: 'failed', reason: out.reason };
  const local = readAccount();
  if (local && local.address !== out.address) forgetAccount(); // enrolled, so recoverable; checked in signIn
  if (!readAccount()) {
    await adoptKey(out.privateKey, step.email, step.password);
    updateAccount({ protection: 'enrolled' });
  }
  const signer = await fromKey(out.privateKey, { name: await nameFor(out.privateKey, out.address, step.email), email: step.email });
  return { kind: 'done', signer, ...(step.previous ? { previous: step.previous } : {}) };
}

/** A session for an account handed over by accounts.yamaletech.com with its open key. */
export async function signerFromHandoff(address: string, email: string, key: Uint8Array): Promise<Signer> {
  return fromKey(key, { name: await nameFor(key, address, email), email });
}

/** Whether this password opens the account on this device: for settings that ask again. */
export async function checkPassword(email: string, password: string): Promise<boolean> {
  const local = readAccount();
  if (!local || local.email !== canonicalEmail(email)) return false;
  try {
    (await unlockAccount(local, password)).fill(0);
    return true;
  } catch {
    return false;
  }
}

export function lastEmail(): string | null {
  return readAccount()?.email ?? null;
}

export function signOut(): void {
  // The account stays on this device, sealed: signing in again needs the password.
}

/**
 * The only thing the interface holds. It is deliberately tiny: who you are,
 * what you have, and pay someone. No address getter for screens: a screen that
 * can read one is a screen that will eventually display one.
 */
export class Signer {
  constructor(readonly profile: Profile, private readonly wallet: OfflineDirectSigner,
    private readonly key: Uint8Array | null) {}

  /** For the SDK only. Never render this. */
  async internalAddress(): Promise<string> {
    const [account] = await this.wallet.getAccounts();
    return account.address;
  }

  offlineSigner(): OfflineDirectSigner {
    return this.wallet;
  }

  /**
   * Headers that sign a request to the Yamale APIs with this account's key, so
   * a screen can ask them for something without ever holding the key.
   */
  async apiHeaders(purpose: TraceabilityPurpose, subject: string): Promise<Record<string, string>> {
    const key = this.key ?? await privateKeyFromMnemonic((this.wallet as unknown as { mnemonic: string }).mnemonic);
    try {
      return signedHeaders(key, purpose, CHAIN_ID, subject);
    } finally {
      if (!this.key) key.fill(0);
    }
  }

  /** Whether this is an old account of this app, with a phrase, rather than a shared one. */
  get hasPhrase(): boolean {
    return this.key === null;
  }

  /**
   * Sign a payment request with this account's key. The interface gets a
   * signature, never the key.
   */
  async sign(payload: string): Promise<Signed> {
    if (this.key) return signRequestWithKey(this.key, payload);
    const mnemonic = (this.wallet as unknown as { mnemonic: string }).mnemonic;
    return signRequest(mnemonic, payload);
  }
}

/**
 * The recovery phrase of an old account of this app, against the password
 * entered again. Shared accounts have none: the services hold their key.
 */
export async function revealPhrase(email: string, password: string): Promise<string | null> {
  const legacy = await openLegacy(email, password);
  return legacy ? (legacy.wallet as unknown as { mnemonic: string }).mnemonic : null;
}

/** Everything this app keeps in this browser, including the account, after closing it. */
export function eraseEverything(): void {
  try {
    const doomed: string[] = [];
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key && key.startsWith('yamale.')) doomed.push(key);
    }
    for (const key of doomed) localStorage.removeItem(key);
  } catch {
    // Private browsing: there was nothing persisted to erase.
  }
}
