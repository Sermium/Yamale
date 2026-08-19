/**
 * Enforcement cases, in the shape an interface can show a person.
 *
 * This is the module that can freeze an account and take what is in it, and
 * the only thing that separates that from an arbitrary power is that every
 * exercise of it is public, attributed and explained. So the translation here
 * leans the same way: a case carries its grounds, its author and its evidence
 * alongside its outcome, and the outcomes are named in words that say what
 * actually happened rather than in enum values that need a lookup table.
 */

import type { Coin } from './denom.ts';

export type CaseAction = 'freeze' | 'seize' | 'unknown';

export type CaseStatus =
  | 'voting'
  | 'passed'
  | 'rejected'
  | 'expired'
  | 'withdrawn'
  | 'reversed'
  | 'unknown';

export interface EnforcementCase {
  id: string;
  target: string;
  /** The operator address of the validator that opened it. */
  opener: string;
  action: CaseAction;
  status: CaseStatus;
  reason: string;
  evidenceUri: string;
  evidenceHash: string;
  openedAtHeight: number;
  votingEndsAtHeight: number;
  resolvedAtHeight: number;
  totalPowerAtOpen: number;
  yesPower: number;
  noPower: number;
  abstainPower: number;
  /** What this case has taken so far. Grows as unbonding funds are swept. */
  recovered: Coin[];
  sweepComplete: boolean;
  /**
   * True when the founders' emergency authority opened it rather than a
   * validator. Surfaced rather than buried: who used a power like this is as
   * much a part of the record as what it did.
   */
  emergency: boolean;
}

export interface EnforcementVote {
  validator: string;
  option: string;
  power: number;
}

const ACTIONS: Record<string, CaseAction> = {
  CASE_ACTION_FREEZE: 'freeze',
  CASE_ACTION_SEIZE: 'seize',
};

const STATUSES: Record<string, CaseStatus> = {
  CASE_STATUS_VOTING: 'voting',
  CASE_STATUS_PASSED: 'passed',
  CASE_STATUS_REJECTED: 'rejected',
  CASE_STATUS_EXPIRED: 'expired',
  CASE_STATUS_WITHDRAWN: 'withdrawn',
  CASE_STATUS_REVERSED: 'reversed',
};

export function toEnforcementCase(raw: any): EnforcementCase {
  return {
    id: String(raw?.id ?? '0'),
    target: raw?.target ?? '',
    opener: raw?.opener ?? '',
    action: ACTIONS[raw?.action] ?? 'unknown',
    status: STATUSES[raw?.status] ?? 'unknown',
    reason: raw?.reason ?? '',
    evidenceUri: raw?.evidence_uri ?? '',
    evidenceHash: raw?.evidence_hash ?? '',
    openedAtHeight: Number(raw?.opened_at_height ?? 0),
    votingEndsAtHeight: Number(raw?.voting_ends_at_height ?? 0),
    resolvedAtHeight: Number(raw?.resolved_at_height ?? 0),
    totalPowerAtOpen: Number(raw?.total_power_at_open ?? 0),
    yesPower: Number(raw?.yes_power ?? 0),
    noPower: Number(raw?.no_power ?? 0),
    abstainPower: Number(raw?.abstain_power ?? 0),
    recovered: (raw?.recovered ?? []).map((c: any) => ({ denom: c.denom, amount: c.amount })),
    sweepComplete: Boolean(raw?.sweep_complete),
    emergency: Boolean(raw?.emergency),
  };
}

/**
 * What a case means, in a sentence, from the point of view of the person it is
 * against.
 *
 * Written in that direction on purpose. An explorer describing a freeze as
 * "case 4: seize, voting" is describing a database row; the person reading it
 * wants to know whether their money is stuck and who decided that.
 */
export function describeCase(c: EnforcementCase): string {
  switch (c.status) {
    case 'voting':
      return c.action === 'seize'
        ? 'Frozen while the validators decide whether to seize the balance. The freeze lifts by itself if they do not agree.'
        : 'Frozen while the validators decide. The freeze lifts by itself if they do not agree.';
    case 'passed':
      return c.action === 'seize'
        ? 'The validators agreed. The balance has been sent to the recovery destination and the account remains frozen.'
        : 'The validators agreed. The account remains frozen until a decision releases it.';
    case 'rejected':
      return 'The validators refused it. The account was released.';
    case 'expired':
      return 'The voting period ended without enough support. The account was released — which is not a finding either way.';
    case 'withdrawn':
      return 'The validator who opened it took it back. The account was released.';
    case 'reversed':
      return 'The case was overturned and the account released. Anything already seized is held by the recovery destination, not returned by the chain.';
    default:
      return 'This case is in a state this interface does not recognise.';
  }
}

/** How much yes power a case needs, given the threshold in basis points. */
export function requiredPower(totalPowerAtOpen: number, thresholdBps = 6667): number {
  if (totalPowerAtOpen <= 0) return 0;
  return Math.ceil((totalPowerAtOpen * thresholdBps) / 10_000);
}

/** Whether the case is still open to votes. */
export function isOpen(c: EnforcementCase): boolean {
  return c.status === 'voting';
}
