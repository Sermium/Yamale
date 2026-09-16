<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run `make docs` to regenerate.
-->

# x/market

The secondary market in tokenised shares: fixed-price listings, auctions for shares with no price to point at, and an order book where there are enough holders for one — every fill recording what it cost.

## Transactions

### MsgBid

`/blockchain.market.v1.MsgBid`

Signed by the `bidder` field.

MsgBid improves on the standing bid.

The money is escrowed here and the previous best bid is refunded in this same message. A bid whose money stayed with the bidder is a bid that can be withdrawn by spending the balance, which means the seller's reserve was met by somebody who was never committed to anything.

| Field | Type | Description |
| --- | --- | --- |
| `bidder` | string |  |
| `auction_id` | uint64 |  |
| `amount` | Coin | The total offered for the whole parcel. Must strictly beat the standing bid, and must meet the reserve. |

### MsgBuy

`/blockchain.market.v1.MsgBuy`

Signed by the `buyer` field.

MsgBuy takes a listing whole.

There is no quantity: a fixed-price listing is a parcel at a sum, and a partial fill of one would need a price per share the seller never named. Somebody who wants to buy part of a holding is asking the book, not a listing.

Buy takes a listing whole: money one way, shares the other, in one message.

| Field | Type | Description |
| --- | --- | --- |
| `buyer` | string |  |
| `listing_id` | uint64 |  |

### MsgCancelAuction

`/blockchain.market.v1.MsgCancelAuction`

Signed by the `seller` field.

MsgCancelAuction withdraws an auction nobody has bid on.

Only while there is no bid. Once somebody has committed money the seller is in it: a seller who could cancel after seeing the bids would be running a free valuation exercise paid for by the bidders, whose capital was escrowed for the duration and who get nothing for it.

| Field | Type | Description |
| --- | --- | --- |
| `seller` | string |  |
| `auction_id` | uint64 |  |

### MsgCancelListing

`/blockchain.market.v1.MsgCancelListing`

Signed by the `seller` field.

MsgCancelListing withdraws an offer.

CancelListing withdraws an offer and returns the escrowed shares.

| Field | Type | Description |
| --- | --- | --- |
| `seller` | string |  |
| `listing_id` | uint64 |  |

### MsgCancelOrder

`/blockchain.market.v1.MsgCancelOrder`

Signed by the `trader` field.

MsgCancelOrder withdraws a resting order.

CancelOrder withdraws a resting order and returns what it still escrows.

| Field | Type | Description |
| --- | --- | --- |
| `trader` | string |  |
| `order_id` | uint64 |  |

### MsgList

`/blockchain.market.v1.MsgList`

Signed by the `seller` field.

MsgList offers a parcel of shares for a fixed total.

The shares leave the seller here, not at fill. A listing backed by shares the seller still holds can be listed again, or transferred away, and the first buyer to pay discovers there is nothing to deliver — at which point the chain has taken somebody's money for an asset that is not there, and the only remedy is off-chain.

| Field | Type | Description |
| --- | --- | --- |
| `seller` | string |  |
| `denom` | string | The share denomination: tok/<asset>/<SYMBOL>. |
| `quantity` | string |  |
| `price` | Coin | The total asked. Must be denominated in the vehicle's income denomination; see Listing.price. |

### MsgOpenAuction

`/blockchain.market.v1.MsgOpenAuction`

Signed by the `seller` field.

MsgOpenAuction offers a parcel to the best bid received by a deadline.

| Field | Type | Description |
| --- | --- | --- |
| `seller` | string |  |
| `denom` | string |  |
| `quantity` | string |  |
| `reserve` | Coin | The least the seller will accept, as a total. Zero means no reserve. |
| `closes_at` | int64 | Unix seconds at which bidding stops. Bounded by the params at both ends. An absolute time rather than a duration, because a duration is measured from whenever the transaction happens to be included — so a seller signing "one week" during congestion gets an auction closing at a moment they did not choose, and their shares are escrowed for the difference. |

### MsgPlaceOrder

`/blockchain.market.v1.MsgPlaceOrder`

Signed by the `trader` field.

MsgPlaceOrder crosses against the book and rests what is left.

#### Escrow first, then fill

The whole of the trader's side is escrowed before a single match is attempted: shares for an ask, money for a bid, at the trader's own limit. Every fill is then paid out of that escrow, and whatever is left over is either the remainder resting or a refund at the end.

The alternative — fill from the trader's own balance and escrow only what rests — is one bank send shorter and wrong in a way that only shows up under load. It leaves a window inside the cross where the trader's balance is still theirs, so a fill part-way through the loop can fail for insufficient funds after earlier fills have already settled, and the handler is then holding half a trade.

#### Price improvement belongs to the taker

A fill happens at the RESTING order's price, never the incoming one. The resting order named a price and waited; the incoming order accepted it. Where a bid crosses an ask below its limit, the difference is refunded to the bidder rather than paid to the seller — the seller asked for their price and got it, and a mechanism that quietly paid them more would be charging the taker for the privilege of having been patient.

| Field | Type | Description |
| --- | --- | --- |
| `trader` | string |  |
| `denom` | string |  |
| `side` | Side |  |
| `quantity` | string |  |
| `unit_price` | uint64 | Settlement units per one share, and a limit rather than a target: a bid will not pay more, an ask will not take less. There is no market order, deliberately — on a book this thin an unpriced order is an instruction to accept whatever single resting order happens to be there, which on a vehicle with four holders is not a price. |

### MsgSettleAuction

`/blockchain.market.v1.MsgSettleAuction`

Signed by the `settler` field.

MsgSettleAuction closes an auction out after its deadline.

Anybody may send it, and that is the point rather than a convenience. The winner's money and the seller's shares are both in the module account; if settlement needed the seller's signature, a seller who disliked the result could strand the winner's capital indefinitely by doing nothing. Nobody should need a counterparty's cooperation to end a mechanism that has already finished.

| Field | Type | Description |
| --- | --- | --- |
| `settler` | string |  |
| `auction_id` | uint64 |  |

### MsgUpdateParams

`/blockchain.market.v1.MsgUpdateParams`

Signed by the `authority` field.

MsgUpdateParams sets the market-wide limits.

Governance only.

| Field | Type | Description |
| --- | --- | --- |
| `authority` | string |  |
| `params` | Params |  |

## Queries

### Auction

`GET /yamale/blockchain/market/v1/auction/{id}`

Auction returns one auction, including its standing bid.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `auction` | Auction |  |

### Auctions

`GET /yamale/blockchain/market/v1/auctions/{denom}`

Auctions returns the auctions over one share denomination.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `open_only` | bool |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `auctions` | repeated Auction |  |

### Book

`GET /yamale/blockchain/market/v1/book/{denom}`

Book returns the resting bids and asks over one share denomination, best price first on each side.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `depth` | uint32 | How many resting orders to return per side. Bounded by the module whatever is asked for, and defaulted when zero — a book query is the one read whose cost grows with how successful the market has been, and a node serving it over a public endpoint should not be able to be asked for all of it. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `bids` | repeated Order | Highest price first. |
| `asks` | repeated Order | Lowest price first. |
| `settlement_denom` | string | The denomination prices on this book are quoted in, which a client otherwise has to learn from x/tokenisation to render a single number. Empty when the denomination names no vehicle this chain knows. |

### Listing

`GET /yamale/blockchain/market/v1/listing/{id}`

Listing returns one fixed-price offer.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `listing` | Listing |  |

### Listings

`GET /yamale/blockchain/market/v1/listings/{denom}`

Listings returns the fixed-price offers over one share denomination.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `denom` | string |  |
| `live_only` | bool | Only the ones that can still be filled. The default is everything, because a filled listing is the record of a price somebody paid — which is the scarcest information about an illiquid share there is. |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `listings` | repeated Listing |  |

### Order

`GET /yamale/blockchain/market/v1/order/{id}`

Order returns one book order.

Request:

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |

Response:

| Field | Type | Description |
| --- | --- | --- |
| `order` | Order |  |

### Params

`GET /yamale/blockchain/market/v1/params`

Params returns the market-wide limits.

Response:

| Field | Type | Description |
| --- | --- | --- |
| `params` | Params |  |

## State

### Auction

Auction is a holder offering a quantity to the best bid by a deadline.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `asset_id` | uint64 |  |
| `denom` | string |  |
| `seller` | string |  |
| `quantity` | string |  |
| `reserve` | Coin | The least the seller will accept, as a total. A bid below it is refused outright rather than recorded and ignored, so a bidder is never left believing they are in an auction they cannot win. May be zero, which is a seller saying they will take whatever it fetches. Zero is stored explicitly and never left unset: an unset Coin decodes with a nil amount, and nil is not zero — it panics on the first comparison. |
| `closes_at` | int64 | When bidding stops. Settlement is a separate message anybody may send afterwards; see the module's AppModule for why there is no sweeper. |
| `best_bidder` | string | The best bid, and the ONLY bid whose money the module holds. #### Why one bid and not a list The obvious design keeps every bid escrowed and refunds the losers when the auction settles. That makes settlement O(bids) — an unbounded loop inside a handler, in a module where the number of iterations is chosen by whoever wanted the loop to be long. An auction with fifty thousand one-unit bids in it is not a hypothetical attack, it is the cheapest one available. So a losing bid is refunded the moment it loses, in the transaction of the bid that beat it, paid for by the bidder who caused the work. At any moment the module holds exactly one bidder's money per auction, settlement moves exactly two things, and there is no loop anywhere in the mechanism. The cost of this is that bids must strictly improve — there is no second place, and a bidder who is outbid is out rather than queued. For an illiquid share sold by deadline that is the right trade: the seller wanted the best price by a date, and a queue of losing bids serves nobody but a bidder hoping the winner defaults, which escrow has already made impossible. |
| `best_bid` | Coin | Zero, in the settlement denomination, until somebody bids. Explicitly zero for the same reason as the reserve. |
| `state` | AuctionState |  |
| `opened_at` | int64 |  |
| `settled_at` | int64 |  |

### Fill

Fill is one trade: who, how much, at what price.

Emitted as an event rather than stored. The chain already holds the two orders and the balances; what a fill adds is the pairing, which an indexer wants and consensus does not — storing every fill for ever would grow the state by the one thing that is pure history.

| Field | Type | Description |
| --- | --- | --- |
| `asset_id` | uint64 |  |
| `denom` | string |  |
| `buyer` | string |  |
| `seller` | string |  |
| `quantity` | string |  |
| `paid` | Coin | What the buyer paid in total, which is also what was written to their cost basis. |

### Listing

Listing is a holder offering a fixed quantity for a fixed total.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `asset_id` | uint64 | The vehicle, resolved from the denomination once at listing time. Stored rather than re-derived because the fill has to write the buyer's cost basis against a vehicle, and a lookup that can fail is a lookup that can fail halfway through a settlement. |
| `denom` | string | The share denomination being sold: tok/<asset>/<SYMBOL>. |
| `seller` | string |  |
| `quantity` | string |  |
| `price` | Coin | The TOTAL asked, not a price per share. Total because that is what the seller is actually deciding. A fixed-price listing is an offer to sell a specific parcel for a specific sum, and expressing it per-share would force the seller to solve a rounding problem they did not ask to have: a quantity that does not divide the sum leaves a remainder that has to go somewhere, and every place it could go is a fraction of a cent the seller did not agree to. Its denomination is the vehicle's income denomination, and that is checked rather than assumed. The cost basis this fill writes is carried in that denomination, so a listing priced in anything else would record a cost in one currency against a return in another. |
| `state` | ListingState |  |
| `listed_at` | int64 |  |
| `filled_by` | string | Who took it, and when. Empty until filled. |
| `filled_at` | int64 |  |

### Order

Order is resting interest in the book at a price its trader chose.

| Field | Type | Description |
| --- | --- | --- |
| `id` | uint64 |  |
| `asset_id` | uint64 |  |
| `denom` | string |  |
| `trader` | string |  |
| `side` | Side |  |
| `quantity` | string | What was asked for, and what is left. Both kept: the original is the record of what somebody intended, the remainder is what can still be crossed, and an order partially filled at several prices cannot reconstruct either from the other. |
| `remaining` | string |  |
| `unit_price` | uint64 | Settlement units per ONE share, and an integer rather than a decimal. #### Why per-share here and a total on a listing Because a book order is partially fillable and a listing is not. A fill of part of an order has to price that part, which needs a rate; a listing is taken whole, so it can name a sum. #### Why uint64 and not math.Int The book is indexed BY this number, and "the best price on this side" has to be the first key in a range scan rather than a sort of every resting order. That needs a fixed-width, order-preserving key, which is what a uint64 gives and an arbitrary-precision integer does not. The ceiling it imposes is about eighteen quintillion settlement units for one share; a price that exceeded it would be a mistyped order rather than a trade. |
| `settlement_denom` | string | The vehicle's income denomination, copied onto the order so that a cross never has to re-derive it for two orders and hope they agree. |
| `state` | OrderState |  |
| `placed_at` | int64 |  |

## Value types

### AuctionState

AuctionState is where an auction has got to.

| Value | Meaning |
| --- | --- |
| `AUCTION_STATE_UNSPECIFIED` |  |
| `AUCTION_STATE_OPEN` | Running. Shares escrowed; the best bid's money escrowed with them. |
| `AUCTION_STATE_SETTLED` | Closed out. Either the winner has their shares and the seller their money, or nobody bid and the shares went back. |
| `AUCTION_STATE_CANCELLED` | Withdrawn before anybody bid. After the first bid a seller cannot withdraw — see Auction.best_bidder. |

### ListingState

ListingState is where a fixed-price offer has got to.

| Value | Meaning |
| --- | --- |
| `LISTING_STATE_UNSPECIFIED` | Never stored on a listing that exists. A listing is created LIVE. |
| `LISTING_STATE_LIVE` | Offered, shares escrowed, fillable by the first eligible buyer. |
| `LISTING_STATE_FILLED` | Bought. Both legs moved in the one message. |
| `LISTING_STATE_CANCELLED` | Withdrawn by the seller, shares returned. |

### OrderState

OrderState is where a book order has got to.

| Value | Meaning |
| --- | --- |
| `ORDER_STATE_UNSPECIFIED` |  |
| `ORDER_STATE_OPEN` | Resting with something still to fill. Only an OPEN order is in the book index, which is what makes the index a list of what can actually be crossed rather than a history. |
| `ORDER_STATE_FILLED` | Nothing left to fill. Kept rather than deleted, because an order is the only record of a price somebody agreed to. |
| `ORDER_STATE_CANCELLED` | Withdrawn, with whatever had not filled returned. |

### Side

Side is which way a resting order points.

| Value | Meaning |
| --- | --- |
| `SIDE_UNSPECIFIED` |  |
| `SIDE_BID` | Buying. The money is escrowed, at the trader's own limit price. |
| `SIDE_ASK` | Selling. The shares are escrowed. |

## Parameters

Changed by governance through `MsgUpdateParams`. Defaults are the values a chain starts with at genesis.

| Parameter | Default | Description |
| --- | --- | --- |
| `max_fills_per_order` | `20` | The most resting orders one incoming order may consume before the rest of it is left to rest. This is the only thing standing between an order book and a transaction that walks it. A cross is the one place in this module where the work done is chosen by the person paying for it: an order priced through the whole book matches every level, and on a book with ten thousand resting asks that is ten thousand bank sends, ten thousand settlements of the send restriction, and a block that does not finish. Bounding it rather than metering it, because gas is not a safety limit — it is a price, and a well-funded participant can pay it. What a bound gives is a ceiling on the work per transaction that does not depend on anybody's willingness to pay for the damage. The remainder is NOT dropped when the bound is reached. It rests at the taker's own price, where the next transaction crosses it — so a large order is filled over several blocks rather than refused or silently truncated. Truncating would be the worst of the three: a trader who asked to buy a million and got two hundred thousand with no error has been told their order succeeded. |
| `min_auction_period` | `3600` | The shortest and longest an auction may run, in seconds. A floor because an auction that closes in the same block it opened is not an auction, it is a sale to whoever was watching the mempool. A ceiling because the seller's shares are escrowed for the whole of it: an auction open for a year is a year in which the seller cannot sell, cannot be paid a distribution on tokens they no longer hold, and cannot withdraw once anybody has bid. |
| `max_auction_period` | `2592000` |  |

## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
| 1 | `ErrNotAuthority` | this message is governance-only |
| 10 | `ErrNotSeller` | only the seller may withdraw this offer |
| 11 | `ErrNoAuction` | no such auction |
| 12 | `ErrAuctionClosed` | this auction is no longer taking bids |
| 13 | `ErrAuctionRunning` | this auction has not reached its deadline yet |
| 14 | `ErrAuctionTooShort` | an auction must run long enough for somebody other than the seller to see it |
| 15 | `ErrAuctionTooLong` | an auction may not escrow the seller's shares for that long |
| 16 | `ErrBidTooLow` | a bid must beat the standing bid, and meet the reserve |
| 17 | `ErrAuctionHasBid` | somebody has bid on this auction; it can no longer be withdrawn, only settled |
| 18 | `ErrNoOrder` | no such order |
| 19 | `ErrOrderClosed` | this order has already been filled or withdrawn |
| 2 | `ErrNotShareDenom` | this denomination is not a tokenised shareholding, so this market cannot price it |
| 20 | `ErrNotTrader` | only the trader who placed an order may withdraw it |
| 21 | `ErrInvalidSide` | an order must be a bid or an ask |
| 22 | `ErrInvalidParams` | these market parameters would disable a protection |
| 3 | `ErrTradingHalted` | this vehicle has been realised or closed; its shares are a claim on a fixed sum and no longer trade |
| 4 | `ErrWrongDenom` | this vehicle settles in a different currency |
| 5 | `ErrInvalidQuantity` | the quantity must be positive |
| 6 | `ErrInvalidPrice` | the price must be positive |
| 7 | `ErrSelfDeal` | you cannot trade with yourself |
| 8 | `ErrNoListing` | no such listing |
| 9 | `ErrListingClosed` | this listing has already been filled or withdrawn |
