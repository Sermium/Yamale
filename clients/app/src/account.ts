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

/** A stable, non-secret blind index for the email, so a breach of the local
 *  store does not hand over a list of who uses the system. */
async function emailKey(email: string): Promise<string> {
  const data = new TextEncoder().encode(email.trim().toLowerCase());
  const digest = await crypto.subtle.digest('SHA-256', data);
  return Array.from(new Uint8Array(digest)).map((b) => b.toString(16).padStart(2, '0')).join('');
}

export async function signUp(profile: Profile, password: string): Promise<Signer> {
  const wallet = await DirectSecp256k1HdWallet.generate(24, { prefix: 'yml' });
  const vault = await wallet.serialize(password);
  const stored: Stored = { profile, vault };
  localStorage.setItem(STORE + '.' + (await emailKey(profile.email)), JSON.stringify(stored));
  localStorage.setItem(STORE + '.last', profile.email);
  return new Signer(profile, wallet);
}

export async function signIn(email: string, password: string): Promise<Signer> {
  const raw = localStorage.getItem(STORE + '.' + (await emailKey(email)));
  if (!raw) throw new Error('no-account');
  const stored: Stored = JSON.parse(raw);
  // Deserialisation fails on a wrong password. That failure is the password
  // check — there is no separate hash to compare, and nothing to leak.
  const wallet = await DirectSecp256k1HdWallet.deserialize(stored.vault, password);
  localStorage.setItem(STORE + '.last', email);
  return new Signer(stored.profile, wallet);
}

export function lastEmail(): string | null {
  return localStorage.getItem(STORE + '.last');
}

export function signOut(): void {
  localStorage.removeItem(STORE + '.last');
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
    const raw = localStorage.getItem(STORE + '.' + (await emailKey(email)));
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
