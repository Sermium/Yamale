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

	// Supplied so the module can refuse a role granted to a plain key. See
	// types.GroupKeeper.
	GroupKeeper types.GroupKeeper
}

type ModuleOutputs struct {
	depinject.Out

	AliasKeeper keeper.Keeper
	Module      appmodule.AppModule
}

// ProvideModule builds the keeper.
//
// It takes two other keepers and asks each of them read-only questions and
// nothing else: who onboarded an account and is that institution still admitted,
// and is a prospective role holder a group account. It cannot move funds, mint,
// or read a balance. That narrowness is the point: an identifier registry with a
// bank keeper in it is a registry that could eventually be made to spend.
//
// Note which direction the dependencies run. x/paymsg and x/group know nothing
// about x/alias, so a module that this one *consults* must never be given this
// keeper by depinject — that is a cycle, and the graph refuses to build. x/paymsg
// is exactly that case, and it is why the payments module receives the perimeter
// check by a setter in app.go rather than through the graph.
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
		in.GroupKeeper,
	)
	return ModuleOutputs{AliasKeeper: k, Module: NewAppModule(in.Cdc, k)}
}
