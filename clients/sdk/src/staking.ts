/**
 * Staking, governance and pool data, interpreted.
 *
 * The chain returns these as raw REST payloads full of base units, string
 * decimals and enum names. Every number a person acts on — a reward rate, a
 * quorum, a price — is derived here rather than in each interface, so two
 * surfaces can never disagree about what the same validator is paying.
 */

import { formatAmount, type Coin, type DenomInfo } from './denom.ts';

export interface Validator {
  operatorAddress: string;
  moniker: string;
  details: string;
  website: string;
  /** Bonded stake in base units. */
  tokens: string;
  /** Share of all bonded stake, 0–1. */
  votingPower: number;
  /** Commission the validator takes from rewards, 0–1. */
  commission: number;
  maxCommission: number;
  jailed: boolean;
  status: string;
  /** True once this validator alone could halt the chain (>1/3 of stake). */
  concerningPower: boolean;
}

export interface StakingOverview {
  validators: Validator[];
  bonded: string;
  /** Share of total supply that is staked, 0–1. */
  bondedRatio: number;
  unbondingSeconds: number;
  /** Annualised reward rate before commission, 0–1, or null when underivable. */
  inflationRate: number | null;
}

/** Interprets a validator as returned by the staking module. */
export function toValidator(raw: any, totalBonded: bigint): Validator {
  const tokens = String(raw.tokens ?? '0');
  const power = totalBonded > 0n ? Number((BigInt(tokens) * 10000n) / totalBonded) / 10000 : 0;

  return {
    operatorAddress: raw.operator_address ?? '',
    moniker: raw.description?.moniker ?? '',
    details: raw.description?.details ?? '',
    website: raw.description?.website ?? '',
    tokens,
    votingPower: power,
    commission: Number(raw.commission?.commission_rates?.rate ?? 0),
    maxCommission: Number(raw.commission?.commission_rates?.max_rate ?? 0),
    jailed: Boolean(raw.jailed),
    status: raw.status ?? '',
    // A single validator holding more than a third of stake can stall the
    // chain on its own under the BFT bound. Surfacing that is more honest than
    // showing a leaderboard that rewards concentration.
    concerningPower: power > 1 / 3,
  };
}

export type ProposalStatus =
  | 'deposit'
  | 'voting'
  | 'passed'
  | 'rejected'
  | 'failed'
  | 'unknown';

export interface Proposal {
  id: string;
  title: string;
  summary: string;
  status: ProposalStatus;
  /** Plain-language description of what happens next. */
  statusLabel: string;
  submittedAt: string;
  votingEndsAt: string | null;
  depositEndsAt: string | null;
  /** Vote tallies as fractions of all votes cast, 0–1. */
  tally: { yes: number; no: number; abstain: number; veto: number };
  /** Total voting power that has voted, in base units. */
  totalVoted: string;
  /** Deposit raised so far. A proposal only reaches a vote once this clears the minimum. */
  totalDeposit: Coin[];
  /** What the proposal would actually do, decoded. */
  actions: string[];
  raw: unknown;
}

const STATUS_MAP: Record<string, { status: ProposalStatus; label: string }> = {
  PROPOSAL_STATUS_DEPOSIT_PERIOD: {
    status: 'deposit',
    label: 'Waiting for enough deposit to go to a vote',
  },
  PROPOSAL_STATUS_VOTING_PERIOD: { status: 'voting', label: 'Open for voting' },
  PROPOSAL_STATUS_PASSED: { status: 'passed', label: 'Passed and applied' },
  PROPOSAL_STATUS_REJECTED: { status: 'rejected', label: 'Rejected by voters' },
  PROPOSAL_STATUS_FAILED: { status: 'failed', label: 'Passed but failed to apply' },
};

/**
 * Interprets a governance proposal.
 *
 * Tallies come back as absolute voting power. They are converted to fractions
 * of votes cast here because that is the question people actually have — "is
 * this passing?" — and comparing four large integers by eye is not a way to
 * answer it.
 */
export function toProposal(raw: any, describeMessage: (msg: any) => string): Proposal {
  const mapped = STATUS_MAP[raw.status] ?? { status: 'unknown' as ProposalStatus, label: raw.status ?? '' };
  const counted = countVotes(raw.final_tally_result ?? raw.tally);

  return {
    id: String(raw.id ?? ''),
    title: raw.title || raw.messages?.[0]?.['@type'] || `Proposal ${raw.id}`,
    summary: raw.summary ?? '',
    status: mapped.status,
    statusLabel: mapped.label,
    submittedAt: raw.submit_time ?? '',
    votingEndsAt: raw.voting_end_time ?? null,
    depositEndsAt: raw.deposit_end_time ?? null,
    tally: counted.tally,
    totalVoted: counted.totalVoted,
    totalDeposit: raw.total_deposit ?? [],
    actions: (raw.messages ?? []).map(describeMessage),
    raw,
  };
}

/**
 * Replaces a proposal's tally with a freshly queried one.
 *
 * Needed because the proposal list reports `final_tally_result`, which is only
 * filled in once voting has closed. A proposal that is open — the one people
 * are actually looking at, and the only one they can still influence — comes
 * back with every count at zero, so rendering the list as-is shows a live vote
 * as having no votes. The current figures live on a separate endpoint.
 */
export function withTally(proposal: Proposal, tallyRaw: any): Proposal {
  const counted = countVotes(tallyRaw?.tally ?? tallyRaw);
  if (counted.totalVoted === '0') return proposal;
  return { ...proposal, tally: counted.tally, totalVoted: counted.totalVoted };
}

function countVotes(raw: any): { tally: Proposal['tally']; totalVoted: string } {
  const t = raw ?? {};
  const yes = safeCount(t.yes_count);
  const no = safeCount(t.no_count);
  const abstain = safeCount(t.abstain_count);
  const veto = safeCount(t.no_with_veto_count);
  const total = yes + no + abstain + veto;

  const share = (v: bigint) => (total > 0n ? Number((v * 10000n) / total) / 10000 : 0);

  return {
    tally: { yes: share(yes), no: share(no), abstain: share(abstain), veto: share(veto) },
    totalVoted: total.toString(),
  };
}

function safeCount(value: unknown): bigint {
  try {
    return BigInt(String(value ?? '0'));
  } catch {
    return 0n;
  }
}

export interface Pool {
  id: string;
  denomA: string;
  denomB: string;
  reserveA: string;
  reserveB: string;
  totalShares: string;
  /** Trading fee as a fraction, 0–1. */
  feeRate: number;
  /** Units of B per unit of A, at current reserves. */
  price: number | null;
}

/** Interprets an AMM pool, deriving the price its reserves imply. */
export function toPool(raw: any, registry?: Record<string, DenomInfo>): Pool {
  const reserveA = BigInt(raw.reserve_a ?? '0');
  const reserveB = BigInt(raw.reserve_b ?? '0');

  // Price is a ratio of base units, so it is only meaningful once both sides
  // are scaled to display units. Comparing raw reserves of denoms with
  // different exponents would be off by orders of magnitude.
  const price = priceOf(reserveA, reserveB, raw.denom_a, raw.denom_b, registry);

  return {
    id: String(raw.id ?? '0'),
    denomA: raw.denom_a ?? '',
    denomB: raw.denom_b ?? '',
    reserveA: reserveA.toString(),
    reserveB: reserveB.toString(),
    totalShares: String(raw.total_shares ?? '0'),
    feeRate: Number(raw.swap_fee_bps ?? 0) / 10000,
    price,
  };
}

function priceOf(
  reserveA: bigint,
  reserveB: bigint,
  denomA: string,
  denomB: string,
  registry?: Record<string, DenomInfo>,
): number | null {
  if (reserveA === 0n) return null;
  const a = Number(formatAmount(reserveA.toString(), denomA, { withSymbol: false, group: false, registry }));
  const b = Number(formatAmount(reserveB.toString(), denomB, { withSymbol: false, group: false, registry }));
  if (!Number.isFinite(a) || !Number.isFinite(b) || a === 0) return null;
  return b / a;
}

/**
 * What a swap would return, using the same constant-product formula and the
 * same rounding direction the chain uses.
 *
 * The rounding matters: the chain truncates the output so any remainder stays
 * with the pool. A client that rounded the other way would quote an amount the
 * chain then refuses, which reads to a user as the trade randomly failing.
 */
export function quoteSwap(pool: Pool, amountIn: string, denomIn: string): { amountOut: string; priceImpact: number } | null {
  let input: bigint;
  try {
    input = BigInt(amountIn);
  } catch {
    return null;
  }
  if (input <= 0n) return null;

  const forward = denomIn === pool.denomA;
  const reserveIn = BigInt(forward ? pool.reserveA : pool.reserveB);
  const reserveOut = BigInt(forward ? pool.reserveB : pool.reserveA);
  if (reserveIn === 0n || reserveOut === 0n) return null;

  const feeBps = BigInt(Math.round(pool.feeRate * 10000));
  const afterFee = (input * (10000n - feeBps)) / 10000n;
  const amountOut = (reserveOut * afterFee) / (reserveIn + afterFee);

  // How far the trade moves the price against the trader, as a fraction.
  const spot = Number(reserveOut) / Number(reserveIn);
  const effective = Number(amountOut) / Number(input);
  const priceImpact = spot > 0 ? Math.max(0, 1 - effective / spot) : 0;

  return { amountOut: amountOut.toString(), priceImpact };
}

/**
 * The least a trade may return before the chain should reject it.
 *
 * This is the number that actually matters at signing time. A quote is what the
 * pool would pay at the reserves seen a moment ago; by the time the transaction
 * lands somebody else may have traded, and the only protection against that is
 * the floor set here — which is exactly what `min_amount_out` on the swap
 * message carries.
 *
 * Rounds down, so the figure shown is never better than the guarantee sent.
 */
export function minimumReceived(amountOut: string, toleranceBps: number): string {
  let out: bigint;
  try {
    out = BigInt(amountOut);
  } catch {
    return '0';
  }
  if (out <= 0n) return '0';

  const bps = BigInt(Math.max(0, Math.min(10000, Math.round(toleranceBps))));
  return ((out * (10000n - bps)) / 10000n).toString();
}

/** Sums a coin list's amount for one denom. */
export function amountOf(coins: Coin[] | undefined, denom: string): string {
  if (!coins) return '0';
  return coins.find((c) => c.denom === denom)?.amount ?? '0';
}
