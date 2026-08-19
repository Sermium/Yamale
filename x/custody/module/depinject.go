package custody

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/custody/keeper"
	"yamale/blockchain/x/custody/types"
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
}

type ModuleOutputs struct {
	depinject.Out

	CustodyKeeper keeper.Keeper
	Module        appmodule.AppModule
}

// ProvideModule builds the keeper.
//
// It takes no other keeper. The module resolves identifiers to addresses and
// nothing else — it cannot move funds, mint, or read a balance — so there is no
// expected_keepers.go and nothing to grant it. That narrowness is the point: an
// identifier registry with a bank keeper in it is a registry that could
// eventually be made to spend.
func ProvideModule(in ModuleInputs) ModuleOutputs {
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}

	k := keeper.NewKeeper(
		in.Cdc,
		in.AddressCodec,
		in.StoreService,
		in.Logger,
		authority.String(),
		in.BankKeeper,
	)
	return ModuleOutputs{CustodyKeeper: k, Module: NewAppModule(in.Cdc, k)}
}
