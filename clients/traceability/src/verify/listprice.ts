/** "17,678.00 USD": the amount in the reader's own digits, then the code. */

import type { ListPrice } from './record.ts';

export function listPriceText(lp: ListPrice, locale: string): string {
  const n = Number(lp.amount);
  const amount = new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(n);
  return `${amount} ${lp.currency}`;
}

/** The day alone, as the record states it: no time, and never shifted by the viewer's timezone. */
export function listPriceDay(at: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(`${at}T00:00:00Z`));
}
