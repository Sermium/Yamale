package market

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/market/keeper"
	"yamale/blockchain/x/market/types"
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

// ModuleInputs are what the market needs.
//
// Neither keeper is optional, and for different reasons.
//
// The bank keeper holds every resting order's property and moves both legs of
// every trade. A market without one could take an order and never return the
// escrow.
//
// The tokenisation keeper is what makes this a market for SHARES rather than a
// generic exchange. It answers which vehicle a denomination belongs to, which
// currency that vehicle settles in, and whether its shares still move — and it
// is where the price of every fill is written as the buyer's cost basis. Nil it
// and the module would happily trade any denomination on the chain at any price
// in any currency, recording nothing, which is not a reduced feature set but a
// different and much worse product.
type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	BankKeeper         types.BankKeeper
	TokenisationKeeper types.TokenisationKeeper
}

type ModuleOutputs struct {
	depinject.Out

	MarketKeeper keeper.Keeper
	Module       appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	// Governance unless overridden. The authority here reaches only the
	// market-wide limits: it cannot cancel an order, reprice a book or touch an
	// escrow, and a market whose operator could do any of those is not one
	// anybody should rest an order on.
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}

	k := keeper.NewKeeper(in.StoreService, in.Cdc, in.AddressCodec, authority,
		in.BankKeeper, in.TokenisationKeeper)
	m := NewAppModule(in.Cdc, k)

	return ModuleOutputs{MarketKeeper: k, Module: m}
}
