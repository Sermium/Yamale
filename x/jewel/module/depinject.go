package jewel

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	circuitkeeper "cosmossdk.io/x/circuit/keeper"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/jewel/keeper"
	"yamale/blockchain/x/jewel/types"
)

var _ depinject.OnePerModuleType = AppModule{}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (AppModule) IsOnePerModuleType() {}

func init() {
	appconfig.Register(&types.Module{}, appconfig.Provide(ProvideModule))
}

// ModuleInputs are what the register needs. No bank keeper: a jewel's sale
// is paid off the chain, and the register records ownership, not payment.
type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	GroupKeeper types.GroupKeeper
	NFTKeeper   types.NFTKeeper
	// The concrete keeper, because IsAllowed has a pointer receiver and
	// depinject hands out the value.
	CircuitKeeper circuitkeeper.Keeper
}

type ModuleOutputs struct {
	depinject.Out

	JewelKeeper keeper.Keeper
	Module      appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}
	circuit := in.CircuitKeeper
	k := keeper.NewKeeper(in.StoreService, in.Cdc, in.AddressCodec, authority,
		in.GroupKeeper, in.NFTKeeper, &circuit)
	return ModuleOutputs{JewelKeeper: k, Module: NewAppModule(in.Cdc, k)}
}
