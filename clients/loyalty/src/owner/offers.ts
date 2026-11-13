/**
 * Whether an offer is still open, by the chain's clock.
 *
 * The chain never sweeps an expired offer: it stays readable, and a new offer
 * may be made over it. So every screen decides openness itself, from a block
 * time it has actually read, and treats an expired offer as no offer.
 */

export function offerIsOpen<T extends { expiresAt: string }>(offer: T | null | undefined, now: number): offer is T {
  return !!offer && Number(offer.expiresAt) > now;
}

/** The claim key of an open link offer, which is the only link worth keeping. */
export function liveClaimKey(offer: { expiresAt: string; claimKey: string } | null | undefined, now: number): string | null {
  return offerIsOpen(offer, now) && offer.claimKey ? offer.claimKey : null;
}
