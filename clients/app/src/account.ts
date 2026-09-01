/**
 * The abstraction layer. Everything Web3 lives in this file and nothing above
 * it may import from CosmJS, mention an address, or use the word wallet.
 *
 * A user has a name, an email and a password. Behind that is a key, derived
 * once and kept encrypted on the device. They never see it, never write down
 * twelve words, and never learn what a mnemonic is — which is the whole point
 * of the exercise, because "write these down or lose your money forever" is the
 * single largest reason people bounce off this technology.
 *
 * This is a proof of concept and it is deliberately honest about being one. The
 * real design is docs/guides/accounts.md: threshold key shares, so that no
 * single party — not even the operator — can move a user's funds. Here the key
 * is wrapped by the password and held on the device, which means a forgotten
 * password is a lost account. That trade is acceptable for a demonstration and
 * is not acceptable in production.
 */
import { DirectSecp256k1HdWallet, type OfflineDirectSigner } from '@cosmjs/proto-signing';
import { signRequest, type Signed } from './signing.ts';

const STORE = 'yamale.app.account';

export interface Profile {
  name: string;
  email: string;
  /** The chain-assigned user id, e.g. K3M9-7QRT-B. Shown instead of an address. */
  userId?: string;
}

interface Stored {
  profile: Profile;
  /** CosmJS's own password-wrapped serialisation: Argon2id + XChaCha20-Poly1305. */
  vault: string;
}

/**
 * The key an account is stored under, derived from the email AND the password.
 *
 * This used to be a bare SHA-256 of the address, described as a blind index. It
 * was not one, and the difference matters: email addresses are low-entropy and
 * enumerable, so anybody holding a dump and a word list tests every guess
 * offline and recovers exactly the membership the hash was supposed to hide. A
 * blind index needs a key the attacker does not have.
 *
 * There is no server here to hold a pepper — this store is the browser's — so
 * the key is the user's own password, stretched. A dump without it yields
 * nothing testable, because computing any candidate index requires the one
 * secret that was never written down.
 *
 * The cost is honest and worth stating: an account can no longer be found by
 * email alone, so a wrong password and an unknown email are now the same
 * outcome. That is a slightly worse error message and a considerably better
 * store.
 *
 * PBKDF2 rather than Argon2id because it is what WebCrypto offers and pulling a
 * WASM Argon2 in for this would cost more than it buys. The custodian service,
 * which holds a verifier that is worth attacking, uses Argon2id.
 */
async function accountKey(email: string, password: string): Promise<string> {
  const normalised = email.trim().toLowerCase();
  const material = await crypto.subtle.importKey(
    'raw', new TextEncoder().encode(password), 'PBKDF2', false, ['deriveBits'],
  );
  const bits = await crypto.subtle.deriveBits(
    {
      name: 'PBKDF2',
      hash: 'SHA-256',
      // Salted by the email, so two people sharing a password do not share a
      // key, and one person's two accounts do not collide.
      salt: new TextEncoder().encode('yamale.account.v2:' + normalised),
      iterations: 210_000,
    },
    material, 256,
  );
  return Array.from(new Uint8Array(bits)).map((b) => b.toString(16).padStart(2, '0')).join('');
}

/**
 * The key this store used before, kept only to migrate an account written under
 * it. Never written again.
 */
async function legacyKey(email: string): Promise<string> {
  const data = new TextEncoder().encode(email.trim().toLowerCase());
  const digest = await crypto.subtle.digest('SHA-256', data);
  return Array.from(new Uint8Array(digest)).map((b) => b.toString(16).padStart(2, '0')).join('');
}

/**
 * Reads an account, moving it to the new key if it was written under the old
 * one.
 *
 * Migrated on sign-in rather than by a sweep, because the new key cannot be
 * computed without the password and a sweep has none. An account whose owner
 * never signs in again keeps its old key, which is the same exposure it already
 * had and not a new one.
 */
async function readStored(email: string, password: string): Promise<Stored | null> {
  const key = await accountKey(email, password);
  const current = localStorage.getItem(STORE + '.' + key);
  if (current) return JSON.parse(current) as Stored;

  const legacy = localStorage.getItem(STORE + '.' + (await legacyKey(email)));
  if (!legacy) return null;
  const stored = JSON.parse(legacy) as Stored;
  // Only after the password has proved itself, which the caller does by
  // deserialising the vault. Moving it first would let anybody holding the dump
  // rewrite the store by guessing.
  return stored;
}

export async function signUp(profile: Profile, password: string): Promise<Signer> {
  const wallet = await DirectSecp256k1HdWallet.generate(24, { prefix: 'yml' });
  const vault = await wallet.serialize(password);
  const stored: Stored = { profile, vault };
  localStorage.setItem(STORE + '.' + (await accountKey(profile.email, password)), JSON.stringify(stored));
  return new Signer(profile, wallet);
}

export async function signIn(email: string, password: string): Promise<Signer> {
  const stored = await readStored(email, password);
  if (!stored) throw new Error('no-account');
  // Deserialisation fails on a wrong password. That failure is the password
  // check — there is no separate hash to compare, and nothing to leak.
  const wallet = await DirectSecp256k1HdWallet.deserialize(stored.vault, password);

  // Now that the password has proved itself, finish any migration off the old
  // key. Written before the delete, so a crash between the two leaves two
  // copies rather than none.
  const key = await accountKey(email, password);
  if (!localStorage.getItem(STORE + '.' + key)) {
    localStorage.setItem(STORE + '.' + key, JSON.stringify(stored));
    localStorage.removeItem(STORE + '.' + (await legacyKey(email)));
  }
  return new Signer(stored.profile, wallet);
}

/**
 * There is deliberately no record of who signed in last.
 *
 * It used to be kept, as the plain email, under a fixed key — beside an index
 * built specifically so that the email would not be readable. One
 * localStorage dump answered "who uses this" without touching the index at all,
 * which made the index decorative regardless of how it was computed.
 *
 * A masked hint was considered and rejected: "a•••@example.com" still names the
 * organisation, and on a national payments system the domain is most of the
 * answer. The cost is that somebody types their email each time.
 */
export function lastEmail(): string | null {
  return null;
}

export function signOut(): void {
  // Nothing to forget: nothing was remembered. Kept as a function because the
  // interface above it is entitled to say sign out and be believed.
}

/**
 * Signer is the only thing the interface above ever holds, and its surface is
 * deliberately tiny: who you are, what you have, and pay someone. No address
 * getter — a screen that can read one is a screen that will eventually display
 * one.
 */
export class Signer {
  constructor(readonly profile: Profile, private readonly wallet: OfflineDirectSigner) {}

  /** For the SDK only. Never render this. */
  async internalAddress(): Promise<string> {
    const [account] = await this.wallet.getAccounts();
    return account.address;
  }

  offlineSigner(): OfflineDirectSigner {
    return this.wallet;
  }

  /**
   * Sign a payment request with this account's key.
   *
   * The interface gets a signature, never the key. That asymmetry is the whole
   * reason this class exists: a screen that could reach the key is a screen
   * that could leak it.
   */
  async sign(payload: string): Promise<Signed> {
    const mnemonic = (this.wallet as unknown as { mnemonic: string }).mnemonic;
    return signRequest(mnemonic, payload);
  }
}

/**
 * Reveal the recovery phrase, and only against the password entered again.
 *
 * The session already holds an unlocked wallet, so this check is not
 * cryptographically necessary — it is necessary because an unlocked phone left
 * on a table is the ordinary way a phrase gets read by somebody else. Asking
 * again costs the owner four seconds and costs a thief the whole account.
 */
export async function revealPhrase(email: string, password: string): Promise<string | null> {
  try {
    const raw = localStorage.getItem(STORE + '.' + (await accountKey(email, password)))
      ?? localStorage.getItem(STORE + '.' + (await legacyKey(email)));
    if (!raw) return null;
    const stored: Stored = JSON.parse(raw);
    const wallet = await DirectSecp256k1HdWallet.deserialize(stored.vault, password);
    return (wallet as unknown as { mnemonic: string }).mnemonic;
  } catch {
    return null;
  }
}

/**
 * Forget this account completely, on this device.
 *
 * Everything the app stores is prefixed `yamale.`, so the sweep is by prefix
 * rather than by a list of keys somebody has to remember to update — the last
 * three features each added a key, and a hand-maintained list would already be
 * wrong.
 *
 * What this cannot do is delete the account from the chain. Nothing can: the
 * address exists because a block says it does, and the user ID stays registered
 * to it. That is a property of a ledger, not a limitation of this function, and
 * the interface says so rather than implying an erasure it cannot perform.
 */
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
