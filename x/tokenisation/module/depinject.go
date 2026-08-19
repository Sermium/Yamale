package tokenisation

import (
	"context"

	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	landkeeper "yamale/blockchain/x/land/keeper"
	"yamale/blockchain/x/tokenisation/keeper"
	"yamale/blockchain/x/tokenisation/types"
)

var _ depinject.OnePerModuleType = AppModule{}

func init() {
	appconfig.Register(
		&types.Module{},
		appconfig.Provide(ProvideModule),
	)
}

type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec
	Logger       log.Logger

	// The narrow bank surface: mint, burn, and move claims. Nothing wider —
	// custody has no business touching anyone else's money.
	BankKeeper types.BankKeeper

	// The land registry, taken concretely and narrowed by landBridge below.
	//
	// Required rather than optional: depinject would hand an absent keeper its
	// zero value, which reads as "a registry that answers no to everything"
	// only until it panics on a nil store. A chain that means to carry land
	// assets must wire x/land, and one that does not carries no parcel ids.
	LandKeeper landkeeper.Keeper
}

type ModuleOutputs struct {
	depinject.Out

	TokenisationKeeper keeper.Keeper
	Module        appmodule.AppModule
}

// ProvideModule builds the keeper.
//
// The bank surface is deliberately narrow: mint and burn the shareholding this
// module issued, move income into and out of vaults it owns, and read balances
// to settle the index. Nothing wider. A registry that can move arbitrary funds
// is a registry that can eventually be made to spend them.
func ProvideModule(in ModuleInputs) ModuleOutputs {
	authority := authtypes.NewModuleAddress(govtypes.ModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}

	k := keeper.NewKeeper(
		in.Cdc,
		in.StoreService,
		in.AddressCodec,
		authority,
		in.BankKeeper,
		NewLandBridge(in.LandKeeper),
		in.Logger,
	)
	return ModuleOutputs{TokenisationKeeper: k, Module: NewAppModule(in.Cdc, k)}
}

// landBridge narrows the land registry to the two read-only questions this
// module asks, and translates the registry's records into this module's own
// structs.
//
// The translation lives here, in the wiring, rather than in either keeper. Put
// in x/land it would make the registry depend on a vehicle module built over
// it; put in x/tokenisation/types it would drag x/land's records into the
// interface every consumer implements. Here it costs one adapter and both
// modules stay buildable, and reasonable, alone.
type landBridge struct{ k landkeeper.Keeper }

// NewLandBridge adapts a land registry keeper to the surface x/tokenisation
// consults. Exported for tests, which wire a real registry rather than a stub —
// the rules being enforced are the registry's, and a stub would only prove this
// module can read a struct it wrote itself.
func NewLandBridge(k landkeeper.Keeper) types.LandKeeper { return landBridge{k: k} }

func (b landBridge) LandParcel(
	ctx context.Context, parcelID uint64,
) (types.LandParcel, error) {
	parcel, err := b.k.Parcel.Get(ctx, parcelID)
	if err != nil {
		// An unknown parcel is an answer, not a failure. Distinguishing it from
		// a store error would mean deciding here what "not found" looks like in
		// every collections backend; the caller only needs to know it is absent.
		return types.LandParcel{}, nil
	}
	return types.LandParcel{
		Exists:                   true,
		Holder:                   parcel.Holder,
		ForbidsFractionalisation: parcel.ForbidsFractionalisation(),
	}, nil
}

func (b landBridge) LandAuthorisation(
	ctx context.Context, parcelID uint64,
) (types.LandAuthorisation, error) {
	auth, err := b.k.FractionalisationAuthority.Get(ctx, parcelID)
	if err != nil {
		return types.LandAuthorisation{}, nil
	}
	return types.LandAuthorisation{
		Granted:     true,
		Withdrawn:   auth.Withdrawn,
		ExpiresAt:   auth.ExpiresAt,
		MaxShareBps: auth.MaxShareBps,
	}, nil
}
