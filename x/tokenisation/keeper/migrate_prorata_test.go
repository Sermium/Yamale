package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/tokenisation/types"
)

// The migration onto pro-rata temporis.
//
// One of these tests matters more than the rest: money that was owed before the
// upgrade must still be owed after it. Nothing in the new mechanism reads the
// index delta that produced it, so a migration that only initialised fields
// would make every unsettled entitlement on the chain quietly cease to exist.

// preProRata puts a position back into the shape it had before this upgrade:
// an unsettled index delta, and none of the new fields.
func (f *basisFixture) preProRata(t *testing.T, who string, lastIndex math.LegacyDec) {
	t.Helper()
	key := collections.Join(f.assetID, who)
	pos, err := f.k.Positions.Get(f.env.Ctx, key)
	require.NoError(t, err)

	pos.LastIndex = lastIndex
	pos.CostBasis = math.Int{}
	pos.CostQty = math.Int{}
	pos.TokenSeconds = math.Int{}
	pos.TokenSecondsPeriod = 0
	pos.LastTouch = 0
	require.NoError(t, f.k.Positions.Set(f.env.Ctx, key, pos))
}

func TestTheMigrationDoesNotLoseUnsettledIncome(t *testing.T) {
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 1_000)
	f.advance(100 * day)
	f.distribute(t, 1_000_000)

	owedBefore := f.owed(t, f.alice)
	require.True(t, owedBefore.IsPositive())

	// Rewind alice to the pre-upgrade shape: she is owed the whole
	// distribution, unsettled, as an index delta.
	f.preProRata(t, f.alice.String(), math.LegacyZeroDec())

	_, positions, settled, err := f.k.MigrateToProRata(f.env.Ctx)
	require.NoError(t, err)
	require.Positive(t, positions)
	require.True(t, settled.IsPositive(),
		"the migration settled nothing; every unsettled entitlement on the chain would have vanished")

	require.Equal(t, owedBefore, f.owed(t, f.alice),
		"alice was owed %s before the upgrade and %s after", owedBefore, f.owed(t, f.alice))
}

func TestTheMigrationLeavesNothingThatCanPanic(t *testing.T) {
	// A nil math.Int panics on arithmetic rather than erroring, so a position
	// the migration missed would stop every validator on its first transfer.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 600)
	f.send(t, f.sponsor, f.bob, 400)

	for _, who := range []string{f.alice.String(), f.bob.String(), f.sponsor.String()} {
		f.preProRata(t, who, math.LegacyZeroDec())
	}

	_, _, _, err := f.k.MigrateToProRata(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, f.k.AssertProRataReady(f.env.Ctx))

	// And a transfer afterwards must not panic.
	require.NotPanics(t, func() { f.send(t, f.alice, f.bob, 100) })
}

func TestTheGuardRefusesAPositionTheMigrationMissed(t *testing.T) {
	// Without this the migration is trusted rather than checked, and the check
	// is cheap next to a halt.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 100)
	f.preProRata(t, f.alice.String(), math.LegacyZeroDec())

	require.Error(t, f.k.AssertProRataReady(f.env.Ctx),
		"a position with unset amounts must be refused, not reported")
}

func TestEveryVaultStartsItsFirstPeriodAtTheUpgrade(t *testing.T) {
	// Not at issuance. The chain holds no record of when balances moved before
	// the upgrade, so token-seconds for the past cannot be reconstructed, and
	// assuming everyone held from issuance would credit last week's buyer with
	// a year of accrual.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 1_000)
	f.advance(200 * day)

	vaults, _, _, err := f.k.MigrateToProRata(f.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, 1, vaults)

	v, err := f.k.Vaults.Get(f.env.Ctx, f.assetID)
	require.NoError(t, err)
	require.Equal(t, f.env.Ctx.BlockTime().Unix(), v.PeriodStart)
	require.Zero(t, v.PeriodIndex)
}

func TestApportionmentWorksImmediatelyAfterTheMigration(t *testing.T) {
	// The migration is not finished if the mechanism it enables does not then
	// work: a sale halfway through the first post-upgrade period must split it.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 1_000)
	for _, who := range []string{f.alice.String(), f.sponsor.String()} {
		f.preProRata(t, who, math.LegacyZeroDec())
	}
	_, _, _, err := f.k.MigrateToProRata(f.env.Ctx)
	require.NoError(t, err)

	f.advance(180 * day)
	f.send(t, f.alice, f.bob, 1_000)
	f.advance(180 * day)
	f.distribute(t, 1_000_000)

	alice, bob := f.owed(t, f.alice), f.owed(t, f.bob)
	require.True(t, alice.IsPositive() && bob.IsPositive())
	require.True(t, alice.Sub(bob).Abs().LTE(math.NewInt(2)),
		"a mid-period sale should split it: alice %s, bob %s", alice, bob)
}

var _ = types.Position{}
