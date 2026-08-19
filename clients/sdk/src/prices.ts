/**
 * Prices and valuations, interpreted.
 *
 * The chain returns a rate as a decimal string alongside the moment it was
 * agreed, and leaves it to the reader to decide whether that moment is still
 * recent enough to matter. Doing that arithmetic here — once — is the point: the
 * rule a screen applies before showing a price has to be the same rule the chain
 * applies before lending against it, and two implementations of "is this old?"
 * eventually disagree at exactly the wrong moment.
 */

import { resolveDenom, toDisplayAmount, type DenomInfo } from './denom.ts';

/** How much confidence a value deserves, in one word. */
export type Freshness = 'fresh' | 'ageing' | 'stale';

export interface Rate {
  denom: string;
  /** Price of one display unit, as a decimal string. */
  rate: string;
  /** Same, as a number, for arithmetic. NaN is impossible: unparseable rates are rejected. */
  value: number;
  /** Unix seconds when the rate was agreed. */
  updatedAt: number;
  updatedHeight: number;
  /** Share of stake that contributed to this median, 0–1. */
  votingPower: number;
  ageSeconds: number;
  freshness: Freshness;
  /** True once the chain itself would refuse to act on it. */
  stale: boolean;
  /**
   * True when the rate was agreed by a bare quorum rather than by most of the
   * network. Worth surfacing: a price half the validators stood behind is a
   * weaker claim than one they all did, and the number alone does not say so.
   */
  thinlyAgreed: boolean;
}

export interface Appraisal {
  classId: string;
  nftId: string;
  /** Value in base units of valueDenom. */
  value: string;
  valueDenom: string;
  /** The address that signed this valuation. */
  appraiser: string;
  /** When the valuation describes the world, not when it was filed. */
  valuedAt: number;
  submittedAt: number;
  method: string;
  reportUri: string;
  reportHash: string;
  superseded: boolean;
  ageSeconds: number;
  freshness: Freshness;
  stale: boolean;
  /**
   * False when the chain has since withdrawn the signer's authority. The
   * valuation remains valid history — it was properly signed when it was
   * made — but it should be weighed differently.
   */
  appraiserStillApproved: boolean;
}

/**
 * Default maximum ages, mirroring the module's own parameters.
 *
 * They are defaults rather than constants because governance can change them,
 * and a client that hard-coded them would keep showing a price as fresh after
 * the chain had stopped accepting it. Pass the values from
 * `/blockchain/oracle/v1/params` when you have them.
 */
export const DEFAULT_MAX_RATE_AGE = 900;
export const DEFAULT_MAX_APPRAISAL_AGE = 8_640_000;

/**
 * Classifies an age against a maximum.
 *
 * `ageing` exists so an interface can warn before a feed actually expires. The
 * chain has no such state — it is fresh or it is refused — but a person watching
 * a dashboard is better served by "this is getting old" than by a value that
 * silently becomes unusable.
 */
export function freshnessOf(ageSeconds: number, maxAgeSeconds: number): Freshness {
  if (maxAgeSeconds <= 0) return 'fresh';
  if (ageSeconds > maxAgeSeconds) return 'stale';
  if (ageSeconds > maxAgeSeconds / 2) return 'ageing';
  return 'fresh';
}

/**
 * Interprets an exchange rate from the oracle module.
 *
 * `now` is passed in rather than read from the clock so the same payload always
 * produces the same result — which is what makes this testable, and what stops a
 * client whose clock has drifted from disagreeing with the chain about whether a
 * price is usable.
 */
export function toRate(raw: any, now: number, maxAgeSeconds = DEFAULT_MAX_RATE_AGE): Rate {
  const updatedAt = Number(raw.updated_at ?? 0);
  const value = Number(raw.rate ?? '0');

  // A value from the future is treated as maximally old rather than as fresh.
  // Clock skew should never make something look more current than it is.
  const ageSeconds = updatedAt > 0 && now > updatedAt ? now - updatedAt : updatedAt > now ? Infinity : 0;
  const freshness = freshnessOf(ageSeconds, maxAgeSeconds);
  const votingPower = Number(raw.voting_power_bps ?? 0) / 10000;

  return {
    denom: raw.denom ?? '',
    rate: String(raw.rate ?? '0'),
    value: Number.isFinite(value) ? value : 0,
    updatedAt,
    updatedHeight: Number(raw.updated_height ?? 0),
    votingPower,
    ageSeconds: Number.isFinite(ageSeconds) ? ageSeconds : Number.MAX_SAFE_INTEGER,
    freshness,
    stale: freshness === 'stale',
    thinlyAgreed: votingPower > 0 && votingPower < 0.67,
  };
}

/** Interprets a valuation, ageing it from the date it describes. */
export function toAppraisal(
  raw: any,
  now: number,
  maxAgeSeconds = DEFAULT_MAX_APPRAISAL_AGE,
  stillApproved = true,
): Appraisal {
  const valuedAt = Number(raw.valued_at ?? 0);
  const ageSeconds = valuedAt > 0 && now > valuedAt ? now - valuedAt : valuedAt > now ? Infinity : 0;
  const freshness = freshnessOf(ageSeconds, maxAgeSeconds);

  return {
    classId: raw.class_id ?? '',
    nftId: raw.nft_id ?? '',
    value: String(raw.value ?? '0'),
    valueDenom: raw.value_denom ?? '',
    appraiser: raw.appraiser ?? '',
    valuedAt,
    submittedAt: Number(raw.submitted_at ?? 0),
    method: raw.method ?? '',
    reportUri: raw.report_uri ?? '',
    reportHash: raw.report_hash ?? '',
    superseded: Boolean(raw.superseded),
    ageSeconds: Number.isFinite(ageSeconds) ? ageSeconds : Number.MAX_SAFE_INTEGER,
    freshness,
    stale: freshness === 'stale',
    appraiserStillApproved: stillApproved,
  };
}

/**
 * Values an amount of a denom in the quote currency, using the same scaling and
 * the same rounding direction the chain uses.
 *
 * The rate prices one display unit and the amount is in base units, so applying
 * one to the other without scaling overstates the answer by the denom's whole
 * exponent — a factor of a million here. Doing it in one place means that
 * mistake can only be made once.
 *
 * Rounds down, so a position is never shown as worth more than it is. Where this
 * feeds a collateral figure, over-valuing is what lets somebody borrow more than
 * their asset supports.
 *
 * Returns null for a stale rate: a caller that wanted the number anyway can read
 * `rate.value` itself, but the default has to be that an unusable price produces
 * no figure rather than a confident-looking wrong one.
 */
export function valueAt(
  baseAmount: string,
  rate: Rate,
  registry?: Record<string, DenomInfo>,
): number | null {
  if (rate.stale) return null;

  const info = registry ? resolveDenom(rate.denom, registry) : resolveDenom(rate.denom);
  const display = Number(toDisplayAmount(baseAmount, info.exponent));
  if (!Number.isFinite(display) || !Number.isFinite(rate.value)) return null;

  return Math.floor(display * rate.value * 100) / 100;
}

export interface PoolComparison {
  /** Units of B per unit of A that the oracle's rates imply. */
  fairPrice: number;
  /** How far the pool's price sits from that, as a signed fraction. */
  divergence: number;
  /** True once the gap is wide enough to be worth saying out loud. */
  notable: boolean;
  /** True when either side's rate is too old to compare against. */
  stale: boolean;
}

/**
 * How a pool's price compares with the rates validators agreed.
 *
 * Both are legitimate prices and they are allowed to differ — a pool is a real
 * market and the oracle is a reported one. The comparison is worth showing
 * anyway, because a wide gap is the single most useful warning before a trade:
 * either the pool is thin and about to move against you, or it is stale and
 * somebody is about to arbitrage it. Saying which is beyond what the chain
 * knows, so the interface reports the gap and lets the reader judge.
 *
 * Returns null rather than guessing when either rate is missing. A comparison
 * against a price that does not exist is worse than none.
 */
export function comparePoolToRates(
  poolPrice: number | null,
  rateA: Rate | undefined,
  rateB: Rate | undefined,
  notableThreshold = 0.02,
): PoolComparison | null {
  if (poolPrice === null || !rateA || !rateB) return null;
  if (!Number.isFinite(poolPrice) || poolPrice <= 0) return null;
  if (rateA.value <= 0 || rateB.value <= 0) return null;

  // Both rates are quoted in the same currency, so their ratio is the price of
  // one in terms of the other.
  const fairPrice = rateA.value / rateB.value;
  const divergence = (poolPrice - fairPrice) / fairPrice;

  return {
    fairPrice,
    divergence,
    notable: Math.abs(divergence) >= notableThreshold,
    stale: rateA.stale || rateB.stale,
  };
}

/**
 * One sentence explaining how much a value can be relied on.
 *
 * Written for somebody who does not know what an oracle is. "Agreed 3 minutes
 * ago by 100% of validators" answers the question a person actually has, where
 * a timestamp and a basis-point figure do not.
 */
export function describeFreshness(rate: Rate, timeAgo: (seconds: number) => string): string {
  if (rate.updatedAt === 0) return 'No price has been agreed yet.';

  const when = timeAgo(rate.ageSeconds);
  const share = `${Math.round(rate.votingPower * 100)}% of validators`;

  switch (rate.freshness) {
    case 'stale':
      return `Last agreed ${when} by ${share}. Too old to rely on — the price feed may have stopped.`;
    case 'ageing':
      return `Agreed ${when} by ${share}. Due for an update.`;
    default:
      return rate.thinlyAgreed
        ? `Agreed ${when}, but by only ${share}.`
        : `Agreed ${when} by ${share}.`;
  }
}
