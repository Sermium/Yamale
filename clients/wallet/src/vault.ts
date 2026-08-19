import { DirectSecp256k1HdWallet } from '@cosmjs/proto-signing';

/**
 * The key vault: an account held encrypted in this origin's storage, unlocked
 * with a password, kept in memory only while the tab is open.
 *
 * **Why this does not use WebCrypto.** The first version of this file derived a
 * key with `crypto.subtle.importKey` and failed with "Cannot read properties of
 * undefined" on the devnet. `crypto.subtle` exists only in a *secure context* —
 * HTTPS, or localhost. Served from `http://10.0.0.188:8092` it is simply
 * `undefined`, and no amount of correct cryptography helps if the API is not
 * there. (`crypto.getRandomValues` is not gated this way, which is what makes
 * the failure look arbitrary.)
 *
 * So encryption goes through CosmJS's own wallet serialization, which is
 * designed for exactly this and ships its own primitives: Argon2id to stretch
 * the password, XChaCha20-Poly1305 to encrypt and authenticate. That is a
 * better choice than the hand-rolled version regardless of secure contexts —
 * memory-hard stretching beats PBKDF2, and it is the same format the Cosmos
 * CLI's keyring writes.
 *
 * What this does not defend against, stated plainly: anything running script on
 * this origin reads the unlocked wallet out of memory. That is why the wallet
 * is its own origin serving no third-party script.
 */

const STORAGE_KEY = 'yamale.wallet.vault.v2';

interface StoredVault {
  version: 2;
  /** CosmJS's encrypted serialization — a JSON envelope carrying its own kdf
   *  parameters, so a future change to those does not orphan existing vaults. */
  serialized: string;
  /** Shown on the unlock screen so somebody with two accounts knows which this is. */
  label: string;
  /** Kept in the clear on purpose: an address is public, and showing it before
   *  unlocking is how you confirm you are about to open the right account. */
  address: string;
}

export function vaultExists(): boolean {
  return localStorage.getItem(STORAGE_KEY) !== null;
}

export function vaultSummary(): { label: string; address: string } | null {
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) return null;
  try {
    const vault = JSON.parse(raw) as StoredVault;
    return { label: vault.label, address: vault.address };
  } catch {
    return null;
  }
}

export async function createVault(
  mnemonic: string,
  password: string,
  label = 'My account',
): Promise<string> {
  const wallet = await DirectSecp256k1HdWallet.fromMnemonic(mnemonic, { prefix: 'yml' });
  const [account] = await wallet.getAccounts();
  const vault: StoredVault = {
    version: 2,
    serialized: await wallet.serialize(password),
    label,
    address: account!.address,
  };
  localStorage.setItem(STORAGE_KEY, JSON.stringify(vault));
  return account!.address;
}

export class WrongPassword extends Error {
  constructor() {
    super('That password does not open this account.');
    this.name = 'WrongPassword';
  }
}

/** Returns the unlocked wallet itself, not the phrase — the phrase is never
 *  needed again once the vault exists, so it is never reconstructed. */
export async function openVault(password: string): Promise<DirectSecp256k1HdWallet> {
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) throw new Error('No account on this device.');
  const vault = JSON.parse(raw) as StoredVault;
  try {
    return await DirectSecp256k1HdWallet.deserialize(vault.serialized, password);
  } catch {
    // The authentication tag failed. Almost always a wrong password; the
    // alternative is a tampered vault, and neither should return a wallet.
    throw new WrongPassword();
  }
}

export function destroyVault(): void {
  localStorage.removeItem(STORAGE_KEY);
}

/**
 * The unlocked wallet, for this tab only.
 *
 * A module variable rather than anything persistent: a refresh relocks.
 * `sessionStorage` would survive one and is readable by any script on the
 * origin, which is the thing being guarded against.
 */
let unlocked: DirectSecp256k1HdWallet | null = null;
let lockTimer: ReturnType<typeof setTimeout> | undefined;

/** Long enough to approve several transactions, short enough that a
 *  walked-away-from laptop is not a signing terminal. */
const AUTO_LOCK_MS = 15 * 60 * 1000;

export function setUnlocked(wallet: DirectSecp256k1HdWallet): void {
  unlocked = wallet;
  touch();
}

export function getUnlocked(): DirectSecp256k1HdWallet | null {
  return unlocked;
}

export function lock(): void {
  unlocked = null;
  clearTimeout(lockTimer);
}

export function touch(): void {
  clearTimeout(lockTimer);
  lockTimer = setTimeout(lock, AUTO_LOCK_MS);
}
