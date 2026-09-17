/**
 * The message the chain will actually receive, field by field.
 *
 * The same idea as Yamale Pay's instruction table, applied to the three
 * messages a shareholder signs. The claim it makes is narrow and checkable:
 * here is every field of the protobuf message, here is the value this action
 * would put in it, and here is where that value ends up.
 *
 * The interesting rows are the ones that are *not* fields. A dispute bond is
 * nowhere in `MsgDisputeSale` — the keeper computes it from the collection and
 * takes it from the challenger's account in the same block — and a redemption's
 * payout is nowhere in `MsgRedeem`. Both are consequences of signing that the
 * signed bytes do not mention, which makes them exactly the things somebody
 * would otherwise discover afterwards. They are marked `derived` and shown
 * beside the fields rather than in a footnote.
 *
 * Pure, and tested, because a table that misdescribed what was about to be
 * signed would be worse than no table at all.
 */

export type Carried =
  /** Literally in the signed bytes, in this field, with this value. */
  | 'ledger'
  /** Not in the message. The keeper computes it and it happens anyway. */
  | 'derived';

export interface PlanRow {
  /** The protobuf field name, or a bracketed name for a derived consequence. */
  field: string;
  /** The value, as a string. Amounts stay in base units here; the view formats. */
  value: string;
  /** Set when the value is an amount, so the view knows to convert it. */
  denom?: string;
  carried: Carried;
  /** Catalogue key for a sentence explaining a derived row. */
  noteKey?: string;
}

export interface Plan {
  typeUrl: string;
  rows: PlanRow[];
}

export type Draft =
  | {
    kind: 'claim';
    holder: string;
    assetId: string;
    /** What the chain will pay out, from Query/Entitlement. */
    owed: string;
    owedDenom: string;
  }
  | {
    kind: 'redeem';
    holder: string;
    assetId: string;
    /** Shares to burn, in base units — which for shares is a plain count. */
    amount: string;
    shareDenom: string;
    /** What the burn will pay, computed the way the keeper computes it. */
    payout: string;
    payoutDenom: string;
  }
  | {
    kind: 'dispute';
    challenger: string;
    assetId: string;
    reason: string;
    /** The stake, computed the way the keeper computes it. */
    bond: string;
    bondDenom: string;
  };

/**
 * Type URLs, written out rather than built from a template.
 *
 * The prefix is `blockchain.`, not `yamale.blockchain.` — the proto package
 * declared in every one of this module's .proto files. A template that
 * assembled these would make the wrong one just as easy to produce as the right
 * one, and CosmJS's failure for an unregistered type URL names the URL it was
 * given, which reads as a missing encoder rather than as a typo.
 */
export const TYPE_URLS = {
  claim: '/blockchain.tokenisation.v1.MsgClaim',
  redeem: '/blockchain.tokenisation.v1.MsgRedeem',
  dispute: '/blockchain.tokenisation.v1.MsgDisputeSale',
} as const;

export function messagePlan(draft: Draft): Plan {
  switch (draft.kind) {
    case 'claim':
      return {
        typeUrl: TYPE_URLS.claim,
        rows: [
          { field: 'holder', value: draft.holder, carried: 'ledger' },
          { field: 'asset_id', value: draft.assetId, carried: 'ledger' },
          {
            field: 'paid',
            value: draft.owed,
            denom: draft.owedDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.paidNote',
          },
        ],
      };

    case 'redeem':
      return {
        typeUrl: TYPE_URLS.redeem,
        rows: [
          { field: 'holder', value: draft.holder, carried: 'ledger' },
          { field: 'asset_id', value: draft.assetId, carried: 'ledger' },
          { field: 'amount', value: draft.amount, denom: draft.shareDenom, carried: 'ledger' },
          {
            field: 'burned',
            value: draft.amount,
            denom: draft.shareDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.burnNote',
          },
          {
            field: 'paid',
            value: draft.payout,
            denom: draft.payoutDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.payoutNote',
          },
        ],
      };

    case 'dispute':
      return {
        typeUrl: TYPE_URLS.dispute,
        rows: [
          { field: 'challenger', value: draft.challenger, carried: 'ledger' },
          { field: 'asset_id', value: draft.assetId, carried: 'ledger' },
          // Public, permanently, and read by whoever resolves the dispute.
          // Marked `ledger` rather than left to be assumed: somebody typing a
          // reason should know it is being published, not filed.
          { field: 'reason', value: draft.reason, carried: 'ledger' },
          {
            field: 'bond',
            value: draft.bond,
            denom: draft.bondDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.bondNote',
          },
        ],
      };
  }
}

/** The value object the signer encodes, matching the generated field names. */
export function messageValue(draft: Draft): Record<string, string> {
  switch (draft.kind) {
    case 'claim':
      return { holder: draft.holder, assetId: draft.assetId };
    case 'redeem':
      return { holder: draft.holder, assetId: draft.assetId, amount: draft.amount };
    case 'dispute':
      return { challenger: draft.challenger, assetId: draft.assetId, reason: draft.reason };
  }
}

/* ========================================================================== */
/*  The secondary market                                                      */
/* ========================================================================== */
//
// The derived rows here are the point of the table, more than they were for a
// claim. Three things happen when a market message is signed that the signed
// bytes say nothing about:
//
//   - LISTING or AUCTIONING moves the shares out of the seller's account in
//     that block, before any buyer exists. MsgList has no field for that.
//   - BIDDING moves the money out of the bidder's account in that block, and
//     refunds whoever was winning. MsgBid has no field for either.
//   - A BOOK ORDER escrows the whole of the trader's side before a single match
//     is attempted, then fills at the RESTING order's price rather than at the
//     limit the trader typed — so what it costs is not what the message says.
//
// Each of those is the thing somebody would otherwise discover from their
// balance afterwards.

export type MarketDraft =
  | {
    kind: 'list';
    seller: string;
    denom: string;
    /** Shares, a plain count. */
    quantity: string;
    /** The TOTAL asked, in settlement base units. */
    price: string;
    priceDenom: string;
  }
  | {
    kind: 'buy';
    buyer: string;
    listingId: string;
    /** What the buyer pays, from the listing. */
    price: string;
    priceDenom: string;
    /** What they receive. */
    quantity: string;
    shareDenom: string;
  }
  | {
    kind: 'cancel-listing';
    seller: string;
    listingId: string;
    quantity: string;
    shareDenom: string;
  }
  | {
    kind: 'open-auction';
    seller: string;
    denom: string;
    quantity: string;
    reserve: string;
    reserveDenom: string;
    /** Absolute unix seconds, never a duration. */
    closesAt: number;
  }
  | {
    kind: 'bid';
    bidder: string;
    auctionId: string;
    amount: string;
    amountDenom: string;
    /** What is refunded to whoever was winning, which is not a field. */
    displaces: string;
  }
  | {
    kind: 'settle-auction';
    settler: string;
    auctionId: string;
    /** What the winner receives and the seller is paid. */
    quantity: string;
    shareDenom: string;
    price: string;
    priceDenom: string;
  }
  | {
    kind: 'cancel-auction';
    seller: string;
    auctionId: string;
    quantity: string;
    shareDenom: string;
  }
  | {
    kind: 'place-order';
    trader: string;
    denom: string;
    side: 'SIDE_BID' | 'SIDE_ASK';
    quantity: string;
    /** Settlement base units per one share. A limit, not a target. */
    unitPrice: string;
    settlementDenom: string;
    /** What the preview says would cross now, and at what cost. */
    fills: string;
    settles: string;
    rests: string;
  }
  | {
    kind: 'cancel-order';
    trader: string;
    orderId: string;
    remaining: string;
    shareDenom: string;
  };

export const MARKET_TYPE_URLS = {
  list: '/blockchain.market.v1.MsgList',
  buy: '/blockchain.market.v1.MsgBuy',
  'cancel-listing': '/blockchain.market.v1.MsgCancelListing',
  'open-auction': '/blockchain.market.v1.MsgOpenAuction',
  bid: '/blockchain.market.v1.MsgBid',
  'settle-auction': '/blockchain.market.v1.MsgSettleAuction',
  'cancel-auction': '/blockchain.market.v1.MsgCancelAuction',
  'place-order': '/blockchain.market.v1.MsgPlaceOrder',
  'cancel-order': '/blockchain.market.v1.MsgCancelOrder',
} as const;

export function marketPlan(draft: MarketDraft): Plan {
  switch (draft.kind) {
    case 'list':
      return {
        typeUrl: MARKET_TYPE_URLS.list,
        rows: [
          { field: 'seller', value: draft.seller, carried: 'ledger' },
          { field: 'denom', value: draft.denom, carried: 'ledger' },
          { field: 'quantity', value: draft.quantity, denom: draft.denom, carried: 'ledger' },
          { field: 'price', value: draft.price, denom: draft.priceDenom, carried: 'ledger' },
          {
            field: 'escrowed',
            value: draft.quantity,
            denom: draft.denom,
            carried: 'derived',
            noteKey: 'rwa.msg.listEscrowNote',
          },
        ],
      };

    case 'buy':
      return {
        typeUrl: MARKET_TYPE_URLS.buy,
        rows: [
          { field: 'buyer', value: draft.buyer, carried: 'ledger' },
          { field: 'listing_id', value: draft.listingId, carried: 'ledger' },
          {
            field: 'paid',
            value: draft.price,
            denom: draft.priceDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.buyPaidNote',
          },
          {
            field: 'received',
            value: draft.quantity,
            denom: draft.shareDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.buyGotNote',
          },
        ],
      };

    case 'cancel-listing':
      return {
        typeUrl: MARKET_TYPE_URLS['cancel-listing'],
        rows: [
          { field: 'seller', value: draft.seller, carried: 'ledger' },
          { field: 'listing_id', value: draft.listingId, carried: 'ledger' },
          {
            field: 'returned',
            value: draft.quantity,
            denom: draft.shareDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.returnedNote',
          },
        ],
      };

    case 'open-auction':
      return {
        typeUrl: MARKET_TYPE_URLS['open-auction'],
        rows: [
          { field: 'seller', value: draft.seller, carried: 'ledger' },
          { field: 'denom', value: draft.denom, carried: 'ledger' },
          { field: 'quantity', value: draft.quantity, denom: draft.denom, carried: 'ledger' },
          { field: 'reserve', value: draft.reserve, denom: draft.reserveDenom, carried: 'ledger' },
          { field: 'closes_at', value: String(draft.closesAt), carried: 'ledger' },
          {
            field: 'escrowed',
            value: draft.quantity,
            denom: draft.denom,
            carried: 'derived',
            noteKey: 'rwa.msg.auctionEscrowNote',
          },
        ],
      };

    case 'bid':
      return {
        typeUrl: MARKET_TYPE_URLS.bid,
        rows: [
          { field: 'bidder', value: draft.bidder, carried: 'ledger' },
          { field: 'auction_id', value: draft.auctionId, carried: 'ledger' },
          { field: 'amount', value: draft.amount, denom: draft.amountDenom, carried: 'ledger' },
          {
            field: 'committed',
            value: draft.amount,
            denom: draft.amountDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.bidEscrowNote',
          },
          ...(draft.displaces !== '0' && draft.displaces !== ''
            ? [{
              field: 'refunds',
              value: draft.displaces,
              denom: draft.amountDenom,
              carried: 'derived' as const,
              noteKey: 'rwa.msg.bidRefundNote',
            }]
            : []),
        ],
      };

    case 'settle-auction':
      return {
        typeUrl: MARKET_TYPE_URLS['settle-auction'],
        rows: [
          { field: 'settler', value: draft.settler, carried: 'ledger' },
          { field: 'auction_id', value: draft.auctionId, carried: 'ledger' },
          {
            field: 'delivers',
            value: draft.quantity,
            denom: draft.shareDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.settleDeliversNote',
          },
          {
            field: 'pays',
            value: draft.price,
            denom: draft.priceDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.settlePaysNote',
          },
        ],
      };

    case 'cancel-auction':
      return {
        typeUrl: MARKET_TYPE_URLS['cancel-auction'],
        rows: [
          { field: 'seller', value: draft.seller, carried: 'ledger' },
          { field: 'auction_id', value: draft.auctionId, carried: 'ledger' },
          {
            field: 'returned',
            value: draft.quantity,
            denom: draft.shareDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.returnedNote',
          },
        ],
      };

    case 'place-order':
      return {
        typeUrl: MARKET_TYPE_URLS['place-order'],
        rows: [
          { field: 'trader', value: draft.trader, carried: 'ledger' },
          { field: 'denom', value: draft.denom, carried: 'ledger' },
          { field: 'side', value: draft.side, carried: 'ledger' },
          { field: 'quantity', value: draft.quantity, denom: draft.denom, carried: 'ledger' },
          // Deliberately NOT given a denom: it is a price per share in
          // settlement base units, not an amount, and rendering it as money
          // would print sixty cents as sixty thousand dollars.
          { field: 'unit_price', value: draft.unitPrice, carried: 'ledger' },
          {
            field: 'fills now',
            value: draft.fills,
            denom: draft.denom,
            carried: 'derived',
            noteKey: 'rwa.msg.orderFillNote',
          },
          {
            field: 'costs',
            value: draft.settles,
            denom: draft.settlementDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.orderCostNote',
          },
          {
            field: 'rests',
            value: draft.rests,
            denom: draft.denom,
            carried: 'derived',
            noteKey: 'rwa.msg.orderRestNote',
          },
        ],
      };

    case 'cancel-order':
      return {
        typeUrl: MARKET_TYPE_URLS['cancel-order'],
        rows: [
          { field: 'trader', value: draft.trader, carried: 'ledger' },
          { field: 'order_id', value: draft.orderId, carried: 'ledger' },
          {
            field: 'returned',
            value: draft.remaining,
            denom: draft.shareDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.returnedNote',
          },
        ],
      };
  }
}
