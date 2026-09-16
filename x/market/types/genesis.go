package types

import (
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultGenesis returns an empty market with the protective limits.
func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

// Validate refuses a genesis the module could not have produced.
//
// Three families of check, and they guard three different disasters.
//
// The COUNTER checks are the ones that matter most. A counter sitting at or
// below an id already present hands that id out again on the next order, and
// the second record overwrites the first — silently, because a write to an
// existing key is not an error. Here that means one order's escrow paying out
// against another order's terms.
//
// The NIL checks are the ones that look like paranoia and are not. `math.Int`'s
// zero value is nil, and nil PANICS on arithmetic rather than returning an
// error. Every one of these fields is read by a handler that adds or compares
// it, so a single unset amount in an imported file is not a wrong number, it is
// a chain that halts on the first transaction that touches that record — and it
// halts after the import succeeded, so the file that caused it is already the
// chain's own state.
//
// The ESCROW checks tie each record to property in the module account. Every
// live offer here corresponds to real shares or real money the module is
// holding, and a record whose shape says it escrows nothing is a record no
// cancel can return and no fill can spend.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	seenListing := map[uint64]bool{}
	for _, l := range gs.Listings {
		if l.Id == 0 {
			return fmt.Errorf(
				"listing 0 is never issued; a zero id is indistinguishable from an unset field")
		}
		if seenListing[l.Id] {
			return fmt.Errorf("listing %d appears twice", l.Id)
		}
		seenListing[l.Id] = true
		if l.Id >= gs.NextListingId {
			return fmt.Errorf(
				"listing %d exists but next_listing_id is %d, so the next listing would overwrite it",
				l.Id, gs.NextListingId)
		}
		if err := validateListing(l); err != nil {
			return err
		}
	}

	seenAuction := map[uint64]bool{}
	for _, a := range gs.Auctions {
		if a.Id == 0 {
			return fmt.Errorf("auction 0 is never issued")
		}
		if seenAuction[a.Id] {
			return fmt.Errorf("auction %d appears twice", a.Id)
		}
		seenAuction[a.Id] = true
		if a.Id >= gs.NextAuctionId {
			return fmt.Errorf(
				"auction %d exists but next_auction_id is %d", a.Id, gs.NextAuctionId)
		}
		if err := validateAuction(a); err != nil {
			return err
		}
	}

	seenOrder := map[uint64]bool{}
	for _, o := range gs.Orders {
		if o.Id == 0 {
			return fmt.Errorf("order 0 is never issued")
		}
		if seenOrder[o.Id] {
			return fmt.Errorf("order %d appears twice", o.Id)
		}
		seenOrder[o.Id] = true
		if o.Id >= gs.NextOrderId {
			return fmt.Errorf("order %d exists but next_order_id is %d", o.Id, gs.NextOrderId)
		}
		if err := validateOrder(o); err != nil {
			return err
		}
	}

	return nil
}

func validateListing(l Listing) error {
	if l.Denom == "" {
		return fmt.Errorf("listing %d names no share denomination", l.Id)
	}
	if l.Seller == "" {
		return fmt.Errorf("listing %d has no seller", l.Id)
	}
	if l.Quantity.IsNil() {
		return fmt.Errorf("listing %d has no quantity", l.Id)
	}
	if !l.Quantity.IsPositive() {
		return fmt.Errorf("listing %d offers %s shares", l.Id, l.Quantity)
	}
	// A Coin whose amount is nil decodes without complaint and panics on the
	// first comparison. The handler compares this one against what a buyer
	// sends, so an unset price is a halt in whichever block somebody tries to
	// buy it.
	if l.Price.Amount.IsNil() {
		return fmt.Errorf("listing %d has no price", l.Id)
	}
	if !l.Price.IsValid() || !l.Price.Amount.IsPositive() {
		return fmt.Errorf("listing %d asks %s, which is not a price", l.Id, l.Price)
	}
	if l.State == LISTING_STATE_UNSPECIFIED {
		return fmt.Errorf("listing %d has no state; a listing is created LIVE", l.Id)
	}
	return nil
}

func validateAuction(a Auction) error {
	if a.Denom == "" {
		return fmt.Errorf("auction %d names no share denomination", a.Id)
	}
	if a.Seller == "" {
		return fmt.Errorf("auction %d has no seller", a.Id)
	}
	if a.Quantity.IsNil() {
		return fmt.Errorf("auction %d has no quantity", a.Id)
	}
	if !a.Quantity.IsPositive() {
		return fmt.Errorf("auction %d offers %s shares", a.Id, a.Quantity)
	}
	// Both of these may legitimately be zero — no reserve, and no bid yet — so
	// what is checked is that they are zero rather than unset. That distinction
	// is the whole point: a nil amount reads as "no reserve" to a human and
	// panics the first time the module compares a bid against it.
	if a.Reserve.Amount.IsNil() {
		return fmt.Errorf(
			"auction %d has no reserve; an auction without one carries an explicit zero, "+
				"because a nil amount panics rather than comparing as none", a.Id)
	}
	if a.Reserve.Amount.IsNegative() {
		return fmt.Errorf("auction %d has a negative reserve", a.Id)
	}
	if a.BestBid.Amount.IsNil() {
		return fmt.Errorf(
			"auction %d has no standing bid amount; an auction nobody has bid on carries "+
				"an explicit zero", a.Id)
	}
	if a.BestBid.Amount.IsNegative() {
		return fmt.Errorf("auction %d has a negative standing bid", a.Id)
	}

	// The bidder and the money are one fact written twice, and the two halves
	// disagreeing is the shape that loses somebody's capital. A bidder with no
	// amount would be refunded nothing when they were outbid; an amount with no
	// bidder is money in the module account that settlement pays to the seller
	// on behalf of nobody, and which no refund path can ever reach.
	if a.BestBidder == "" && a.BestBid.Amount.IsPositive() {
		return fmt.Errorf(
			"auction %d holds a bid of %s with no bidder; that money has no path out",
			a.Id, a.BestBid)
	}
	if a.BestBidder != "" && !a.BestBid.Amount.IsPositive() {
		return fmt.Errorf(
			"auction %d names %s as the best bidder but escrows nothing for them",
			a.Id, a.BestBidder)
	}
	if a.ClosesAt <= 0 {
		return fmt.Errorf("auction %d has no deadline", a.Id)
	}
	if a.State == AUCTION_STATE_UNSPECIFIED {
		return fmt.Errorf("auction %d has no state; an auction is created OPEN", a.Id)
	}
	// A cancelled auction is one nobody bid on — that is the only way to reach
	// the state — so one carrying a bidder was reached by a path the module
	// does not have, and settling it would pay somebody twice.
	if a.State == AUCTION_STATE_CANCELLED && a.BestBidder != "" {
		return fmt.Errorf(
			"auction %d was withdrawn but names a bidder; an auction with a bid cannot be withdrawn",
			a.Id)
	}
	return nil
}

func validateOrder(o Order) error {
	if o.Denom == "" {
		return fmt.Errorf("order %d names no share denomination", o.Id)
	}
	if o.SettlementDenom == "" {
		return fmt.Errorf("order %d names no settlement denomination", o.Id)
	}
	if o.Trader == "" {
		return fmt.Errorf("order %d has no trader", o.Id)
	}
	if o.Side != SIDE_BID && o.Side != SIDE_ASK {
		return fmt.Errorf("order %d is neither a bid nor an ask", o.Id)
	}
	if o.Quantity.IsNil() || o.Remaining.IsNil() {
		return fmt.Errorf("order %d has no quantity", o.Id)
	}
	if !o.Quantity.IsPositive() {
		return fmt.Errorf("order %d is for %s shares", o.Id, o.Quantity)
	}
	if o.Remaining.IsNegative() {
		return fmt.Errorf("order %d has %s left to fill", o.Id, o.Remaining)
	}
	if o.Remaining.GT(o.Quantity) {
		return fmt.Errorf(
			"order %d has %s left of an order for %s, so it would deliver more than it escrowed",
			o.Id, o.Remaining, o.Quantity)
	}
	if o.UnitPrice == 0 {
		return fmt.Errorf(
			"order %d has no price; a resting order at zero would give its shares away", o.Id)
	}
	if o.State == ORDER_STATE_UNSPECIFIED {
		return fmt.Errorf("order %d has no state; an order is created OPEN", o.Id)
	}
	// OPEN is what puts an order in the book, and what the book promises is
	// that every key in it can be crossed. An open order with nothing left
	// would be matched against, fill zero, and be matched against again by the
	// next order — which is the one way this module's bounded cross could still
	// fail to make progress.
	if o.State == ORDER_STATE_OPEN && !o.Remaining.IsPositive() {
		return fmt.Errorf(
			"order %d is open with nothing left to fill; it would sit in the book matching "+
				"every incoming order for zero", o.Id)
	}
	if o.State == ORDER_STATE_FILLED && o.Remaining.IsPositive() {
		return fmt.Errorf(
			"order %d is filled but still has %s outstanding", o.Id, o.Remaining)
	}
	return nil
}

// EscrowedBy is what the module account must be holding for one resting order.
//
// An ask escrows the shares it has left to deliver; a bid escrows the money it
// has left to pay, at its own limit price. Used by the cancel path so that what
// is returned is computed from the order rather than remembered separately —
// two numbers that must agree are one number too many.
func EscrowedBy(o Order) sdk.Coin {
	if o.Side == SIDE_ASK {
		return sdk.NewCoin(o.Denom, o.Remaining)
	}
	return sdk.NewCoin(o.SettlementDenom, o.Remaining.Mul(math.NewIntFromUint64(o.UnitPrice)))
}
