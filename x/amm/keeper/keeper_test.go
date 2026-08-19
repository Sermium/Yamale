package keeper_test

import (
	"context"
	"testing"

	"cosmossdk.io/core/address"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/amm/keeper"
	module "yamale/blockchain/x/amm/module"
	"yamale/blockchain/x/amm/types"
)

type fixture struct {
	ctx          context.Context
	keeper       keeper.Keeper
	addressCodec address.Codec
	env          *integration.Env
}

func initFixture(t *testing.T) *fixture {
	t.Helper()

	env := integration.New(t, types.ModuleName, module.AppModule{})

	k := keeper.NewKeeper(
		env.StoreService,
		env.Codec,
		env.AddressCodec,
		env.Authority,
		env.BankKeeper,
	)

	// Initialize params
	if err := k.Params.Set(env.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("failed to set params: %v", err)
	}

	return &fixture{
		ctx:          env.Ctx,
		keeper:       k,
		addressCodec: env.AddressCodec,
		env:          env,
	}
}
