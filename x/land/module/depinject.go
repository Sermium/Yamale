package land

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/land/keeper"
	"yamale/blockchain/x/land/types"
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

// ModuleInputs are what the registry needs. Notably absent: a bank keeper. The
// module never moves money — a sale is paid for off-chain and recorded here as
// a declared price, because pretending the chain settled it would make the
// record false.
type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	// Supplied so the registry can refuse an office that is a single key.
	GroupKeeper types.GroupKeeper

	// Supplied so the registry can refuse an office acting outside the
	// jurisdiction governance granted it. See types.ScopeKeeper.
	ScopeKeeper types.ScopeKeeper
}

type ModuleOutputs struct {
	depinject.Out

	LandKeeper keeper.Keeper
	Module     appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	// Governance unless overridden, and overriding it is a decision worth
	// noticing: whoever holds this authority admits the registry offices.
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}

	k := keeper.NewKeeper(in.StoreService, in.Cdc, in.AddressCodec, authority,
		in.GroupKeeper, in.ScopeKeeper)
	m := NewAppModule(in.Cdc, k)

	return ModuleOutputs{LandKeeper: k, Module: m}
}
