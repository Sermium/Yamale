/**
 * The secondary market, in the terms somebody decides a trade in.
 *
 * Everything here is pure and takes the chain's own integers as strings, for
 * the reason vehicle.ts gives: the keeper does this arithmetic in integers, and
 * the only way an interface can promise a figure is to do the same arithmetic
 * on the same integers. Every function that mirrors a keeper says which one.
 *
 * # The conversion that is the whole risk
 *
 * `unit_price` is SETTLEMENT BASE UNITS PER ONE SHARE BASE UNIT. Both sides are
 * base units, and the keeper's expression is one line:
 *
 *     cost := qty.Mul(price)          msg_server_book.go
 *
 * A share resolves at exponent 0 — it is an indivisible count fixed at
 * fractionalisation — and `uusd` at exponent 6. So sixty cents a share is a
 * unit_price of 600000, not of 0.6 and not of 60.
 *
 * An interface that put a typed "0.60" into the field would offer a hundred
 * million shares for six cents, the chain would accept it because it is a
 * perfectly valid order, and the first person to read the book would take it.
 * That is the single most expensive mistake available on this surface, so the
 * conversion lives in one tested function and no component does it by hand.
 */

// The denom subpath rather than the barrel: this module is pure and tested
// with `node --test`, which cannot load the .tsx the barrel reaches through.
// The registry is the SDK's, not a copy — an exponent guessed here rather
// than resolved is the mistake this whole file exists to prevent.
import { resolveDenom, toBaseUnits, toDisplayAmount } from '@yamale/chain/denom';

/* ------------------------------------------------------------ chain shapes */

/** blockchain.market.v1.Side. */
export type Side = 'SIDE_UNSPECIFIED' | 'SIDE_BID' | 'SIDE_ASK';

/** blockchain.market.v1.ListingState. */
export type ListingState =
  | 'LISTING_STATE_UNSPECIFIED'
  | 'LISTING_STATE_LIVE'
  | 'LISTING_STATE_FILLED'
  | 'LISTING_STATE_CANCELLED';

/** blockchain.market.v1.AuctionState. */
export type AuctionState =
  | 'AUCTION_STATE_UNSPECIFIED'
  | 'AUCTION_STATE_OPEN'
  | 'AUCTION_STATE_SETTLED'
  | 'AUCTION_STATE_CANCELLED';

/** blockchain.market.v1.OrderState. */
export type OrderState =
  | 'ORDER_STATE_UNSPECIFIED'
  | 'ORDER_STATE_OPEN'
  | 'ORDER_STATE_FILLED'
  | 'ORDER_STATE_CANCELLED';

export interface Coin {
  denom: string;
  amount: string;
}

export interface Listing {
  id: string;
  assetId: string;
  denom: string;
  seller: string;
  /** Shares, in base units. A share is exponent 0, so this is a count. */
  quantity: string;
  /** The TOTAL asked for the parcel, not a price per share. */
  price: Coin;
  state: ListingState;
  listedAt: number;
  filledBy: string;
  filledAt: number;
}

export interface Auction {
  id: string;
  assetId: string;
  denom: string;
  seller: string;
  quantity: string;
  /** The least the seller will accept, as a total. Zero means no reserve. */
  reserve: Coin;
  closesAt: number;
  bestBidder: string;
  bestBid: Coin;
  state: AuctionState;
  openedAt: number;
  settledAt: number;
}

export interface Order {
  id: string;
  assetId: string;
  denom: string;
  trader: string;
  side: Side;
  quantity: string;
  remaining: string;
  /** Settlement base units per one share base unit. See the note at the top. */
  unitPrice: string;
  settlementDenom: string;
  state: OrderState;
  placedAt: number;
}

export interface Book {
  /** Highest price first, as the chain returns them. */
  bids: Order[];
  /** Lowest price first. */
  asks: Order[];
  settlementDenom: string;
}

export interface MarketParams {
  maxFillsPerOrder: number;
  minAuctionPeriod: number;
  maxAuctionPeriod: number;
}

/* ---------------------------------------------------------- the conversion */

/**
 * A typed price per share into the integer the chain stores.
 *
 * Returns null when the input is not a number the settlement denom can hold,
 * and `truncated` when digits the person typed are not in the price that will
 * be signed — which the caller owes them a note about, because a price is the
 * one field where a silently dropped digit is a different offer.
 *
 * Zero is refused rather than returned: a resting order at zero gives its
 * shares away or bids nothing for them, and the keeper refuses it
 * (ErrInvalidPrice). Refusing here means the person is told before they sign
 * rather than after the chain rejects it.
 */
export function unitPriceFromDisplay(
  pricePerShare: string,
  settlementDenom: string,
): { unitPrice: string; truncated: boolean } | null {
  const info = resolveDenom(settlementDenom);
  const parsed = toBaseUnits(pricePerShare, info.exponent);
  if (!parsed) return null;
  if (BigInt(parsed.base) <= 0n) return null;
  return { unitPrice: parsed.base, truncated: parsed.truncated };
}

/** The same, back: the integer the chain stores as a price a person reads. */
export function displayFromUnitPrice(unitPrice: string, settlementDenom: string): string {
  const info = resolveDenom(settlementDenom);
  return toDisplayAmount(unitPrice, info.exponent);
}

/**
 * What a parcel costs at a unit price: `quantity * unit_price`.
 *
 * Mirrors the keeper exactly (msg_server_book.go): integers, multiplied, no
 * rounding anywhere because there is nothing to round — both operands are whole
 * base units and so is the product.
 */
export function totalOf(quantity: string, unitPrice: string): string {
  try {
    return (BigInt(quantity) * BigInt(unitPrice)).toString();
  } catch {
    return '0';
  }
}

/**
 * The price per share implied by a LISTING, which names a total instead.
 *
 * Truncating division, and the remainder is why this returns `exact`. A parcel
 * of three shares at a total of ten pays 3.33… each, and a screen that printed
 * "3" beside a total of "10" would be inviting somebody to multiply the two and
 * find the wrong answer. The caller shows the total as the fact and this as the
 * gloss.
 */
export function impliedUnitPrice(
  total: string,
  quantity: string,
): { unitPrice: string; exact: boolean } | null {
  try {
    const q = BigInt(quantity);
    if (q <= 0n) return null;
    const t = BigInt(total);
    return { unitPrice: (t / q).toString(), exact: t % q === 0n };
  } catch {
    return null;
  }
}

/* ------------------------------------------------------------ the listings */

/** True when a listing is still there to be taken. */
export function isLive(listing: Listing): boolean {
  return listing.state === 'LISTING_STATE_LIVE';
}

/**
 * What a reader may do with a listing, given who they are.
 *
 * A seller cannot buy their own listing — the keeper refuses it — so offering
 * the button and letting the chain say no would be showing somebody an action
 * that never existed.
 */
export function listingActions(
  listing: Listing,
  viewer: string,
): { buy: boolean; cancel: boolean } {
  if (!isLive(listing)) return { buy: false, cancel: false };
  const mine = viewer !== '' && viewer === listing.seller;
  return { buy: !mine && viewer !== '', cancel: mine };
}

/* ------------------------------------------------------------ the auctions */

/**
 * Where an auction is in its life, as one word a screen can branch on.
 *
 * `closed` is the state with no equivalent in the enum and the one that matters
 * most: bidding has stopped, the money and the shares are both still in the
 * module account, and anybody at all may settle it. The chain records it as
 * OPEN until somebody does, so a screen reading the enum alone would invite a
 * bid that the keeper refuses.
 */
export type AuctionPhase = 'bidding' | 'closed' | 'settled' | 'cancelled' | 'unknown';

export function auctionPhase(auction: Auction, nowSeconds: number): AuctionPhase {
  switch (auction.state) {
    case 'AUCTION_STATE_SETTLED':
      return 'settled';
    case 'AUCTION_STATE_CANCELLED':
      return 'cancelled';
    case 'AUCTION_STATE_OPEN':
      return nowSeconds >= auction.closesAt ? 'closed' : 'bidding';
    default:
      return 'unknown';
  }
}

/** Seconds until bidding stops; zero once it has. */
export function secondsRemaining(auction: Auction, nowSeconds: number): number {
  return Math.max(0, auction.closesAt - nowSeconds);
}

/** True when a bid has been made at all. */
export function hasBid(auction: Auction): boolean {
  try {
    return auction.bestBid.amount !== '' && BigInt(auction.bestBid.amount) > 0n;
  } catch {
    return false;
  }
}

/** True when the standing bid meets the seller's reserve. */
export function reserveMet(auction: Auction): boolean {
  try {
    const reserve = auction.reserve.amount === '' ? 0n : BigInt(auction.reserve.amount);
    if (reserve === 0n) return hasBid(auction);
    return hasBid(auction) && BigInt(auction.bestBid.amount) >= reserve;
  } catch {
    return false;
  }
}

/**
 * The least a new bid may be, as a total in settlement base units.
 *
 * A bid must STRICTLY beat the standing one and must meet the reserve, so the
 * floor is one unit above the better of the two — and when nobody has bid, it
 * is the reserve itself rather than one above it. Computed here rather than
 * left to the person, because "must beat" is the sort of rule a form gets
 * wrong by one and the chain then refuses after they have signed.
 */
export function minimumBid(auction: Auction): string {
  let floor = 0n;
  try {
    const reserve = auction.reserve.amount === '' ? 0n : BigInt(auction.reserve.amount);
    if (hasBid(auction)) {
      const best = BigInt(auction.bestBid.amount);
      floor = best + 1n;
      if (reserve > floor) floor = reserve;
    } else {
      floor = reserve > 0n ? reserve : 1n;
    }
  } catch {
    return '1';
  }
  return floor.toString();
}

/**
 * What a reader may do with an auction.
 *
 * Settling is offered to everybody once bidding has stopped, and that is the
 * module's own decision rather than a convenience: if settlement needed the
 * seller's signature, a seller who disliked the result could strand the
 * winner's capital indefinitely by doing nothing.
 */
export function auctionActions(
  auction: Auction,
  viewer: string,
  nowSeconds: number,
): { bid: boolean; settle: boolean; cancel: boolean } {
  const phase = auctionPhase(auction, nowSeconds);
  const mine = viewer !== '' && viewer === auction.seller;
  return {
    bid: phase === 'bidding' && viewer !== '' && !mine,
    settle: phase === 'closed',
    // Only while nobody has bid. Once money is escrowed behind a bid the seller
    // cannot take the parcel back; the keeper refuses it.
    cancel: mine && phase === 'bidding' && !hasBid(auction),
  };
}

/* ---------------------------------------------------------------- the book */

/** One price level, which is what a reader compares — not individual orders. */
export interface Level {
  unitPrice: string;
  /** Shares resting at this price, summed across every order at it. */
  quantity: string;
  /** How many separate orders make it up. */
  orders: number;
  /** Shares resting at this price or better, walking from the top of the book. */
  cumulative: string;
}

/**
 * Orders aggregated into price levels, in the order the chain returned them.
 *
 * The query returns individual orders because that is what it stores; a reader
 * comparing depth wants levels, and three traders resting 100 each at the same
 * price is one level of 300 rather than three rows that look like three
 * different prices at a glance.
 *
 * The input order is preserved rather than re-sorted: the chain returns bids
 * highest-first and asks lowest-first, which is already best-first for each
 * side, and re-sorting here would silently disagree with the chain if its
 * ordering ever changed.
 */
export function levels(orders: Order[]): Level[] {
  const out: Level[] = [];
  let running = 0n;
  for (const order of orders) {
    let qty: bigint;
    try {
      qty = BigInt(order.remaining);
    } catch {
      continue;
    }
    if (qty <= 0n) continue;

    const last = out[out.length - 1];
    if (last && last.unitPrice === order.unitPrice) {
      last.quantity = (BigInt(last.quantity) + qty).toString();
      last.orders += 1;
      running += qty;
      last.cumulative = running.toString();
      continue;
    }
    running += qty;
    out.push({
      unitPrice: order.unitPrice,
      quantity: qty.toString(),
      orders: 1,
      cumulative: running.toString(),
    });
  }
  return out;
}

/** The best bid and the best ask, and the gap between them. */
export interface Spread {
  bid: string | null;
  ask: string | null;
  /** ask - bid, or null when either side is empty. */
  spread: string | null;
  /** True when the best bid is at or above the best ask. */
  crossed: boolean;
}

export function spreadOf(book: Book): Spread {
  const bid = book.bids.find((o) => BigInt(o.remaining || '0') > 0n)?.unitPrice ?? null;
  const ask = book.asks.find((o) => BigInt(o.remaining || '0') > 0n)?.unitPrice ?? null;
  if (bid === null || ask === null) return { bid, ask, spread: null, crossed: false };
  const gap = BigInt(ask) - BigInt(bid);
  return { bid, ask, spread: gap.toString(), crossed: gap <= 0n };
}

/**
 * What an order would do against the book as it stands right now.
 *
 * A forecast, and labelled as one everywhere it is shown. It walks the resting
 * side at the MAKER's price, exactly as the keeper does, and stops at the
 * per-transaction fill bound — which is the part a reader cannot work out for
 * themselves and the reason a remainder can rest with a full book behind it.
 *
 * It cannot promise the outcome: the book may move between the preview and the
 * block. That is why `filled` and `resting` are returned separately rather than
 * as a single "you will get" number.
 */
export interface Preview {
  /** Shares that would cross immediately. */
  filled: string;
  /** What those fills would cost or raise, in settlement base units. */
  settled: string;
  /** Shares that would rest in the book afterwards. */
  resting: string;
  /** True when the fill bound stopped it rather than the price running out. */
  boundReached: boolean;
}

export function previewOrder(
  book: Book,
  side: Side,
  quantity: string,
  unitPrice: string,
  maxFills: number,
): Preview {
  let want: bigint;
  let limit: bigint;
  try {
    want = BigInt(quantity);
    limit = BigInt(unitPrice);
  } catch {
    return { filled: '0', settled: '0', resting: '0', boundReached: false };
  }
  if (want <= 0n || limit <= 0n) {
    return { filled: '0', settled: '0', resting: want > 0n ? want.toString() : '0', boundReached: false };
  }

  // A bid takes from the asks, and vice versa.
  const resting = side === 'SIDE_BID' ? book.asks : book.bids;
  let filled = 0n;
  let settled = 0n;
  let fills = 0;
  let boundReached = false;

  for (const maker of resting) {
    if (want === 0n) break;
    if (fills >= maxFills) {
      boundReached = true;
      break;
    }
    let available: bigint;
    let price: bigint;
    try {
      available = BigInt(maker.remaining);
      price = BigInt(maker.unitPrice);
    } catch {
      continue;
    }
    if (available <= 0n) continue;

    // The crossing rule, from the keeper: a bid takes an ask at or below its
    // limit, an ask takes a bid at or above it.
    const crosses = side === 'SIDE_BID' ? price <= limit : price >= limit;
    if (!crosses) break;

    const qty = available < want ? available : want;
    filled += qty;
    // The MAKER's price, never the incoming one. Where a bid crosses an ask
    // below its limit the difference is refunded to the bidder rather than paid
    // to the seller.
    settled += qty * price;
    want -= qty;
    fills += 1;
  }

  return {
    filled: filled.toString(),
    settled: settled.toString(),
    resting: want.toString(),
    boundReached,
  };
}

/* ------------------------------------------------------- what may be sold */

/**
 * Whether a holder can offer a parcel at all, and why not when they cannot.
 *
 * The balance check is the one worth doing before the form rather than after
 * the signature: the shares leave the seller at LISTING, not at fill, so an
 * offer of more than is held fails at the escrow send with a bank error that
 * says nothing about markets.
 */
export type SellRefusal = 'no-shares' | 'more-than-held' | 'not-a-number' | 'zero' | null;

export function checkParcel(quantity: string, balance: string): SellRefusal {
  let held: bigint;
  let want: bigint;
  try {
    held = BigInt(balance || '0');
    want = BigInt(quantity);
  } catch {
    return 'not-a-number';
  }
  if (held <= 0n) return 'no-shares';
  if (want <= 0n) return 'zero';
  if (want > held) return 'more-than-held';
  return null;
}

/**
 * The bounds an auction's closing time has to sit inside, as absolute seconds.
 *
 * The message carries an absolute time rather than a duration, and the proto
 * says why: a duration is measured from whenever the transaction is included,
 * so a seller signing "one week" during congestion gets an auction closing at a
 * moment they did not choose, with their shares escrowed for the difference.
 * A form therefore has to turn the duration a person picks into a time, and
 * these are the bounds it must keep inside.
 */
export function auctionWindow(
  params: MarketParams,
  nowSeconds: number,
): { earliest: number; latest: number } {
  return {
    earliest: nowSeconds + params.minAuctionPeriod,
    latest: nowSeconds + params.maxAuctionPeriod,
  };
}
