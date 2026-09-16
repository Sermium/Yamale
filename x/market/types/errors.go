package types

import "cosmossdk.io/errors"

// Errors are numbered from 1 so a zero code is never a market error, and worded
// as what the person reading them actually needs to know.
//
// The audience is a holder whose transaction has just been refused, and the
// difference between "this vehicle no longer trades", "somebody has already
// bid more than that" and "you are trying to cross your own order" is the
// difference between a person fixing it themselves and a person deciding the
// market is broken.
var (
	ErrNotAuthority = errors.Register(ModuleName, 1,
		"this message is governance-only")

	ErrNotShareDenom = errors.Register(ModuleName, 2,
		"this denomination is not a tokenised shareholding, so this market cannot price it")
	ErrTradingHalted = errors.Register(ModuleName, 3,
		"this vehicle has been realised or closed; its shares are a claim on a fixed sum and no longer trade")
	ErrWrongDenom = errors.Register(ModuleName, 4,
		"this vehicle settles in a different currency")

	ErrInvalidQuantity = errors.Register(ModuleName, 5,
		"the quantity must be positive")
	ErrInvalidPrice = errors.Register(ModuleName, 6,
		"the price must be positive")
	ErrSelfDeal = errors.Register(ModuleName, 7,
		"you cannot trade with yourself")

	ErrNoListing     = errors.Register(ModuleName, 8, "no such listing")
	ErrListingClosed = errors.Register(ModuleName, 9,
		"this listing has already been filled or withdrawn")
	ErrNotSeller = errors.Register(ModuleName, 10,
		"only the seller may withdraw this offer")

	ErrNoAuction     = errors.Register(ModuleName, 11, "no such auction")
	ErrAuctionClosed = errors.Register(ModuleName, 12,
		"this auction is no longer taking bids")
	ErrAuctionRunning = errors.Register(ModuleName, 13,
		"this auction has not reached its deadline yet")
	ErrAuctionTooShort = errors.Register(ModuleName, 14,
		"an auction must run long enough for somebody other than the seller to see it")
	ErrAuctionTooLong = errors.Register(ModuleName, 15,
		"an auction may not escrow the seller's shares for that long")
	ErrBidTooLow = errors.Register(ModuleName, 16,
		"a bid must beat the standing bid, and meet the reserve")
	ErrAuctionHasBid = errors.Register(ModuleName, 17,
		"somebody has bid on this auction; it can no longer be withdrawn, only settled")

	ErrNoOrder     = errors.Register(ModuleName, 18, "no such order")
	ErrOrderClosed = errors.Register(ModuleName, 19,
		"this order has already been filled or withdrawn")
	ErrNotTrader = errors.Register(ModuleName, 20,
		"only the trader who placed an order may withdraw it")
	ErrInvalidSide = errors.Register(ModuleName, 21,
		"an order must be a bid or an ask")

	ErrInvalidParams = errors.Register(ModuleName, 22,
		"these market parameters would disable a protection")
)
