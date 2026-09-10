package herd

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/herd/keeper"
	"yamale/blockchain/x/herd/types"
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

// ModuleInputs are what the registry needs.
//
// Notably absent: a bank keeper. The module never moves money. A sale is paid
// for off-chain — in cash, at a market — and recorded here as a declared price,
// because pretending the chain settled it would make the record false in the
// one place a court would look.
//
// Also absent: a ScopeKeeper. x/land takes one so an office cannot act outside
// the jurisdiction governance granted it, and this module will want the same
// eventually. It is left out until the perimeter question is settled for
// livestock, which is not the same question as for land: a parcel sits in one
// country for ever, and a transhumant herd crosses a border twice a year.
// Wiring a check whose semantics are undecided would be worse than having
// none, because it would look enforced.
type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	// Supplied so the registry can refuse an office that is a single key.
	GroupKeeper types.GroupKeeper
}

type ModuleOutputs struct {
	depinject.Out

	HerdKeeper keeper.Keeper
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
		in.GroupKeeper)
	m := NewAppModule(in.Cdc, k)

	return ModuleOutputs{HerdKeeper: k, Module: m}
}
