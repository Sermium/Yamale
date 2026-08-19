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

	NextAssetID collections.Sequence
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
		NextAssetID: collections.NewSequence(sb, types.NextAssetIDKey, "next_asset_id"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() []byte { return k.authority }

// AccrueIncome raises an asset's index by an amount paid into its vault.
//
// Only holder_share_bps of the payment reaches the index. The remainder is the
// sponsor's share of their own asset and is never taken from them.
func (k Keeper) AccrueIncome(ctx context.Context, assetID uint64, amount math.Int) error {
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

	holderCut := amount.MulRaw(int64(asset.HolderShareBps)).QuoRaw(10_000)
	if !holderCut.IsPositive() {
		return types.ErrAmountTooSmall
	}

	// Truncation here favours the vault, never the holders: a fraction of a
	// unit that cannot be divided stays behind rather than being conjured. The
	// same rule governs redemption and x/amm.
	perToken := math.LegacyNewDecFromInt(holderCut).QuoTruncate(math.LegacyNewDecFromInt(supply))

	vault.CumulativePerToken = vault.CumulativePerToken.Add(perToken)
	vault.Funded = sdk.Coins(vault.Funded).Add(sdk.NewCoin(vault.Denom, amount))
	return k.Vaults.Set(ctx, assetID, vault)
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
		}
	}

	delta := vault.CumulativePerToken.Sub(pos.LastIndex)
	if delta.IsPositive() && balance.IsPositive() {
		earned := delta.MulInt(balance).TruncateInt()
		pos.Accrued = pos.Accrued.Add(earned)
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
	if pos, err := k.Positions.Get(ctx, key); err == nil {
		owed = pos.Accrued
		balance := k.bankKeeper.GetBalance(sdk.UnwrapSDKContext(ctx), holder, asset.FractionDenom).Amount
		if delta := vault.CumulativePerToken.Sub(pos.LastIndex); delta.IsPositive() && balance.IsPositive() {
			owed = owed.Add(delta.MulInt(balance).TruncateInt())
		}
	}
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
