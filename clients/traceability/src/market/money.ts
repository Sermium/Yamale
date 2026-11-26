import { formatAmount } from '@yamale/chain';

/** Where money is added to, or taken out of, an account. */
export const PAY_APP = 'https://pay.yamaletech.com';

const ACTIVE = 'yamale.purse.denom';

/** The currency chosen in the purse: the one a sale is priced in by default. */
export function activeDenom(): string | null {
  try { return localStorage.getItem(ACTIVE); } catch { return null; }
}

export function setActiveDenom(denom: string): void {
  try { localStorage.setItem(ACTIVE, denom); } catch { /* private mode: chosen for this page only */ }
}

/** A price as people read it: `1 200 USD`, rounded the way the currency is quoted. */
export function money(c: { denom: string; amount: string }): string {
  return formatAmount(c.amount, c.denom);
}

/** How much more of a currency an account needs, in base units; 0 when it has enough. */
export function shortfall(price: string, balance: string): bigint {
  const d = BigInt(price) - BigInt(balance);
  return d > 0n ? d : 0n;
}
