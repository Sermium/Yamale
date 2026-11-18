/**
 * What the owner's screen shows for one issued code. Pure, so the decision
 * is tested without a browser.
 *
 * A code is single-use consent: once the chain records a use after the code
 * was issued, it is closed, and once it lapses it is never renewed silently.
 */

export interface UseMark { count: number; last: number }

export type VoucherPhase = 'showing' | 'lapsed' | 'used';

/** The size and newest date of a benefit's use list, as read from the chain. */
export function markOf(uses: string[]): UseMark {
  return { count: uses.length, last: uses.reduce((m, u) => Math.max(m, Number(u)), 0) };
}

export function voucherPhase(
  issued: UseMark, current: UseMark | null, nowMs: number, deadlineMs: number,
): VoucherPhase {
  if (current && (current.count > issued.count || current.last > issued.last)) return 'used';
  return nowMs >= deadlineMs ? 'lapsed' : 'showing';
}
