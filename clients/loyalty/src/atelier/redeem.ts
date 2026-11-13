/**
 * What the counter checks before it asks the staff member to sign a
 * redemption. These are the chain's own checks, in the order a person would
 * want them explained; the chain stays the authority. Pure, with the chain's
 * clock passed in, so that no decision here depends on the phone's.
 */

import { benefitState } from '../verify/history.ts';

export interface RedeemInput {
  /** The benefit the code names, as the chain holds it, or undefined when there is none. */
  benefit: { jewelerId: string; allowance: number; period: string; expiresAt: string; uses: string[] } | undefined;
  /** The staff member's jeweler. */
  jewelerId: string;
  /** Who signed the code, or null when the signature is not good. */
  signer: string | null;
  /** The jewel's current owner. */
  owner: string;
  /** When the code lapses, unix seconds. */
  codeExpiresAt: number;
  stolen: boolean;
  /** The chain's latest block time, unix seconds. */
  now: number;
}

export type RedeemCheck =
  | { ok: true; left: number }
  | { ok: false; key: string };

export function checkRedeem(i: RedeemInput): RedeemCheck {
  if (!i.benefit || i.benefit.jewelerId !== i.jewelerId) return { ok: false, key: 'loyalty.voucherNotOurs' };
  if (i.signer === null || i.signer !== i.owner) return { ok: false, key: 'loyalty.voucherNotOwner' };
  if (i.codeExpiresAt <= i.now) return { ok: false, key: 'loyalty.voucherStale' };
  if (i.stolen) return { ok: false, key: 'loyalty.err.stolen' };
  // The staff member is active (the screen requires it), so the issuer is honouring its benefits.
  const s = benefitState(i.benefit, true, i.now);
  if (s.state === 'expired') return { ok: false, key: 'loyalty.err.benefitExpired' };
  if (s.state !== 'active') return { ok: false, key: 'loyalty.voucherNotOurs' };
  if (s.left === 0) return { ok: false, key: 'loyalty.err.allowanceUsed' };
  return { ok: true, left: s.left };
}
