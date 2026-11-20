/** "17,678.00 USD": the amount in the reader's own digits, then the code. */

import type { ListPrice } from './record.ts';

export function listPriceText(lp: ListPrice, locale: string): string {
  const n = Number(lp.amount);
  const amount = new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(n);
  return `${amount} ${lp.currency}`;
}
