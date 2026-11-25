/**
 * The jewelers' directory, outside React so it can be tested: which shops are
 * listed, in what order, and what a search matches.
 */

import type { Contact } from '../api.ts';

export interface Listed { id: string; name: string; contact?: Contact; logo?: string; colour?: string }

const collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true });

/** Lower case, accents and marks removed, so "Thiès" is found by "thies". */
export function fold(s: string): string {
  return s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
}

/** Every word of the query must appear in the name, the city or the country (code or name). */
export function matches(shop: Listed, query: string, countryName: (code: string) => string = (c) => c): boolean {
  const words = fold(query).split(/\s+/).filter(Boolean);
  if (words.length === 0) return true;
  const c = shop.contact;
  const hay = fold([shop.name, c?.city ?? '', c?.country ?? '', c?.country ? countryName(c.country) : ''].join(' '));
  return words.every((w) => hay.includes(w));
}

/** By name, as people read them; ties by id, so the order never shuffles between loads. */
export function sortListed(shops: Listed[]): Listed[] {
  return [...shops].sort((a, b) => collator.compare(a.name, b.name) || collator.compare(a.id, b.id));
}

/** A WhatsApp chat link: digits only, as wa.me wants them. */
export function whatsappLink(number: string): string {
  return `https://wa.me/${number.replace(/[^0-9]/g, '')}`;
}

/** A tel: link keeps the leading + and the digits. */
export function telLink(number: string): string {
  return `tel:${number.replace(/[^0-9+]/g, '')}`;
}

/** The country's name in the reader's language, or the code when the browser cannot name it. */
export function countryNamer(locale: string): (code: string) => string {
  try {
    const names = new Intl.DisplayNames([locale], { type: 'region' });
    return (code) => { try { return names.of(code) ?? code; } catch { return code; } };
  } catch {
    return (code) => code;
  }
}
