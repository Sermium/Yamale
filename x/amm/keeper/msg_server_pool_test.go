package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/amm/keeper"
	"yamale/blockchain/x/amm/types"
)

const (
	denomA = "uyml"
	denomB = "uusd"
)

func coins(pairs ...any) sdk.Coins {
	c := sdk.NewCoins()
	for i := 0; i < len(pairs); i += 2 {
		c = c.Add(sdk.NewCoin(pairs[i].(string), math.NewInt(int64(pairs[i+1].(int)))))
	}
	return c
}

// newPool creates a pool with the given reserves and fee, returning the pool
// id and the creator's address.
func newPool(t *testing.T, f *fixture, ms types.MsgServer, reserveA, reserveB int, feeBps uint64) (uint64, sdk.AccAddress, string) {
	t.Helper()

	creator, creatorStr := f.env.NewFundedAddr(t, coins(denomA, reserveA, denomB, reserveB))
	_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator:    creatorStr,
		DenomA:     denomA,
		AmountA:    math.NewInt(int64(reserveA)).String(),
		DenomB:     denomB,
		AmountB:    math.NewInt(int64(reserveB)).String(),
		SwapFeeBps: feeBps,
	})
	require.NoError(t, err)

	// The first pool created in a fixture gets id 0.
	return 0, creator, creatorStr
}

func TestCreatePool(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, creator, _ := newPool(t, f, ms, 1_000_000, 4_000_000, 30)

	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.Equal(t, denomA, pool.DenomA)
	require.Equal(t, denomB, pool.DenomB)
	require.Equal(t, "1000000", pool.ReserveA)
	require.Equal(t, "4000000", pool.ReserveB)
	require.Equal(t, uint64(30), pool.SwapFeeBps)

	// Initial shares are sqrt(1e6 * 4e6) = 2e6.
	require.Equal(t, "2000000", pool.TotalShares)

	// The deposit left the creator and is held by the module account.
	require.True(t, f.env.Balance(creator, denomA).IsZero())
	require.True(t, f.env.Balance(creator, denomB).IsZero())
	require.Equal(t, math.NewInt(1_000_000), f.env.ModuleBalance(types.ModuleName, denomA))
	require.Equal(t, math.NewInt(4_000_000), f.env.ModuleBalance(types.ModuleName, denomB))

	// The creator holds every LP share, and they are real bank coins.
	lpDenom := types.LPDenom(poolID)
	require.Equal(t, math.NewInt(2_000_000), f.env.Balance(creator, lpDenom))
	require.Equal(t, math.NewInt(2_000_000), f.env.Supply(lpDenom))
}

func TestCreatePoolRejectsBadInput(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, senderStr := f.env.NewFundedAddr(t, coins(denomA, 1_000_000, denomB, 1_000_000))

	testCases := []struct {
		name   string
		msg    *types.MsgCreatePool
		expErr error
	}{
		{
			name:   "same denom on both sides",
			msg:    &types.MsgCreatePool{Creator: senderStr, DenomA: denomA, AmountA: "100", DenomB: denomA, AmountB: "100"},
			expErr: types.ErrSameDenom,
		},
		{
			name:   "zero amount",
			msg:    &types.MsgCreatePool{Creator: senderStr, DenomA: denomA, AmountA: "0", DenomB: denomB, AmountB: "100"},
			expErr: types.ErrInvalidAmount,
		},
		{
			name:   "non-numeric amount",
			msg:    &types.MsgCreatePool{Creator: senderStr, DenomA: denomA, AmountA: "not-a-number", DenomB: denomB, AmountB: "100"},
			expErr: types.ErrInvalidAmount,
		},
		{
			name:   "negative amount",
			msg:    &types.MsgCreatePool{Creator: senderStr, DenomA: denomA, AmountA: "100", DenomB: denomB, AmountB: "-1"},
			expErr: types.ErrInvalidAmount,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.CreatePool(f.ctx, tc.msg)
			require.ErrorIs(t, err, tc.expErr)
		})
	}
}

func TestCreatePoolRejectsInvalidCreator(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator: "not-a-bech32-address",
		DenomA:  denomA, AmountA: "100",
		DenomB: denomB, AmountB: "100",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid creator address")
}

func TestCreatePoolRequiresFundedCreator(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	// Funded on one side only.
	_, senderStr := f.env.NewFundedAddr(t, coins(denomA, 1_000_000))

	_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator: senderStr,
		DenomA:  denomA, AmountA: "1000000",
		DenomB: denomB, AmountB: "1000000",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient funds")
}

func TestJoinPoolMintsProportionalShares(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000_000, 4_000_000, 30)
	lpDenom := types.LPDenom(poolID)

	// Join with 10% of reserveA. The pool ratio is 1:4, so this requires
	// 400_000 of denomB and mints 10% of the 2_000_000 existing shares.
	joiner, joinerStr := f.env.NewFundedAddr(t, coins(denomA, 100_000, denomB, 1_000_000))

	_, err := ms.JoinPool(f.ctx, &types.MsgJoinPool{
		Sender:  joinerStr,
		PoolId:  poolID,
		AmountA: "100000",
		AmountB: "1000000", // declared maximum, far above what is required
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(200_000), f.env.Balance(joiner, lpDenom))

	// Only the required 400_000 denomB was taken, not the declared maximum.
	require.True(t, f.env.Balance(joiner, denomA).IsZero())
	require.Equal(t, math.NewInt(600_000), f.env.Balance(joiner, denomB))

	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.Equal(t, "1100000", pool.ReserveA)
	require.Equal(t, "4400000", pool.ReserveB)
	require.Equal(t, "2200000", pool.TotalShares)

	// Reserves and module balances stay in lockstep.
	require.Equal(t, math.NewInt(1_100_000), f.env.ModuleBalance(types.ModuleName, denomA))
	require.Equal(t, math.NewInt(4_400_000), f.env.ModuleBalance(types.ModuleName, denomB))
}

func TestJoinPoolEnforcesDeclaredMaximum(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000_000, 4_000_000, 30)

	joiner, joinerStr := f.env.NewFundedAddr(t, coins(denomA, 100_000, denomB, 1_000_000))

	// 100_000 denomA requires 400_000 denomB, but only 399_999 is offered.
	_, err := ms.JoinPool(f.ctx, &types.MsgJoinPool{
		Sender:  joinerStr,
		PoolId:  poolID,
		AmountA: "100000",
		AmountB: "399999",
	})
	require.ErrorIs(t, err, types.ErrInsufficientDeposit)

	// Nothing was taken.
	require.Equal(t, math.NewInt(100_000), f.env.Balance(joiner, denomA))
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(joiner, denomB))
}

func TestJoinPoolRejectsDustDeposit(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	// A pool whose reserveA dwarfs its share count, so a tiny deposit rounds
	// down to zero shares and must be rejected rather than silently donated.
	poolID, _, _ := newPool(t, f, ms, 1_000_000, 1, 30)

	_, joinerStr := f.env.NewFundedAddr(t, coins(denomA, 10, denomB, 10))

	_, err := ms.JoinPool(f.ctx, &types.MsgJoinPool{
		Sender:  joinerStr,
		PoolId:  poolID,
		AmountA: "1",
		AmountB: "10",
	})
	require.ErrorIs(t, err, types.ErrInvalidAmount)
	require.Contains(t, err.Error(), "too small to mint any LP shares")
}

func TestJoinPoolUnknownPool(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, senderStr := f.env.NewFundedAddr(t, coins(denomA, 100, denomB, 100))

	_, err := ms.JoinPool(f.ctx, &types.MsgJoinPool{
		Sender: senderStr, PoolId: 99, AmountA: "10", AmountB: "10",
	})
	require.ErrorIs(t, err, types.ErrPoolNotFound)
}

func TestExitPoolReturnsProportionalReserves(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, creator, creatorStr := newPool(t, f, ms, 1_000_000, 4_000_000, 30)
	lpDenom := types.LPDenom(poolID)

	// Redeem a quarter of the 2_000_000 shares.
	_, err := ms.ExitPool(f.ctx, &types.MsgExitPool{
		Sender: creatorStr, PoolId: poolID, Shares: "500000",
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(250_000), f.env.Balance(creator, denomA))
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(creator, denomB))
	require.Equal(t, math.NewInt(1_500_000), f.env.Balance(creator, lpDenom))

	// Redeemed shares are burned, not parked in the module account.
	require.Equal(t, math.NewInt(1_500_000), f.env.Supply(lpDenom))
	require.True(t, f.env.ModuleBalance(types.ModuleName, lpDenom).IsZero())

	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.Equal(t, "750000", pool.ReserveA)
	require.Equal(t, "3000000", pool.ReserveB)
	require.Equal(t, "1500000", pool.TotalShares)
}

func TestExitPoolRejectsMoreSharesThanExist(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, creatorStr := newPool(t, f, ms, 1_000_000, 4_000_000, 30)

	_, err := ms.ExitPool(f.ctx, &types.MsgExitPool{
		Sender: creatorStr, PoolId: poolID, Shares: "2000001",
	})
	require.ErrorIs(t, err, types.ErrInsufficientShares)
}

// A holder of some shares must not be able to redeem against shares they do
// not own, even though the amount is within the pool's total.
func TestExitPoolRejectsSharesNotHeld(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000_000, 4_000_000, 30)

	_, strangerStr := f.env.Addr(t)
	_, err := ms.ExitPool(f.ctx, &types.MsgExitPool{
		Sender: strangerStr, PoolId: poolID, Shares: "1000000",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient LP shares")

	// The pool is untouched.
	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.Equal(t, "2000000", pool.TotalShares)
}

// A full round trip of join then exit must never return more than was put in.
func TestJoinThenExitDoesNotCreateValue(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000_000, 4_000_000, 30)
	lpDenom := types.LPDenom(poolID)

	joiner, joinerStr := f.env.NewFundedAddr(t, coins(denomA, 100_000, denomB, 400_000))

	_, err := ms.JoinPool(f.ctx, &types.MsgJoinPool{
		Sender: joinerStr, PoolId: poolID, AmountA: "100000", AmountB: "400000",
	})
	require.NoError(t, err)

	shares := f.env.Balance(joiner, lpDenom)
	_, err = ms.ExitPool(f.ctx, &types.MsgExitPool{
		Sender: joinerStr, PoolId: poolID, Shares: shares.String(),
	})
	require.NoError(t, err)

	require.True(t, f.env.Balance(joiner, denomA).LTE(math.NewInt(100_000)),
		"joiner recovered more denomA than deposited")
	require.True(t, f.env.Balance(joiner, denomB).LTE(math.NewInt(400_000)),
		"joiner recovered more denomB than deposited")
	require.True(t, f.env.Balance(joiner, lpDenom).IsZero())
}

func TestSwapConstantProduct(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	// A fee-free pool, so the output is exactly the constant-product result.
	poolID, _, _ := newPool(t, f, ms, 1_000_000, 1_000_000, 0)

	trader, traderStr := f.env.NewFundedAddr(t, coins(denomA, 100_000))

	// amountOut = 1e6 * 1e5 / (1e6 + 1e5) = 90909.09..., truncated to 90909.
	// The fractional remainder is left in the pool, never handed to the trader.
	_, err := ms.Swap(f.ctx, &types.MsgSwap{
		Sender:        traderStr,
		PoolId:        poolID,
		TokenInDenom:  denomA,
		TokenInAmount: "100000",
		TokenOutDenom: denomB,
		MinAmountOut:  "0",
	})
	require.NoError(t, err)

	require.True(t, f.env.Balance(trader, denomA).IsZero())
	require.Equal(t, math.NewInt(90_909), f.env.Balance(trader, denomB))

	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.Equal(t, "1100000", pool.ReserveA)
	require.Equal(t, "909091", pool.ReserveB)

	// Reserves still match what the module account actually holds.
	require.Equal(t, math.NewInt(1_100_000), f.env.ModuleBalance(types.ModuleName, denomA))
	require.Equal(t, math.NewInt(909_091), f.env.ModuleBalance(types.ModuleName, denomB))
}

// The swap fee is retained in the pool rather than paid out, so k grows and
// existing LPs are the ones who benefit.
func TestSwapFeeAccruesToLiquidityProviders(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000_000, 1_000_000, 30) // 0.30%

	before, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	kBefore := mustInt(t, before.ReserveA).Mul(mustInt(t, before.ReserveB))

	_, traderStr := f.env.NewFundedAddr(t, coins(denomA, 100_000))
	_, err = ms.Swap(f.ctx, &types.MsgSwap{
		Sender: traderStr, PoolId: poolID,
		TokenInDenom: denomA, TokenInAmount: "100000",
		TokenOutDenom: denomB, MinAmountOut: "0",
	})
	require.NoError(t, err)

	after, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	kAfter := mustInt(t, after.ReserveA).Mul(mustInt(t, after.ReserveB))

	require.True(t, kAfter.GT(kBefore), "swap fee should increase the pool invariant")
	require.Equal(t, "1100000", after.ReserveA, "the full input, fee included, stays in the pool")
	require.Equal(t, before.TotalShares, after.TotalShares, "a swap must not mint or burn LP shares")
}

// A larger fee must produce a strictly worse output for the same input.
func TestSwapFeeReducesOutput(t *testing.T) {
	out := func(feeBps uint64) math.Int {
		g := initFixture(t)
		gms := keeper.NewMsgServerImpl(g.keeper)
		poolID, _, _ := newPool(t, g, gms, 1_000_000, 1_000_000, feeBps)

		trader, traderStr := g.env.NewFundedAddr(t, coins(denomA, 100_000))
		_, err := gms.Swap(g.ctx, &types.MsgSwap{
			Sender: traderStr, PoolId: poolID,
			TokenInDenom: denomA, TokenInAmount: "100000",
			TokenOutDenom: denomB, MinAmountOut: "0",
		})
		require.NoError(t, err)
		return g.env.Balance(trader, denomB)
	}

	require.True(t, out(0).GT(out(30)))
	require.True(t, out(30).GT(out(100)))
}

func TestSwapRespectsSlippageBound(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000_000, 1_000_000, 0)

	trader, traderStr := f.env.NewFundedAddr(t, coins(denomA, 100_000))

	// The swap yields 90_909; asking for one more must fail.
	_, err := ms.Swap(f.ctx, &types.MsgSwap{
		Sender: traderStr, PoolId: poolID,
		TokenInDenom: denomA, TokenInAmount: "100000",
		TokenOutDenom: denomB, MinAmountOut: "90910",
	})
	require.ErrorIs(t, err, types.ErrSlippage)

	// The trader's funds and the pool are both untouched.
	require.Equal(t, math.NewInt(100_000), f.env.Balance(trader, denomA))
	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.Equal(t, "1000000", pool.ReserveA)

	// Asking for exactly the output succeeds.
	_, err = ms.Swap(f.ctx, &types.MsgSwap{
		Sender: traderStr, PoolId: poolID,
		TokenInDenom: denomA, TokenInAmount: "100000",
		TokenOutDenom: denomB, MinAmountOut: "90909",
	})
	require.NoError(t, err)
}

func TestSwapReverseDirection(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000_000, 1_000_000, 0)

	trader, traderStr := f.env.NewFundedAddr(t, coins(denomB, 100_000))

	_, err := ms.Swap(f.ctx, &types.MsgSwap{
		Sender: traderStr, PoolId: poolID,
		TokenInDenom: denomB, TokenInAmount: "100000",
		TokenOutDenom: denomA, MinAmountOut: "0",
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(90_909), f.env.Balance(trader, denomA))

	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.Equal(t, "909091", pool.ReserveA, "the B-for-A direction must update the right reserve")
	require.Equal(t, "1100000", pool.ReserveB)
}

func TestSwapRejectsBadInput(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000_000, 1_000_000, 0)
	_, traderStr := f.env.NewFundedAddr(t, coins(denomA, 100_000))

	testCases := []struct {
		name   string
		msg    *types.MsgSwap
		expErr error
	}{
		{
			name: "denom not in pool",
			msg: &types.MsgSwap{Sender: traderStr, PoolId: poolID,
				TokenInDenom: denomA, TokenInAmount: "100", TokenOutDenom: "uchf", MinAmountOut: "0"},
			expErr: types.ErrDenomNotInPool,
		},
		{
			name: "same denom in and out",
			msg: &types.MsgSwap{Sender: traderStr, PoolId: poolID,
				TokenInDenom: denomA, TokenInAmount: "100", TokenOutDenom: denomA, MinAmountOut: "0"},
			expErr: types.ErrDenomNotInPool,
		},
		{
			name: "zero input",
			msg: &types.MsgSwap{Sender: traderStr, PoolId: poolID,
				TokenInDenom: denomA, TokenInAmount: "0", TokenOutDenom: denomB, MinAmountOut: "0"},
			expErr: types.ErrInvalidAmount,
		},
		{
			name: "negative minimum out",
			msg: &types.MsgSwap{Sender: traderStr, PoolId: poolID,
				TokenInDenom: denomA, TokenInAmount: "100", TokenOutDenom: denomB, MinAmountOut: "-1"},
			expErr: types.ErrInvalidAmount,
		},
		{
			name: "unknown pool",
			msg: &types.MsgSwap{Sender: traderStr, PoolId: 42,
				TokenInDenom: denomA, TokenInAmount: "100", TokenOutDenom: denomB, MinAmountOut: "0"},
			expErr: types.ErrPoolNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.Swap(f.ctx, tc.msg)
			require.ErrorIs(t, err, tc.expErr)
		})
	}
}

func TestSwapRequiresFundedTrader(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000_000, 1_000_000, 0)
	_, traderStr := f.env.Addr(t)

	_, err := ms.Swap(f.ctx, &types.MsgSwap{
		Sender: traderStr, PoolId: poolID,
		TokenInDenom: denomA, TokenInAmount: "100000",
		TokenOutDenom: denomB, MinAmountOut: "0",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient funds")

	// A failed swap must leave the pool exactly as it was.
	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.Equal(t, "1000000", pool.ReserveA)
	require.Equal(t, "1000000", pool.ReserveB)
}

// However large a swap is relative to the pool, it can never take the whole
// output reserve: rounding the output down always leaves at least one unit
// behind, so the pool stays solvent and quotable.
func TestSwapCannotDrainPool(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000, 1_000, 0)

	trader, traderStr := f.env.NewFundedAddr(t, coins(denomA, 1_000_000_000))

	_, err := ms.Swap(f.ctx, &types.MsgSwap{
		Sender: traderStr, PoolId: poolID,
		TokenInDenom: denomA, TokenInAmount: "1000000000",
		TokenOutDenom: denomB, MinAmountOut: "0",
	})
	require.NoError(t, err)

	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.True(t, mustInt(t, pool.ReserveB).IsPositive(), "output reserve was drained to zero")
	require.True(t, f.env.Balance(trader, denomB).LT(math.NewInt(1_000)),
		"a trader can never extract the pool's entire output reserve")

	// Reserves still match the module account after an extreme trade.
	require.Equal(t, mustInt(t, pool.ReserveA), f.env.ModuleBalance(types.ModuleName, denomA))
	require.Equal(t, mustInt(t, pool.ReserveB), f.env.ModuleBalance(types.ModuleName, denomB))
}

// A swap must never hand out more than the constant-product curve allows.
// Computing the output as reserveOut - (reserveIn*reserveOut)/D rounds up and
// lets a trader take one unit too many; each such unit lowers x*y, so a stream
// of small swaps would bleed the pool. Rounding down keeps k monotonic.
func TestSwapNeverDecreasesInvariant(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	// A fee-free pool with awkward reserves, so truncation bites on almost
	// every trade and the fee cannot mask a rounding error.
	poolID, _, _ := newPool(t, f, ms, 999_983, 100_003, 0)

	_, traderStr := f.env.NewFundedAddr(t, coins(denomA, 10_000_000, denomB, 10_000_000))

	for i := 1; i <= 40; i++ {
		before, err := f.keeper.Pool.Get(f.ctx, poolID)
		require.NoError(t, err)
		kBefore := mustInt(t, before.ReserveA).Mul(mustInt(t, before.ReserveB))

		tokenIn, tokenOut := denomA, denomB
		if i%2 == 0 {
			tokenIn, tokenOut = denomB, denomA
		}

		_, err = ms.Swap(f.ctx, &types.MsgSwap{
			Sender: traderStr, PoolId: poolID,
			TokenInDenom: tokenIn, TokenInAmount: math.NewInt(int64(i * 7)).String(),
			TokenOutDenom: tokenOut, MinAmountOut: "0",
		})
		require.NoError(t, err)

		after, err := f.keeper.Pool.Get(f.ctx, poolID)
		require.NoError(t, err)
		kAfter := mustInt(t, after.ReserveA).Mul(mustInt(t, after.ReserveB))

		require.True(t, kAfter.GTE(kBefore),
			"swap %d dropped the invariant from %s to %s", i, kBefore, kAfter)
	}
}

// A large but non-draining swap goes through and always leaves the output
// reserve positive.
func TestSwapLargeButNonDraining(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	poolID, _, _ := newPool(t, f, ms, 1_000, 1_000, 0)

	trader, traderStr := f.env.NewFundedAddr(t, coins(denomA, 99_000))

	_, err := ms.Swap(f.ctx, &types.MsgSwap{
		Sender: traderStr, PoolId: poolID,
		TokenInDenom: denomA, TokenInAmount: "99000",
		TokenOutDenom: denomB, MinAmountOut: "0",
	})
	require.NoError(t, err)

	pool, err := f.keeper.Pool.Get(f.ctx, poolID)
	require.NoError(t, err)
	require.True(t, mustInt(t, pool.ReserveB).IsPositive(), "output reserve was drained to zero")
	require.True(t, f.env.Balance(trader, denomB).LT(math.NewInt(1_000)),
		"a trader can never extract the pool's entire output reserve")
}

func mustInt(t *testing.T, s string) math.Int {
	t.Helper()
	i, ok := math.NewIntFromString(s)
	require.True(t, ok, "not an integer: %s", s)
	return i
}
