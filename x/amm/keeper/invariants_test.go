package keeper_test

import (
	"fmt"
	"math/rand"
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/amm/keeper"
	"yamale/blockchain/x/amm/types"
)

// The example-based tests above pin down what each handler does for a specific
// input. These property tests do the complementary job: they drive long random
// sequences of pool operations and assert, after every single one, the
// accounting rules that must hold no matter what order things happen in.
//
// These are the properties whose violation would mean real money is wrong:
// coins the module owes but does not hold, LP shares that do not correspond to
// a claim on any reserve, or tokens appearing from nowhere.

// assertPoolInvariants checks every accounting rule the AMM must satisfy.
func assertPoolInvariants(t *testing.T, f *fixture, tradedDenoms []string, initialSupply map[string]math.Int) {
	t.Helper()

	iter, err := f.keeper.Pool.Iterate(f.ctx, new(collections.Range[uint64]))
	require.NoError(t, err)
	defer iter.Close()

	pools, err := iter.Values()
	require.NoError(t, err)

	// Property 1: every LP share in circulation is accounted for by exactly one
	// pool's share total. A mismatch means either shares were minted without a
	// deposit, or a redemption failed to burn what it paid out for.
	expectedReserves := map[string]math.Int{}
	for _, denom := range tradedDenoms {
		expectedReserves[denom] = math.ZeroInt()
	}

	for _, pool := range pools {
		lpDenom := types.LPDenom(pool.Id)
		require.Equal(t, mustInt(t, pool.TotalShares).String(), f.env.Supply(lpDenom).String(),
			"pool %d: TotalShares does not match the LP denom's bank supply", pool.Id)

		expectedReserves[pool.DenomA] = expectedReserves[pool.DenomA].Add(mustInt(t, pool.ReserveA))
		expectedReserves[pool.DenomB] = expectedReserves[pool.DenomB].Add(mustInt(t, pool.ReserveB))
	}

	// Property 2: the module account actually holds every coin the pools claim
	// as a reserve — no more, no less. If this drifts, some depositor's exit
	// will fail because the coins are not there.
	for _, denom := range tradedDenoms {
		require.Equal(t, expectedReserves[denom].String(), f.env.ModuleBalance(types.ModuleName, denom).String(),
			"module account balance of %s does not back the pools' reserves", denom)
	}

	// Property 3: the AMM moves traded denoms around, it never creates or
	// destroys them. Only LP share denoms may change supply.
	for _, denom := range tradedDenoms {
		require.Equal(t, initialSupply[denom].String(), f.env.Supply(denom).String(),
			"total supply of %s changed; the AMM must never mint or burn a traded denom", denom)
	}
}

// poolK returns the constant-product invariant for a pool.
func poolK(t *testing.T, pool types.Pool) math.Int {
	t.Helper()
	return mustInt(t, pool.ReserveA).Mul(mustInt(t, pool.ReserveB))
}

func TestAMMAccountingPropertiesUnderRandomOperations(t *testing.T) {
	const (
		numTraders    = 4
		numOperations = 120
	)
	tradedDenoms := []string{"uyml", "uusd", "uchf"}

	// Fixed seeds keep a failure reproducible; each explores a different
	// interleaving of pool operations.
	for _, seed := range []int64{1, 7, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			r := rand.New(rand.NewSource(seed))

			f := initFixture(t)
			ms := keeper.NewMsgServerImpl(f.keeper)

			// Fund the traders generously so most operations are limited by the
			// handlers' own rules rather than by empty wallets.
			funding := sdk.NewCoins()
			for _, denom := range tradedDenoms {
				funding = funding.Add(sdk.NewCoin(denom, math.NewInt(1_000_000_000)))
			}

			traders := make([]sdk.AccAddress, numTraders)
			traderStrs := make([]string, numTraders)
			for i := range traders {
				traders[i], traderStrs[i] = f.env.NewFundedAddr(t, funding)
			}

			initialSupply := map[string]math.Int{}
			for _, denom := range tradedDenoms {
				initialSupply[denom] = f.env.Supply(denom)
			}

			// Seed one pool so join/exit/swap have something to act on from the
			// first iteration.
			_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
				Creator: traderStrs[0],
				DenomA:  tradedDenoms[0], AmountA: "10000000",
				DenomB: tradedDenoms[1], AmountB: "20000000",
				SwapFeeBps: 30,
			})
			require.NoError(t, err)
			assertPoolInvariants(t, f, tradedDenoms, initialSupply)

			for op := 0; op < numOperations; op++ {
				i := r.Intn(numTraders)
				sender := traderStrs[i]

				switch r.Intn(4) {
				case 0:
					randomCreatePool(t, f, ms, r, sender, tradedDenoms)
				case 1:
					randomJoinPool(t, f, ms, r, sender)
				case 2:
					randomExitPool(t, f, ms, r, sender, traders[i])
				case 3:
					randomSwap(t, f, ms, r, sender)
				}

				assertPoolInvariants(t, f, tradedDenoms, initialSupply)
			}
		})
	}
}

// randomCreatePool attempts a pool between two random denoms. A rejection is a
// valid outcome; what matters is that the invariants hold either way.
func randomCreatePool(t *testing.T, f *fixture, ms types.MsgServer, r *rand.Rand, sender string, denoms []string) {
	t.Helper()

	a := r.Intn(len(denoms))
	b := r.Intn(len(denoms))

	_, _ = ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator: sender,
		DenomA:  denoms[a], AmountA: randAmount(r, 1_000_000).String(),
		DenomB: denoms[b], AmountB: randAmount(r, 1_000_000).String(),
		SwapFeeBps: uint64(r.Intn(101)),
	})
}

func randomJoinPool(t *testing.T, f *fixture, ms types.MsgServer, r *rand.Rand, sender string) {
	t.Helper()

	pool, ok := randomExistingPool(t, f, r)
	if !ok {
		return
	}

	_, _ = ms.JoinPool(f.ctx, &types.MsgJoinPool{
		Sender:  sender,
		PoolId:  pool.Id,
		AmountA: randAmount(r, 100_000).String(),
		AmountB: randAmount(r, 100_000_000).String(),
	})
}

func randomExitPool(t *testing.T, f *fixture, ms types.MsgServer, r *rand.Rand, sender string, senderAddr sdk.AccAddress) {
	t.Helper()

	pool, ok := randomExistingPool(t, f, r)
	if !ok {
		return
	}

	held := f.env.Balance(senderAddr, types.LPDenom(pool.Id))
	if !held.IsPositive() {
		return
	}

	_, _ = ms.ExitPool(f.ctx, &types.MsgExitPool{
		Sender: sender,
		PoolId: pool.Id,
		Shares: randAmountUpTo(r, held).String(),
	})
}

// randomSwap trades against a random pool and, when the trade succeeds,
// additionally asserts the constant-product invariant never decreased — that
// is what guarantees the swap fee accrues to liquidity providers rather than
// leaking out of the pool.
func randomSwap(t *testing.T, f *fixture, ms types.MsgServer, r *rand.Rand, sender string) {
	t.Helper()

	pool, ok := randomExistingPool(t, f, r)
	if !ok {
		return
	}

	tokenIn, tokenOut := pool.DenomA, pool.DenomB
	if r.Intn(2) == 0 {
		tokenIn, tokenOut = pool.DenomB, pool.DenomA
	}

	kBefore := poolK(t, pool)

	_, err := ms.Swap(f.ctx, &types.MsgSwap{
		Sender: sender, PoolId: pool.Id,
		TokenInDenom: tokenIn, TokenInAmount: randAmount(r, 500_000).String(),
		TokenOutDenom: tokenOut, MinAmountOut: "0",
	})
	if err != nil {
		return
	}

	after, err := f.keeper.Pool.Get(f.ctx, pool.Id)
	require.NoError(t, err)
	require.True(t, poolK(t, after).GTE(kBefore),
		"pool %d: swap decreased the constant-product invariant from %s to %s",
		pool.Id, kBefore, poolK(t, after))
}

func randomExistingPool(t *testing.T, f *fixture, r *rand.Rand) (types.Pool, bool) {
	t.Helper()

	iter, err := f.keeper.Pool.Iterate(f.ctx, new(collections.Range[uint64]))
	require.NoError(t, err)
	defer iter.Close()

	pools, err := iter.Values()
	require.NoError(t, err)
	if len(pools) == 0 {
		return types.Pool{}, false
	}
	return pools[r.Intn(len(pools))], true
}

func randAmount(r *rand.Rand, max int64) math.Int {
	return math.NewInt(r.Int63n(max) + 1)
}

func randAmountUpTo(r *rand.Rand, max math.Int) math.Int {
	if max.LTE(math.OneInt()) {
		return math.OneInt()
	}
	return math.NewInt(r.Int63n(max.Int64()) + 1)
}
