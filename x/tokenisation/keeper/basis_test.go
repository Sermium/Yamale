package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/tokenisation/keeper"
	"yamale/blockchain/x/tokenisation/types"
)

// What a holding cost.
//
// The tests that matter here are not the arithmetic -- they are the three ways
// a cost basis can lie:
//
//   - reporting a cost of zero for tokens whose price nobody recorded, which
//     computes an infinite return;
//   - letting a basis cover more tokens than the account holds, which reports a
//     return on tokens nobody has;
//   - manufacturing cost by moving tokens back and forth, where each transfer
//     rounds in the mover's favour.
//
// Each has a test below that names it.

// basisFixture is the shared setup: a vehicle, its authority, and two holders.
type basisFixture struct {
	env       *integration.Env
	k         keeper.Keeper
	srv       types.MsgServer
	assetID   uint64
	denom     string
	authority string
	sponsor   sdk.AccAddress
	alice     sdk.AccAddress
	bob       sdk.AccAddress
}

func newBasisFixture(t *testing.T, supply int64) *basisFixture {
	t.Helper()
	env, k, ms := setup(t)

	// The restriction is what settles both sides of a transfer and carries the
	// cost with the tokens, so a fixture without it tests the function and not
	// the path. This module's own setup() does not register it -- which is why
	// every basis assertion below failed the first time they were run, and
	// worth knowing about the tests that were already here.
	env.BankKeeper.AppendSendRestriction(k.SendRestrictionFn)

	id, minter, denom := vehicle(t, env, ms, supply)

	toBytes := func(s string) sdk.AccAddress {
		b, err := env.AddressCodec.StringToBytes(s)
		require.NoError(t, err)
		return b
	}
	_, aliceStr := env.Addr(t)
	_, bobStr := env.Addr(t)

	return &basisFixture{
		env: env, k: k, srv: ms, assetID: id, denom: denom,
		authority: minter,
		sponsor:   toBytes(minter),
		alice:     toBytes(aliceStr),
		bob:       toBytes(bobStr),
	}
}

// send moves tokens through the bank, which is what runs the send restriction
// and therefore the basis carry. Calling CarryBasis directly would test the
// function and not the path.
func (f *basisFixture) send(t *testing.T, from, to sdk.AccAddress, amount int64) {
	t.Helper()
	require.NoError(t, f.env.BankKeeper.SendCoins(f.env.Ctx, from, to,
		sdk.NewCoins(sdk.NewCoin(f.denom, math.NewInt(amount)))))
}

func (f *basisFixture) subscribe(t *testing.T, who sdk.AccAddress, qty, paid int64) error {
	t.Helper()
	whoStr, err := f.env.AddressCodec.BytesToString(who)
	require.NoError(t, err)
	_, err = f.srv.RecordSubscription(f.env.Ctx, &types.MsgRecordSubscription{
		Authority: f.authority, AssetId: f.assetID, Investor: whoStr,
		Quantity: math.NewInt(qty), Paid: math.NewInt(paid),
	})
	return err
}

func (f *basisFixture) position(t *testing.T, who sdk.AccAddress) types.Position {
	t.Helper()
	pos, err := f.k.Positions.Get(f.env.Ctx, collections.Join(f.assetID, who.String()))
	require.NoError(t, err)
	return pos
}

func TestASubscriptionRecordsWhatWasPaid(t *testing.T) {
	f := newBasisFixture(t, 1_000_000)
	f.send(t, f.sponsor, f.alice, 100_000)

	require.NoError(t, f.subscribe(t, f.alice, 100_000, 60_000)) // $0.60 a token

	pos := f.position(t, f.alice)
	require.Equal(t, math.NewInt(60_000), pos.CostBasis)
	require.Equal(t, math.NewInt(100_000), pos.CostQty)
}

func TestOnlyTheCollectionAuthorityMayRecordACost(t *testing.T) {
	// An investor able to state their own cost could publish a flattering
	// return beside a real balance, on a register whose whole value is that its
	// figures are not the holder's word.
	f := newBasisFixture(t, 1_000_000)
	f.send(t, f.sponsor, f.alice, 100_000)

	aliceStr, err := f.env.AddressCodec.BytesToString(f.alice)
	require.NoError(t, err)
	_, err = f.srv.RecordSubscription(f.env.Ctx, &types.MsgRecordSubscription{
		Authority: aliceStr, AssetId: f.assetID, Investor: aliceStr,
		Quantity: math.NewInt(100_000), Paid: math.NewInt(1),
	})
	require.ErrorIs(t, err, types.ErrNotAuthority)
}

func TestACostCannotCoverMoreTokensThanAreHeld(t *testing.T) {
	// A basis covering tokens nobody holds reports a return on them, and the
	// error is invisible until somebody sells.
	f := newBasisFixture(t, 1_000_000)
	f.send(t, f.sponsor, f.alice, 100_000)

	require.ErrorIs(t, f.subscribe(t, f.alice, 100_001, 60_000), types.ErrInvalidSubscription)
}

func TestTwoSubscriptionsAverage(t *testing.T) {
	// Somebody who subscribed twice at two prices carries a weighted average,
	// which is what an average cost is.
	f := newBasisFixture(t, 1_000_000)
	f.send(t, f.sponsor, f.alice, 200_000)

	require.NoError(t, f.subscribe(t, f.alice, 100_000, 60_000))
	require.NoError(t, f.subscribe(t, f.alice, 100_000, 80_000))

	pos := f.position(t, f.alice)
	require.Equal(t, math.NewInt(140_000), pos.CostBasis)
	require.Equal(t, math.NewInt(200_000), pos.CostQty)
}

/* -------------------------------------------------------- carrying on move */

func TestCostMovesWithTheTokens(t *testing.T) {
	f := newBasisFixture(t, 1_000_000)
	f.send(t, f.sponsor, f.alice, 100_000)
	require.NoError(t, f.subscribe(t, f.alice, 100_000, 60_000))

	// Alice gives a quarter of her holding away.
	f.send(t, f.alice, f.bob, 25_000)

	alice := f.position(t, f.alice)
	require.Equal(t, math.NewInt(45_000), alice.CostBasis, "three quarters of the cost stays")
	require.Equal(t, math.NewInt(75_000), alice.CostQty)

	bob := f.position(t, f.bob)
	require.Equal(t, math.NewInt(15_000), bob.CostBasis, "a quarter of the cost moves")
	require.Equal(t, math.NewInt(25_000), bob.CostQty)
}

func TestTokensFromSomebodyWithNoRecordedCostCarryNone(t *testing.T) {
	// The recipient must read as COST NOT RECORDED rather than cost zero. Cost
	// zero reports an infinite return, which is the most flattering and most
	// wrong figure this module could produce -- and a zero QUANTITY is what
	// distinguishes the two, because a zero basis over a positive quantity is a
	// claim that somebody paid nothing.
	f := newBasisFixture(t, 1_000_000)
	f.send(t, f.sponsor, f.alice, 100_000)

	pos := f.position(t, f.alice)
	require.True(t, pos.CostBasis.IsZero())
	require.True(t, pos.CostQty.IsZero())
}

func TestMovingMoreThanTheCostCoversCarriesOnlyTheCoveredPart(t *testing.T) {
	// Part subscribed, part received untracked, all sent on together.
	f := newBasisFixture(t, 1_000_000)
	f.send(t, f.sponsor, f.alice, 100_000)
	require.NoError(t, f.subscribe(t, f.alice, 40_000, 24_000))

	f.send(t, f.alice, f.bob, 100_000)

	alice := f.position(t, f.alice)
	require.True(t, alice.CostBasis.IsZero())
	require.True(t, alice.CostQty.IsZero())

	bob := f.position(t, f.bob)
	require.Equal(t, math.NewInt(24_000), bob.CostBasis)
	require.Equal(t, math.NewInt(40_000), bob.CostQty, "only the covered part carries")
}

func TestTransfersCannotManufactureCost(t *testing.T) {
	// The rounding direction is the whole of this test. Carried cost is
	// truncated, so the fraction stays with the sender and the total basis
	// across both sides never grows. Rounding the other way would let a loop of
	// one-token transfers inflate a return without anybody paying anything.
	f := newBasisFixture(t, 1_000_000)
	f.send(t, f.sponsor, f.alice, 999)
	require.NoError(t, f.subscribe(t, f.alice, 999, 1_000))

	before := math.NewInt(1_000)
	for i := 0; i < 12; i++ {
		f.send(t, f.alice, f.bob, 1)
		f.send(t, f.bob, f.alice, 1)

		total := f.position(t, f.alice).CostBasis.Add(f.position(t, f.bob).CostBasis)
		require.True(t, total.LTE(before),
			"round %d: total basis grew from %s to %s", i, before, total)
		before = total
	}
}
