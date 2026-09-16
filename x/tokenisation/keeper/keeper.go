package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	storetypes "cosmossdk.io/core/store"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/tokenisation/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	addressCodec address.Codec

	authority []byte

	bankKeeper types.BankKeeper

	// The land registry, consulted only for assets that name a parcel. Nil on a
	// chain with no x/land, in which case naming a parcel is refused rather
	// than waved through — see MsgMintAsset.parcel_id.
	landKeeper types.LandKeeper

	Schema collections.Schema

	Params      collections.Item[types.Params]
	Collections collections.Map[string, types.Collection]
	Assets      collections.Map[uint64, types.Asset]
	Vaults      collections.Map[uint64, types.Vault]

	// Positions is keyed (asset, holder). A holder's place in one asset's index
	// is a single read; listing every holder of an asset is a prefix scan.
	Positions collections.Map[collections.Pair[uint64, string], types.Position]

	// Sales holds a reported price for as long as it matters — through its
	// challenge window, and through a dispute if one is raised.
	Sales collections.Map[uint64, types.SaleReport]

	// ByDenom maps a fraction denom back to its asset. The send restriction
	// runs on every bank transfer on the chain, so it needs one cheap lookup to
	// decide a transfer is none of its business.
	ByDenom collections.Map[string, uint64]

	// ParcelIssuedBps totals, per x/land parcel, the share already issued over
	// it across every vehicle.
	//
	// A ceiling checked one vehicle at a time is not a ceiling: an owner
	// permitted to sell 40% mints a second asset over the same parcel and sells
	// 40% again. The registry authorises a fraction of the land, not a fraction
	// per vehicle, so the total is what has to be bounded.
	//
	// Derived from the assets and written in InitGenesis rather than exported,
	// exactly like ByDenom — a total that is sometimes absent is a ceiling that
	// is sometimes not enforced.
	ParcelIssuedBps collections.Map[uint64, uint32]

	// Covenants by id, with a key set beside them so one vehicle's filings are a
	// prefix walk rather than a scan of every filing on the chain.
	// (asset, period) -> what that distribution paid. Read by a settle, never
	// walked: two entries answer any gap.
	PeriodRates collections.Map[collections.Pair[uint64, uint64], types.PeriodRate]

	Covenants        collections.Map[uint64, types.Covenant]
	CovenantsByAsset collections.KeySet[collections.Pair[uint64, uint64]]

	NextAssetID    collections.Sequence
	NextCovenantID collections.Sequence

	// escrowAccounts are accounts that hold shares on somebody else's behalf.
	//
	// Cost stands still when shares cross one of these, and that is a rule about
	// meaning rather than an optimisation. A marketplace escrow is not a holder:
	// shares arrive in it from many sellers and leave it to many buyers, so the
	// proportional carry that is right between two people would hand a buyer a
	// slice of a pooled average of every seller's cost — a figure belonging to
	// nobody, computed from trades the buyer had no part in.
	//
	// The worse half is the round trip. Without this, a holder who listed shares
	// and then withdrew the listing would get the shares back with their cost
	// basis spent in the escrow account — a loss the chain manufactured out of an
	// offer nobody took, and one they could only see by noticing their reported
	// return had risen for no reason.
	//
	// What replaces the carry is not nothing: a marketplace fill knows the price,
	// so it writes the buyer's cost from the trade and releases the seller's
	// against it. See RecordPurchase and ReleaseBasis.
	//
	// A map rather than a slice field because this keeper is held and copied BY
	// VALUE everywhere in the app, so a registration made after construction has
	// to reach copies that already exist. A map header is shared between copies;
	// a slice appended to after the fact is not.
	escrowAccounts map[string]struct{}
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService storetypes.KVStoreService,
	addressCodec address.Codec,
	authority []byte,
	bankKeeper types.BankKeeper,
	landKeeper types.LandKeeper,
	logger log.Logger,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,
		bankKeeper:   bankKeeper,
		landKeeper:   landKeeper,

		Params:      collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		Collections: collections.NewMap(sb, types.CollectionsKey, "collections", collections.StringKey, codec.CollValue[types.Collection](cdc)),
		Assets:      collections.NewMap(sb, types.AssetsKey, "assets", collections.Uint64Key, codec.CollValue[types.Asset](cdc)),
		Vaults:      collections.NewMap(sb, types.VaultsKey, "vaults", collections.Uint64Key, codec.CollValue[types.Vault](cdc)),
		Positions: collections.NewMap(sb, types.PositionsKey, "positions",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.Position](cdc)),
		Sales:   collections.NewMap(sb, types.SalesKey, "sales", collections.Uint64Key, codec.CollValue[types.SaleReport](cdc)),
		ByDenom: collections.NewMap(sb, types.ByDenomKey, "by_denom", collections.StringKey, collections.Uint64Value),
		ParcelIssuedBps: collections.NewMap(sb, types.ParcelIssuedBpsKey, "parcel_issued_bps",
			collections.Uint64Key, collections.Uint32Value),
		PeriodRates: collections.NewMap(sb, types.PeriodRatesKey, "period_rates",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key),
			codec.CollValue[types.PeriodRate](cdc)),

		Covenants: collections.NewMap(sb, types.CovenantsKey, "covenants",
			collections.Uint64Key, codec.CollValue[types.Covenant](cdc)),
		CovenantsByAsset: collections.NewKeySet(sb, types.CovenantsByAssetKey, "covenants_by_asset",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),

		NextAssetID:    collections.NewSequence(sb, types.NextAssetIDKey, "next_asset_id"),
		NextCovenantID: collections.NewSequence(sb, types.NextCovenantIDKey, "next_covenant_id"),

		// Allocated here and never reassigned, so every copy of this keeper shares
		// one map and a registration made during app wiring is visible to all of
		// them.
		escrowAccounts: map[string]struct{}{},
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() []byte { return k.authority }

// RegisterEscrow marks an account as holding shares on somebody else's behalf.
//
// Called during app wiring, by the profile that links the module owning the
// account, so that the registration happens identically on every node before a
// block is ever processed. It is deliberately NOT a message: which accounts are
// escrows is a property of which modules are compiled in, and a chain where
// that could be changed by a transaction would have a governance vote able to
// silently redirect where cost basis flows.
func (k Keeper) RegisterEscrow(addr sdk.AccAddress) {
	k.escrowAccounts[addr.String()] = struct{}{}
}

// IsEscrow reports whether cost should stand still when shares cross an
// account.
//
// Exported because the registration is done by app wiring rather than by a
// message, which makes it the kind of fact that is true until somebody removes
// one line and nothing fails. The consequence of that line going missing is not
// a crash: it is a chain that trades correctly and reports every holder’s cost
// wrong, so there has to be a way to assert it from outside.
func (k Keeper) IsEscrow(addr sdk.AccAddress) bool {
	_, ok := k.escrowAccounts[addr.String()]
	return ok
}

// ShareAsset resolves a fraction denomination to what a market needs to know.
//
// Three facts, together rather than as three lookups, because a caller that
// took them separately could act on a vehicle that stopped trading between the
// second and the third — and the caller here is a marketplace whose next step
// is to move somebody's money.
//
// The settlement denomination is the vault's, and it is the one a trade in
// these shares must be priced in. Cost basis is carried in it, so a fill priced
// in anything else would record what a holding cost in one currency against
// what it returned in another, and the subtraction that produces a return would
// be meaningless while still producing a number.
func (k Keeper) ShareAsset(
	ctx context.Context, denom string,
) (assetID uint64, settlementDenom string, tradable bool, err error) {
	assetID, err = k.ByDenom.Get(ctx, denom)
	if err != nil {
		return 0, "", false, types.ErrAssetNotFound.Wrapf(
			"%q is not a fraction denomination issued by this module", denom)
	}
	asset, err := k.Assets.Get(ctx, assetID)
	if err != nil {
		return 0, "", false, err
	}
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return 0, "", false, err
	}

	// The same two statuses the send restriction halts on. Reported here so a
	// market can refuse an order with a reason of its own rather than letting a
	// bank send fail somewhere in the middle of a settlement.
	tradable = asset.Status != types.STATUS_REALISED && asset.Status != types.STATUS_CLOSED
	return assetID, vault.Denom, tradable, nil
}

// AccrueIncome raises an asset's index by a gross income figure.
//
// Only holder_share_bps of it reaches the index. The remainder is the
// sponsor's share of their own asset and is never taken from them — which is
// now true of the money as well as of the arithmetic: the collecting handler
// takes the cut and leaves the rest where it is.
func (k Keeper) AccrueIncome(ctx context.Context, assetID uint64, gross math.Int) error {
	asset, err := k.Assets.Get(ctx, assetID)
	if err != nil {
		return err
	}
	return k.creditHolders(ctx, assetID, holderCut(asset, gross), gross)
}

// creditHolders is the only path to the index, and it will not take a step the
// vault cannot pay for.
//
// FinaliseSale used to credit a whole sale price here with no coins having
// moved at all. The index rose, redemption opened, and the first holder to
// call Redeem was paid out of the module account — which holds every other
// vehicle's rent. The vehicle that reported the sale had funded nothing; the
// vehicle that lost the money had done nothing. The check below is what makes
// that a refused transaction rather than a theft.
func (k Keeper) creditHolders(ctx context.Context, assetID uint64, credited, gross math.Int) error {
	asset, err := k.Assets.Get(ctx, assetID)
	if err != nil {
		return err
	}
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return err
	}

	supply := k.bankKeeper.GetSupply(sdk.UnwrapSDKContext(ctx), asset.FractionDenom).Amount
	if !supply.IsPositive() {
		// Nothing is fractionalised, so there is no denominator and nobody to
		// credit. Refusing is right: silently keeping the money would leave the
		// vault holding funds no index can ever pay out.
		return types.ErrNoShareholders
	}
	if !credited.IsPositive() {
		return types.ErrAmountTooSmall
	}
	if sdk.Coins(vault.Held).AmountOf(vault.Denom).LT(credited) {
		return types.ErrVaultUnfunded.Wrapf(
			"vehicle %d holds %s%s and is being credited %s", assetID,
			sdk.Coins(vault.Held).AmountOf(vault.Denom), vault.Denom, credited)
	}

	// The period closes here, which is what apportions this distribution by the
	// time each holder actually held rather than by who is present now.
	// ClosePeriod records the rate and advances the cumulative figure, so the
	// index this used to bump directly now lives in the period record.
	//
	// Funded is written first and the vault re-read inside ClosePeriod, so the
	// two writes cannot disagree about which is authoritative.
	vault.Funded = sdk.Coins(vault.Funded).Add(sdk.NewCoin(vault.Denom, gross))
	if err := k.Vaults.Set(ctx, assetID, vault); err != nil {
		return err
	}
	return k.ClosePeriod(ctx, assetID, credited, supply)
}

// Settle brings one holder's position up to date with the index.
//
// Called before any balance change, so what an account is owed is always
// balance * (cumulative - last_index) for the balance it actually held over
// that stretch. This is what makes the scheme time-weighted, and time-weighting
// is what makes it unsnipeable: two blocks of holding earn two blocks of
// income, not a whole quarter's.
func (k Keeper) Settle(ctx context.Context, assetID uint64, holder sdk.AccAddress, balance math.Int) error {
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return err
	}

	key := collections.Join(assetID, holder.String())
	pos, err := k.Positions.Get(ctx, key)
	if err != nil {
		// First sight of this holder. They start at the current index rather
		// than at zero — crediting them the asset's entire history would pay
		// them for a period during which they held nothing.
		pos = types.Position{
			AssetId:   assetID,
			Holder:    holder.String(),
			LastIndex: vault.CumulativePerToken,
			Accrued:   math.ZeroInt(),
			// Explicitly zero, never left unset. math.Int's zero value is nil
			// and panics on arithmetic rather than erroring, so a position
			// created without these is a halt waiting for the first transfer.
			CostBasis: math.ZeroInt(),
			CostQty:   math.ZeroInt(),
		}
	}

	// Positions written before cost_basis existed decode with both fields nil.
	// The upgrade migrates them; this is the belt to that braces, because the
	// cost of a missed row is a panic in a block rather than a wrong number.
	pos.CostBasis = zeroIfNil(pos.CostBasis)
	pos.CostQty = zeroIfNil(pos.CostQty)
	pos.Accrued = zeroIfNil(pos.Accrued)

	// Pro rata temporis. This is what pays a holder for the time they held
	// rather than for being present when a cheque was declared, and it credits
	// everything owed for periods that have closed since they last touched.
	//
	// It replaces the bare index delta that used to be here. The index itself
	// survives inside PeriodRate.cumulative_per_token, which is what settles
	// whole periods in one subtraction.
	if err := k.AccruePosition(ctx, assetID, &pos, balance, blockSeconds(ctx)); err != nil {
		return err
	}
	pos.LastIndex = vault.CumulativePerToken

	return k.Positions.Set(ctx, key, pos)
}

// Entitlement is what a holder could withdraw right now, settled or not.
func (k Keeper) Entitlement(ctx context.Context, assetID uint64, holder sdk.AccAddress) (math.Int, error) {
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return math.ZeroInt(), err
	}
	asset, err := k.Assets.Get(ctx, assetID)
	if err != nil {
		return math.ZeroInt(), err
	}

	owed := math.ZeroInt()
	key := collections.Join(assetID, holder.String())
	pos, err := k.Positions.Get(ctx, key)
	if err != nil {
		return owed, nil
	}

	// Computed on a COPY, never written. A query that settled as a side effect
	// would make reading a balance change it, and two readers in one block
	// would then disagree about what a third account is owed.
	balance := k.bankKeeper.GetBalance(sdk.UnwrapSDKContext(ctx), holder, asset.FractionDenom).Amount
	settled := pos
	if err := k.AccruePosition(ctx, assetID, &settled, balance, blockSeconds(ctx)); err != nil {
		return math.ZeroInt(), err
	}
	owed = zeroIfNil(settled.Accrued)

	// What is accruing in the CURRENT period is deliberately NOT added. Its
	// rate is unknown until the period closes, so converting it at the last
	// period's rate would put a forecast beside a settled figure in the same
	// units, and a reader cannot tell those apart once both are money.
	// PendingTokenSeconds is how an interface shows it, in its own units.
	_ = vault
	return owed, nil
}

// ModuleAddress is the account the module holds income and escrow in.
//
// It is derived the way every module account on this chain is derived. An
// earlier version cast a formatted string straight to sdk.AccAddress, which
// produces bytes of the wrong length that match no real account: the send
// restriction's exemption for redemption never fired, so redeeming at REALISED
// was refused and every shareholder's money was locked in permanently.
func (k Keeper) ModuleAddress() sdk.AccAddress {
	return authtypes.NewModuleAddress(types.ModuleName)
}

func (k Keeper) Logger(ctx context.Context) log.Logger {
	return sdk.UnwrapSDKContext(ctx).Logger().With("module", "x/"+types.ModuleName)
}

// blockSeconds is the chain's clock. Every accrual in this module is an
// integral of balance over time, so it has to come from the block rather than
// from anything a caller could choose.
func blockSeconds(ctx context.Context) int64 {
	return sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
}
