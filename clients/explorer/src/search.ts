/**
 * One box that accepts whatever is in somebody's clipboard.
 *
 * Asking a person to choose "address / transaction / block" from a dropdown
 * before searching asks them to already know the answer. Worse, the previous
 * version accepted only those three and told everybody else "that does not look
 * like an account, a transaction or a block number" — which was true and
 * useless, because the things it rejected were a Yamale user ID (the identifier
 * this chain issues to *people*, and the only one a citizen has ever seen), a
 * currency code, and a validator's name.
 *
 * Classification is deliberately ordered from unambiguous to contextual. The
 * first four kinds are decidable from the string alone, so they resolve without
 * a round trip; a denom and a validator need the chain's own lists, which the
 * caller passes in when it has them.
 */

// Leaf imports: see amount.ts for why not the barrel.
import { normaliseUserId, validUserId } from '../../sdk/src/alias.ts';
import type { DenomInfo } from '../../sdk/src/denom.ts';

export type SearchKind =
  | 'empty'
  | 'height'
  | 'tx'
  | 'address'
  | 'userId'
  | 'denom'
  | 'validator'
  | 'unknown';

export interface SearchGuess {
  kind: SearchKind;
  /**
   * The term in the form the rest of the app wants: a hash upper-cased, a user
   * ID normalised and hyphenated, a denom resolved to its base unit, a
   * validator resolved to its operator address.
   */
  value: string;
  /** What the term was recognised as, for the "showing results for" line. */
  label?: string;
}

export interface SearchContext {
  /** Denom metadata from the chain, so a currency added after launch is findable. */
  registry?: Record<string, DenomInfo>;
  /** Validator monikers by operator address. */
  monikers?: Record<string, string>;
}

/**
 * A bech32 account address on this chain.
 *
 * Deliberately not a full bech32 checksum: a mistyped address should reach the
 * account page and be told "nothing here" rather than be rejected by the search
 * box, because "no account" and "not an address" send somebody looking in two
 * different places and only one of them is right.
 */
const ADDRESS = /^yml1[02-9ac-hj-np-z]{38,}$/;
const VALOPER = /^ymlvaloper1[02-9ac-hj-np-z]{38,}$/;
const TX_HASH = /^[0-9A-Fa-f]{64}$/;
const HEIGHT = /^\d{1,12}$/;

export function classifySearch(term: string, ctx: SearchContext = {}): SearchGuess {
  const value = term.trim();
  if (!value) return { kind: 'empty', value: '' };

  if (HEIGHT.test(value)) return { kind: 'height', value, label: 'block' };
  if (TX_HASH.test(value)) return { kind: 'tx', value: value.toUpperCase(), label: 'transaction' };
  if (VALOPER.test(value)) return { kind: 'validator', value, label: 'validator' };
  if (ADDRESS.test(value.toLowerCase())) {
    return { kind: 'address', value: value.toLowerCase(), label: 'account' };
  }

  // A user ID carries its own check character, so this is a real answer rather
  // than a shape match: "NG-K3M9-7QRT-B" typed wrong is rejected here instead of
  // going to the node and coming back "not found", which reads to a person as
  // "that account does not exist" rather than "you typed it wrong".
  const id = normaliseUserId(value);
  if (validUserId(id)) return { kind: 'userId', value: id, label: 'user ID' };

  const denom = matchDenom(value, ctx.registry);
  if (denom) return { kind: 'denom', value: denom, label: 'currency' };

  const validator = matchValidator(value, ctx.monikers);
  if (validator) return { kind: 'validator', value: validator, label: 'validator' };

  return { kind: 'unknown', value };
}

/**
 * A currency, by the code the chain stores or the one people write.
 *
 * `uyml` and `YML` are the same currency, and nobody outside this repository
 * types the first one.
 */
export function matchDenom(
  term: string,
  registry: Record<string, DenomInfo> | undefined,
): string | null {
  if (!registry) return null;
  if (registry[term]) return term;

  const wanted = term.toUpperCase();
  for (const info of Object.values(registry)) {
    if (info.base.toUpperCase() === wanted) return info.base;
    if (info.symbol.toUpperCase() === wanted) return info.base;
  }
  return null;
}

/**
 * A validator, by moniker.
 *
 * Exact match first, then a unique prefix, then a unique substring. Ambiguity
 * returns nothing rather than picking one: sending somebody to the wrong
 * validator's page is worse than telling them the name was not specific enough.
 */
export function matchValidator(
  term: string,
  monikers: Record<string, string> | undefined,
): string | null {
  if (!monikers) return null;
  const wanted = term.trim().toLowerCase();
  if (!wanted) return null;

  const entries = Object.entries(monikers);
  const exact = entries.filter(([, name]) => name.toLowerCase() === wanted);
  if (exact.length === 1) return exact[0]![0];

  const prefix = entries.filter(([, name]) => name.toLowerCase().startsWith(wanted));
  if (prefix.length === 1) return prefix[0]![0];

  const inside = entries.filter(([, name]) => name.toLowerCase().includes(wanted));
  if (inside.length === 1) return inside[0]![0];

  return null;
}
