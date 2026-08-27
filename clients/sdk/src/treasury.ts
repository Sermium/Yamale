/**
 * Treasuries, in the shape a safe interface needs.
 *
 * The model is Orion Safe's, and so is the invariant that matters most:
 * **committed funds are not spendable funds**. A treasury's balance is not one
 * number. It is what it holds, what it has already promised to somebody — a
 * vesting grant, a scheduled disbursement — and what is left over. An interface
 * that shows only the first number will eventually show a treasurer a balance
 * they cannot spend and let them propose a payment that fails at execution,
 * after the approvals were collected.
 *
 * So `available` is what this module leads with, and `total` is the secondary
 * figure.
 */

import { resolveDenom, type Coin } from './denom.ts';

/** A treasury: shared funds with an admin, roles and policies. */
export interface Treasury {
  id: string;
  name: string;
  admin: string;
  paused: boolean;
  createdAtHeight: number;
}

/**
 * What a treasury holds in one denom, split three ways.
 *
 * `locked` is the sum of everything committed to a beneficiary and not yet
 * claimed. It cannot be spent by any proposal, including one that clears the
 * signing threshold — which is the whole point of committing it.
 */
export interface TreasuryBalance {
  denom: string;
  total: string;
  locked: string;
  available: string;
}

/** Who may do what inside a treasury. */
export interface RoleAssignment {
  address: string;
  role: string;
}

/**
 * A spending policy for one denom: how much may leave, how often, and to whom.
 *
 * A limit that has been reached is not an error state. It is the policy doing
 * its job, and an interface should say so in those terms rather than reporting
 * a failure.
 *
 * The fields are the chain's. This interface previously declared `limit`,
 * `periodBlocks` and `requiresApproval`, none of which exist in
 * `blockchain.treasury.v1.SpendPolicy` — the real policy is a per-transaction
 * cap, a per-window cap with the window in **seconds**, and two destination
 * lists. Nothing read it, so the mismatch cost nothing; the first screen to
 * trust it would have rendered `undefined` in every field.
 *
 * Every cap is optional, and absent is not zero. An empty `perTransaction`
 * means a spender is bounded only by the balance, which is the opposite of a
 * cap of nothing.
 */
export interface SpendPolicy {
  treasuryId: string;
  denom: string;
  /** Cap on one payment, in base units. Absent means uncapped. */
  perTransaction?: string;
  /** Cap on everything paid within one window, in base units. Absent means uncapped. */
  perPeriod?: string;
  /** The window length in seconds. Meaningless without `perPeriod`. */
  periodSeconds?: number;
  /** When non-empty, the only destinations this treasury may pay. */
  allowlist: string[];
  /** Destinations refused outright. Checked after the allowlist, so both means denied. */
  blocklist: string[];
}

/**
 * How much of the current window's allowance is left, and what else bounds a
 * payment right now.
 *
 * Both figures are reported because a spend is bounded by whichever is smaller,
 * and a screen showing only one of them will eventually tell a treasurer they
 * may spend money the treasury does not have — or that they may not spend money
 * that is sitting there.
 */
export interface SpendCapacity {
  denom: string;
  /** What the period limit still allows, in base units. */
  remainingThisPeriod: string;
  /** What the treasury actually holds unlocked, in base units. */
  available: string;
  /** Cap on a single payment, when there is one. */
  perTransactionLimit?: string;
  /** Unix seconds at which the window resets, when there is a window. */
  periodResetsAt?: number;
}

export function toSpendPolicy(raw: any): SpendPolicy {
  const optional = (value: unknown) =>
    typeof value === 'string' && value !== '' && value !== '0' ? value : undefined;
  return {
    treasuryId: String(raw?.treasury_id ?? '0'),
    denom: raw?.denom ?? '',
    perTransaction: optional(raw?.per_transaction_limit),
    perPeriod: optional(raw?.period_limit),
    periodSeconds: Number(raw?.period_seconds ?? 0) || undefined,
    allowlist: Array.isArray(raw?.allowlist) ? raw.allowlist : [],
    blocklist: Array.isArray(raw?.blocklist) ? raw.blocklist : [],
  };
}

/**
 * What a policy refuses, as catalogue keys rather than as sentences.
 *
 * Written as refusals rather than as permissions on purpose. "Up to 5,000 YML
 * a day" and "refuses anything over 5,000 YML in a day" are the same rule, but
 * only the second answers the question a treasurer actually has when a payment
 * will not go through.
 *
 * Returning keys rather than English is the difference between a Limits tab
 * that is translated and one that is in English on a French console. The
 * amounts are formatted by the caller, because formatting needs the denom
 * registry and this module has no business holding one.
 */
export interface PolicyRefusal {
  key: string;
  vars: Record<string, string>;
}

export function policyRefusals(
  policy: SpendPolicy | null,
  format: (amount: string, denom: string) => string,
  duration: (seconds: number) => string,
): PolicyRefusal[] {
  if (!policy) return [];
  const out: PolicyRefusal[] = [];

  if (policy.perTransaction) {
    out.push({ key: 'safe.refusePerTx', vars: { limit: format(policy.perTransaction, policy.denom) } });
  }
  if (policy.perPeriod) {
    const limit = format(policy.perPeriod, policy.denom);
    out.push(
      policy.periodSeconds
        ? { key: 'safe.refusePerPeriod', vars: { limit, window: duration(policy.periodSeconds) } }
        : { key: 'safe.refusePerPeriodNoWindow', vars: { limit } },
    );
  }
  // Singular and plural as separate keys rather than an inflected English
  // string: "one of the 1 approved address" is what the arithmetic version
  // produced, and half the languages here do not pluralise the way English
  // does anyway.
  if (policy.allowlist.length === 1) {
    out.push({ key: 'safe.refuseAllowlistOne', vars: {} });
  } else if (policy.allowlist.length > 1) {
    out.push({ key: 'safe.refuseAllowlistMany', vars: { n: String(policy.allowlist.length) } });
  }
  if (policy.blocklist.length === 1) {
    out.push({ key: 'safe.refuseBlocklistOne', vars: {} });
  } else if (policy.blocklist.length > 1) {
    out.push({ key: 'safe.refuseBlocklistMany', vars: { n: String(policy.blocklist.length) } });
  }
  return out;
}

export const ROLE_LABELS: Record<string, string> = {
  ROLE_UNSPECIFIED: 'None',
  ROLE_SPENDER: 'Spender',
  ROLE_APPROVER: 'Approver',
  ROLE_VIEWER: 'Viewer',
};

export function toTreasury(raw: any): Treasury {
  return {
    id: String(raw?.id ?? '0'),
    name: raw?.name ?? '',
    admin: raw?.admin ?? '',
    paused: Boolean(raw?.paused),
    createdAtHeight: Number(raw?.created_at_height ?? 0),
  };
}

export function toTreasuryBalance(raw: any): TreasuryBalance {
  return {
    denom: raw?.denom ?? '',
    total: String(raw?.total ?? '0'),
    locked: String(raw?.locked ?? '0'),
    available: String(raw?.available ?? '0'),
  };
}

export function toRoleAssignment(raw: any): RoleAssignment {
  return { address: raw?.address ?? '', role: raw?.role ?? 'ROLE_UNSPECIFIED' };
}

/** The coins a treasury could actually pay out right now. */
export function spendable(balances: TreasuryBalance[]): Coin[] {
  return balances
    .filter((b) => b.available !== '0')
    .map((b) => ({ denom: b.denom, amount: b.available }));
}

/** Everything committed and not yet claimed, across denoms. */
export function committed(balances: TreasuryBalance[]): Coin[] {
  return balances
    .filter((b) => b.locked !== '0')
    .map((b) => ({ denom: b.denom, amount: b.locked }));
}

/**
 * Whether a proposed spend can actually be paid, and if not, why.
 *
 * Checked before the approvals are collected rather than after. Discovering at
 * execution that a treasury is paused, or that the funds were committed to
 * somebody else, wastes everyone's signatures and reads as a chain failure
 * rather than as the policy it is.
 */
export function checkSpend(
  treasury: Treasury | null,
  balances: TreasuryBalance[],
  amount: Coin[],
): { ok: boolean; reason?: string } {
  if (!treasury) return { ok: false, reason: 'This treasury does not exist.' };
  if (treasury.paused) {
    return { ok: false, reason: 'This treasury is paused. Nothing can leave it until an admin unpauses it.' };
  }

  for (const coin of amount) {
    const balance = balances.find((b) => b.denom === coin.denom);
    if (!balance) {
      // The symbol, not the base denom. "This treasury holds no uyml" asks a
      // treasurer to know that uyml is YML divided by a million, which is the
      // one fact this whole layer exists so that nobody has to know.
      return { ok: false, reason: `This treasury holds no ${resolveDenom(coin.denom).symbol}.` };
    }
    if (BigInt(balance.available) < BigInt(coin.amount)) {
      const locked = BigInt(balance.locked);
      if (locked > 0n && BigInt(balance.total) >= BigInt(coin.amount)) {
        return {
          ok: false,
          reason:
            'The treasury holds enough, but part of it is committed to a beneficiary and cannot be spent — not even by a proposal that reaches the threshold.',
        };
      }
      return { ok: false, reason: 'More than the treasury has available.' };
    }
  }

  return { ok: true };
}
