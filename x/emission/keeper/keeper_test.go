package keeper_test

import (
	"context"
	"testing"

	"cosmossdk.io/core/address"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/emission/keeper"
	module "yamale/blockchain/x/emission/module"
	"yamale/blockchain/x/emission/types"
)

type fixture struct {
	ctx          context.Context
	keeper       keeper.Keeper
	addressCodec address.Codec
	env          *integration.Env
}

// stubStaking names the bond denomination, which is the one thing emission
// asks x/staking. Named explicitly here rather than left to sdk.DefaultBondDenom
// — the point of the change this stands in for is that the global is not the
// chain's answer.
type stubStaking struct{ denom string }

func (s stubStaking) BondDenom(context.Context) (string, error) { return s.denom, nil }

func initFixture(t *testing.T) *fixture {
	t.Helper()

	env := integration.New(t, types.ModuleName, module.AppModule{})

	k := keeper.NewKeeper(
		env.StoreService,
		env.Codec,
		env.AddressCodec,
		env.Authority,
		env.BankKeeper,
		stubStaking{denom: "uyml"},
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
