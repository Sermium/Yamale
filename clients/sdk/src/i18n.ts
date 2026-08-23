// One catalogue, every surface. The apps, the site and the docs read the same
// keys, so a term translated once is translated everywhere and a term that
// drifts is visible immediately.
//
// Deliberately not a translation framework. The whole surface is a lookup, an
// interpolation and a plural rule, and a dependency that ships its own loader,
// its own React bindings and its own detection logic buys none of those three.

import { toDisplayAmount } from './denom.ts';

export type Locale = string;

/** Direction is a property of the script, not the country. */
const RTL_LANGUAGES = new Set(['ar', 'he', 'fa', 'ur', 'ku', 'dv']);

export function direction(locale: Locale): 'ltr' | 'rtl' {
  return RTL_LANGUAGES.has(locale.split('-')[0]) ? 'rtl' : 'ltr';
}

export interface LanguageInfo {
  code: Locale;
  /** The language's own name for itself. Never the English name -- a speaker
   *  looking for their language is looking for the word they use. */
  endonym: string;
  english: string;
  tier: 1 | 2;
}

// Tier 1 reaches the official language of roughly fifty of the fifty-four
// member states. Swahili sits here on daily use rather than on status.
// Tier 2 is where people actually count money -- see docs/guides/languages.md.
export const LANGUAGES: LanguageInfo[] = [
  { code: 'en', endonym: 'English',    english: 'English',    tier: 1 },
  { code: 'fr', endonym: 'Français',   english: 'French',     tier: 1 },
  { code: 'ar', endonym: 'العربية',     english: 'Arabic',     tier: 1 },
  { code: 'pt', endonym: 'Português',  english: 'Portuguese', tier: 1 },
  { code: 'sw', endonym: 'Kiswahili',  english: 'Swahili',    tier: 1 },

  { code: 'ha', endonym: 'Hausa',      english: 'Hausa',      tier: 2 },
  { code: 'yo', endonym: 'Yorùbá',     english: 'Yoruba',     tier: 2 },
  { code: 'ig', endonym: 'Igbo',       english: 'Igbo',       tier: 2 },
  { code: 'am', endonym: 'አማርኛ',       english: 'Amharic',    tier: 2 },
  { code: 'om', endonym: 'Afaan Oromoo', english: 'Oromo',    tier: 2 },
  { code: 'so', endonym: 'Soomaali',   english: 'Somali',     tier: 2 },
  { code: 'zu', endonym: 'isiZulu',    english: 'Zulu',       tier: 2 },
  { code: 'xh', endonym: 'isiXhosa',   english: 'Xhosa',      tier: 2 },
  { code: 'sn', endonym: 'chiShona',   english: 'Shona',      tier: 2 },
  { code: 'rw', endonym: 'Kinyarwanda', english: 'Kinyarwanda', tier: 2 },
  { code: 'mg', endonym: 'Malagasy',   english: 'Malagasy',   tier: 2 },
  { code: 'wo', endonym: 'Wolof',      english: 'Wolof',      tier: 2 },
  { code: 'bm', endonym: 'Bamanankan', english: 'Bambara',    tier: 2 },
  { code: 'ff', endonym: 'Fulfulde',   english: 'Fula',       tier: 2 },
  { code: 'ak', endonym: 'Akan',       english: 'Akan/Twi',   tier: 2 },
];

export type Catalogue = Record<string, string>;

const catalogues = new Map<Locale, Catalogue>();
let active: Locale = 'en';

export function register(locale: Locale, catalogue: Catalogue): void {
  catalogues.set(locale, { ...(catalogues.get(locale) ?? {}), ...catalogue });
}

/**
 * Everyone who needs to re-render when the language changes.
 *
 * The picker used to call location.reload() instead, on the reasoning that
 * `dir` is a document-level property and a reload was more honest than a
 * half-mirrored screen. The reasoning was fine and the consequence was not:
 * this app holds the unlocked signer in memory and deliberately never persists
 * it, so a reload is a sign-out. Choosing a language logged people out.
 *
 * setLocale already writes `lang` and `dir` on the document itself, so the
 * mirroring happens without a reload — all that was missing was telling React.
 */
const listeners = new Set<() => void>();

export function onLocaleChange(listener: () => void): () => void {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

export function setLocale(locale: Locale): void {
  active = locale;
  if (typeof document !== 'undefined') {
    // Both, always. `dir` drives every logical property in the stylesheets;
    // `lang` drives font selection, hyphenation and the screen reader's voice.
    document.documentElement.lang = locale;
    document.documentElement.dir = direction(locale);
  }
  if (typeof localStorage !== 'undefined') {
    try { localStorage.setItem('yamale.locale', locale); } catch { /* private mode */ }
  }
  // Last, so a listener that reads getLocale() sees the new value.
  for (const listener of listeners) listener();
}

export function getLocale(): Locale {
  return active;
}

/**
 * Explicit choice, then device, then English.
 *
 * Never IP. A Senegalese banker in London reads Wolof or French, and guessing
 * English from an address is both wrong and slightly insulting.
 */
export function resolveLocale(available: Locale[] = ['en']): Locale {
  const has = (c: string) => available.find(a => a === c || a.split('-')[0] === c);

  if (typeof localStorage !== 'undefined') {
    try {
      const saved = localStorage.getItem('yamale.locale');
      const hit = saved && has(saved.split('-')[0]);
      if (hit) return hit;
    } catch { /* ignore */ }
  }
  if (typeof navigator !== 'undefined') {
    for (const pref of navigator.languages ?? [navigator.language]) {
      const hit = pref && has(pref.split('-')[0]);
      if (hit) return hit;
    }
  }
  return 'en';
}

/**
 * Look up `key`, interpolating {named} placeholders.
 *
 * A missing key returns the key itself rather than an empty string: a screen
 * reading `wallet.send.confirm` is obviously broken, while a screen with a
 * blank button looks deliberate and ships.
 */
export function t(key: string, vars?: Record<string, string | number>): string {
  const cat = catalogues.get(active) ?? catalogues.get(active.split('-')[0]);
  let s = cat?.[key] ?? catalogues.get('en')?.[key] ?? key;
  if (vars) {
    for (const [k, v] of Object.entries(vars)) {
      s = s.split(`{${k}}`).join(String(v));
    }
  }
  return s;
}

/**
 * Plural selection through the platform's own CLDR rules.
 *
 * Arabic has six forms and most Bantu languages have noun-class agreement that
 * plural rules do not model at all, so the count and the noun are never
 * concatenated in code -- the catalogue carries the whole phrase per form.
 */
export function plural(key: string, count: number, vars?: Record<string, string | number>): string {
  const rule = new Intl.PluralRules(active).select(count);
  const cat = catalogues.get(active) ?? catalogues.get('en');
  const chosen = cat?.[`${key}.${rule}`] !== undefined ? `${key}.${rule}` : `${key}.other`;
  return t(chosen, { count, ...vars });
}

/**
 * Money is locale data, never string concatenation. Decimal separators, digit
 * grouping, symbol placement and numeral system all vary, and Arabic locales
 * may render Eastern Arabic digits.
 *
 * `amount` is in base units -- the chain has no other kind -- so the exponent
 * is required rather than assumed.
 */
export function formatMoney(amount: bigint | string, exponent: number, currency?: string): string {
  let raw: bigint;
  try {
    raw = typeof amount === 'bigint' ? amount : BigInt((amount ?? '').toString().trim() || '0');
  } catch {
    return String(amount ?? '');
  }

  // A currency style asks Intl for a symbol and a symbol's placement, which is
  // locale data this function has no business reimplementing. It is also the
  // only branch that needs a Number, and it is used for quoted prices rather
  // than for ledger balances.
  if (currency) {
    return new Intl.NumberFormat(active, {
      minimumFractionDigits: 0,
      maximumFractionDigits: exponent,
      style: 'currency',
      currency,
    }).format(Number(raw) / Math.pow(10, exponent));
  }

  return formatExactDecimal(toDisplayAmount(raw, exponent), active, exponent);
}

/**
 * A decimal string, grouped and punctuated for a locale, with no float in the
 * path.
 *
 * The integer part is handed to Intl as a BigInt — exact at any size — and the
 * fraction is formatted as its own integer with `minimumIntegerDigits`, which
 * is what carries it into the locale's numbering system (Eastern Arabic digits
 * for ar-EG, Latin for ar-SA) instead of being concatenated as ASCII. The
 * decimal separator is read out of the locale rather than assumed, because half
 * the languages this ships in write it as a comma.
 *
 * This replaces `Number(base) / 10 ** exponent`. Above 2^53 that expression
 * silently misstates a balance, and a misstated balance is the one defect this
 * whole layer exists to prevent.
 */
function formatExactDecimal(decimal: string, locale: Locale, maxFraction: number): string {
  const negative = decimal.startsWith('-');
  const unsigned = negative ? decimal.slice(1) : decimal;
  const [whole = '0', fraction = ''] = unsigned.split('.');

  const kept = fraction.slice(0, maxFraction).replace(/0+$/, '');

  let out: string;
  try {
    out = new Intl.NumberFormat(locale).format(BigInt(whole || '0'));
    if (kept !== '') {
      const separator =
        new Intl.NumberFormat(locale).formatToParts(1.1).find((p) => p.type === 'decimal')?.value ?? '.';
      const digits = new Intl.NumberFormat(locale, {
        useGrouping: false,
        minimumIntegerDigits: kept.length,
      }).format(BigInt(kept));
      out = `${out}${separator}${digits}`;
    }
  } catch {
    out = kept === '' ? whole : `${whole}.${kept}`;
  }

  // The sign from the locale, for the same reason as the separator: a minus is
  // written differently in several of them, and Intl already knows which.
  if (!negative) return out;
  const minus =
    new Intl.NumberFormat(locale).formatToParts(-1).find((p) => p.type === 'minusSign')?.value ?? '-';
  return `${minus}${out}`;
}

export function formatDate(d: Date | string | number): string {
  return new Intl.DateTimeFormat(active, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(d));
}
