package keeper_test

import (
	"context"
	"testing"

	"cosmossdk.io/core/address"

	"yamale/blockchain/testutil/integration"
	constitutionkeeper "yamale/blockchain/x/constitution/keeper"
	constitutiontestutil "yamale/blockchain/x/constitution/testutil"
	constitutiontypes "yamale/blockchain/x/constitution/types"
	"yamale/blockchain/x/validatorgov/keeper"
	module "yamale/blockchain/x/validatorgov/module"
	vgtestutil "yamale/blockchain/x/validatorgov/testutil"
	"yamale/blockchain/x/validatorgov/types"
)

type fixture struct {
	ctx          context.Context
	keeper       keeper.Keeper
	addressCodec address.Codec
	env          *integration.Env
	staking      *vgtestutil.StakingKeeper
	authz        *vgtestutil.AuthzKeeper

	// constitution is real, not a stub. The ceilings the epoch check enforces
	// are this module reading another module's settlement, and a test against a
	// hand-written struct would only prove it can read one it wrote itself.
	constitution constitutionkeeper.Keeper
	invariants   constitutiontypes.Invariants
}

func initFixture(t *testing.T) *fixture {
	t.Helper()
	return newFixture(t, true)
}

// newFixture builds the module's world. Pass false for initGenesis to get a
// keeper that has never been through genesis — the state a scaffolded test, or
// a module added to a running chain by an upgrade that forgot its migration,
// actually starts in.
func newFixture(t *testing.T, initGenesis bool) *fixture {
	t.Helper()

	env := integration.NewWith(t, []string{types.ModuleName, constitutiontypes.ModuleName}, module.AppModule{})

	staking := vgtestutil.NewStakingKeeper()
	authzKeeper := vgtestutil.NewAuthzKeeper()

	_, destination := env.Addr(t)
	invariants := constitutiontestutil.Invariants(destination)
	constitution := constitutiontestutil.Init(t, env, staking, invariants)

	k := keeper.NewKeeper(
		env.StoreService,
		env.Codec,
		env.AddressCodec,
		env.Authority,
		staking,
		authzKeeper,
		env.AuthKeeper,
		env.BankKeeper,
		constitution,
	)

	// Initialised the way a chain does, rather than by setting params alone:
	// the rotation sequence is seeded here, and a fixture that skipped it would
	// be testing a keeper no chain ever runs.
	if initGenesis {
		if err := k.InitGenesis(env.Ctx, *types.DefaultGenesis()); err != nil {
			t.Fatalf("failed to initialize genesis: %v", err)
		}
	} else if err := k.Params.Set(env.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("failed to set params: %v", err)
	}

	return &fixture{
		ctx:          env.Ctx,
		keeper:       k,
		addressCodec: env.AddressCodec,
		env:          env,
		staking:      staking,
		authz:        authzKeeper,
		constitution: constitution,
		invariants:   invariants,
	}
}
