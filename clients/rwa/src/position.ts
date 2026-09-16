/**
 * What a holding cost, and what is accruing on it right now.
 *
 * Two facts the chain added today and one it still does not hold. Everything
 * here is BigInt over the chain's own integers, for the reason vehicle.ts gives:
 * these figures are computed in a keeper by integer arithmetic, and the only
 * way an interface can promise one is to do the same arithmetic on the same
 * integers.
 *
 * The three rules this file exists to enforce, all of them from x/tokenisation's
 * own comments:
 *
 *   1. `cost_qty == 0` is COST NOT RECORDED and never cost zero. Cost zero
 *      reports an infinite return, "the most flattering and most wrong figure
 *      this module could produce".
 *   2. A holding can be PARTLY covered, and a known cost is never averaged with
 *      an unknown one. The covered part reports a return; the rest is stated as
 *      uncovered and left alone.
 *   3. Income accruing in the CURRENT period is token-seconds, not money. Its
 *      rate is unknown until the period closes, and converting it at the last
 *      period's rate would put a forecast beside a settled figure in the same
 *      units, where nobody can tell them apart.
 */

/* ------------------------------------------------------------ chain shapes */

/** blockchain.tokenisation.v1.Position, as this app reads it. */
export interface Position {
  assetId: string;
  holder: string;
  /** Settled but not yet withdrawn, in the vault's denomination. */
  accrued: string;
  /** What this holder paid for the tokens `costQty` covers. */
  costBasis: string;
  costQty: string;
  /** Token-seconds accrued in ONE period and not yet turned into money. */
  tokenSeconds: string;
  /** Which period those seconds belong to. */
  tokenSecondsPeriod: string;
  /** Unix seconds: when this position last accrued. */
  lastTouch: number;
}

/**
 * A chain integer as a BigInt, with the absent case folded into zero.
 *
 * `math.Int`'s zero value is nil on the chain and decodes to an empty string
 * here, so every one of these fields arrives as '' on a position written before
 * the field existed. The keeper has `zeroIfNil` for exactly this and for the
 * same reason: the alternative is a thrown BigInt inside a render path, which
 * takes the page with it.
 *
 * Folding to zero is safe ONLY because no caller here treats a zero cost as a
 * cost — see `costOf`, where zero quantity is the "not recorded" answer rather
 * than a number to divide by.
 */
export function int(value: string | number | undefined | null): bigint {
  if (value === undefined || value === null) return 0n;
  const text = String(value).trim();
  if (!/^-?\d+$/.test(text)) return 0n;
  return BigInt(text);
}

/* ========================================================================== */
/*  What it cost                                                              */
/* ========================================================================== */

export type CostState =
  /** No basis at all: the tokens arrived without a price the chain could record. */
  | 'none'
  /** A basis covers every token held. */
  | 'full'
  /** A basis covers part of the holding; the rest arrived untracked. */
  | 'partial';

export interface Cost {
  state: CostState;
  /** What was paid, in the vault's income denomination. Base units. */
  basis: string;
  /** How many tokens that basis covers. Never more than the balance. */
  covered: string;
  /** Tokens held with no recorded cost. Never mixed into the average. */
  uncovered: string;
  /**
   * True when a cost IS recorded and it is zero.
   *
   * Distinct from `state === 'none'` and the distinction matters: a free
   * allocation recorded as a subscription at a price of nothing is a real,
   * deliberate fact, while an absent basis is the chain not knowing. Neither
   * may produce a return percentage, and they need different sentences.
   */
  zeroCost: boolean;
}

/*
 * There is deliberately no cost-per-token here.
 *
 * It is the obvious fourth field and it cannot be rendered honestly. A basis is
 * in the income denomination's base units and a holding is millions of tokens,
 * so the quotient is routinely below one base unit -- 500,000 ucdf over
 * 5,000,000 tokens is a tenth of a ucdf -- and every formatter on this surface
 * would print that as 0.00. A cost per token of zero is the same lie as a cost
 * of zero, arrived at by rounding instead of by omission.
 *
 * It would also have nothing to be compared against: this console shows no
 * price per token anywhere, on purpose, because nothing on this chain knows
 * what a token is worth until a sale is reported. A unit cost with no unit
 * price beside it answers no question a reader has.
 */

/**
 * What a holding cost, split into the part the chain can vouch for and the part
 * it cannot.
 *
 * `covered` is clamped to the balance rather than trusted. RecordSubscription
 * refuses a basis covering more tokens than the account holds, and CarryBasis
 * reduces both sides proportionally, so the invariant should hold — but a
 * position read at one height beside a bank balance read at another can
 * disagree by a transfer, and clamping is what stops that transient showing a
 * negative uncovered figure.
 */
export function costOf(position: Position | null, balance: string): Cost {
  const held = int(balance);
  const basis = int(position?.costBasis);
  const qty = int(position?.costQty);

  // The headline rule. A quantity of zero means nobody ever recorded what these
  // tokens cost, and the only honest rendering is that sentence -- never a cost
  // of zero, which divides into an infinite return.
  if (qty <= 0n) {
    return {
      state: 'none',
      basis: '0',
      covered: '0',
      uncovered: (held > 0n ? held : 0n).toString(),
      zeroCost: false,
    };
  }

  const covered = qty > held ? held : qty;
  const uncovered = held > covered ? held - covered : 0n;

  // A basis recorded against a quantity, where the basis is zero. Real and not
  // the same as unrecorded: somebody was given these. It still yields no
  // percentage, because every return against a cost of nothing is infinite.
  const zeroCost = basis <= 0n;

  return {
    state: uncovered > 0n ? 'partial' : 'full',
    basis: basis.toString(),
    covered: covered.toString(),
    uncovered: uncovered.toString(),
    zeroCost,
  };
}

/* ========================================================================== */
/*  What it returned                                                          */
/* ========================================================================== */

export interface ReturnView {
  /**
   * Income attributable to the covered tokens, in the vault's denomination.
   * Base units. Null when there is no cost to measure it against.
   */
  income: string | null;
  /** Income over cost, as a percentage. Null whenever it would be meaningless. */
  percent: number | null;
  /**
   * Why there is no percentage, as a catalogue key. Null when there is one.
   *
   * Every branch that refuses to produce a figure says why, because "—" in a
   * return column reads as "zero" to most people and as "broken" to the rest.
   */
  whyKey: string | null;
}

/**
 * What the covered part of a holding has returned, on the only basis the chain
 * can support.
 *
 * # The numerator, and what it is not
 *
 * `owed` is Query/Entitlement: everything this holder could withdraw right now,
 * settled and unsettled. It is split to the covered tokens by
 * `owed * covered / balance`, and that split is EXACT rather than an
 * approximation -- every token inside one position carries the same last_index
 * and the same accrual, so income within a position is strictly proportional to
 * token count.
 *
 * What it is not is a lifetime return. MsgClaim sets `accrued` to zero and pays
 * out, and the chain keeps no running total of what an account has withdrawn.
 * So for a holder who has ever claimed, this is a FLOOR: it counts what is still
 * there and cannot count what was taken. The interface has to say so — see
 * `rwa.cost.returnFloor` — because a floor presented as a return is a lie in the
 * one direction that looks like modesty.
 *
 * Truncated toward zero, like every division in x/tokenisation, so the figure
 * quoted is never larger than what the chain would actually pay.
 */
export function returnOn(cost: Cost, owed: string | null, balance: string): ReturnView {
  if (cost.state === 'none') {
    return { income: null, percent: null, whyKey: 'rwa.cost.noBasis' };
  }
  if (cost.zeroCost) {
    return { income: null, percent: null, whyKey: 'rwa.cost.zeroBasis' };
  }
  if (owed === null) {
    return { income: null, percent: null, whyKey: 'rwa.cost.owedUnknown' };
  }

  const held = int(balance);
  const covered = int(cost.covered);
  const basis = int(cost.basis);
  if (held <= 0n || covered <= 0n || basis <= 0n) {
    return { income: null, percent: null, whyKey: 'rwa.cost.noBasis' };
  }

  const income = (int(owed) * covered) / held;

  // Taken at 1e6 before the division so a return of a hundredth of a per cent
  // does not collapse to zero. The percentage is for a human to read and is the
  // only float in this file; the amount above it is an integer and stays one.
  const SCALE = 1_000_000n;
  const percent = Number((income * SCALE) / basis) / Number(SCALE) * 100;

  return { income: income.toString(), percent, whyKey: null };
}

/* ========================================================================== */
/*  What is accruing, which is not money yet                                  */
/* ========================================================================== */

export interface Pending {
  /** Token-seconds accrued in the current, undeclared period. */
  tokenSeconds: string;
  /**
   * How long the holder has been accruing in this period, in seconds.
   *
   * Token-seconds divided by the balance: the same quantity said in a unit a
   * person has intuitions about. Null when the balance is zero, where the
   * division says nothing.
   */
  heldSeconds: number | null;
  /** Which period these belong to, and when it opened. */
  periodIndex: string;
  periodStart: number;
  /** True when no distribution has ever been made, so this is the first period. */
  first: boolean;
}

/**
 * Token-seconds accrued in the CURRENT period.
 *
 * Mirrors x/tokenisation's `PendingTokenSeconds` exactly, including the branch
 * that matters most: when the position's stored seconds belong to a period that
 * has since closed, those seconds are MONEY now rather than seconds, so the
 * current period starts fresh from whichever is later of the period's start and
 * the holder's last touch. Carrying them forward would double-count a
 * distribution the holder has already been credited for.
 *
 * Deliberately returns a quantity that is not a currency, and there is no
 * function in this file that converts it into one. The keeper's comment is the
 * specification: "An interface that converted them at the last period's rate
 * would be publishing a forecast beside a settled figure in the same units, and
 * a reader cannot tell those apart once they are both in dollars."
 *
 * `now` is the chain's block time. Against a laptop clock this figure drifts,
 * and it drifts in the flattering direction on a clock that runs fast.
 */
export function pendingAccrual(
  position: Position | null,
  vault: { periodIndex: string; periodStart: number },
  balance: string,
  nowSeconds: number,
): Pending {
  const periodIndex = vault.periodIndex ?? '0';
  const periodStart = vault.periodStart ?? 0;
  const first = int(periodIndex) === 0n;

  const held = int(balance);

  // No position means no accrual: a holder cannot hold without one, because the
  // send restriction settles the recipient before the tokens land.
  if (!position) {
    return { tokenSeconds: '0', heldSeconds: null, periodIndex, periodStart, first };
  }

  let seconds = int(position.tokenSeconds);
  let from = position.lastTouch;

  if (int(position.tokenSecondsPeriod) !== int(periodIndex)) {
    seconds = 0n;
    from = periodStart > position.lastTouch ? periodStart : position.lastTouch;
  }

  const elapsed = nowSeconds - from;
  if (elapsed > 0 && held > 0n) {
    seconds += held * BigInt(Math.trunc(elapsed));
  }

  return {
    tokenSeconds: seconds.toString(),
    // The integral divided by the balance is the time the balance was held for.
    // Only meaningful while the balance is the one that produced the integral,
    // which is why a zero balance gets null rather than a zero.
    heldSeconds: held > 0n ? Number(seconds / held) : null,
    periodIndex,
    periodStart,
    first,
  };
}

/* ========================================================================== */
/*  Reaching a position at all                                                */
/* ========================================================================== */

/**
 * The store key one position lives under.
 *
 * # Why this exists instead of a query
 *
 * x/tokenisation has no RPC that returns a Position. Query/Entitlement answers
 * with one Coin and nothing else, so `cost_basis`, `cost_qty` and
 * `token_seconds` -- every field this file is about -- are unreachable over
 * gRPC. They are reachable over the node's raw store query, which CometBFT
 * serves at `/store/<name>/key` and which the proxy does not gate, so that is
 * how they are read until the module grows a Query/Position.
 *
 * # Why this is safe to depend on
 *
 * The layout is `collections.Map[Pair[uint64, string], Position]` under prefix
 * 4, and x/tokenisation's keys.go states the guarantee this relies on in its own
 * words: "Prefixes are distinct and permanent." A pair codec encodes its first
 * key non-terminally -- uint64 as 8 bytes big-endian -- and its last key
 * terminally, which for a string is the raw bytes with no length prefix and no
 * terminator, because nothing follows it.
 *
 * Verified against the live node rather than inferred: a subspace walk of prefix
 * 4 on yamale-devnet-2 returns keys of exactly this shape.
 *
 * If x/tokenisation ever renumbers the prefix, this read returns nothing and the
 * cost panel says "not recorded" -- wrong, and quietly so. That is the argument
 * for a real query, and it is in the report rather than in a comment nobody
 * reads.
 */
export const POSITIONS_PREFIX = 4;

export function positionKey(assetId: string, holder: string): Uint8Array {
  const id = int(assetId);
  if (id < 0n) throw new Error('asset id must not be negative');

  const address = new TextEncoder().encode(holder);
  const key = new Uint8Array(1 + 8 + address.length);

  key[0] = POSITIONS_PREFIX;
  // Big-endian, which is what makes the map's iteration order numeric. A
  // little-endian id would land every asset in the wrong place and read back a
  // different holder's position rather than failing.
  let v = id;
  for (let i = 8; i >= 1; i--) {
    key[i] = Number(v & 0xffn);
    v >>= 8n;
  }
  key.set(address, 9);
  return key;
}
