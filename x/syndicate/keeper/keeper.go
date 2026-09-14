package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/syndicate/types"
)

// Keeper holds the syndication layer.
type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec

	// Governance: the authority that admits platform operators. An operator
	// admits investors and moves their money, so who admits operators is the
	// only question that cannot be delegated to an operator.
	authority []byte

	bankKeeper  types.BankKeeper
	groupKeeper types.GroupKeeper

	Schema collections.Schema
	Params collections.Item[types.Params]

	Operator collections.Map[string, types.Operator]
	Investor collections.Map[string, types.Investor]
	Deal     collections.Map[uint64, types.Deal]
	Listing  collections.Map[uint64, types.Listing]
	Document collections.Map[uint64, types.Document]

	// (deal, investor) and (deal, holder). One deal's commitments or holders
	// are a prefix walk; one investor's place in one deal is a single read.
	Commitment collections.Map[collections.Pair[uint64, string], types.Commitment]
	Position   collections.Map[collections.Pair[uint64, string], types.Position]

	// deal reference -> deal id. How an operator reconciles with its own file.
	ByDealRef collections.Map[string, uint64]
	// deal denom -> deal id. The send restriction runs on every bank transfer
	// on the chain, so it needs one cheap lookup to decide a transfer is none
	// of its business.
	ByDenom collections.Map[string, uint64]
	// (deal, listing) so one deal's market is a prefix walk.
	ListingsByDeal collections.KeySet[collections.Pair[uint64, uint64]]
	// (deal, document), same reason and the same shape.
	DocumentsByDeal collections.KeySet[collections.Pair[uint64, uint64]]

	NextDealID     collections.Sequence
	NextListingID  collections.Sequence
	NextDocumentID collections.Sequence
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,
	bankKeeper types.BankKeeper,
	groupKeeper types.GroupKeeper,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		storeService: storeService,
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,
		bankKeeper:   bankKeeper,
		groupKeeper:  groupKeeper,

		Params: collections.NewItem(sb, types.ParamsKey, "params",
			codec.CollValue[types.Params](cdc)),

		Operator: collections.NewMap(sb, types.OperatorKey, "operator",
			collections.StringKey, codec.CollValue[types.Operator](cdc)),
		Investor: collections.NewMap(sb, types.InvestorKey, "investor",
			collections.StringKey, codec.CollValue[types.Investor](cdc)),
		Deal: collections.NewMap(sb, types.DealKey, "deal",
			collections.Uint64Key, codec.CollValue[types.Deal](cdc)),
		Listing: collections.NewMap(sb, types.ListingKey, "listing",
			collections.Uint64Key, codec.CollValue[types.Listing](cdc)),
		Document: collections.NewMap(sb, types.DocumentKey, "document",
			collections.Uint64Key, codec.CollValue[types.Document](cdc)),

		Commitment: collections.NewMap(sb, types.CommitmentKey, "commitment",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.Commitment](cdc)),
		Position: collections.NewMap(sb, types.PositionKey, "position",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.Position](cdc)),

		ByDealRef: collections.NewMap(sb, types.ByDealRefKey, "by_deal_ref",
			collections.StringKey, collections.Uint64Value),
		ByDenom: collections.NewMap(sb, types.ByDenomKey, "by_denom",
			collections.StringKey, collections.Uint64Value),
		ListingsByDeal: collections.NewKeySet(sb, types.ListingsByDealKey, "listings_by_deal",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		DocumentsByDeal: collections.NewKeySet(sb, types.DocumentsByDealKey, "documents_by_deal",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),

		NextDealID:     collections.NewSequence(sb, types.NextDealIDKey, "next_deal_id"),
		NextListingID:  collections.NewSequence(sb, types.NextListingIDKey, "next_listing_id"),
		NextDocumentID: collections.NewSequence(sb, types.NextDocumentIDKey, "next_document_id"),
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

// ModuleAddress is the escrow account: where committed money sits between
// commitment and close, and where a deal's distribution pot lives.
func (k Keeper) ModuleAddress() sdk.AccAddress {
	return authtypes.NewModuleAddress(types.ModuleName)
}

// nextID hands out an id, skipping zero.
//
// Zero is never issued. In proto3 a zero id is indistinguishable from an unset
// field, and this module has several messages that name a deal or a listing by
// id — a message that forgot to set one would otherwise address deal zero and
// succeed.
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

// activeOperator resolves a signer to the platform operator it is.
//
// An operator struck off keeps its record — every deal names the operator that
// opened it, and "was this platform admitted at the time" has to stay
// answerable — so the check is on `active` rather than on existence.
func (k Keeper) activeOperator(ctx context.Context, signer string) (types.Operator, error) {
	op, err := k.Operator.Get(ctx, signer)
	if err != nil {
		if isNotFound(err) {
			return types.Operator{}, types.ErrNotOperator.Wrapf("%s", signer)
		}
		return types.Operator{}, err
	}
	if !op.Active {
		return types.Operator{}, types.ErrOperatorInactive.Wrapf("%s", signer)
	}
	return op, nil
}

// eligible reports whether an address may ACQUIRE a deal token.
//
// Acquire, not hold, and the asymmetry is deliberate. An investor whose
// onboarding is later withdrawn keeps what they already own and keeps being
// paid on it: their money is in a company, the chain cannot get it out, and
// freezing the position would strand it without helping anybody. What they lose
// is the ability to take on more.
//
// The check lives here rather than in a user interface because the deal token
// is a bank denomination. A gate in a web application binds exactly nobody — a
// bank send does not go through a web application — so it is enforced in the
// send restriction, on every transfer, including ones this module never sees.
func (k Keeper) eligible(ctx context.Context, addr string) error {
	inv, err := k.Investor.Get(ctx, addr)
	if err != nil {
		if isNotFound(err) {
			return types.ErrNotOnboarded.Wrapf("%s", addr)
		}
		return err
	}
	if !inv.Active {
		return types.ErrInvestorInactive.Wrapf("%s", addr)
	}
	return nil
}

/* ========================================================================== */
/*  The distribution index                                                    */
/* ========================================================================== */

// Settle brings one holder's accrued entitlement up to date.
//
// This is the mechanism the whole product rests on, so it is worth being exact
// about what it replaces.
//
// The naive design pays a distribution by walking every holder and crediting
// each one. That is O(holders) inside one transaction — which is a gas bomb on
// a well-subscribed deal — and, worse, it has to decide who the holders WERE,
// which means the platform is back to tracking a chain of historical owners
// through private secondary sales. That reconciliation is precisely the
// administrative burden this platform exists to remove.
//
// Instead the deal carries a running total of distributions per token, scaled.
// A holder's entitlement is
//
//	balance * (deal.index - position.index) / IndexScale
//
// which is O(1) and needs no list of holders at all. A distribution is one
// addition to one number. Whoever holds the token when that addition happens is
// entitled to their share of it, automatically, whether they bought it from the
// original subscriber, from somebody who bought it from them, or five minutes
// ago in a private sale nobody told the platform about.
//
// The correctness condition is that a position must be settled against the
// balance the holder ACTUALLY HELD over the interval. That is why transfers
// settle both sides BEFORE the balances move — settling afterwards would credit
// the seller's holding period to the buyer.
func (k Keeper) Settle(ctx context.Context, dealID uint64, holder sdk.AccAddress, balance math.Int) error {
	addr, err := k.addressCodec.BytesToString(holder)
	if err != nil {
		return err
	}

	deal, err := k.Deal.Get(ctx, dealID)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}

	key := collections.Join(dealID, addr)
	pos, err := k.Position.Get(ctx, key)
	if err != nil {
		if !isNotFound(err) {
			return err
		}
		// A holder the module has never seen starts at the CURRENT index, not
		// at zero. Starting at zero would pay them every distribution the deal
		// has ever made, out of a pot sized for one — the same defect as
		// resetting a position on genesis import.
		//
		// The corollary is what makes CloseDeal write positions explicitly: an
		// ORIGINAL subscriber is not a new holder, and if their position were
		// left to be created here they would be started at whatever index the
		// deal had reached by the time they first claimed, and paid nothing.
		// This branch is for somebody who genuinely arrived after the fact.
		pos = types.Position{
			DealId:  dealID,
			Holder:  addr,
			Index:   deal.DistributionIndex,
			Accrued: math.ZeroInt(),
		}
	}

	if deal.DistributionIndex.GT(pos.Index) && balance.IsPositive() {
		delta := deal.DistributionIndex.Sub(pos.Index)
		// Truncating division. The remainder stays in the pot and is paid with
		// the next distribution rather than being swept, because sweeping it
		// needs somebody to sweep it to — and on a deal that has been realised
		// there is nobody left with a claim on it but the holders themselves.
		earned := balance.Mul(delta).Quo(math.NewInt(types.IndexScale))
		pos.Accrued = pos.Accrued.Add(earned)
	}
	pos.Index = deal.DistributionIndex

	return k.Position.Set(ctx, key, pos)
}

// Entitlement is what a holder could claim right now: what has been settled,
// plus what has accrued since, without writing anything.
func (k Keeper) Entitlement(
	ctx context.Context, dealID uint64, holder sdk.AccAddress,
) (math.Int, math.Int, error) {
	addr, err := k.addressCodec.BytesToString(holder)
	if err != nil {
		return math.ZeroInt(), math.ZeroInt(), err
	}

	deal, err := k.Deal.Get(ctx, dealID)
	if err != nil {
		return math.ZeroInt(), math.ZeroInt(), err
	}

	balance := math.ZeroInt()
	if deal.Denom != "" {
		balance = k.bankKeeper.GetBalance(
			sdk.UnwrapSDKContext(ctx), holder, deal.Denom).Amount
	}

	accrued := math.ZeroInt()
	index := deal.DistributionIndex
	if pos, err := k.Position.Get(ctx, collections.Join(dealID, addr)); err == nil {
		accrued = pos.Accrued
		index = pos.Index
	} else if !isNotFound(err) {
		return math.ZeroInt(), math.ZeroInt(), err
	}

	if deal.DistributionIndex.GT(index) && balance.IsPositive() {
		delta := deal.DistributionIndex.Sub(index)
		accrued = accrued.Add(balance.Mul(delta).Quo(math.NewInt(types.IndexScale)))
	}

	return balance, accrued, nil
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, collections.ErrNotFound)
}
