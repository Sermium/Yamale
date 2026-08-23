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
 * A spending policy for one denom: how much may leave, per period.
 *
 * A limit that has been reached is not an error state. It is the policy doing
 * its job, and an interface should say so in those terms rather than reporting
 * a failure.
 */
export interface SpendPolicy {
  denom: string;
  limit: string;
  periodBlocks: number;
  requiresApproval: boolean;
}

/** How much of the current period's allowance is left. */
export interface SpendCapacity {
  denom: string;
  remaining: string;
  limit: string;
  periodEndsAtHeight: number;
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
