package validatorgov

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"

	aliaskeeper "yamale/blockchain/x/alias/keeper"
	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/validatorgov/keeper"
	"yamale/blockchain/x/validatorgov/types"
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

	// AuthzKeeper is how a completed rotation hands the incoming operator
	// authority over the outgoing one's validator. x/staking has no operation
	// that re-keys a validator record, so a grant is the only way to move who
	// signs without moving the delegations behind it.
	AuthzKeeper authzkeeper.Keeper

	// ConstitutionKeeper supplies the concentration ceilings. It is read-only
	// and one-directional: x/constitution depends on x/staking and on nothing
	// in this repository, so this module and x/enforcement can both consult it
	// without depinject having a cycle to resolve.
	ConstitutionKeeper types.ConstitutionKeeper

	// AliasKeeper supplies the jurisdiction the chain has on record for an
	// account, so the JurisdictionReconciliation query can set it beside the one
	// the validator declared here. Read-only and one-directional in the same way
	// as the constitution above: x/alias imports nothing from this module, so
	// depinject has no cycle to resolve.
	//
	// Taken as the concrete keeper rather than as an interface, following
	// AuthzKeeper. The record this module needs is x/alias's exported
	// Jurisdictions collection, and a collection is a field — no interface can
	// name it. Adapting it here is what keeps the alternative off the table,
	// which was adding a reader to another module's keeper for one consumer.
	AliasKeeper aliaskeeper.Keeper
}

type ModuleOutputs struct {
	depinject.Out

	ValidatorgovKeeper keeper.Keeper
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
		in.AuthzKeeper,
		in.AuthKeeper,
		in.BankKeeper,
		in.ConstitutionKeeper,
		NewAliasJurisdictions(in.AliasKeeper),
	)
	m := NewAppModule(in.Cdc, k, in.AuthKeeper, in.BankKeeper)

	return ModuleOutputs{ValidatorgovKeeper: k, Module: m}
}

var _ types.AliasKeeper = AliasJurisdictions{}

// AliasJurisdictions is the one read this module makes of x/alias's jurisdiction
// registry, and nothing else.
//
// It exists so that x/alias needs no change to be consulted. Its Jurisdictions
// collection is already exported, so the record is already readable; what was
// missing was a method to name in an interface, and inventing one inside the
// other module would have put a reader there for a single caller.
//
// Nothing here decides anything, which is the point of keeping it this thin. A
// missing record becomes found=false and every other store error is returned
// untouched. There is no path on which a failed lookup produces a country.
type AliasJurisdictions struct{ k aliaskeeper.Keeper }

// NewAliasJurisdictions wraps an x/alias keeper as the read this module needs.
// Exported so that a test wiring the real registry uses the same adapter the
// chain does, rather than a stub that would answer with whatever the test
// already believed.
func NewAliasJurisdictions(k aliaskeeper.Keeper) AliasJurisdictions {
	return AliasJurisdictions{k: k}
}

// JurisdictionOf returns the record x/alias holds for an account, if any.
func (a AliasJurisdictions) JurisdictionOf(ctx context.Context, address string) (aliastypes.Jurisdiction, bool, error) {
	record, err := a.k.Jurisdictions.Get(ctx, address)
	switch {
	case err == nil:
		return record, true, nil
	case errors.Is(err, collections.ErrNotFound):
		return aliastypes.Jurisdiction{}, false, nil
	default:
		return aliastypes.Jurisdiction{}, false, err
	}
}
