package alias

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/alias/keeper"
	"yamale/blockchain/x/alias/types"
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

	// Supplied so the module can refuse a jurisdiction recorded by anyone but
	// the institution that onboarded the account. See types.ParticipantKeeper.
	ParticipantKeeper types.ParticipantKeeper
}

type ModuleOutputs struct {
	depinject.Out

	AliasKeeper keeper.Keeper
	Module      appmodule.AppModule
}

// ProvideModule builds the keeper.
//
// It takes one other keeper, and only to ask two read-only questions: who
// onboarded an account, and is that institution still admitted. It cannot move
// funds, mint, or read a balance, and the dependency runs one way — x/paymsg
// knows nothing about x/alias. That narrowness is the point: an identifier
// registry with a bank keeper in it is a registry that could eventually be made
// to spend.
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
		in.ParticipantKeeper,
	)
	return ModuleOutputs{AliasKeeper: k, Module: NewAppModule(in.Cdc, k)}
}
