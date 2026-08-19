/**
 * Secured payments: the seller is paid once the buyer confirms delivery.
 *
 * The buyer commits the money up front, so the seller can ship knowing it
 * exists. The seller is not paid until the buyer says the thing arrived. That
 * is the whole product, and it is worth 1% to both of them because the
 * alternative is one of them trusting a stranger.
 *
 * ## The two ways it ends
 *
 * **Released.** The buyer confirms, the seller is paid. The happy path, and the
 * only one anybody plans for.
 *
 * **Moderated.** Either party opens a case, and a moderator decides. This is
 * what makes the arrangement complete rather than merely hopeful: the failure
 * that actually happens is not fraud but a buyer who received their goods and
 * moved on, or lost their phone. Because the **seller** can escalate too,
 * silence is not a weapon — it moves the decision to a person instead of
 * leaving the money stranded.
 *
 * There is deliberately **no automatic release on a timer**. A deadline that
 * pays the seller with nobody looking rewards precisely the seller who ships
 * nothing and waits, and it does so most reliably against buyers who are ill,
 * travelling, or simply offline. A human decision is slower, and is the right
 * trade.
 *
 * ## Where the money actually is
 *
 * In the chain's module account, held by `x/treasury`'s conditional locks —
 * not by this application and not by whoever operates it. Release needs the
 * buyer's signature, a case needs one of the two parties, and deciding a case
 * needs the moderator named when the deal was struck. None of those is us.
 *
 * What lives in *this* file is only the part that does not need consensus: the
 * fee arithmetic, and a local mirror of the deal list so the screen has
 * something to render before the chain answers. The chain is the record; this
 * is a cache, and where they disagree the chain is right.
 */
import { CURRENCIES } from './money.ts';

/** Platform fee, in basis points, charged **on top** of the amount escrowed. */
export const FEE_BPS = 100; // 1%

export type DealState = 'awaiting_seller' | 'in_progress' | 'in_review' | 'released' | 'refunded';

/** Who escalated. Recorded because a moderator reading the case needs to know
 *  which side asked, and because "the seller opened it" and "the buyer opened
 *  it" describe two completely different disputes. */
export type CaseParty = 'buyer' | 'seller';

export interface Case {
  openedBy: CaseParty;
  reason: string;
  openedAt: number;
}

export interface Deal {
  id: string;
  /** Counterparty user id. */
  seller: string;
  buyer: string;
  /** Base units of `denom`, excluding the fee. */
  amount: string;
  denom: string;
  /** Base units. Charged to the buyer on top of the amount. */
  fee: string;
  what: string;
  state: DealState;
  createdAt: number;
  /** Present once either party has escalated. */
  dispute?: Case;
}

const STORE = 'yamale.app.deals';

/**
 * The fee, rounded **up**.
 *
 * A fee that truncates lets somebody structure a payment to pay nothing —
 * split a large deal into many small ones and every fee rounds to zero. Round
 * up and the platform is never worse off for the split, which removes the
 * incentive to do it.
 */
export function feeFor(amount: string): string {
  const base = BigInt(amount || '0');
  const num = base * BigInt(FEE_BPS);
  const den = BigInt(10_000);
  return ((num + den - BigInt(1)) / den).toString();
}

/** What leaves the buyer's account: the amount plus the fee. */
export function totalFor(amount: string): string {
  return (BigInt(amount || '0') + BigInt(feeFor(amount))).toString();
}

export function deals(): Deal[] {
  try {
    const raw = localStorage.getItem(STORE);
    const list: Deal[] = raw ? JSON.parse(raw) : [];
    return list.sort((a, b) => b.createdAt - a.createdAt);
  } catch {
    return [];
  }
}

export function open(deal: Omit<Deal, 'id' | 'fee' | 'state' | 'createdAt'>): Deal {
  const now = Date.now();
  const created: Deal = {
    ...deal,
    // Ids are random rather than sequential: a counter tells anyone who reads
    // one how much business the platform did that week.
    id: cryptoId(),
    fee: feeFor(deal.amount),
    state: 'awaiting_seller',
    createdAt: now,
  };
  localStorage.setItem(STORE, JSON.stringify([created, ...deals()]));
  return created;
}

export function update(id: string, state: DealState): void {
  const list = deals().map((d) => (d.id === id ? { ...d, state } : d));
  localStorage.setItem(STORE, JSON.stringify(list));
}

/**
 * Open a case. Either side may, at any point after the money is held.
 *
 * The reason is required and kept: a moderator deciding between two strangers
 * has nothing else to go on, and a case with no statement is a case that will
 * be decided on whoever complains loudest afterwards.
 */
export function raiseCase(id: string, openedBy: CaseParty, reason: string): void {
  const list = deals().map((d) =>
    d.id === id && (d.state === 'in_progress' || d.state === 'awaiting_seller')
      ? { ...d, state: 'in_review' as DealState, dispute: { openedBy, reason, openedAt: Date.now() } }
      : d,
  );
  localStorage.setItem(STORE, JSON.stringify(list));
}

export function currencyName(denom: string): string {
  return CURRENCIES.find((c) => c.denom === denom)?.name ?? denom;
}

function cryptoId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(8));
  return Array.from(bytes).map((b) => b.toString(16).padStart(2, '0')).join('');
}
