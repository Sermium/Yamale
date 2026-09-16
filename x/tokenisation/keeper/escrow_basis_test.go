package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/tokenisation/types"
)

// Cost, and the accounts that hold shares on somebody else's behalf.
//
// A marketplace escrow is not a holder. Shares arrive in it from many sellers
// and leave it to many buyers, so the proportional carry that is right between
// two people is meaningless across one — and the round trip through it is where
// a holder silently loses the record of what they paid.
//
// These are the tests for the two keeper entry points a marketplace uses
// instead, and for the rule that keeps them correct.

// escrowName stands in for a marketplace's module account.
const escrowName = "testescrow"

func escrowAddr() sdk.AccAddress { return authtypes.NewModuleAddress(escrowName) }

// A holder who offers shares and then withdraws the offer must be left exactly
// as they were, even when somebody else's shares are in the escrow too.
//
// The second holder is the whole test. With one seller an escrow that carried
// cost would round-trip correctly by accident — what goes in comes back — so a
// test with a single holder passes whether the rule exists or not.
//
// With two it does not. The escrow's position is a POOL: alice's shares cost
// 0.60 each and bob's cost 0.20, and a proportional carry out of the pool pays
// alice the blended 0.40. She withdraws an offer nobody took and comes back with
// two thirds of the cost she had, permanently, having lost a third of it into
// bob's slice. Nothing fails, no balance is wrong, and the only visible effect
// is that her reported return has risen — which is the most flattering direction
// the error could go, and therefore the one nobody reports.
func TestCostStandsStillAcrossAnEscrowSharedWithAnotherHolder(t *testing.T) {
	f := newBasisFixture(t, 1_000_000)
	f.k.RegisterEscrow(escrowAddr())

	// Two holders of the same vehicle who paid very different prices.
	f.send(t, f.sponsor, f.alice, 100_000)
	require.NoError(t, f.subscribe(t, f.alice, 100_000, 60_000)) // 0.60 each
	f.send(t, f.sponsor, f.bob, 100_000)
	require.NoError(t, f.subscribe(t, f.bob, 100_000, 20_000)) // 0.20 each

	before := f.position(t, f.alice)

	// Both offer everything; only alice withdraws.
	f.send(t, f.alice, escrowAddr(), 100_000)
	f.send(t, f.bob, escrowAddr(), 100_000)
	f.send(t, escrowAddr(), f.alice, 100_000)

	after := f.position(t, f.alice)
	require.Equal(t, math.NewInt(60_000), after.CostBasis,
		"alice must get her own cost back, not the pool's blended 40,000")
	require.Equal(t, before.CostBasis, after.CostBasis)
	require.Equal(t, before.CostQty, after.CostQty)

	// And bob, still in the escrow, keeps his.
	require.Equal(t, math.NewInt(20_000), f.position(t, f.bob).CostBasis)
}

// A fill writes the price the buyer actually paid, additively.
//
// This is the second of the two events allowed to write a cost, and the only
// one that needs no trusted party: the price is in the trade rather than in
// somebody's assertion about it.
func TestAPurchaseRecordsThePricePaidAndAddsToWhatWasThereBefore(t *testing.T) {
	f := newBasisFixture(t, 1_000_000)

	f.send(t, f.sponsor, f.bob, 200_000)
	require.NoError(t, f.subscribe(t, f.bob, 100_000, 50_000))

	require.NoError(t, f.k.RecordPurchase(
		f.env.Ctx, f.assetID, f.bob, math.NewInt(100_000), math.NewInt(90_000)))

	pos := f.position(t, f.bob)
	require.Equal(t, math.NewInt(140_000), pos.CostBasis)
	require.Equal(t, math.NewInt(200_000), pos.CostQty)
}

// Releasing cost is proportional and rounds toward the seller.
//
// Rounding the other way would let a sequence of small sales release more cost
// than was ever recorded, and cost that can be manufactured is a reported
// return that can be manufactured with it.
func TestReleasingCostIsProportionalAndKeepsTheFractionWithTheSeller(t *testing.T) {
	f := newBasisFixture(t, 1_000_000)

	f.send(t, f.sponsor, f.alice, 300_000)
	require.NoError(t, f.subscribe(t, f.alice, 300_000, 100_000))

	// A third of the shares. 100,000 / 3 does not divide, and the remainder
	// stays with the seller.
	require.NoError(t, f.k.ReleaseBasis(f.env.Ctx, f.assetID, f.alice, math.NewInt(100_000)))

	pos := f.position(t, f.alice)
	require.Equal(t, math.NewInt(200_000), pos.CostQty)
	require.Equal(t, math.NewInt(66_667), pos.CostBasis,
		"33,333 released of 100,000, so 66,667 remains: the fraction stays with the seller")
}

// Releasing more than the basis covers releases only the covered part.
//
// A seller may hold shares partly subscribed at a recorded price and partly
// received as a plain transfer from somebody with no basis at all. That is the
// ordinary case, not an edge one, and the uncovered part is what the interface
// reports as not covered rather than as free.
func TestReleasingMoreThanTheBasisCoversReleasesOnlyTheCoveredPart(t *testing.T) {
	f := newBasisFixture(t, 1_000_000)

	f.send(t, f.sponsor, f.alice, 300_000)
	require.NoError(t, f.subscribe(t, f.alice, 100_000, 50_000))

	require.NoError(t, f.k.ReleaseBasis(f.env.Ctx, f.assetID, f.alice, math.NewInt(300_000)))

	pos := f.position(t, f.alice)
	require.True(t, pos.CostQty.IsZero())
	require.True(t, pos.CostBasis.IsZero())
}

// A halted vehicle's shares must still be recoverable from an escrow.
//
// Once a vehicle is realised the register refuses every transfer of its shares,
// which is right for a trade and a confiscation for an escrow: a holder with an
// offer standing when the vehicle was sold owns those shares exactly as much as
// one who never offered them. Without the exemption the shares sit in a module
// account with no key, permanently, because a realised vehicle never becomes
// tradable again.
func TestARealisedVehiclesSharesCanStillLeaveAnEscrow(t *testing.T) {
	f := newBasisFixture(t, 1_000_000)
	f.k.RegisterEscrow(escrowAddr())

	f.send(t, f.sponsor, f.alice, 100_000)
	f.send(t, f.alice, escrowAddr(), 40_000)

	asset, err := f.k.Assets.Get(f.env.Ctx, f.assetID)
	require.NoError(t, err)
	asset.Status = types.STATUS_REALISED
	require.NoError(t, f.k.Assets.Set(f.env.Ctx, f.assetID, asset))

	// An ordinary transfer between two holders is refused, as it must be.
	err = f.env.BankKeeper.SendCoins(f.env.Ctx, f.alice, f.bob,
		sdk.NewCoins(sdk.NewCoin(f.denom, math.NewInt(1))))
	require.ErrorIs(t, err, types.ErrTradingHalted)

	// Returning escrowed property to its owner is not.
	require.NoError(t, f.env.BankKeeper.SendCoins(f.env.Ctx, escrowAddr(), f.alice,
		sdk.NewCoins(sdk.NewCoin(f.denom, math.NewInt(40_000)))))
	require.Equal(t, math.NewInt(100_000),
		f.env.BankKeeper.GetBalance(f.env.Ctx, f.alice, f.denom).Amount)
}
