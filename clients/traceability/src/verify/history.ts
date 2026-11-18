/**
 * What the public page says about a jewel's owners and benefits: dates and
 * counts, never names. Pure, so the arithmetic is tested against the chain's.
 */

export interface OwnerStep { kind: 'stock' | 'owner'; ordinal: number; at: number }

export function ownershipSteps(j: { registeredAt: string; ownerSince: string[] }): OwnerStep[] {
  return [
    { kind: 'stock', ordinal: 0, at: Number(j.registeredAt) },
    ...j.ownerSince.map((t, i) => ({ kind: 'owner' as const, ordinal: i + 1, at: Number(t) })),
  ];
}

export type BenefitState =
  | { state: 'active'; left: number; resetsAt: number | null; expiresAt: number | null }
  | { state: 'expired'; expiredAt: number }
  | { state: 'withdrawn' };

/** A suspended issuer is a fact; an issuer that could not be read is only unknown. */
export type IssuerStatus = 'active' | 'suspended' | 'unknown';

/**
 * Counts uses exactly as RedeemBenefit in x/jewel/keeper/msg_server_benefit.go
 * does: with a period, a use counts while u > now - period; without one,
 * every use counts. A suspended issuer's benefits are shown as withdrawn,
 * because nobody will honour them. An unknown issuer (the read failed) is not
 * a withdrawal: the benefit is described as the chain holds it.
 */
export function benefitState(
  b: { allowance: number; period: string; expiresAt: string; uses: string[] },
  issuer: IssuerStatus, now: number,
): BenefitState {
  if (issuer === 'suspended') return { state: 'withdrawn' };
  const expiresAt = Number(b.expiresAt);
  if (expiresAt > 0 && now >= expiresAt) return { state: 'expired', expiredAt: expiresAt };
  const period = Number(b.period);
  const counted = b.uses.map(Number).filter((u) => period === 0 || u > now - period);
  const left = Math.max(0, b.allowance - counted.length);
  const resetsAt = left === 0 && period > 0 && counted.length > 0 ? Math.min(...counted) + period : null;
  return { state: 'active', left, resetsAt, expiresAt: expiresAt > 0 ? expiresAt : null };
}
