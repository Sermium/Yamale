package syndicate

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/syndicate/keeper"
	"yamale/blockchain/x/syndicate/types"
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

// ModuleInputs are what the syndication layer needs.
//
// The bank keeper is not optional here, unlike in x/herd where the module
// deliberately never moves money. This one holds other people's capital between
// commitment and close and mints the token that represents it, so a nil bank
// keeper is not a reduced feature set — it is a platform that can take a
// commitment and never return it.
type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	BankKeeper types.BankKeeper
	// Supplied so the platform can refuse an operator that is a single key.
	GroupKeeper types.GroupKeeper
}

type ModuleOutputs struct {
	depinject.Out

	SyndicateKeeper keeper.Keeper
	Module          appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	// Governance unless overridden, and overriding it is a decision worth
	// noticing: whoever holds this authority admits the platform operators, and
	// an operator admits the investors and moves their money.
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}

	k := keeper.NewKeeper(in.StoreService, in.Cdc, in.AddressCodec, authority,
		in.BankKeeper, in.GroupKeeper)
	m := NewAppModule(in.Cdc, k)

	return ModuleOutputs{SyndicateKeeper: k, Module: m}
}
