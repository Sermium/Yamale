/**
 * Whether a proposal is passing, and what still has to happen.
 *
 * A tally on its own does not answer the question people actually have. "Yes
 * 80%" looks decisive and can still fail, because the vote has to clear a quorum
 * of *all* staked tokens, abstentions count toward that quorum but not toward
 * the yes share, and a veto minority can reject a proposal the majority
 * supports. Working that out from four numbers and three parameters is not
 * something a reader should be asked to do, so it is done here — once, in the
 * same arithmetic the chain uses.
 */

import type { Coin } from './denom.ts';
import type { Proposal } from './staking.ts';

export interface GovParams {
  /** Share of all bonded stake that must vote for the result to count, 0–1. */
  quorum: number;
  /** Share of yes among yes+no+veto needed to pass, 0–1. */
  threshold: number;
  /** Share of veto among all votes cast that rejects regardless, 0–1. */
  vetoThreshold: number;
  minDeposit: Coin[];
  votingPeriodSeconds: number;
  maxDepositPeriodSeconds: number;
}

export const DEFAULT_GOV_PARAMS: GovParams = {
  quorum: 0.334,
  threshold: 0.5,
  vetoThreshold: 0.334,
  minDeposit: [],
  votingPeriodSeconds: 0,
  maxDepositPeriodSeconds: 0,
};

/** Reads the governance parameters as the chain returns them. */
export function toGovParams(raw: any): GovParams {
  const p = raw?.params ?? raw ?? {};
  return {
    quorum: fraction(p.quorum, DEFAULT_GOV_PARAMS.quorum),
    threshold: fraction(p.threshold, DEFAULT_GOV_PARAMS.threshold),
    vetoThreshold: fraction(p.veto_threshold, DEFAULT_GOV_PARAMS.vetoThreshold),
    minDeposit: p.min_deposit ?? [],
    votingPeriodSeconds: duration(p.voting_period),
    maxDepositPeriodSeconds: duration(p.max_deposit_period),
  };
}

export interface ProposalAssessment {
  /** Share of all bonded stake that has voted, 0–1. */
  quorumProgress: number;
  quorumMet: boolean;
  /** Yes as a share of yes+no+veto. Abstaining does not count against a proposal. */
  yesShare: number;
  /** Veto as a share of every vote cast, including abstentions. */
  vetoShare: number;
  vetoed: boolean;
  /** True if the vote closed right now and the proposal passed. */
  wouldPass: boolean;
  /** One sentence a non-expert can act on. */
  verdict: string;
  /** What still has to happen for it to pass, or null when nothing does. */
  blocker: string | null;
  /** Deposit raised as a share of the minimum, 0–1. Only meaningful in the deposit period. */
  depositProgress: number;
}

/**
 * Applies the chain's own passing rules to a proposal.
 *
 * `bondedTokens` is the denominator for quorum: the question is what share of
 * *staked* tokens voted, not what share of holders or of supply. Passing it in
 * rather than assuming keeps this a pure function, which is what makes the
 * boundary cases — nobody voted, nothing bonded — testable rather than
 * discovered in production as a division by zero.
 */
export function assessProposal(
  proposal: Proposal,
  params: GovParams,
  bondedTokens: string,
): ProposalAssessment {
  const bonded = safeBigInt(bondedTokens);
  const cast = safeBigInt(proposal.totalVoted);

  const quorumProgress = bonded > 0n ? Number((cast * 10000n) / bonded) / 10000 : 0;
  const quorumMet = quorumProgress >= params.quorum;

  // Abstain is excluded from the threshold denominator on purpose: the chain
  // treats it as "present but not opposed", so an abstention helps reach quorum
  // without counting against the proposal.
  const decisive = proposal.tally.yes + proposal.tally.no + proposal.tally.veto;
  const yesShare = decisive > 0 ? proposal.tally.yes / decisive : 0;
  const vetoShare = proposal.tally.veto;
  const vetoed = vetoShare > params.vetoThreshold;

  const wouldPass = quorumMet && !vetoed && yesShare > params.threshold;

  const depositProgress = depositShare(proposal, params);

  return {
    quorumProgress,
    quorumMet,
    yesShare,
    vetoShare,
    vetoed,
    wouldPass,
    verdict: verdictFor(proposal, { quorumMet, vetoed, wouldPass, yesShare }, params),
    blocker: blockerFor(proposal, { quorumMet, vetoed, wouldPass }, params, quorumProgress),
    depositProgress,
  };
}

function verdictFor(
  proposal: Proposal,
  state: { quorumMet: boolean; vetoed: boolean; wouldPass: boolean; yesShare: number },
  params: GovParams,
): string {
  if (proposal.status === 'passed') return 'Passed and applied to the chain.';
  if (proposal.status === 'failed') return 'Passed the vote but failed to apply.';
  if (proposal.status === 'rejected') return 'Rejected.';
  if (proposal.status === 'deposit') {
    return 'Not yet up for a vote — it needs a larger deposit first.';
  }
  if (proposal.status !== 'voting') return proposal.statusLabel;

  if (state.vetoed) {
    return `On course to be rejected: more than ${percent(params.vetoThreshold)} of votes are vetoes.`;
  }
  if (!state.quorumMet) {
    return 'On course to fail: not enough of the staked tokens have voted.';
  }
  return state.wouldPass
    ? 'On course to pass if voting closed now.'
    : `On course to be rejected: yes needs to pass ${percent(params.threshold)}.`;
}

function blockerFor(
  proposal: Proposal,
  state: { quorumMet: boolean; vetoed: boolean; wouldPass: boolean },
  params: GovParams,
  quorumProgress: number,
): string | null {
  if (proposal.status !== 'voting') return null;
  if (state.vetoed) return null;
  if (!state.quorumMet) {
    const needed = Math.max(0, params.quorum - quorumProgress);
    return `A further ${percent(needed)} of all staked tokens needs to vote.`;
  }
  if (!state.wouldPass) return `Yes has to reach ${percent(params.threshold)} of the decisive votes.`;
  return null;
}

/** How much of the minimum deposit has been raised, 0–1. */
function depositShare(proposal: Proposal, params: GovParams): number {
  const required = params.minDeposit[0];
  if (!required) return 0;

  const have = proposal.totalDeposit.find((c) => c.denom === required.denom);
  const target = safeBigInt(required.amount);
  if (target <= 0n) return 1;

  const raised = safeBigInt(have?.amount ?? '0');
  return Math.min(1, Number((raised * 10000n) / target) / 10000);
}

function fraction(value: unknown, fallback: number): number {
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}

function duration(value: unknown): number {
  if (!value) return 0;
  const seconds = Number(String(value).replace(/s$/, ''));
  return Number.isFinite(seconds) ? seconds : 0;
}

function safeBigInt(value: string | undefined): bigint {
  try {
    return BigInt(value ?? '0');
  } catch {
    return 0n;
  }
}

function percent(value: number): string {
  return `${Math.round(value * 1000) / 10}%`;
}
