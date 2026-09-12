package selfcustody

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/selfcustody/keeper"
	"yamale/blockchain/x/selfcustody/types"
)

var _ depinject.OnePerModuleType = AppModule{}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (AppModule) IsOnePerModuleType() {}

func init() {
	appconfig.Register(
		&types.Module{},
		appconfig.Provide(ProvideModule),
	)
}

// ModuleInputs are what the pathway needs, which is almost nothing.
//
// No bank keeper, no account keeper, no custody keeper. This module never moves
// money and never touches key material; it writes down a decision and the terms
// it was made under. The short list is the design — a module that could both
// authorise a disclosure and cause one would be a single component whose
// compromise hands over every account on the chain.
type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec
}

type ModuleOutputs struct {
	depinject.Out

	SelfCustodyKeeper keeper.Keeper
	Module            appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	// Governance unless overridden. Whoever holds this authority sets the
	// waiting period and writes the warning every holder acknowledges, so
	// pointing it at the operator would hand the operator both ends of the
	// thing this module exists to take away from them.
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}

	k := keeper.NewKeeper(in.StoreService, in.Cdc, in.AddressCodec, authority)
	m := NewAppModule(in.Cdc, k)

	return ModuleOutputs{SelfCustodyKeeper: k, Module: m}
}
