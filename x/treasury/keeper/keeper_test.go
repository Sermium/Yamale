package keeper_test

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/treasury/keeper"
	module "yamale/blockchain/x/treasury/module"
	"yamale/blockchain/x/treasury/types"
)

const denom = "uyml"

type fixture struct {
	ctx          context.Context
	keeper       keeper.Keeper
	addressCodec address.Codec
	env          *integration.Env
	ms           types.MsgServer
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

	// Through InitGenesis rather than Params.Set alone: the id sequences are
	// seeded here and nowhere else, so a fixture that skips it hands out lock
	// and treasury id zero — a state no running chain is ever in, and one that
	// hides an off-by-one in the handlers under it.
	if err := k.InitGenesis(env.Ctx, *types.DefaultGenesis()); err != nil {
		t.Fatalf("failed to initialise genesis: %v", err)
	}

	// A definite block time, since every lock decision is a function of it.
	env.Ctx = env.Ctx.WithBlockTime(time.Unix(1000, 0))

	return &fixture{
		ctx:          env.Ctx,
		keeper:       k,
		addressCodec: env.AddressCodec,
		env:          env,
		ms:           keeper.NewMsgServerImpl(k),
	}
}

// at moves the chain's clock, which is what drives vesting.
func (f *fixture) at(unix int64) {
	f.env.Ctx = f.env.Ctx.WithBlockTime(time.Unix(unix, 0))
	f.ctx = f.env.Ctx
}

func coins(amount int64) sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin(denom, math.NewInt(amount)))
}

// newTreasury creates a treasury funded with the given amount, returning its id
// and its admin.
func (f *fixture) newTreasury(t *testing.T, funding int64) (uint64, sdk.AccAddress, string) {
	t.Helper()

	admin, adminStr := f.env.NewFundedAddr(t, coins(funding))

	resp, err := f.ms.CreateTreasury(f.ctx, &types.MsgCreateTreasury{
		Creator: adminStr,
		Name:    "Test Treasury",
	})
	require.NoError(t, err)

	if funding > 0 {
		_, err = f.ms.Deposit(f.ctx, &types.MsgDeposit{
			Depositor:  adminStr,
			TreasuryId: resp.Id,
			Amount:     coins(funding),
		})
		require.NoError(t, err)
	}

	return resp.Id, admin, adminStr
}

// available reads a treasury's spendable balance.
func (f *fixture) available(t *testing.T, id uint64) math.Int {
	t.Helper()
	v, err := f.keeper.AvailableBalance(f.ctx, id, denom)
	require.NoError(t, err)
	return v
}

// locked reads a treasury's committed balance.
func (f *fixture) locked(t *testing.T, id uint64) math.Int {
	t.Helper()
	v, err := f.keeper.LockedBalance(f.ctx, id, denom)
	require.NoError(t, err)
	return v
}
