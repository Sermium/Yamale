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
import { fromBech32 } from '@cosmjs/encoding';
import type { OfflineSigner } from '@cosmjs/proto-signing';

export type Account =
  | { mode: 'none' }
  | { mode: 'watching'; address: string }
  | { mode: 'connected'; address: string; signer: OfflineSigner; wallet: 'keplr' | 'leap' };

/**
 * Whether a string is an address on this chain — checksum and all.
 *
 * This started as a shape regex fixed at 42 characters, and it was wrong twice
 * over. A 42-character rule rejects every module and x/group account on the
 * chain, which are 32-byte and therefore 62 characters: the registry offices
 * this app displays on every land vehicle are exactly that shape, so watching
 * one would have been refused. And a shape rule accepts a mistyped address
 * whose checksum is broken — which the node answers with a 400, which this app
 * then honestly reports as "the chain did not answer". Truthful, and useless:
 * the chain answered perfectly well, and the problem was the typing.
 *
 * bech32 exists to catch exactly that transposition, and the decoder is already
 * in the dependency tree. So the check is the real one, and the length is
 * whatever the encoding says rather than whatever this file guessed.
 *
 * Prefix-checked as well: `ymlvaloper1…` decodes cleanly and is a validator
 * operator, not an account that can hold shares.
 */
export function looksLikeAddress(value: string): boolean {
  try {
    return fromBech32(value.trim()).prefix === 'yml';
  } catch {
    return false;
  }
}

/** True when this account can put a signature on something. */
export function canSign(account: Account): account is Extract<Account, { mode: 'connected' }> {
  return account.mode === 'connected';
}

/** The address being looked at, in either mode. */
export function addressOf(account: Account): string {
  return account.mode === 'none' ? '' : account.address;
}
