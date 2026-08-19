package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/amm/keeper"
	"yamale/blockchain/x/amm/types"
)

// Findings from the pre-genesis review of this module.

// The swap fee is a uint64 set by whoever opens the pool and was never bounded.
// Above 10,000 basis points the fee arithmetic goes negative, and what stops
// the result being exploitable is a guard the code itself documents as
// unreachable. Depending on an unreachable branch for safety is not a margin
// worth keeping — and a pool with a nonsense fee is a footgun regardless of
// whether it can be drained.
func TestSwapFeeIsBounded(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	creator, creatorStr := f.env.NewFundedAddr(t, sdk.NewCoins(
		sdk.NewCoin(denomA, math.NewInt(10_000_000)),
		sdk.NewCoin(denomB, math.NewInt(10_000_000)),
	))
	_ = creator

	for _, fee := range []uint64{10_001, 20_000, 1 << 63} {
		_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
			Creator: creatorStr,
			DenomA:  denomA, AmountA: "1000000",
			DenomB: denomB, AmountB: "1000000",
			SwapFeeBps: fee,
		})
		require.Error(t, err, "a %d bps fee must be refused at creation", fee)
	}

	// A realistic fee still works.
	_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator: creatorStr,
		DenomA:  denomA, AmountA: "1000000",
		DenomB: denomB, AmountB: "1000000",
		SwapFeeBps: 30,
	})
	require.NoError(t, err)
}

// Denoms reach sdk.NewCoin, which panics rather than erroring on an invalid
// one. The panic is recovered into a failed transaction, so this is robustness
// rather than a halt — but a handler should refuse its own bad input.
func TestPoolDenomsAreValidated(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	_, creatorStr := f.env.NewFundedAddr(t, sdk.NewCoins(sdk.NewCoin(denomA, math.NewInt(10_000_000))))

	for _, bad := range []string{"", "1nvalid", "a"} {
		require.NotPanics(t, func() {
			_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
				Creator: creatorStr,
				DenomA:  denomA, AmountA: "1000",
				DenomB: bad, AmountB: "1000",
				SwapFeeBps: 30,
			})
			require.Error(t, err, "denom %q must be refused", bad)
		}, "denom %q must not panic", bad)
	}
}

// Every division in a pool has a beneficiary, and it must be the pool. The
// second side of a deposit was rounded down, so a depositor paid slightly less
// than their proportional share while the shares they received were rounded
// down in the pool's favour — the two do not cancel, and the shortfall comes
// out of the existing liquidity providers.
func TestJoiningRoundsAgainstTheDepositor(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, creatorStr := f.env.NewFundedAddr(t, sdk.NewCoins(
		sdk.NewCoin(denomA, math.NewInt(1_000_000_000)),
		sdk.NewCoin(denomB, math.NewInt(1_000_000_000)),
	))

	// Reserves whose ratio does not divide evenly, so the second side of a
	// deposit lands between two integers.
	_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator: creatorStr,
		DenomA:  denomA, AmountA: "1000003",
		DenomB: denomB, AmountB: "700001",
		SwapFeeBps: 30,
	})
	require.NoError(t, err)

	joiner, joinerStr := f.env.NewFundedAddr(t, sdk.NewCoins(
		sdk.NewCoin(denomA, math.NewInt(1_000_000)),
		sdk.NewCoin(denomB, math.NewInt(1_000_000)),
	))

	before := f.env.Balance(joiner, denomB)
	_, err = ms.JoinPool(f.ctx, &types.MsgJoinPool{
		Sender: joinerStr, PoolId: 0, AmountA: "9997", AmountB: "1000000",
	})
	require.NoError(t, err)
	paidB := before.Sub(f.env.Balance(joiner, denomB))

	pool, err := f.keeper.Pool.Get(f.ctx, 0)
	require.NoError(t, err)
	reserveB, _ := math.NewIntFromString(pool.ReserveB)
	reserveA, _ := math.NewIntFromString(pool.ReserveA)

	// The deposit must never leave the pool holding less of B, per unit of A,
	// than it held before: that ratio is what the existing providers own.
	//
	//   reserveB_after * reserveA_before >= reserveB_before * reserveA_after
	beforeA, beforeB := math.NewInt(1_000_003), math.NewInt(700_001)
	require.True(t,
		reserveB.Mul(beforeA).GTE(beforeB.Mul(reserveA)),
		"the deposit diluted the pool's B-per-A ratio: paid %s of B", paidB)
}
