/**
 * Amounts, at the display boundary, without ever touching a float.
 *
 * The chain stores money in base units and nothing else: `1250500000uyml`. A
 * person reads `1,250.50 YML`. This module is the only place in the explorer
 * where that conversion happens, and it does it entirely in strings and
 * BigInt.
 *
 * Why not just use the SDK's `formatAmount`? Because its last step is
 * `groupDigits`, which inserts thousands separators via
 * `Number(digits).toLocaleString()`. That is a double. A `uyml` balance above
 * 9,007,199,254,740,991 base units — nine billion YML, which is not an
 * unusual total supply for a national payments network — comes back with the
 * wrong digits at the end, silently, with no error and no rounding notice.
 * Grouping is a text operation on a decimal string and never needs arithmetic
 * at all, so this one does it with a regex over the digits.
 *
 * The exponent is *never* guessed. An unknown denom is shown in its base unit
 * with the unit named as-is, because inventing a factor of a million is worse
 * than showing somebody a raw symbol they can look up. `unknownDenom` says so
 * to the caller, and the interface says so to the reader.
 */

// Reached at the leaf rather than through `@yamale/chain`. The package's entry
// point re-exports two `.tsx` modules, and Node's type-stripping test runner
// cannot load JSX — so importing the barrel here would make every module that
// depends on this one untestable without adding a bundler to the test path.
// The exponent rule is the thing most worth a test in the whole explorer, so
// the import goes where the rule lives.
import {
  KNOWN_DENOMS,
  resolveDenom,
  toDisplayAmount,
  type Coin,
  type DenomInfo,
} from '../../sdk/src/denom.ts';

export type DenomRegistry = Record<string, DenomInfo>;

export interface DisplayAmount {
  /** What to print: grouped, rounded to the currency's own minor units. */
  value: string;
  /** The unit people read: `YML`, `NGN`, or the raw denom when unknown. */
  symbol: string;
  /** Every decimal the chain holds, grouped: the reveal behind the rounded one. */
  exact: string;
  /** The base-unit integer, untouched, for the raw disclosure. */
  base: string;
  /** The base denom, untouched. */
  denom: string;
  /** True when `value` dropped decimals that `exact` still carries. */
  rounded: boolean;
  /** True when no metadata named this denom, so no exponent was applied. */
  unknownDenom: boolean;
}

/**
 * Thousands separators, by text.
 *
 * Operates on the integer part of an already-decimal string. No Number(), no
 * toLocaleString(), no arithmetic — so a 40-digit integer groups exactly as
 * well as a 4-digit one.
 */
export function groupInteger(digits: string): string {
  const negative = digits.startsWith('-');
  const body = negative ? digits.slice(1) : digits;
  if (!/^\d*$/.test(body)) return digits;
  return (negative ? '-' : '') + body.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

/** Thousands separators over a whole decimal string, leaving the fraction alone. */
export function groupDecimal(value: string): string {
  const [whole, fraction] = value.split('.');
  const grouped = groupInteger(whole ?? '0');
  return fraction ? `${grouped}.${fraction}` : grouped;
}

/**
 * Cuts a decimal string to at most `places` decimals, downward.
 *
 * Truncation rather than rounding, deliberately: a displayed balance must never
 * be larger than the real one, because somebody acting on a rounded-up figure
 * has their transaction rejected and no way to see why.
 */
export function truncateDecimals(value: string, places: number): string {
  const [whole, fraction] = value.split('.');
  if (!fraction || places < 0) return whole ?? '0';
  if (places === 0) return whole ?? '0';
  const kept = fraction.slice(0, places).replace(/0+$/, '');
  return kept ? `${whole}.${kept}` : (whole ?? '0');
}

/**
 * A base-unit amount, ready to print.
 *
 * `minorUnits` decides the rounding: six decimals is how this chain *stores* a
 * balance, not how anybody reads one. `1,359.844414 NGN` is a ledger entry;
 * `1,359.84 NGN` is a price; and a CFA franc printed to six places looks broken
 * to every person who spends them.
 */
export function displayAmount(
  base: string,
  denom: string,
  registry: DenomRegistry = KNOWN_DENOMS,
): DisplayAmount {
  const info = resolveDenom(denom, registry);
  const unknownDenom = !(denom in registry) && info.exponent === 0 && info.symbol === denom;

  const full = toDisplayAmount(base, info.exponent);
  const places = info.minorUnits;
  const cut = places === undefined ? full : truncateDecimals(full, places);

  return {
    value: groupDecimal(cut),
    symbol: info.symbol,
    exact: groupDecimal(full),
    base: String(base),
    denom,
    rounded: cut !== full,
    unknownDenom,
  };
}

/** `1,250.50 YML`. The one-liner for a row that has room for a single figure. */
export function formatAmount(base: string, denom: string, registry?: DenomRegistry): string {
  const a = displayAmount(base, denom, registry);
  return `${a.value} ${a.symbol}`;
}

/**
 * A bag of coins as a phrase: `1,250.50 YML and 40 USD`.
 *
 * Empty returns the empty string rather than the word "nothing": a row with no
 * amount should show no amount, and printing "nothing" in the column where
 * money goes reads as a balance of zero, which is a different claim.
 */
export function formatCoinList(
  coins: Coin[] | undefined | null,
  registry?: DenomRegistry,
): string {
  if (!coins || coins.length === 0) return '';
  const parts = coins.map((c) => formatAmount(c.amount, c.denom, registry));
  if (parts.length === 1) return parts[0]!;
  return `${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}`;
}

/**
 * One base-unit quantity as a fraction of another.
 *
 * Exists because the SDK's `bondedRatio` divides by ten thousand in integer
 * arithmetic *before* converting to a number, so every share is quantised to
 * four decimal places. On this chain that turns a bonded share of 0.0175% into
 * 0.010% — a figure wrong by a factor of two, printed to three decimals it does
 * not have. Both numbers are small; the difference between them is the answer to
 * "is this chain's stake meaningfully bonded".
 *
 * BigInt for the division, so the numerator can be any size, and one Number at
 * the end where the value is guaranteed small.
 */
export function shareOf(part: string, whole: string, digits = 8): number | null {
  let numerator: bigint;
  let denominator: bigint;
  try {
    numerator = BigInt(part);
    denominator = BigInt(whole);
  } catch {
    return null;
  }
  if (denominator === 0n) return null;

  const scale = 10n ** BigInt(digits);
  return Number((numerator * scale) / denominator) / Number(scale);
}

/**
 * Adds up coins by denom, in base units.
 *
 * BigInt because that is the only kind of addition that is safe here, and
 * because a total is exactly where a float error compounds instead of
 * cancelling.
 */
export function sumCoins(coins: Coin[]): Coin[] {
  const totals = new Map<string, bigint>();
  for (const c of coins) {
    let amount: bigint;
    try {
      amount = BigInt(c.amount);
    } catch {
      continue;
    }
    totals.set(c.denom, (totals.get(c.denom) ?? 0n) + amount);
  }
  return [...totals].map(([denom, amount]) => ({ denom, amount: amount.toString() }));
}
