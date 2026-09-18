/**
 * Where a shareholding can be sold, and what it is worth to somebody else.
 *
 * Until this existed the register knew what everybody held and nothing about
 * what any of it cost, because a bank send names no price. Three mechanisms
 * answer that, and they are not three flavours of the same thing:
 *
 *   - A LISTING is a parcel at a total. The first buyer takes the whole of it.
 *     It is the right shape when a seller knows what they want and does not
 *     want to discover a price.
 *   - An AUCTION lets the market say what a parcel is worth by a deadline. It
 *     is the right shape when nobody knows, which on a vehicle with seven
 *     holders is most of the time.
 *   - The BOOK is for the part of a holding somebody wants to move at a price
 *     they name, in whatever size crosses.
 *
 * # What this screen refuses to do
 *
 * It shows no "market price" and no valuation. Six holders and no trades is not
 * a price, and a headline figure derived from one fill would be the most quoted
 * number on the page and the least supported. The best bid and the best ask are
 * shown as what they are — two offers — with the gap between them stated rather
 * than averaged away.
 */
import { useCallback, useEffect, useState } from 'react';
import { formatAmount, resolveDenom, t, useLocale } from '@yamale/chain';

import {
  auctions as fetchAuctions,
  bid as sendBid,
  book as fetchBook,
  buyListing as sendBuy,
  cancelAuction as sendCancelAuction,
  cancelListing as sendCancelListing,
  cancelOrder as sendCancelOrder,
  listParcel as sendList,
  listings as fetchListings,
  marketParams as fetchMarketParams,
  openAuction as sendOpenAuction,
  placeOrder as sendPlaceOrder,
  settleAuction as sendSettle,
} from './chain.ts';
import { Dialog, Outcome, PlanTable, type Phase } from './Act.tsx';
import { marketPlan, type MarketDraft } from './message.ts';
import { formatDuration } from './duration.ts';
import {
  auctionActions,
  auctionPhase,
  auctionWindow,
  checkParcel,
  displayFromUnitPrice,
  hasBid,
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
  type MarketParams,
  type Order,
  type SellRefusal,
} from './market.ts';
import {
  Address, Amount, Chip, Detail, Empty, Field, Fields, Kpi, KpiBand, Loading, Note, Panel,
  Unreachable,
} from './ui.tsx';
import type { Account } from './address.ts';
import type { Outcome as Result } from './abci.ts';

/* ------------------------------------------------------------------ data */

interface MarketView {
  listings: Listing[];
  auctions: Auction[];
  book: Book;
  params: MarketParams;
}

type Load<T> = { loading: true } | { loading: false; outcome: Result<T> };

/**
 * Everything this screen needs, in one round of four queries.
 *
 * They are fetched together rather than per panel because they are read
 * together: a spread is meaningless beside a stale listing, and four panels
 * that refreshed independently would show four different moments and invite
 * somebody to compare them.
 */
function useMarket(denom: string, nonce: number): Load<MarketView> {
  const [state, setState] = useState<Load<MarketView>>({ loading: true });

  useEffect(() => {
    let live = true;
    if (!denom) {
      setState({
        loading: false,
        outcome: {
          ok: true,
          value: {
            listings: [], auctions: [],
            book: { bids: [], asks: [], settlementDenom: '' },
            params: { maxFillsPerOrder: 0, minAuctionPeriod: 0, maxAuctionPeriod: 0 },
          },
          height: 0,
        },
      });
      return;
    }

    setState({ loading: true });
    Promise.all([
      fetchListings(denom, false),
      fetchAuctions(denom, false),
      fetchBook(denom, 20),
      fetchMarketParams(),
    ]).then(([l, a, b, p]) => {
      if (!live) return;
      // Any one of the four failing makes the whole view a failure rather than
      // a partial one. A book shown beside a listings panel that silently could
      // not be read is a screen saying "nothing is for sale" on no evidence.
      for (const part of [l, a, b, p]) {
        if (!part.ok) { setState({ loading: false, outcome: part }); return; }
      }
      setState({
        loading: false,
        outcome: {
          ok: true,
          height: b.ok ? b.height : 0,
          value: {
            listings: l.ok ? l.value : [],
            auctions: a.ok ? a.value : [],
            book: b.ok ? b.value : { bids: [], asks: [], settlementDenom: '' },
            params: p.ok ? p.value : { maxFillsPerOrder: 0, minAuctionPeriod: 0, maxAuctionPeriod: 0 },
          },
        },
      });
    });

    return () => { live = false; };
  }, [denom, nonce]);

  return state;
}

/* ------------------------------------------------------------- the pieces */

/**
 * A price per share, from the integer the chain stores.
 *
 * `formatAmount` rather than a bare conversion: it groups the digits, which
 * matters the moment a price is not under a dollar, and it appends the symbol
 * exactly once. Nothing else may append it again — the <Amount>/<Symbol> pair
 * printed "USD USD" on this app's vault panel and there is a test refusing it.
 */
function UnitPrice({ unitPrice, settlementDenom }: { unitPrice: string; settlementDenom: string }) {
  return <span className="y-num">{formatAmount(unitPrice, settlementDenom)}</span>;
}

/** The top of the book, and the gap, stated rather than averaged. */
function Spread({ book }: { book: Book }) {
  const s = spreadOf(book);
  const denom = book.settlementDenom;

  // Three across rather than three stacked. The comparison between the best bid
  // and the best ask IS the content — a reader who has to scroll from one to
  // the other is being asked to hold a price in their head, which is exactly
  // the work a market screen exists to save them.
  return (
    <KpiBand three>
      <Kpi label={t('mkt.bestBid')} hint={t('mkt.bestBidHint')}
           value={s.bid
             ? <UnitPrice unitPrice={s.bid} settlementDenom={denom} />
             : <span className="amount--empty">—</span>} />
      <Kpi label={t('mkt.bestAsk')} hint={t('mkt.bestAskHint')}
           value={s.ask
             ? <UnitPrice unitPrice={s.ask} settlementDenom={denom} />
             : <span className="amount--empty">—</span>} />
      <Kpi label={t('mkt.spread')} tone={s.crossed ? 'warn' : undefined}
           hint={s.spread === null ? t('mkt.spreadOneSided') : t('mkt.spreadHint')}
           value={s.spread
             ? <UnitPrice unitPrice={s.spread} settlementDenom={denom} />
             : <span className="amount--empty">—</span>} />
    </KpiBand>
  );
}

/**
 * One side of the book, aggregated into price levels.
 *
 * The three-column table — price, shares, cumulative — became one row per level
 * with the cumulative drawn as the bar behind it. The cumulative column is the
 * one a reader converts into a mental bar chart anyway, and at 358px three
 * numeric columns is either a sideways scroll or three cramped ones. The figure
 * itself is not lost: it sits under the size, in small type, because somebody
 * sizing an order against the book needs the number and not the picture.
 */
function BookSide({ orders, denom, settlementDenom, title, tone }: {
  orders: Order[];
  denom: string;
  settlementDenom: string;
  title: string;
  tone: 'bid' | 'ask';
}) {
  const rows = levels(orders);
  if (rows.length === 0) {
    return (
      <div className="panel">
        <h4 className="y-label">{title}</h4>
        <p className="muted">{t('mkt.sideEmpty')}</p>
      </div>
    );
  }

  // The deepest level sets the scale. Taken from the levels themselves rather
  // than from a constant, so a book with one order still draws a full bar and a
  // book with twenty draws a shape.
  const deepest = rows.reduce((m, l) => (BigInt(l.cumulative) > m ? BigInt(l.cumulative) : m), 0n);
  const width = (cumulative: string) => (deepest > 0n
    ? `${Number((BigInt(cumulative) * 1000n) / deepest) / 10}%`
    : '0%');

  return (
    <div className="panel">
      <h4 className="y-label">{title}</h4>
      <ul className="levels">
        {rows.map((level) => (
          <li key={level.unitPrice} className={`level level--${tone}`}>
            <span className="level__bar" aria-hidden="true"
                  style={{ inlineSize: width(level.cumulative) }} />
            <span className="level__price">
              <UnitPrice unitPrice={level.unitPrice} settlementDenom={settlementDenom} />
              {level.orders > 1 && (
                <span className="mkt__count"> {t('mkt.ordersAt', { n: level.orders })}</span>
              )}
            </span>
            <span className="level__size">
              <Amount amount={level.quantity} denom={denom} />
              <span className="level__cum">
                {t('mkt.colCumulative')} <Amount amount={level.cumulative} denom={denom} />
              </span>
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/* ------------------------------------------------------------ the dialogs */

type Pending =
  | { kind: 'list' }
  | { kind: 'auction' }
  | { kind: 'order' }
  | { kind: 'buy'; listing: Listing }
  | { kind: 'cancel-listing'; listing: Listing }
  | { kind: 'bid'; auction: Auction }
  | { kind: 'settle'; auction: Auction }
  | { kind: 'cancel-auction'; auction: Auction }
  | { kind: 'cancel-order'; order: Order }
  | null;

interface ActProps {
  account: Extract<Account, { mode: 'connected' }>;
  denom: string;
  settlementDenom: string;
  balance: string;
  params: MarketParams;
  onClose: () => void;
  onDone: () => void;
}

/** The refusal a quantity earned, in words, before anything is signed. */
function parcelNote(refusal: SellRefusal, balance: string, denom: string) {
  if (!refusal) return null;
  const key = {
    'no-shares': 'mkt.refuseNoShares',
    'more-than-held': 'mkt.refuseTooMany',
    'not-a-number': 'mkt.refuseNotANumber',
    zero: 'mkt.refuseZero',
  }[refusal];
  return (
    <Note tone="bad">
      {t(key)}{' '}
      {refusal === 'more-than-held' && t('mkt.youHold', {
        amount: formatAmount(balance, denom),
      })}
    </Note>
  );
}

function ListDialog({ account, denom, settlementDenom, balance, onClose, onDone }: ActProps) {
  const [quantity, setQuantity] = useState('');
  const [total, setTotal] = useState('');
  const [phase, setPhase] = useState<Phase>({ at: 'form' });

  const refusal = quantity === '' ? null : checkParcel(quantity, balance);
  const priced = total === '' ? null : unitPriceFromDisplay(total, settlementDenom);
  const each = priced && quantity !== '' && !refusal
    ? impliedUnitPrice(priced.unitPrice, quantity)
    : null;
  const ready = !refusal && quantity !== '' && priced !== null && phase.at === 'form';

  const draft: MarketDraft | null = ready
    ? {
      kind: 'list', seller: account.address, denom, quantity,
      price: priced.unitPrice, priceDenom: settlementDenom,
    }
    : null;

  const go = async () => {
    if (!priced) return;
    setPhase({ at: 'signing' });
    const result = await sendList(account.signer, account.address, denom, quantity, {
      denom: settlementDenom, amount: priced.unitPrice,
    });
    setPhase({ at: 'done', result });
    if (result.ok) onDone();
  };

  return (
    <Dialog title={t('mkt.listTitle')} lede={t('mkt.listLede')} tone="warn" onClose={onClose}>
      <label className="field2">
        <span className="y-label">{t('mkt.shares')}</span>
        <input className="y-num" inputMode="numeric" value={quantity}
               onChange={(e) => setQuantity(e.target.value.trim())}
               placeholder={formatAmount(balance, denom)} />
      </label>
      {parcelNote(refusal, balance, denom)}

      <label className="field2">
        <span className="y-label">{t('mkt.totalAsked', { unit: resolveDenom(settlementDenom).symbol })}</span>
        <input className="y-num" inputMode="decimal" value={total}
               onChange={(e) => setTotal(e.target.value.trim())} placeholder="0.00" />
      </label>
      {total !== '' && priced === null && <Note tone="bad">{t('mkt.refusePrice')}</Note>}
      {priced?.truncated && <Note tone="warn">{t('mkt.truncated')}</Note>}

      {each && (
        <p className="muted">
          {t('mkt.impliedEach')}{' '}
          <UnitPrice unitPrice={each.unitPrice} settlementDenom={settlementDenom} />
          {!each.exact && <> {t('mkt.notExact')}</>}
        </p>
      )}

      {/* The consequence the message does not mention, stated before the button
          rather than discovered from a balance afterwards. */}
      <Note tone="warn" title={t('mkt.escrowTitle')}>{t('mkt.listEscrow')}</Note>

      {draft && <PlanTable plan={marketPlan(draft)} />}

      {phase.at === 'done' ? <Outcome result={phase.result} onClose={onClose} /> : (
        <div className="sheet__actions">
          <button type="button" className="btn btn--ghost" onClick={onClose}>{t('action.cancel')}</button>
          <button type="button" className="btn" disabled={!ready} onClick={go}>
            {phase.at === 'signing' ? t('rwa.signing') : t('rwa.sign')}
          </button>
        </div>
      )}
    </Dialog>
  );
}

function AuctionDialog({ account, denom, settlementDenom, balance, params, onClose, onDone }: ActProps) {
  const [quantity, setQuantity] = useState('');
  const [reserve, setReserve] = useState('');
  const [hours, setHours] = useState('168');
  const [phase, setPhase] = useState<Phase>({ at: 'form' });

  const now = Math.floor(Date.now() / 1000);
  const window = auctionWindow(params, now);
  const closesAt = now + Math.round(Number(hours) * 3600);
  const inWindow = Number.isFinite(closesAt)
    && closesAt >= window.earliest && closesAt <= window.latest;

  const refusal = quantity === '' ? null : checkParcel(quantity, balance);
  // A reserve is optional: an empty field is no reserve, which is a real choice
  // ("take whatever it fetches") rather than a missing answer.
  const priced = reserve === '' ? { unitPrice: '0', truncated: false }
    : unitPriceFromDisplay(reserve, settlementDenom);
  const ready = !refusal && quantity !== '' && priced !== null && inWindow && phase.at === 'form';

  const draft: MarketDraft | null = ready
    ? {
      kind: 'open-auction', seller: account.address, denom, quantity,
      reserve: priced.unitPrice, reserveDenom: settlementDenom, closesAt,
    }
    : null;

  const go = async () => {
    if (!priced) return;
    setPhase({ at: 'signing' });
    const result = await sendOpenAuction(account.signer, account.address, denom, quantity,
      { denom: settlementDenom, amount: priced.unitPrice }, closesAt);
    setPhase({ at: 'done', result });
    if (result.ok) onDone();
  };

  return (
    <Dialog title={t('mkt.auctionTitle')} lede={t('mkt.auctionLede')} tone="warn" onClose={onClose}>
      <label className="field2">
        <span className="y-label">{t('mkt.shares')}</span>
        <input className="y-num" inputMode="numeric" value={quantity}
               onChange={(e) => setQuantity(e.target.value.trim())}
               placeholder={formatAmount(balance, denom)} />
      </label>
      {parcelNote(refusal, balance, denom)}

      <label className="field2">
        <span className="y-label">{t('mkt.reserve', { unit: resolveDenom(settlementDenom).symbol })}</span>
        <input className="y-num" inputMode="decimal" value={reserve}
               onChange={(e) => setReserve(e.target.value.trim())} placeholder={t('mkt.noReserve')} />
        <span className="field2__hint">{t('mkt.reserveHint')}</span>
      </label>
      {reserve !== '' && priced === null && <Note tone="bad">{t('mkt.refusePrice')}</Note>}

      <label className="field2">
        <span className="y-label">{t('mkt.runsFor')}</span>
        <input className="y-num" inputMode="numeric" value={hours}
               onChange={(e) => setHours(e.target.value.trim())} />
        <span className="field2__hint">
          {t('mkt.runsForHint', {
            min: formatDuration(params.minAuctionPeriod, 'en'),
            max: formatDuration(params.maxAuctionPeriod, 'en'),
          })}
        </span>
      </label>
      {!inWindow && hours !== '' && <Note tone="bad">{t('mkt.refuseWindow')}</Note>}

      {/* Why the message carries a time and not a duration — the seller is
          choosing a moment, and it is worth their knowing which one. */}
      <Note tone="warn" title={t('mkt.escrowTitle')}>{t('mkt.auctionEscrow')}</Note>

      {draft && <PlanTable plan={marketPlan(draft)} />}

      {phase.at === 'done' ? <Outcome result={phase.result} onClose={onClose} /> : (
        <div className="sheet__actions">
          <button type="button" className="btn btn--ghost" onClick={onClose}>{t('action.cancel')}</button>
          <button type="button" className="btn" disabled={!ready} onClick={go}>
            {phase.at === 'signing' ? t('rwa.signing') : t('rwa.sign')}
          </button>
        </div>
      )}
    </Dialog>
  );
}

function OrderDialog({ account, denom, settlementDenom, balance, params, book, onClose, onDone }:
ActProps & { book: Book }) {
  const [side, setSide] = useState<'SIDE_BID' | 'SIDE_ASK'>('SIDE_BID');
  const [quantity, setQuantity] = useState('');
  const [price, setPrice] = useState('');
  const [phase, setPhase] = useState<Phase>({ at: 'form' });

  // Only an ask is bounded by the holding; a bid is bounded by money, which
  // this screen does not read. The chain refuses an unfunded bid at escrow.
  const refusal = side === 'SIDE_ASK' && quantity !== '' ? checkParcel(quantity, balance) : null;
  const priced = price === '' ? null : unitPriceFromDisplay(price, settlementDenom);
  const ready = !refusal && quantity !== '' && priced !== null && phase.at === 'form';

  const preview = ready
    ? previewOrder(book, side, quantity, priced.unitPrice, params.maxFillsPerOrder)
    : null;

  const draft: MarketDraft | null = ready && preview
    ? {
      kind: 'place-order', trader: account.address, denom, side, quantity,
      unitPrice: priced.unitPrice, settlementDenom,
      fills: preview.filled, settles: preview.settled, rests: preview.resting,
    }
    : null;

  const go = async () => {
    if (!priced) return;
    setPhase({ at: 'signing' });
    const result = await sendPlaceOrder(account.signer, account.address, denom, side,
      quantity, priced.unitPrice);
    setPhase({ at: 'done', result });
    if (result.ok) onDone();
  };

  return (
    <Dialog title={t('mkt.orderTitle')} lede={t('mkt.orderLede')} tone="warn" onClose={onClose}>
      <div className="mkt__sides" role="group" aria-label={t('mkt.side')}>
        <button type="button"
                className={`btn btn--ghost${side === 'SIDE_BID' ? ' btn--on' : ''}`}
                aria-pressed={side === 'SIDE_BID'}
                onClick={() => setSide('SIDE_BID')}>{t('mkt.sideBuy')}</button>
        <button type="button"
                className={`btn btn--ghost${side === 'SIDE_ASK' ? ' btn--on' : ''}`}
                aria-pressed={side === 'SIDE_ASK'}
                onClick={() => setSide('SIDE_ASK')}>{t('mkt.sideSell')}</button>
      </div>

      <label className="field2">
        <span className="y-label">{t('mkt.shares')}</span>
        <input className="y-num" inputMode="numeric" value={quantity}
               onChange={(e) => setQuantity(e.target.value.trim())} />
      </label>
      {parcelNote(refusal, balance, denom)}

      <label className="field2">
        <span className="y-label">{t('mkt.limitPrice', { unit: resolveDenom(settlementDenom).symbol })}</span>
        <input className="y-num" inputMode="decimal" value={price}
               onChange={(e) => setPrice(e.target.value.trim())} placeholder="0.00" />
        <span className="field2__hint">{t('mkt.limitHint')}</span>
      </label>
      {price !== '' && priced === null && <Note tone="bad">{t('mkt.refusePrice')}</Note>}
      {priced?.truncated && <Note tone="warn">{t('mkt.truncated')}</Note>}

      {preview && (
        <div className="mkt__preview">
          <p className="y-label">{t('mkt.previewTitle')}</p>
          <dl className="fields">
            <div className="field">
              <dt className="y-label field__label">{t('mkt.fillsNow')}</dt>
              <dd className="field__value"><Amount amount={preview.filled} denom={denom} /></dd>
            </div>
            <div className="field">
              <dt className="y-label field__label">{side === 'SIDE_BID' ? t('mkt.costsNow') : t('mkt.raisesNow')}</dt>
              <dd className="field__value"><Amount amount={preview.settled} denom={settlementDenom} /></dd>
            </div>
            <div className="field">
              <dt className="y-label field__label">{t('mkt.rests')}</dt>
              <dd className="field__value"><Amount amount={preview.resting} denom={denom} /></dd>
            </div>
          </dl>
          {/* A forecast, and said so. The book can move between here and the
              block, and a figure presented as a promise would be a lie the
              first time it did. */}
          <p className="muted">{t('mkt.previewIsAForecast')}</p>
          {preview.boundReached && <Note tone="warn">{t('mkt.boundReached', {
            n: params.maxFillsPerOrder,
          })}</Note>}
        </div>
      )}

      {draft && <PlanTable plan={marketPlan(draft)} />}

      {phase.at === 'done' ? <Outcome result={phase.result} onClose={onClose} /> : (
        <div className="sheet__actions">
          <button type="button" className="btn btn--ghost" onClick={onClose}>{t('action.cancel')}</button>
          <button type="button" className="btn" disabled={!ready} onClick={go}>
            {phase.at === 'signing' ? t('rwa.signing') : t('rwa.sign')}
          </button>
        </div>
      )}
    </Dialog>
  );
}

/** Buy, cancel, settle: the actions with nothing to fill in. */
function ConfirmDialog({ title, lede, tone, draft, run, onClose, onDone }: {
  title: string;
  lede: string;
  tone: 'ok' | 'warn' | 'bad';
  draft: MarketDraft;
  run: () => Promise<import('./chain.ts').Signed>;
  onClose: () => void;
  onDone: () => void;
}) {
  const [phase, setPhase] = useState<Phase>({ at: 'form' });

  const go = async () => {
    setPhase({ at: 'signing' });
    const result = await run();
    setPhase({ at: 'done', result });
    if (result.ok) onDone();
  };

  return (
    <Dialog title={title} lede={lede} tone={tone} onClose={onClose}>
      <PlanTable plan={marketPlan(draft)} />
      {phase.at === 'done' ? <Outcome result={phase.result} onClose={onClose} /> : (
        <div className="sheet__actions">
          <button type="button" className="btn btn--ghost" onClick={onClose}>{t('action.cancel')}</button>
          <button type="button" className="btn" disabled={phase.at === 'signing'} onClick={go}>
            {phase.at === 'signing' ? t('rwa.signing') : t('rwa.sign')}
          </button>
        </div>
      )}
    </Dialog>
  );
}

/** A bid needs a figure, and a floor the chain will enforce anyway. */
function BidDialog({ account, auction, settlementDenom, onClose, onDone }: {
  account: Extract<Account, { mode: 'connected' }>;
  auction: Auction;
  settlementDenom: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const floor = minimumBid(auction);
  const [amount, setAmount] = useState(displayFromUnitPrice(floor, settlementDenom));
  const [phase, setPhase] = useState<Phase>({ at: 'form' });

  const priced = amount === '' ? null : unitPriceFromDisplay(amount, settlementDenom);
  const tooLow = priced !== null && BigInt(priced.unitPrice) < BigInt(floor);
  const ready = priced !== null && !tooLow && phase.at === 'form';

  const draft: MarketDraft | null = ready
    ? {
      kind: 'bid', bidder: account.address, auctionId: auction.id,
      amount: priced.unitPrice, amountDenom: settlementDenom,
      displaces: hasBid(auction) ? auction.bestBid.amount : '0',
    }
    : null;

  const go = async () => {
    if (!priced) return;
    setPhase({ at: 'signing' });
    const result = await sendBid(account.signer, account.address, auction.id,
      { denom: settlementDenom, amount: priced.unitPrice });
    setPhase({ at: 'done', result });
    if (result.ok) onDone();
  };

  return (
    <Dialog title={t('mkt.bidTitle')} lede={t('mkt.bidLede')} tone="warn" onClose={onClose}>
      <div className="sheet__headline">
        <span className="y-label">{t('mkt.youWouldGet')}</span>
        <Amount amount={auction.quantity} denom={auction.denom} className="sheet__figure" />
      </div>

      <label className="field2">
        <span className="y-label">{t('mkt.yourBid', { unit: resolveDenom(settlementDenom).symbol })}</span>
        <input className="y-num" inputMode="decimal" value={amount}
               onChange={(e) => setAmount(e.target.value.trim())} />
        <span className="field2__hint">
          {t('mkt.atLeast')} <Amount amount={floor} denom={settlementDenom} />
        </span>
      </label>
      {tooLow && <Note tone="bad">{t('mkt.refuseBidLow')}</Note>}
      {amount !== '' && priced === null && <Note tone="bad">{t('mkt.refusePrice')}</Note>}

      <Note tone="warn" title={t('mkt.escrowTitle')}>
        {hasBid(auction) ? t('mkt.bidEscrowDisplaces') : t('mkt.bidEscrow')}
      </Note>

      {draft && <PlanTable plan={marketPlan(draft)} />}

      {phase.at === 'done' ? <Outcome result={phase.result} onClose={onClose} /> : (
        <div className="sheet__actions">
          <button type="button" className="btn btn--ghost" onClick={onClose}>{t('action.cancel')}</button>
          <button type="button" className="btn" disabled={!ready} onClick={go}>
            {phase.at === 'signing' ? t('rwa.signing') : t('rwa.sign')}
          </button>
        </div>
      )}
    </Dialog>
  );
}

/* --------------------------------------------------------------- the panel */

export function Market({ assetId, denom, account, balance }: {
  assetId: string;
  denom: string;
  account: Account;
  /** What this viewer holds of the share, for the sell side. */
  balance: string;
}) {
  const locale = useLocale();
  const [nonce, setNonce] = useState(0);
  const [pending, setPending] = useState<Pending>(null);
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));
  const state = useMarket(denom, nonce);
  const again = useCallback(() => setNonce((n) => n + 1), []);

  // A deadline that does not move is a deadline somebody misses. One tick a
  // second, and only while an auction is actually open.
  const hasOpen = !state.loading && state.outcome.ok
    && state.outcome.value.auctions.some((a) => auctionPhase(a, now) === 'bidding');
  useEffect(() => {
    if (!hasOpen) return;
    const id = setInterval(() => setNow(Math.floor(Date.now() / 1000)), 1000);
    return () => clearInterval(id);
  }, [hasOpen]);

  if (state.loading) return <Loading what={t('mkt.title')} />;
  if (!state.outcome.ok) {
    return <Unreachable detail={state.outcome.detail} onRetry={again} />;
  }

  const { listings, auctions, book, params } = state.outcome.value;
  const settlementDenom = book.settlementDenom;
  const viewer = account.mode === 'none' ? '' : account.address;
  const connected = account.mode === 'connected' ? account : null;
  const live = listings.filter((l) => l.state === 'LISTING_STATE_LIVE');
  const open = auctions.filter((a) => auctionPhase(a, now) !== 'settled'
    && auctionPhase(a, now) !== 'cancelled');

  const act: Omit<ActProps, 'onClose' | 'onDone'> | null = connected
    ? { account: connected, denom, settlementDenom, balance, params }
    : null;
  const close = () => setPending(null);
  const done = () => { setPending(null); again(); };

  return (
    <>
      <Panel eyebrow={t('mkt.eyebrow')} title={t('mkt.title')}>
        <p className="panel__lede">{t('mkt.lede')}</p>
        <Spread book={book} />
      </Panel>

      {/* What a holder can do, before the tables of what everybody else has
          done. An investor arrives here to act. */}
      {connected && (
        <Panel eyebrow={t('mkt.sellEyebrow')} title={t('mkt.sellTitle')}>
          {BigInt(balance || '0') > 0n ? (
            <>
              <p className="panel__lede">{t('mkt.sellLede', { held: formatAmount(balance, denom) })}</p>
              <div className="mkt__actions">
                <button type="button" className="btn" onClick={() => setPending({ kind: 'list' })}>
                  {t('mkt.doList')}
                </button>
                <button type="button" className="btn btn--ghost" onClick={() => setPending({ kind: 'auction' })}>
                  {t('mkt.doAuction')}
                </button>
                <button type="button" className="btn btn--ghost" onClick={() => setPending({ kind: 'order' })}>
                  {t('mkt.doOrder')}
                </button>
              </div>
            </>
          ) : (
            <>
              <p className="panel__lede">{t('mkt.noSharesToSell')}</p>
              <div className="mkt__actions">
                <button type="button" className="btn" onClick={() => setPending({ kind: 'order' })}>
                  {t('mkt.doBid')}
                </button>
              </div>
            </>
          )}
        </Panel>
      )}

      {/*
        A listing is a parcel at a total. The six-column table it used to be —
        shares, total, each, seller, state, action — became one card per
        listing, with the TOTAL as the figure, because the total is the number a
        buyer decides on: it is what leaves their account. The price per share
        sits under it as the number that makes two listings comparable, and the
        seller and the state go one tap deeper.
      */}
      <Panel eyebrow={t('mkt.listingsEyebrow')} title={t('mkt.listingsTitle')}>
        <p className="panel__lede">{t('mkt.listingsLede')}</p>
        {listings.length === 0 ? (
          <Empty title={t('mkt.noListings')}>{t('mkt.noListingsBody')}</Empty>
        ) : (
          <ul className="y-cards">
            {listings.map((l) => {
              const can = listingActions(l, viewer);
              const each = impliedUnitPrice(l.price.amount, l.quantity);
              return (
                <li key={l.id} className="y-card y-card--money">
                  <div className="y-card__head">
                    <span className="y-card__amount">
                      <Amount amount={l.price.amount} denom={l.price.denom} />
                    </span>
                    <Chip tone={l.state === 'LISTING_STATE_LIVE' ? 'ok' : 'mute'}>
                      {t(`mkt.listing.${l.state}`)}
                    </Chip>
                  </div>
                  <p className="y-card__note">
                    <Amount amount={l.quantity} denom={l.denom} />
                    {each && (
                      <>
                        {' · '}
                        {t('mkt.impliedEach')}{' '}
                        <UnitPrice unitPrice={each.unitPrice} settlementDenom={l.price.denom} />
                        {!each.exact && <span className="mkt__count"> ≈</span>}
                      </>
                    )}
                  </p>

                  {connected && (can.buy || can.cancel) && (
                    <div className="mkt__actions">
                      {can.buy && (
                        <button type="button" className="btn btn--block"
                                onClick={() => setPending({ kind: 'buy', listing: l })}>
                          {t('mkt.doBuy')}
                        </button>
                      )}
                      {can.cancel && (
                        <button type="button" className="btn btn--ghost btn--block"
                                onClick={() => setPending({ kind: 'cancel-listing', listing: l })}>
                          {t('mkt.doWithdraw')}
                        </button>
                      )}
                    </div>
                  )}

                  <Detail summary={t('rwa.detail')}>
                    <Fields>
                      <Field label={t('mkt.colSeller')}><Address value={l.seller} /></Field>
                      <Field label={t('mkt.colShares')}>
                        <Amount amount={l.quantity} denom={l.denom} />
                      </Field>
                      <Field label={t('mkt.colTotal')}>
                        <Amount amount={l.price.amount} denom={l.price.denom} />
                      </Field>
                    </Fields>
                  </Detail>
                </li>
              );
            })}
          </ul>
        )}
      </Panel>

      <Panel eyebrow={t('mkt.auctionsEyebrow')} title={t('mkt.auctionsTitle')}>
        <p className="panel__lede">{t('mkt.auctionsLede')}</p>
        {open.length === 0 ? (
          <Empty title={t('mkt.noAuctions')}>{t('mkt.noAuctionsBody')}</Empty>
        ) : (
          <ul className="y-cards">
            {open.map((a) => {
              const phase = auctionPhase(a, now);
              const can = auctionActions(a, viewer, now);
              const left = secondsRemaining(a, now);
              return (
                <li key={a.id} className="y-card y-card--money">
                  <div className="y-card__head">
                    <span className="mkt__parcel">
                      <Amount amount={a.quantity} denom={a.denom} />
                    </span>
                    <Chip tone={phase === 'bidding' ? 'ok' : phase === 'closed' ? 'warn' : 'mute'}>
                      {t(`mkt.phase.${phase}`)}
                    </Chip>
                  </div>

                  {/* The two facts a bidder decides on, on the face of the card:
                      what it stands at, and how long is left. A countdown found
                      one tap deeper is a countdown somebody misses. */}
                  <div className="card__foot y-row">
                    <div className="y-row__main">
                      <div className="y-row__sub">{t('mkt.standingBid')}</div>
                      <div className="y-row__title">
                        {hasBid(a)
                          ? <Amount amount={a.bestBid.amount} denom={a.bestBid.denom} />
                          : <span className="muted">{t('mkt.noBidYet')}</span>}
                      </div>
                    </div>
                    <div className="y-row__end">
                      <div className="y-row__sub">{t('mkt.closesIn')}</div>
                      {/* `y-num` only while it IS a number. Once bidding is
                          over this cell holds a sentence, and a sentence set in
                          tabular mono beside a price reads as a second
                          figure. */}
                      <div className={phase === 'bidding' ? 'y-row__title y-num' : 'y-row__title'}>
                        {phase === 'bidding'
                          ? formatDuration(left, locale)
                          : <span className="muted">{t('mkt.biddingOver')}</span>}
                      </div>
                    </div>
                  </div>

                  {/* Why anybody at all is offered the settle button. */}
                  {phase === 'closed' && <Note tone="warn">{t('mkt.settleAnyone')}</Note>}

                  {connected && (can.bid || can.settle || can.cancel) && (
                    <div className="mkt__actions">
                      {can.bid && (
                        <button type="button" className="btn btn--block"
                                onClick={() => setPending({ kind: 'bid', auction: a })}>
                          {t('mkt.doBidOn')}
                        </button>
                      )}
                      {can.settle && (
                        <button type="button" className="btn btn--block"
                                onClick={() => setPending({ kind: 'settle', auction: a })}>
                          {t('mkt.doSettle')}
                        </button>
                      )}
                      {can.cancel && (
                        <button type="button" className="btn btn--ghost btn--block"
                                onClick={() => setPending({ kind: 'cancel-auction', auction: a })}>
                          {t('mkt.doWithdraw')}
                        </button>
                      )}
                    </div>
                  )}

                  <Detail summary={t('rwa.detail')}>
                    <Fields>
                      <Field label={t('mkt.reserveLabel')}>
                        {a.reserve.amount === '0' || a.reserve.amount === ''
                          ? <span className="muted">{t('mkt.noReserve')}</span>
                          : <>
                            <Amount amount={a.reserve.amount} denom={a.reserve.denom} />{' '}
                            <Chip tone={reserveMet(a) ? 'ok' : 'warn'}>
                              {reserveMet(a) ? t('mkt.reserveMet') : t('mkt.reserveNotMet')}
                            </Chip>
                          </>}
                      </Field>
                      <Field label={t('mkt.colSeller')}><Address value={a.seller} /></Field>
                    </Fields>
                  </Detail>
                </li>
              );
            })}
          </ul>
        )}
      </Panel>

      <Panel eyebrow={t('mkt.bookEyebrow')} title={t('mkt.bookTitle')}>
        <p className="panel__lede">{t('mkt.bookLede')}</p>
        {/* Bids above asks, stacked rather than side by side. Two half-width
            columns of prices at 179px each is where a figure starts breaking
            across lines, and a price broken in two is a different price. */}
        <BookSide orders={book.bids} denom={denom} settlementDenom={settlementDenom}
                  title={t('mkt.bids')} tone="bid" />
        <BookSide orders={book.asks} denom={denom} settlementDenom={settlementDenom}
                  title={t('mkt.asks')} tone="ask" />

        {/* A trader's own resting orders, which are the only ones they can
            withdraw and the only ones worth singling out. */}
        {connected && (() => {
          const mine = [...book.bids, ...book.asks].filter((o) => o.trader === viewer);
          if (mine.length === 0) return null;
          return (
            <>
              <h4 className="y-label">{t('mkt.yourOrders')}</h4>
              <ul className="y-cards">
                {mine.map((o) => (
                  <li key={o.id} className="y-card">
                    <div className="y-card__head">
                      <span className="y-card__amount">
                        <UnitPrice unitPrice={o.unitPrice} settlementDenom={o.settlementDenom} />
                      </span>
                      <Chip tone={o.side === 'SIDE_BID' ? 'ok' : 'brass'}>
                        {t(o.side === 'SIDE_BID' ? 'mkt.sideBuy' : 'mkt.sideSell')}
                      </Chip>
                    </div>
                    <p className="y-card__note">
                      <Amount amount={o.remaining} denom={o.denom} />
                    </p>
                    <button type="button" className="btn btn--ghost btn--block btn--small"
                            onClick={() => setPending({ kind: 'cancel-order', order: o })}>
                      {t('mkt.doWithdraw')}
                    </button>
                  </li>
                ))}
              </ul>
            </>
          );
        })()}
      </Panel>

      {/* ------------------------------------------------------- the sheets */}

      {pending?.kind === 'list' && act && (
        <ListDialog {...act} onClose={close} onDone={done} />
      )}
      {pending?.kind === 'auction' && act && (
        <AuctionDialog {...act} onClose={close} onDone={done} />
      )}
      {pending?.kind === 'order' && act && (
        <OrderDialog {...act} book={book} onClose={close} onDone={done} />
      )}
      {pending?.kind === 'bid' && connected && (
        <BidDialog account={connected} auction={pending.auction}
                   settlementDenom={settlementDenom} onClose={close} onDone={done} />
      )}
      {pending?.kind === 'buy' && connected && (
        <ConfirmDialog
          title={t('mkt.buyTitle')} lede={t('mkt.buyLede')} tone="warn"
          draft={{
            kind: 'buy', buyer: connected.address, listingId: pending.listing.id,
            price: pending.listing.price.amount, priceDenom: pending.listing.price.denom,
            quantity: pending.listing.quantity, shareDenom: pending.listing.denom,
          }}
          run={() => sendBuy(connected.signer, connected.address, pending.listing.id)}
          onClose={close} onDone={done} />
      )}
      {pending?.kind === 'cancel-listing' && connected && (
        <ConfirmDialog
          title={t('mkt.withdrawTitle')} lede={t('mkt.withdrawListingLede')} tone="ok"
          draft={{
            kind: 'cancel-listing', seller: connected.address, listingId: pending.listing.id,
            quantity: pending.listing.quantity, shareDenom: pending.listing.denom,
          }}
          run={() => sendCancelListing(connected.signer, connected.address, pending.listing.id)}
          onClose={close} onDone={done} />
      )}
      {pending?.kind === 'settle' && connected && (
        <ConfirmDialog
          title={t('mkt.settleTitle')} lede={t('mkt.settleLede')} tone="ok"
          draft={{
            kind: 'settle-auction', settler: connected.address, auctionId: pending.auction.id,
            quantity: pending.auction.quantity, shareDenom: pending.auction.denom,
            price: pending.auction.bestBid.amount, priceDenom: pending.auction.bestBid.denom,
          }}
          run={() => sendSettle(connected.signer, connected.address, pending.auction.id)}
          onClose={close} onDone={done} />
      )}
      {pending?.kind === 'cancel-auction' && connected && (
        <ConfirmDialog
          title={t('mkt.withdrawTitle')} lede={t('mkt.withdrawAuctionLede')} tone="ok"
          draft={{
            kind: 'cancel-auction', seller: connected.address, auctionId: pending.auction.id,
            quantity: pending.auction.quantity, shareDenom: pending.auction.denom,
          }}
          run={() => sendCancelAuction(connected.signer, connected.address, pending.auction.id)}
          onClose={close} onDone={done} />
      )}
      {pending?.kind === 'cancel-order' && connected && (
        <ConfirmDialog
          title={t('mkt.withdrawTitle')} lede={t('mkt.withdrawOrderLede')} tone="ok"
          draft={{
            kind: 'cancel-order', trader: connected.address, orderId: pending.order.id,
            remaining: pending.order.remaining, shareDenom: pending.order.denom,
          }}
          run={() => sendCancelOrder(connected.signer, connected.address, pending.order.id)}
          onClose={close} onDone={done} />
      )}
    </>
  );
}
