package keeper_test

import (
	"context"
	"testing"

	"cosmossdk.io/core/address"

	"yamale/blockchain/testutil/integration"
	aliasmodule "yamale/blockchain/x/alias/module"
	aliastestutil "yamale/blockchain/x/alias/testutil"
	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/paymsg/keeper"
	module "yamale/blockchain/x/paymsg/module"
	"yamale/blockchain/x/paymsg/types"
)

type fixture struct {
	ctx          context.Context
	keeper       keeper.Keeper
	addressCodec address.Codec
	env          *integration.Env

	// perimeter is a real x/alias keeper. The rule the delegated approval path
	// rests on is that module's — who may act where — and a stub of it would be
	// this test writing down the answer it wanted.
	perimeter *aliastestutil.Perimeter
}

func initFixture(t *testing.T) *fixture {
	t.Helper()

	env := integration.NewWith(t,
		[]string{types.ModuleName, aliastypes.ModuleName},
		module.AppModule{}, aliasmodule.AppModule{})
	perimeter := aliastestutil.Init(t, env)

	k := keeper.NewKeeper(
		env.StoreService,
		env.Codec,
		env.AddressCodec,
		env.Authority,
		env.BankKeeper,
	)
	// Handed over after construction, exactly as app.go does it: x/alias consults
	// this module, so the perimeter cannot arrive through the dependency graph
	// without making a cycle of it.
	k.SetScopeKeeper(perimeter.Keeper)

	// Initialize params
	if err := k.Params.Set(env.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("failed to set params: %v", err)
	}

	return &fixture{
		ctx:          env.Ctx,
		keeper:       k,
		addressCodec: env.AddressCodec,
		env:          env,
		perimeter:    perimeter,
	}
}
