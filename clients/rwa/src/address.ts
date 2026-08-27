/**
 * Who is looking, and whether they can sign.
 *
 * Two modes, and keeping them apart is the whole design. Most of what this app
 * answers — what a vehicle is over, whether the registry still permits it, what
 * a reported price is and how long anyone has to contest it — is public, and
 * should be readable by somebody deciding whether to put money in, before they
 * have an account and without anyone knowing they looked. So `watching` is a
 * first-class mode rather than a degraded one: paste an address, see everything
 * that address owns, sign nothing.
 *
 * `connected` adds exactly three abilities: claim, redeem, dispute. This app
 * never holds a key or a recovery phrase. A wallet extension does, and that is
 * the correct place for it — an investor app that asked for a mnemonic would be
 * teaching the habit that gets people robbed.
 *
 * Separated from the React hook in account.ts so these can be tested by the
 * repo's runner, which strips types but does not transform JSX and so cannot
 * load anything that reaches the SDK's React entry points.
 */
import type { OfflineSigner } from '@cosmjs/proto-signing';

export type Account =
  | { mode: 'none' }
  | { mode: 'watching'; address: string }
  | { mode: 'connected'; address: string; signer: OfflineSigner; wallet: 'keplr' | 'leap' };

/**
 * Whether a string could be an address on this chain.
 *
 * A shape check, not a checksum: the point is to refuse "my wallet" and a
 * pasted transaction hash before they become a query that returns nothing and
 * looks like an empty account. Bech32's alphabet deliberately omits 1, b, i and
 * o, so a lookalike character is caught here rather than three screens later.
 *
 * A wrong-but-well-formed address still gets through, and that is fine —
 * watching one shows an empty account, which is the truth about it.
 */
export function looksLikeAddress(value: string): boolean {
  return /^yml1[023456789acdefghjklmnpqrstuvwxyz]{38}$/.test(value.trim());
}

/** True when this account can put a signature on something. */
export function canSign(account: Account): account is Extract<Account, { mode: 'connected' }> {
  return account.mode === 'connected';
}

/** The address being looked at, in either mode. */
export function addressOf(account: Account): string {
  return account.mode === 'none' ? '' : account.address;
}
