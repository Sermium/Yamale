/**
 * Changing one currency into another.
 *
 * The chain does this with liquidity pools, and a person does not need to know
 * that. What they need to know is three things, in their own words: what they
 * are giving, roughly what they will get, and the floor below which the deal is
 * refused. Everything else — pool ids, reserves, the constant-product formula,
 * the word "slippage" — stays in this file.
 *
 * The floor is the part that must never be quietly handled for somebody. A
 * quote is computed from reserves that can move before the transaction lands,
 * and a swap sent without a minimum is a swap that can be sandwiched for
 * whatever the market will bear. So the interface always shows the guaranteed
 * worst case next to the estimate, in money, not as a percentage.
 */
import { CURRENCIES, currencyOf } from './money.ts';

export interface Pool {
  id: string;
  denomA: string;
  denomB: string;
  reserveA: bigint;
  reserveB: bigint;
  /** Fee in basis points, taken by the pool from the input. */
  feeBps: number;
}

export async function pools(): Promise<Pool[]> {
  try {
    const res = await fetch('/api/rest/yamale/blockchain/amm/v1/pool');
    if (!res.ok) return [];
    const json = await res.json();
    return (json.pool ?? json.pools ?? []).map((p: Record<string, string>) => ({
      id: String(p.id),
      denomA: p.denom_a ?? p.denomA,
      denomB: p.denom_b ?? p.denomB,
      reserveA: BigInt(p.reserve_a ?? p.reserveA ?? '0'),
      reserveB: BigInt(p.reserve_b ?? p.reserveB ?? '0'),
      feeBps: Number(p.fee_bps ?? p.feeBps ?? 30),
    })).filter((p: Pool) => p.reserveA > 0n && p.reserveB > 0n);
  } catch {
    return [];
  }
}

/** The pool that can trade this pair, in either direction. */
export function findPool(list: Pool[], from: string, to: string): Pool | undefined {
  return list.find(
    (p) => (p.denomA === from && p.denomB === to) || (p.denomB === from && p.denomA === to),
  );
}

/**
 * What comes out, by the constant-product rule the chain applies.
 *
 * Written as `reserveOut * amountInAfterFee / (reserveIn + amountInAfterFee)`
 * rather than the algebraically identical subtraction form. In integer
 * arithmetic those two round in opposite directions, and the other one rounds
 * the payout *up* — which quotes a person more than the chain will pay and
 * makes every swap look like it failed by one unit.
 */
export function quote(pool: Pool, from: string, amountIn: bigint): bigint {
  if (amountIn <= 0n) return 0n;
  const forward = pool.denomA === from;
  const reserveIn = forward ? pool.reserveA : pool.reserveB;
  const reserveOut = forward ? pool.reserveB : pool.reserveA;

  const afterFee = (amountIn * BigInt(10_000 - pool.feeBps)) / 10_000n;
  if (afterFee <= 0n) return 0n;
  return (reserveOut * afterFee) / (reserveIn + afterFee);
}

/**
 * The floor, one percent below the estimate.
 *
 * Not configurable in this interface, deliberately. A number most people cannot
 * evaluate is a number they will set wrong, and the two ways to get it wrong
 * are losing money and having every swap refused. One percent is generous
 * enough that ordinary movement does not cancel the trade and tight enough that
 * nothing dramatic can happen inside it.
 */
export function floorFor(estimate: bigint): bigint {
  return (estimate * 99n) / 100n;
}

/** Currencies this account can actually change, given the pools that exist. */
export function tradable(list: Pool[]): string[] {
  const denoms = new Set<string>();
  for (const p of list) {
    if (currencyOf(p.denomA)) denoms.add(p.denomA);
    if (currencyOf(p.denomB)) denoms.add(p.denomB);
  }
  return CURRENCIES.filter((c) => denoms.has(c.denom)).map((c) => c.denom);
}

/**
 * How far this trade moves the price, in basis points.
 *
 * The spot rate is the reserve ratio — what an infinitely small trade would
 * get. The execution rate is what this trade actually gets. The gap between
 * them is not a fee and is not the pool cheating: it is the cost of being large
 * relative to the liquidity, and it grows with the square of the trade.
 *
 * Worth showing because the cause is actionable. A person seeing 4% on a modest
 * amount is not looking at a broken quote, they are looking at a thin pool —
 * and the answer is either a smaller trade or more liquidity, not a different
 * app.
 */
export function priceImpactBps(pool: Pool, from: string, amountIn: bigint, amountOut: bigint): number {
  if (amountIn <= 0n || amountOut <= 0n) return 0;
  const forward = pool.denomA === from;
  const reserveIn = forward ? pool.reserveA : pool.reserveB;
  const reserveOut = forward ? pool.reserveB : pool.reserveA;
  if (reserveIn <= 0n || reserveOut <= 0n) return 0;

  // Scaled integer arithmetic throughout: a float here would disagree with the
  // chain in the last digits, and a quote that disagrees with settlement is
  // worse than no quote.
  const SCALE = 1_000_000n;
  const spot = (reserveOut * SCALE) / reserveIn;          // out per in, scaled
  const executed = (amountOut * SCALE) / amountIn;
  if (spot <= 0n) return 0;

  const bps = ((spot - executed) * 10_000n) / spot;
  return Number(bps < 0n ? 0n : bps);
}

/** Depth of the side being sold into, for explaining a large impact. */
export function depthOf(pool: Pool, from: string): bigint {
  return pool.denomA === from ? pool.reserveA : pool.reserveB;
}
