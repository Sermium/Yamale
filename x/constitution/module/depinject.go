package constitution

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/constitution/keeper"
	"yamale/blockchain/x/constitution/types"
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

// ModuleInputs is deliberately short. This module depends on x/staking and
// nothing else on the chain: x/validatorgov and x/enforcement both consult it,
// so anything it consulted back would be a dependency cycle depinject cannot
// wire — and, more to the point, a constitution that read a value from the
// module it constrains would not be constraining it.
type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	StakingKeeper types.StakingKeeper
}

type ModuleOutputs struct {
	depinject.Out

	ConstitutionKeeper keeper.Keeper
	Module             appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	// default to governance authority if not provided
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}
	k := keeper.NewKeeper(
		in.StoreService,
		in.Cdc,
		in.AddressCodec,
		authority,
		in.StakingKeeper,
	)
	m := NewAppModule(in.Cdc, k)

	return ModuleOutputs{ConstitutionKeeper: k, Module: m}
}
