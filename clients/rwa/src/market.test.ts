/**
 * The secondary market's arithmetic.
 *
 * The first test is the one this file exists for. Everything else is a
 * mechanism that can be reasoned about from the screen; the unit price cannot,
 * because the number a person types and the number the chain stores differ by a
 * factor of a million and nothing on the page says so.
 */
import { strict as assert } from 'node:assert';
import { test } from 'node:test';

import {
  auctionActions,
  auctionPhase,
  auctionWindow,
  checkParcel,
  displayFromUnitPrice,
  impliedUnitPrice,
  levels,
  listingActions,
  minimumBid,
  previewOrder,
  reserveMet,
  secondsRemaining,
  spreadOf,
  totalOf,
  unitPriceFromDisplay,
  type Auction,
  type Book,
  type Listing,
  type Order,
} from './market.ts';

const SHARE = 'tok/4/SRMT';
const USD = 'uusd';

function order(o: Partial<Order>): Order {
  return {
    id: '1', assetId: '4', denom: SHARE, trader: 'yml1a', side: 'SIDE_BID',
    quantity: '100', remaining: '100', unitPrice: '600000', settlementDenom: USD,
    state: 'ORDER_STATE_OPEN', placedAt: 0, ...o,
  };
}

function auction(a: Partial<Auction>): Auction {
  return {
    id: '1', assetId: '4', denom: SHARE, seller: 'yml1seller', quantity: '1000',
    reserve: { denom: USD, amount: '0' }, closesAt: 1_000,
    bestBidder: '', bestBid: { denom: USD, amount: '0' },
    state: 'AUCTION_STATE_OPEN', openedAt: 0, settledAt: 0, ...a,
  };
}

function listing(l: Partial<Listing>): Listing {
  return {
    id: '1', assetId: '4', denom: SHARE, seller: 'yml1seller', quantity: '1000',
    price: { denom: USD, amount: '600000000' }, state: 'LISTING_STATE_LIVE',
    listedAt: 0, filledBy: '', filledAt: 0, ...l,
  };
}

/* ============================ THE ONE THAT MATTERS ======================= */

test('sixty cents a share is a unit price of six hundred thousand', () => {
  // A share is exponent 0 — an indivisible count fixed at fractionalisation.
  // uusd is exponent 6. The keeper multiplies the two base-unit integers
  // directly, so the price field carries settlement base units per one share.
  const got = unitPriceFromDisplay('0.60', USD);
  assert.ok(got);
  assert.equal(got.unitPrice, '600000');
  assert.equal(got.truncated, false);
});

test('the whole timber holding at sixty cents is sixty million dollars, not sixty', () => {
  // The failure this guards: passing the typed 0.60 through unconverted would
  // offer 100,000,000 shares for 60 base units of uusd — six cents — and the
  // chain would accept it, because it is a perfectly valid order.
  const priced = unitPriceFromDisplay('0.60', USD);
  assert.ok(priced);
  const total = totalOf('100000000', priced.unitPrice);
  assert.equal(total, '60000000000000');           // base units of uusd
  assert.equal(displayFromUnitPrice(total, USD), '60000000'); // $60,000,000

  const naive = totalOf('100000000', '60');        // what the bug would produce
  assert.equal(displayFromUnitPrice(naive, USD), '6000');
  assert.notEqual(total, naive);
});

test('a price round-trips through the chain and back', () => {
  for (const typed of ['0.6', '0.60', '1', '12.345678', '0.000001']) {
    const there = unitPriceFromDisplay(typed, USD);
    assert.ok(there, `${typed} should convert`);
    const back = displayFromUnitPrice(there.unitPrice, USD);
    assert.equal(Number(back), Number(typed), `${typed} -> ${there.unitPrice} -> ${back}`);
  }
});

test('digits the denom cannot hold are reported, not silently dropped', () => {
  // uusd holds six places. A seventh is a digit the person typed that will not
  // be in the price they sign, and a price is the one field where that is a
  // different offer rather than a rounding.
  const got = unitPriceFromDisplay('0.1234567', USD);
  assert.ok(got);
  assert.equal(got.unitPrice, '123456');
  assert.equal(got.truncated, true);
});

test('a price of zero is refused before it reaches the chain', () => {
  // The keeper refuses it (ErrInvalidPrice): a resting order at zero gives its
  // shares away or bids nothing for them. Refusing here means being told before
  // signing rather than after.
  assert.equal(unitPriceFromDisplay('0', USD), null);
  assert.equal(unitPriceFromDisplay('0.0000001', USD), null, 'rounds to zero at this exponent');
  assert.equal(unitPriceFromDisplay('', USD), null);
  assert.equal(unitPriceFromDisplay('abc', USD), null);
});

/* ------------------------------------------------------------- a listing */

test('a listing names a total, and the price per share is a gloss on it', () => {
  const exact = impliedUnitPrice('600000000', '1000');
  assert.deepEqual(exact, { unitPrice: '600000', exact: true });

  // Three shares for ten pays 3.33… each. The total is the fact; this is not.
  const inexact = impliedUnitPrice('10', '3');
  assert.deepEqual(inexact, { unitPrice: '3', exact: false });
});

test('a seller is not offered a button to buy their own listing', () => {
  const l = listing({ seller: 'yml1me' });
  assert.deepEqual(listingActions(l, 'yml1me'), { buy: false, cancel: true });
  assert.deepEqual(listingActions(l, 'yml1you'), { buy: true, cancel: false });
  // Nobody attached: reading is fine, acting is not.
  assert.deepEqual(listingActions(l, ''), { buy: false, cancel: false });
});

test('a filled listing offers nothing to anybody', () => {
  const l = listing({ state: 'LISTING_STATE_FILLED', seller: 'yml1me' });
  assert.deepEqual(listingActions(l, 'yml1me'), { buy: false, cancel: false });
  assert.deepEqual(listingActions(l, 'yml1you'), { buy: false, cancel: false });
});

/* ------------------------------------------------------------ an auction */

test('an auction past its deadline is closed, which the enum does not say', () => {
  // The chain records it OPEN until somebody settles it. A screen reading the
  // enum alone would invite a bid the keeper refuses.
  const a = auction({ closesAt: 1_000 });
  assert.equal(auctionPhase(a, 999), 'bidding');
  assert.equal(auctionPhase(a, 1_000), 'closed');
  assert.equal(auctionPhase(a, 5_000), 'closed');
  assert.equal(secondsRemaining(a, 900), 100);
  assert.equal(secondsRemaining(a, 5_000), 0, 'never negative');
});

test('anybody may settle a closed auction, including the winner', () => {
  // The module's decision, not a convenience: if settlement needed the
  // seller's signature, a seller who disliked the result could strand the
  // winner's capital indefinitely by doing nothing.
  const a = auction({ closesAt: 1_000, seller: 'yml1seller' });
  for (const viewer of ['yml1seller', 'yml1bidder', 'yml1stranger', '']) {
    assert.equal(auctionActions(a, viewer, 2_000).settle, true, `settle for ${viewer || '(nobody)'}`);
  }
});

test('a seller may withdraw an auction only while nobody has bid', () => {
  const quiet = auction({ seller: 'yml1me', closesAt: 1_000 });
  assert.equal(auctionActions(quiet, 'yml1me', 500).cancel, true);

  const bidOn = auction({
    seller: 'yml1me', closesAt: 1_000,
    bestBidder: 'yml1you', bestBid: { denom: 'uusd', amount: '5' },
  });
  assert.equal(auctionActions(bidOn, 'yml1me', 500).cancel, false,
    'money is escrowed behind that bid; the parcel cannot be taken back');
});

test('the minimum bid strictly beats the standing one and clears the reserve', () => {
  // No bid, no reserve: anything positive.
  assert.equal(minimumBid(auction({})), '1');

  // No bid, with a reserve: the reserve itself, not one above it.
  assert.equal(minimumBid(auction({ reserve: { denom: USD, amount: '1000' } })), '1000');

  // A standing bid: one above it.
  assert.equal(minimumBid(auction({
    bestBid: { denom: USD, amount: '1000' }, bestBidder: 'yml1a',
  })), '1001');

  // A standing bid below the reserve — possible when the reserve was never
  // met — takes the reserve, which is higher.
  assert.equal(minimumBid(auction({
    reserve: { denom: USD, amount: '5000' },
    bestBid: { denom: USD, amount: '1000' }, bestBidder: 'yml1a',
  })), '5000');
});

test('a reserve of zero is met by any bid, and by no bid at all it is not', () => {
  assert.equal(reserveMet(auction({})), false, 'nobody has bid');
  assert.equal(reserveMet(auction({
    bestBid: { denom: USD, amount: '1' }, bestBidder: 'yml1a',
  })), true);
  assert.equal(reserveMet(auction({
    reserve: { denom: USD, amount: '100' },
    bestBid: { denom: USD, amount: '99' }, bestBidder: 'yml1a',
  })), false);
});

test('an auction window is bounded at both ends by the chain params', () => {
  const w = auctionWindow({ maxFillsPerOrder: 20, minAuctionPeriod: 3600, maxAuctionPeriod: 2_592_000 }, 1_000);
  assert.deepEqual(w, { earliest: 4_600, latest: 2_593_000 });
});

/* ---------------------------------------------------------------- the book */

test('orders at one price are one level, not three rows that look like three prices', () => {
  const out = levels([
    order({ id: '1', unitPrice: '600000', remaining: '100' }),
    order({ id: '2', unitPrice: '600000', remaining: '200' }),
    order({ id: '3', unitPrice: '550000', remaining: '50' }),
  ]);
  assert.equal(out.length, 2);
  assert.deepEqual(out[0], { unitPrice: '600000', quantity: '300', orders: 2, cumulative: '300' });
  assert.deepEqual(out[1], { unitPrice: '550000', quantity: '50', orders: 1, cumulative: '350' });
});

test('an order with nothing left of it is not a level', () => {
  const out = levels([
    order({ id: '1', unitPrice: '600000', remaining: '0' }),
    order({ id: '2', unitPrice: '550000', remaining: '10' }),
  ]);
  assert.equal(out.length, 1);
  assert.equal(out[0]!.unitPrice, '550000');
});

test('the spread is null rather than zero when one side is empty', () => {
  // Zero would read as "the book is tight", which is the opposite of "there is
  // nothing on one side of it".
  const oneSided: Book = { bids: [order({ unitPrice: '500000' })], asks: [], settlementDenom: USD };
  assert.deepEqual(spreadOf(oneSided), { bid: '500000', ask: null, spread: null, crossed: false });

  const both: Book = {
    bids: [order({ side: 'SIDE_BID', unitPrice: '500000' })],
    asks: [order({ side: 'SIDE_ASK', unitPrice: '600000' })],
    settlementDenom: USD,
  };
  assert.deepEqual(spreadOf(both), { bid: '500000', ask: '600000', spread: '100000', crossed: false });
});

/* ------------------------------------------------------------- the preview */

test('a bid fills at the resting ask price, not at its own limit', () => {
  // Price improvement belongs to the taker: the difference is refunded to the
  // bidder rather than paid to the seller. A preview that used the limit would
  // overstate the cost and understate what comes back.
  const book: Book = {
    bids: [],
    asks: [order({ side: 'SIDE_ASK', unitPrice: '500000', remaining: '100' })],
    settlementDenom: USD,
  };
  const p = previewOrder(book, 'SIDE_BID', '100', '600000', 20);
  assert.equal(p.filled, '100');
  assert.equal(p.settled, '50000000', 'paid at 500000, the ask, not at the 600000 limit');
  assert.equal(p.resting, '0');
});

test('a bid stops at its limit and rests the remainder', () => {
  const book: Book = {
    bids: [],
    asks: [
      order({ id: '1', side: 'SIDE_ASK', unitPrice: '500000', remaining: '30' }),
      order({ id: '2', side: 'SIDE_ASK', unitPrice: '900000', remaining: '70' }),
    ],
    settlementDenom: USD,
  };
  const p = previewOrder(book, 'SIDE_BID', '100', '600000', 20);
  assert.equal(p.filled, '30', 'the 900000 ask is above the limit');
  assert.equal(p.settled, '15000000');
  assert.equal(p.resting, '70');
  assert.equal(p.boundReached, false);
});

test('the fill bound is reported, because a remainder with a full book means something else', () => {
  // Non-zero resting with depth still behind it is the per-transaction bound,
  // not the price running out — and the next transaction continues it. A reader
  // told only "70 rested" would think their price was wrong.
  const asks = Array.from({ length: 5 }, (_, i) =>
    order({ id: String(i + 1), side: 'SIDE_ASK', unitPrice: '500000', remaining: '10' }));
  const book: Book = { bids: [], asks, settlementDenom: USD };

  const bounded = previewOrder(book, 'SIDE_BID', '50', '600000', 2);
  assert.equal(bounded.filled, '20');
  assert.equal(bounded.resting, '30');
  assert.equal(bounded.boundReached, true);

  const unbounded = previewOrder(book, 'SIDE_BID', '50', '600000', 20);
  assert.equal(unbounded.filled, '50');
  assert.equal(unbounded.boundReached, false);
});

test('an ask takes bids at or above its limit', () => {
  const book: Book = {
    bids: [
      order({ id: '1', side: 'SIDE_BID', unitPrice: '700000', remaining: '10' }),
      order({ id: '2', side: 'SIDE_BID', unitPrice: '400000', remaining: '10' }),
    ],
    asks: [],
    settlementDenom: USD,
  };
  const p = previewOrder(book, 'SIDE_ASK', '20', '600000', 20);
  assert.equal(p.filled, '10', 'the 400000 bid is below the ask limit');
  assert.equal(p.settled, '7000000');
  assert.equal(p.resting, '10');
});

test('an empty book fills nothing and rests everything', () => {
  const empty: Book = { bids: [], asks: [], settlementDenom: USD };
  const p = previewOrder(empty, 'SIDE_BID', '100', '600000', 20);
  assert.deepEqual(p, { filled: '0', settled: '0', resting: '100', boundReached: false });
});

/* ----------------------------------------------------- what may be offered */

test('a parcel larger than the holding is refused before the signature', () => {
  // The shares leave the seller at LISTING, not at fill, so an offer of more
  // than is held fails at the escrow send with a bank error that says nothing
  // about markets.
  assert.equal(checkParcel('100', '1000'), null);
  assert.equal(checkParcel('1000', '1000'), null, 'all of it is allowed');
  assert.equal(checkParcel('1001', '1000'), 'more-than-held');
  assert.equal(checkParcel('100', '0'), 'no-shares');
  assert.equal(checkParcel('0', '1000'), 'zero');
  assert.equal(checkParcel('x', '1000'), 'not-a-number');
});
