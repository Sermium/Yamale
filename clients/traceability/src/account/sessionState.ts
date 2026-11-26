import { readAccount, type KeyStore, type StoredAccount } from '../../../shared/account/device.ts';

/**
 * What the session holds for the account on this device: always locked, since
 * nothing here opens a key. Read at mount, and again by Session.refresh() when
 * another part of the app (the move-in route) writes the account afterwards.
 */
export function sessionAccountState(store?: KeyStore): { account: StoredAccount | null; unlocked: false } {
  return { account: readAccount(store), unlocked: false };
}
