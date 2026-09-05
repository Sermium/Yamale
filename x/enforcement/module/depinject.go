package enforcement

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/enforcement/keeper"
	"yamale/blockchain/x/enforcement/types"
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

type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	AuthKeeper    types.AuthKeeper
	BankKeeper    types.BankKeeper
	StakingKeeper types.StakingKeeper

	// DistributionKeeper is how a freeze reaches the staking rewards, which a
	// send restriction cannot see: the sender on a reward payout is the
	// distribution module account, not the frozen delegator.
	DistributionKeeper types.DistributionKeeper

	// ConstitutionKeeper holds the parameters this module may read and may not
	// change.
	ConstitutionKeeper types.ConstitutionKeeper

	// ScopeKeeper is the jurisdictional perimeter this module may read and may
	// not change. See types.ScopeKeeper.
	ScopeKeeper types.ScopeKeeper
}

type ModuleOutputs struct {
	depinject.Out

	EnforcementKeeper keeper.Keeper
	Module            appmodule.AppModule
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
		in.AuthKeeper,
		in.BankKeeper,
		in.StakingKeeper,
		in.DistributionKeeper,
		in.ConstitutionKeeper,
		in.ScopeKeeper,
	)
	m := NewAppModule(in.Cdc, k, in.AuthKeeper, in.BankKeeper)

	return ModuleOutputs{EnforcementKeeper: k, Module: m}
}
