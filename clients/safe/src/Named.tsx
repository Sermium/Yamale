import { AccountName } from '@yamale/chain';

import { client } from './chain.ts';

/**
 * The lookup, hoisted out of the render.
 *
 * `useDisplayName` in the SDK keys its effect on `[address, lookup]`. Passing
 * `(a) => client.userIdOf(a)` inline made a new function identity on every
 * render, so the effect re-ran on every render and re-queried the chain —
 * measured against the live node, one address produced five identical requests
 * for `alias_of/…` in a few seconds, each one a 404, because an account with no
 * user ID is exactly the case that never resolves and therefore never stops
 * re-rendering.
 *
 * `client` is a module singleton, so this is stable by construction rather than
 * by remembering to wrap it in useCallback at each call site.
 *
 * The same shape will bite any other caller that passes an inline lookup; the
 * durable fix is for the SDK hook to hold the lookup in a ref, which is a
 * change to a file six surfaces share and is reported rather than made here.
 */
const lookupUserId = (address: string) => client.userIdOf(address);

/**
 * An account, named by the shared rule: your address-book name, else the
 * chain's user ID, else the address.
 *
 * A thin wrapper so every call site does not repeat the lookup wiring — and so
 * the Safe, the wallet and the explorer stay on one implementation rather than
 * three that drift.
 */
export function Named({ address, full }: { address: string; full?: boolean }) {
  return <AccountName address={address} lookup={lookupUserId} full={full} />;
}
