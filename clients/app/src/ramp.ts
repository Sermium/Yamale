/**
 * On-ramp and off-ramp — getting money in and out.
 *
 * The chain can move value between anyone in seconds. What it cannot do by
 * itself is turn a bank transfer into tokens, or tokens into notes in a hand.
 * Something outside the chain has to take custody of the real money at one end.
 * This file is the honest boundary of that: it models the *request* to ramp,
 * and who settles it, without pretending the app has a banking licence.
 *
 * Two routes, deliberately different in nature:
 *
 *   - **Partner** — a bank, card processor or mobile-money operator. Money in
 *     from an account, tokens out to the wallet. Instant-ish, but it needs the
 *     sender to have an account somewhere, which is exactly what the unbanked
 *     do not have.
 *   - **Agent** — a shop. Cash over a counter, tokens over the same counter.
 *     This is the route that works for somebody with no bank at all, and it is
 *     why the agent directory exists.
 *
 * A ramp request is a *claim on a settlement*, not a transfer. Nothing moves
 * on-chain until the counterparty confirms they have the money — which for the
 * PoC is simulated, and in production would be an escrow release (the funds
 * sit in x/treasury escrow until both sides confirm, exactly like a secured
 * payment).
 */
import { CURRENCIES } from './money.ts';

export type RampKind = 'in' | 'out';
export type RampRoute = 'partner' | 'agent';

export interface RampQuote {
  kind: RampKind;
  route: RampRoute;
  denom: string;
  /**
   * Whose account the money lands in, on the way **in**.
   *
   * Empty means your own. Anything else is the whole point of a remittance: the
   * person paying cash at a counter in Paris is not the person who needs it in
   * Dakar, and a ramp that could only credit the payer would make the app a
   * currency exchange rather than a way to send money home. The beneficiary is
   * named as an alias or an address and the credit goes straight to their
   * account — the sender never holds it, so there is nothing for them to forget
   * to forward.
   *
   * Unused on the way out: money out always leaves the account of whoever is
   * signing, because taking value out of somebody else's account is not a ramp,
   * it is a withdrawal.
   */
  beneficiary?: string;
  /**
   * How the fiat arrives, on the way **out** — and in which currency.
   *
   * Chosen per settlement rather than per shop, because one counter often does
   * notes, mobile money and a bank push at three different prices.
   */
  payout?: { fiat: string; method: string; feeBps: number };
  /** What the person hands over (in) or receives (out), in base units. */
  amount: bigint;
  /** The counterparty's cut, in base units. */
  fee: bigint;
  /** What actually lands, after the fee. */
  net: bigint;
  feeBps: number;
  counterparty: string;
}

/**
 * The remittance currencies.
 *
 * USDC and EURC exist for one reason: a corridor needs a stable unit that both
 * ends recognise. Somebody in Paris sends EURC; their family in Dakar takes XOF
 * from a shop. The chain swaps EURC→XOF through the YML hub in the middle, and
 * neither end has to think about it.
 */
export const RAMP_DENOMS = ['uusdc', 'ueurc'];

export function isRampDenom(denom: string): boolean {
  return RAMP_DENOMS.includes(denom);
}

/**
 * Partner rails, with the fee each charges.
 *
 * Dummy for the PoC. The numbers are indicative of what these rails really cost
 * in the corridors this chain serves — card is dear, mobile money is cheap and
 * ubiquitous, bank transfer is slow and mid-priced. Showing them side by side is
 * the point: remittance corridors are expensive mostly because nobody shows the
 * customer the comparison.
 */
export const PARTNERS = [
  { id: 'card', name: 'Card', feeBps: 290, note: 'Visa or Mastercard. Instant, dearest.' },
  { id: 'bank', name: 'Bank transfer', feeBps: 90, note: 'SEPA or SWIFT. One to two days.' },
  { id: 'momo', name: 'Mobile money', feeBps: 120, note: 'M-Pesa, Orange Money, Wave.' },
];

/** Quote a ramp: what it costs and what actually arrives. */
export function quoteRamp(
  kind: RampKind,
  route: RampRoute,
  denom: string,
  amount: bigint,
  feeBps: number,
  counterparty: string,
  extra: { beneficiary?: string; payout?: RampQuote['payout'] } = {},
): RampQuote {
  // Fee is taken off the top in both directions: putting money in costs the
  // same as taking it out, and hiding one of them would make the cheaper
  // direction look free when it is not.
  // The payout method's own charge is added to the counterparty's, because the
  // beneficiary pays both and only cares about the total. Showing the shop's fee
  // alone would make a bank push look like it costs what cash costs.
  const total = feeBps + (extra.payout?.feeBps ?? 0);
  const fee = (amount * BigInt(total)) / 10_000n;
  return {
    kind, route, denom, amount, fee,
    net: amount - fee,
    feeBps: total, counterparty,
    beneficiary: extra.beneficiary,
    payout: extra.payout,
  };
}

export interface RampRequest extends RampQuote {
  id: string;
  createdAt: number;
  status: 'awaiting' | 'settled' | 'cancelled';
  /** The code the person shows the agent, or quotes to the partner. */
  reference: string;
}

const STORE = 'yamale.app.ramps';

/**
 * Ramp requests are kept on the device, not the chain.
 *
 * A pending request is an intention, and an intention is nobody else's
 * business. Once it settles, the resulting transfer is on-chain like any other
 * — that is the part that needs to be public and permanent.
 */
export function saved(): RampRequest[] {
  try {
    const raw = JSON.parse(localStorage.getItem(STORE) ?? '[]');
    return Array.isArray(raw) ? raw.map(reviveBigints) : [];
  } catch {
    return [];
  }
}

function reviveBigints(r: any): RampRequest {
  return { ...r, amount: BigInt(r.amount), fee: BigInt(r.fee), net: BigInt(r.net) };
}

export function save(req: RampRequest): void {
  const all = saved();
  const next = [req, ...all.filter((r) => r.id !== req.id)].slice(0, 30);
  try {
    localStorage.setItem(STORE, JSON.stringify(next.map((r) => ({
      ...r, amount: r.amount.toString(), fee: r.fee.toString(), net: r.net.toString(),
    }))));
  } catch {
    // Private browsing: the request still works for this session.
  }
}

export function updateStatus(id: string, status: RampRequest['status']): void {
  const all = saved();
  const hit = all.find((r) => r.id === id);
  if (!hit) return;
  save({ ...hit, status });
}

/**
 * A short reference the counterparty can read back.
 *
 * Six characters from an unambiguous alphabet — no I, O, 0 or 1, because this
 * gets read aloud across a counter in a noisy market and written on paper.
 */
export function reference(): string {
  const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  const bytes = new Uint8Array(6);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => alphabet[b % alphabet.length]).join('');
}

export function newRequest(quote: RampQuote): RampRequest {
  return {
    ...quote,
    id: reference() + '-' + Date.now().toString(36),
    createdAt: Date.now(),
    status: 'awaiting',
    reference: reference(),
  };
}

/** Currencies an agent or partner can ramp, as app currency records. */
export function rampCurrencies() {
  return CURRENCIES.filter((c) => RAMP_DENOMS.includes(c.denom));
}
