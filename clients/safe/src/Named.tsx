import { AccountName } from '@yamale/chain';

import { client } from './chain.ts';

/**
 * An account, named by the shared rule: your address-book name, else the
 * chain's user ID, else the address.
 *
 * A thin wrapper so every call site does not repeat the lookup wiring — and so
 * the Safe, the wallet and the explorer stay on one implementation rather than
 * three that drift.
 */
export function Named({ address, full }: { address: string; full?: boolean }) {
  return <AccountName address={address} lookup={(a) => client.userIdOf(a)} full={full} />;
}
