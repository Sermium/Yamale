import { formatAmount } from '@yamale/chain';

/** A price as people read it: `1 200 USD`, rounded the way the currency is quoted. */
export function money(c: { denom: string; amount: string }): string {
  return formatAmount(c.amount, c.denom);
}

/** How much more of a currency an account needs, in base units; 0 when it has enough. */
export function shortfall(price: string, balance: string): bigint {
  const d = BigInt(price) - BigInt(balance);
  return d > 0n ? d : 0n;
}
