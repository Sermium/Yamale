package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/market/types"
)

// Keeper holds the secondary market.
type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec

	// Governance, and only for the limits. Nothing here lets an authority
	// cancel somebody's order, reprice a book or take an escrow — a market
	// whose operator can reach into it is not one an investor can rely on to
	// still be holding their shares tomorrow.
	authority []byte

	bankKeeper         types.BankKeeper
	tokenisationKeeper types.TokenisationKeeper

	Schema collections.Schema
	Params collections.Item[types.Params]

	Listing collections.Map[uint64, types.Listing]
	Auction collections.Map[uint64, types.Auction]
	Order   collections.Map[uint64, types.Order]

	// (denom, id), so one share's offers are a prefix walk.
	ListingsByDenom collections.KeySet[collections.Pair[string, uint64]]
	AuctionsByDenom collections.KeySet[collections.Pair[string, uint64]]

	// The book, by side: (denom, unit_price, order id), holding only OPEN
	// orders. The price sits ahead of the id so that the best price on a side is
	// the first key of a range scan rather than the result of walking every
	// resting order. See types.BidsKey.
	Bids collections.KeySet[collections.Triple[string, uint64, uint64]]
	Asks collections.KeySet[collections.Triple[string, uint64, uint64]]

	NextListingID collections.Sequence
	NextAuctionID collections.Sequence
	NextOrderID   collections.Sequence
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,
	bankKeeper types.BankKeeper,
	tokenisationKeeper types.TokenisationKeeper,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		storeService:       storeService,
		cdc:                cdc,
		addressCodec:       addressCodec,
		authority:          authority,
		bankKeeper:         bankKeeper,
		tokenisationKeeper: tokenisationKeeper,

		Params: collections.NewItem(sb, types.ParamsKey, "params",
			codec.CollValue[types.Params](cdc)),

		Listing: collections.NewMap(sb, types.ListingKey, "listing",
			collections.Uint64Key, codec.CollValue[types.Listing](cdc)),
		Auction: collections.NewMap(sb, types.AuctionKey, "auction",
			collections.Uint64Key, codec.CollValue[types.Auction](cdc)),
		Order: collections.NewMap(sb, types.OrderKey, "order",
			collections.Uint64Key, codec.CollValue[types.Order](cdc)),

		ListingsByDenom: collections.NewKeySet(sb, types.ListingsByDenomKey, "listings_by_denom",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		AuctionsByDenom: collections.NewKeySet(sb, types.AuctionsByDenomKey, "auctions_by_denom",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		Bids: collections.NewKeySet(sb, types.BidsKey, "bids",
			collections.TripleKeyCodec(collections.StringKey, collections.Uint64Key, collections.Uint64Key)),
		Asks: collections.NewKeySet(sb, types.AsksKey, "asks",
			collections.TripleKeyCodec(collections.StringKey, collections.Uint64Key, collections.Uint64Key)),

		NextListingID: collections.NewSequence(sb, types.NextListingIDKey, "next_listing_id"),
		NextAuctionID: collections.NewSequence(sb, types.NextAuctionIDKey, "next_auction_id"),
		NextOrderID:   collections.NewSequence(sb, types.NextOrderIDKey, "next_order_id"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// GetAuthority returns the module's governance authority.
func (k Keeper) GetAuthority() []byte { return k.authority }

// ModuleAddress is the escrow account: every resting order's shares or money.
//
// Derived the way every module account on this chain is derived, rather than by
// casting a formatted string to sdk.AccAddress — which produces bytes of the
// wrong length that match no real account, and whose symptom in x/tokenisation
// was that an exemption in the send restriction never fired and every
// shareholder's money was locked in permanently.
func (k Keeper) ModuleAddress() sdk.AccAddress {
	return authtypes.NewModuleAddress(types.ModuleName)
}

// nextID hands out an id, skipping zero.
//
// Zero is never issued. In proto3 a zero id is indistinguishable from an unset
// field, and every message in this module addresses a listing, an auction or an
// order by id — a client that forgot to set one would otherwise address order
// zero and be told it succeeded.
func nextID(ctx context.Context, seq collections.Sequence) (uint64, error) {
	id, err := seq.Next(ctx)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return seq.Next(ctx)
	}
	return id, nil
}

// tradableShare resolves a denomination to a vehicle this market may trade.
//
// Every entry point goes through here before touching anybody's property, so
// that a refusal names the reason. Without it the first thing to fail would be a
// bank send, part-way through a settlement, with an error about a denomination
// rather than about the vehicle having been sold.
func (k Keeper) tradableShare(
	ctx context.Context, denom string,
) (assetID uint64, settlementDenom string, err error) {
	assetID, settlementDenom, tradable, err := k.tokenisationKeeper.ShareAsset(ctx, denom)
	if err != nil {
		return 0, "", types.ErrNotShareDenom.Wrapf("%q", denom)
	}
	// A realised vehicle's shares are a claim on a known fixed sum, and the send
	// restriction refuses to move them at all. An order resting over one would
	// be an offer that can never be filled and whose escrow can only be
	// cancelled, so it is refused where somebody is present to read why.
	if !tradable {
		return 0, "", types.ErrTradingHalted.Wrapf("%q", denom)
	}
	if settlementDenom == "" {
		return 0, "", types.ErrNotShareDenom.Wrapf(
			"vehicle %d names no income denomination, so a trade in it has no currency", assetID)
	}
	return assetID, settlementDenom, nil
}

// blockTime is the chain's clock. Every deadline in this module is a moment a
// person experiences, so it comes from the block rather than from anything a
// caller could choose.
func blockTime(ctx context.Context) int64 {
	return sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, collections.ErrNotFound)
}
