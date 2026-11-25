/**
 * Ownership cards, outside React so they can be tested.
 *
 * A card's secret is made on this device just before its MsgIssueCard and kept
 * in this browser (keptCard below), so the shop can show, share or print the
 * card again until the piece is sold. It reaches no server. Only cards whose
 * transaction the chain accepted are printed: a card printed for a key the
 * chain does not hold would be a card that claims nothing.
 */

import { compressedPublicKey, parseCardFragment, toHex } from '../../../sdk/src/jewel.ts';

export interface CardPiece { jewelId: string; title: string | null; inscription: string }
export interface IssuedCard extends CardPiece { privateKey: Uint8Array }
export type IssueOutcome = 'issued' | 'failed' | 'unknown' | 'notRun';

export interface IssueResult {
  issued: IssuedCard[];
  outcomes: Map<string, IssueOutcome>;
  /** What stopped the run, already worded for people; null when every batch went through. */
  stopped: string | null;
}

/**
 * Cards per transaction. An issue writes one small entry; registration fits 23
 * a transaction at about 51,200 gas each, and a card costs well under that, so
 * 25 keeps the same headroom.
 */
export const CARD_BATCH = 25;

export type BatchSend = (cards: { jewelId: string; cardKey: string; privateKey: Uint8Array }[]) => Promise<{ ok: true } | { ok: false; text: string }>;

/**
 * Issues cards batch by batch and stops at the first batch that does not go
 * through. A batch that threw may or may not have reached the chain: it is
 * "unknown", and printing that piece again settles it, since a new card
 * replaces whatever the chain holds.
 */
export async function issueCards(pieces: CardPiece[], send: BatchSend, size = CARD_BATCH,
  makeKey: () => Uint8Array = randomKey, progress: (done: number, of: number) => void = () => {}): Promise<IssueResult> {
  const outcomes = new Map<string, IssueOutcome>(pieces.map((p) => [p.jewelId, 'notRun']));
  const issued: IssuedCard[] = [];
  progress(0, pieces.length);
  for (let i = 0; i < pieces.length; i += size) {
    const batch = pieces.slice(i, i + size).map((p) => ({ ...p, privateKey: makeKey() }));
    let sent: Awaited<ReturnType<BatchSend>>;
    try {
      sent = await send(batch.map((c) => ({ jewelId: c.jewelId, cardKey: toHex(compressedPublicKey(c.privateKey)), privateKey: c.privateKey })));
    } catch (e) {
      // Not wiped: the batch may have reached the chain, and a kept copy is what lets the shop find the card.
      for (const c of batch) outcomes.set(c.jewelId, 'unknown');
      return { issued, outcomes, stopped: e instanceof Error ? e.message : String(e) };
    }
    if (!sent.ok) {
      for (const c of batch) { outcomes.set(c.jewelId, 'failed'); c.privateKey.fill(0); }
      return { issued, outcomes, stopped: sent.text };
    }
    for (const c of batch) { outcomes.set(c.jewelId, 'issued'); issued.push(c); }
    progress(issued.length, pieces.length);
  }
  return { issued, outcomes, stopped: null };
}

function randomKey(): Uint8Array {
  // A valid secp256k1 scalar: 32 random bytes are one with overwhelming odds,
  // and parseCardFragment rejects the rest, so draw again rather than risk it.
  for (;;) {
    const k = crypto.getRandomValues(new Uint8Array(32));
    if (parseCardFragment(`1.${toHex(k)}`)) return k;
  }
}

/* ------------------------------------------------- the scan, kept per tab */

/**
 * Signing up or signing in happens on the card page itself, but a reload, or a
 * recovery that leaves the page, would drop the fragment. The card is kept in
 * this tab's sessionStorage until the claim is filed, then erased. It is the
 * buyer's own phone, holding a secret they just scanned off their own card.
 */
const KEY = 'yamale.traceability.card';

export interface ScannedCard { jewelId: string; privateKey: Uint8Array }

export function cardFromLocation(hash: string, store: Storage | null = session()): ScannedCard | null {
  const fresh = parseCardFragment(hash);
  if (fresh) {
    try { store?.setItem(KEY, `${fresh.jewelId}.${toHex(fresh.privateKey)}`); } catch { /* storage refused: the fragment still works */ }
    return fresh;
  }
  if (hash.replace(/^#/, '') !== '') return null;
  try {
    const kept = store?.getItem(KEY);
    return kept ? parseCardFragment(kept) : null;
  } catch {
    return null;
  }
}

export function forgetScannedCard(store: Storage | null = session()): void {
  try { store?.removeItem(KEY); } catch { /* nothing kept */ }
}

function session(): Storage | null {
  try { return typeof sessionStorage === 'undefined' ? null : sessionStorage; } catch { return null; }
}

/* ------------------------------------------ cards kept in the shop's browser */

/**
 * The shop's own browser keeps each card it issued, one per jewel (the newest),
 * so a card can be shown at the counter, shared to a customer or printed again
 * until the piece leaves stock. A card held here can only file a claim, which
 * the shop itself has to validate, so keeping it adds nothing a thief could use
 * against the shop. Forgotten once the jewel has no card or is no longer stock.
 */
const KEPT = 'yamale.traceability.card.';

export function keepCard(jewelId: string, privateKey: Uint8Array, store: Storage | null = local()): void {
  try { store?.setItem(KEPT + jewelId, toHex(privateKey)); } catch { /* storage refused: print it now */ }
}

export function keptCard(jewelId: string, store: Storage | null = local()): Uint8Array | null {
  try {
    const hex = store?.getItem(KEPT + jewelId);
    return hex ? parseCardFragment(`${jewelId}.${hex}`)?.privateKey ?? null : null;
  } catch {
    return null;
  }
}

export function forgetKeptCard(jewelId: string, store: Storage | null = local()): void {
  try { store?.removeItem(KEPT + jewelId); } catch { /* nothing kept */ }
}

function local(): Storage | null {
  try { return typeof localStorage === 'undefined' ? null : localStorage; } catch { return null; }
}

/* ------------------------------------------------------------ choosing */

export interface StockPiece extends CardPiece { hasCard: boolean; design: string }

export type Selection = { kind: 'without' } | { kind: 'design'; design: string } | { kind: 'one'; jewelId: string } | { kind: 'some'; jewelIds: string[] };

/** The pieces a selection prints, in the order the stock lists them. */
export function selectPieces(stock: StockPiece[], sel: Selection): StockPiece[] {
  switch (sel.kind) {
    case 'without': return stock.filter((p) => !p.hasCard);
    case 'design': return stock.filter((p) => p.design === sel.design);
    case 'one': return stock.filter((p) => p.jewelId === sel.jewelId);
    case 'some': return stock.filter((p) => sel.jewelIds.includes(p.jewelId));
  }
}

/** How many of the chosen pieces already have a card that printing would cancel. */
export function cancels(pieces: StockPiece[]): number {
  return pieces.filter((p) => p.hasCard).length;
}

/** Cards per printed A4 page: two columns of four, each about 90 × 60 mm. */
export const CARDS_PER_PAGE = 8;

export function pages<T>(items: T[], per = CARDS_PER_PAGE): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < items.length; i += per) out.push(items.slice(i, i + per));
  return out;
}
