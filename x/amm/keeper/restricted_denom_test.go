package keeper_test

import (
	"context"
	"errors"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/amm/keeper"
	"yamale/blockchain/x/amm/types"
)

// K-1, at the AMM's own boundary.
//
// A pool pays both reserve legs in one SendCoins, so a denom whose transfer can
// be refused takes the whole payout — and the other leg — down with it. The
// tokenisation fraction denom is exactly that once its vehicle is realised. The
// composition test below drives the full trap end to end; this one pins the
// single line that refuses it, so a regression here fails fast and named.

// classifier is a stub RestrictedDenomKeeper. It restricts one denom and can be
// told to fail, because an error from the real classifier must refuse the pool
// rather than wave it through.
type classifier struct {
	restricted string
	boom       bool
}

var errBoom = errors.New("classifier is unavailable")

func (c classifier) IsRestrictedDenom(_ context.Context, denom string) (bool, error) {
	if c.boom {
		return false, errBoom
	}
	return denom == c.restricted, nil
}

func TestCreatePoolRefusesARestrictedDenom(t *testing.T) {
	f := initFixture(t)
	f.keeper.SetRestrictedDenomKeeper(classifier{restricted: denomB})
	ms := keeper.NewMsgServerImpl(f.keeper)

	creator, creatorStr := f.env.NewFundedAddr(t, coins(denomA, 1_000_000, denomB, 1_000_000))

	_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator: creatorStr,
		DenomA:  denomA, AmountA: "1000000",
		DenomB: denomB, AmountB: "1000000",
		SwapFeeBps: 30,
	})
	require.ErrorIs(t, err, types.ErrRestrictedDenom)

	// The restricted denom refuses whichever side it is on.
	_, err = ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator: creatorStr,
		DenomA:  denomB, AmountA: "1000000",
		DenomB: denomA, AmountB: "1000000",
		SwapFeeBps: 30,
	})
	require.ErrorIs(t, err, types.ErrRestrictedDenom)

	// Nothing moved and no pool exists.
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(creator, denomA))
	count, err := f.keeper.PoolSeq.Peek(f.ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

// A classifier that errors is a refusal, not a pass: a pool the AMM could not
// vet is one it should not have created.
func TestCreatePoolRefusesWhenTheClassifierErrors(t *testing.T) {
	f := initFixture(t)
	f.keeper.SetRestrictedDenomKeeper(classifier{boom: true})
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, creatorStr := f.env.NewFundedAddr(t, coins(denomA, 1_000_000, denomB, 1_000_000))
	_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator: creatorStr,
		DenomA:  denomA, AmountA: "1000000",
		DenomB: denomB, AmountB: "1000000",
		SwapFeeBps: 30,
	})
	require.Error(t, err)
}

// With no classifier wired, every denom is poolable — which is correct on a
// chain with no fraction denoms, and is the state every other AMM test runs in.
// The danger the wiring guards against is the reverse: a fraction denom waved
// through on a chain that HAS them, which the composition test and the app
// source-wiring test both pin.
func TestCreatePoolIsPermissionlessWithoutAClassifier(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, creatorStr := f.env.NewFundedAddr(t, coins(denomA, 1_000_000, denomB, 1_000_000))
	_, err := ms.CreatePool(f.ctx, &types.MsgCreatePool{
		Creator: creatorStr,
		DenomA:  denomA, AmountA: "1000000",
		DenomB: denomB, AmountB: "1000000",
		SwapFeeBps: 30,
	})
	require.NoError(t, err)
}
