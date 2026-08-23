/**
 * The money abstraction.
 *
 * The chain speaks in base units and denoms: 1500000 uxof. A person speaks in
 * 1 500 CFA. Every number crossing into the interface passes through here, and
 * nothing above this file is allowed to know that a denom exists.
 *
 * Formatting goes through Intl with the active locale, so an Arabic reader sees
 * Eastern Arabic numerals and a French reader sees a comma — the same figure,
 * written the way each of them writes figures.
 */
import { formatMoney, getLocale, toBaseUnits as parseBaseUnits, type BaseUnits } from '@yamale/chain';

export interface Currency {
  /** What the chain calls it. Never rendered. */
  denom: string;
  /** What a person calls it. */
  name: string;
  code: string;
  exponent: number;
}

// The demo set. A production build reads this from the chain's denom metadata
// rather than hardcoding it, but the shape is the same: a person-facing name
// and a machine-facing denom, joined in one place.
export const CURRENCIES: Currency[] = [
  // Generated from scripts/currencies/african-currencies.json. YML leads
  // because every pool pairs against it — cross-currency swaps route through
  // it as the hub. USDC and EURC are the remittance on/off-ramp stablecoins.
  { denom: "uyml", name: "Yamale", code: "YML", exponent: 6 },
  { denom: "uxof", name: "West African CFA franc", code: "XOF", exponent: 6 },
  { denom: "uxaf", name: "Central African CFA franc", code: "XAF", exponent: 6 },
  { denom: "ungn", name: "Nigerian Naira", code: "NGN", exponent: 6 },
  { denom: "ukes", name: "Kenyan Shilling", code: "KES", exponent: 6 },
  { denom: "uzar", name: "South African Rand", code: "ZAR", exponent: 6 },
  { denom: "ughs", name: "Ghanaian Cedi", code: "GHS", exponent: 6 },
  { denom: "uegp", name: "Egyptian Pound", code: "EGP", exponent: 6 },
  { denom: "umad", name: "Moroccan Dirham", code: "MAD", exponent: 6 },
  { denom: "udzd", name: "Algerian Dinar", code: "DZD", exponent: 6 },
  { denom: "utnd", name: "Tunisian Dinar", code: "TND", exponent: 6 },
  { denom: "uetb", name: "Ethiopian Birr", code: "ETB", exponent: 6 },
  { denom: "uugx", name: "Ugandan Shilling", code: "UGX", exponent: 6 },
  { denom: "utzs", name: "Tanzanian Shilling", code: "TZS", exponent: 6 },
  { denom: "urwf", name: "Rwandan Franc", code: "RWF", exponent: 6 },
  { denom: "uzmw", name: "Zambian Kwacha", code: "ZMW", exponent: 6 },
  { denom: "umzn", name: "Mozambican Metical", code: "MZN", exponent: 6 },
  { denom: "uaoa", name: "Angolan Kwanza", code: "AOA", exponent: 6 },
  { denom: "ubwp", name: "Botswana Pula", code: "BWP", exponent: 6 },
  { denom: "unad", name: "Namibian Dollar", code: "NAD", exponent: 6 },
  { denom: "umur", name: "Mauritian Rupee", code: "MUR", exponent: 6 },
  { denom: "ugmd", name: "Gambian Dalasi", code: "GMD", exponent: 6 },
  { denom: "ugnf", name: "Guinean Franc", code: "GNF", exponent: 6 },
  { denom: "ulrd", name: "Liberian Dollar", code: "LRD", exponent: 6 },
  { denom: "usle", name: "Sierra Leonean Leone", code: "SLE", exponent: 6 },
  { denom: "umwk", name: "Malawian Kwacha", code: "MWK", exponent: 6 },
  { denom: "umga", name: "Malagasy Ariary", code: "MGA", exponent: 6 },
  { denom: "ucdf", name: "Congolese Franc", code: "CDF", exponent: 6 },
  { denom: "usdg", name: "Sudanese Pound", code: "SDG", exponent: 6 },
  { denom: "ussp", name: "South Sudanese Pound", code: "SSP", exponent: 6 },
  { denom: "ulyd", name: "Libyan Dinar", code: "LYD", exponent: 6 },
  { denom: "usos", name: "Somali Shilling", code: "SOS", exponent: 6 },
  { denom: "udjf", name: "Djiboutian Franc", code: "DJF", exponent: 6 },
  { denom: "uern", name: "Eritrean Nakfa", code: "ERN", exponent: 6 },
  { denom: "ubif", name: "Burundian Franc", code: "BIF", exponent: 6 },
  { denom: "uscr", name: "Seychellois Rupee", code: "SCR", exponent: 6 },
  { denom: "ucve", name: "Cape Verdean Escudo", code: "CVE", exponent: 6 },
  { denom: "ustn", name: "S\u00e3o Tom\u00e9 and Pr\u00edncipe Dobra", code: "STN", exponent: 6 },
  { denom: "ukmf", name: "Comorian Franc", code: "KMF", exponent: 6 },
  { denom: "ulsl", name: "Lesotho Loti", code: "LSL", exponent: 6 },
  { denom: "uszl", name: "Eswatini Lilangeni", code: "SZL", exponent: 6 },
  { denom: "umru", name: "Mauritanian Ouguiya", code: "MRU", exponent: 6 },
  { denom: "uzwg", name: "Zimbabwe Gold", code: "ZWG", exponent: 6 },
  { denom: "uusdc", name: "USD Coin", code: "USDC", exponent: 6 },
  { denom: "ueurc", name: "Euro Coin", code: "EURC", exponent: 6 },
];

export function currencyOf(denom: string): Currency | undefined {
  return CURRENCIES.find((c) => c.denom === denom);
}

/** Base units to something a person reads. */
export function display(amount: string, denom: string): string {
  const c = currencyOf(denom);
  if (!c) return '';
  return formatMoney(amount, c.exponent) + ' ' + c.code;
}

/**
 * What a person typed, as the chain's base units — or null when it is not an
 * amount at all.
 *
 * Delegated to the SDK rather than reimplemented, because there is one correct
 * answer to "what does this text mean in base units" and three places in these
 * clients used to have three different ones. The SDK version is BigInt
 * throughout and reports when it had to drop decimal places the denom cannot
 * hold, which is the difference between a payment that is nearly right and one
 * that says so.
 *
 * Null rather than a silent zero: a screen can then disable its own button and
 * state the precondition, where a zero would have submitted a payment of
 * nothing on the strength of a typo.
 */
export function parseAmount(input: string, denom: string): BaseUnits | null {
  const c = currencyOf(denom);
  if (!c) return null;
  return parseBaseUnits(input, c.exponent);
}

/**
 * The same, for callers that cannot act on the difference between "zero" and
 * "not a number" — a statement footer, a display round-trip.
 *
 * Kept because several screens read it, but it is the weaker of the two and a
 * new caller should prefer `parseAmount`.
 */
export function toBaseUnits(input: string, denom: string): string {
  return parseAmount(input, denom)?.base ?? '0';
}

/** Locale-aware, for the amount keypad. */
export function groupDigits(raw: string): string {
  if (!raw) return '';
  const [whole, frac] = raw.split('.');
  const grouped = new Intl.NumberFormat(getLocale()).format(BigInt(whole || '0'));
  return frac === undefined ? grouped : grouped + '.' + frac;
}

/**
 * Base units back to the plain decimal a person would type.
 *
 * Deliberately not the localised form: this value goes back into the amount
 * input, which parses with a dot, and handing it a French comma would make
 * "use my whole balance" fail for exactly the people reading in French.
 */
export function rawAmount(amount: string, denom: string): string {
  const c = currencyOf(denom);
  if (!c) return '0';
  const padded = (amount || '0').padStart(c.exponent + 1, '0');
  const whole = padded.slice(0, -c.exponent) || '0';
  const frac = padded.slice(-c.exponent).replace(/0+$/, '');
  return frac ? `${whole}.${frac}` : whole;
}

/**
 * The currency this person thinks in.
 *
 * Held per browser rather than on the chain: it changes nothing about what is
 * owned, only which balance is shown first, and putting it on-chain would mean
 * paying a fee to reorder a screen. West African CFA is the fallback because it
 * is the widest single-currency zone the chain serves, not because it is more
 * important than the others.
 */
const DEFAULT_KEY = 'yamale.app.currency';

export function defaultDenom(): string {
  try {
    const saved = localStorage.getItem(DEFAULT_KEY);
    // Validated on read: a denom saved before a currency was retired would
    // otherwise leave the main account permanently empty.
    if (saved && CURRENCIES.some((c) => c.denom === saved)) return saved;
  } catch {
    // Storage unavailable (private browsing); the fallback is correct.
  }
  return 'uxof';
}

export function setDefaultDenom(denom: string): void {
  try {
    localStorage.setItem(DEFAULT_KEY, denom);
  } catch {
    // Not worth failing a settings change over.
  }
}
