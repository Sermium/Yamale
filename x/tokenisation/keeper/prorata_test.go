package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/tokenisation/types"
)

// Pro rata temporis.
//
// The claim being tested is one sentence: hold January to June, sell, and still
// earn half the year — even though the cheque is declared in December.
//
// Everything below is that sentence under a different stress. The two tests
// that matter most are the last two: that a distribution is never paid twice
// and never paid short, because an apportionment scheme that leaks is worse
// than one that does not apportion.

const day = 24 * 60 * 60

// advance moves the chain's clock, which is the only clock any accrual here
// reads.
func (f *basisFixture) advance(seconds int64) {
	f.env.Ctx = f.env.Ctx.WithBlockTime(
		f.env.Ctx.BlockTime().Add(time.Duration(seconds) * time.Second))
}

// distribute pays income in, which closes the period.
func (f *basisFixture) distribute(t *testing.T, amount int64) {
	t.Helper()
	sponsorStr, err := f.env.AddressCodec.BytesToString(f.sponsor)
	require.NoError(t, err)
	f.env.Fund(t, f.sponsor, sdk.NewCoins(sdk.NewCoin(income, math.NewInt(amount))))
	_, err = f.srv.FundVault(f.env.Ctx, &types.MsgFundVault{
		Funder: sponsorStr, AssetId: f.assetID,
		Amount: sdk.NewCoin(income, math.NewInt(amount)),
	})
	require.NoError(t, err)
}

func (f *basisFixture) owed(t *testing.T, who sdk.AccAddress) math.Int {
	t.Helper()
	v, err := f.k.Entitlement(f.env.Ctx, f.assetID, who)
	require.NoError(t, err)
	return v
}

// THE TEST THIS WHOLE MECHANISM EXISTS FOR.
//
// Alice holds the first half of the period and sells to Bob, who holds the
// second. The dividend is declared at the end. Under the old index Alice earned
// nothing and Bob took all of it; under this one they split it by the time each
// actually held.
func TestASaleHalfwayThroughSplitsTheDividend(t *testing.T) {
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 1_000) // alice holds the lot
	f.advance(180 * day)

	f.send(t, f.alice, f.bob, 1_000) // and sells it all, halfway
	f.advance(180 * day)

	f.distribute(t, 1_000_000)

	// Settle both by touching nothing: Entitlement accrues on a copy.
	alice, bob := f.owed(t, f.alice), f.owed(t, f.bob)

	require.True(t, alice.IsPositive(),
		"the seller held half the year and must be paid for it; under the old index this was zero")
	require.True(t, bob.IsPositive())

	// Half each, within the truncation a whole-number division leaves.
	diff := alice.Sub(bob).Abs()
	require.True(t, diff.LTE(math.NewInt(2)),
		"alice %s and bob %s should be within rounding of each other", alice, bob)
}

func TestHoldingThroughoutPaysTheSameAsItAlwaysDid(t *testing.T) {
	// The property that makes a bounded implementation possible: a balance that
	// did not move across a whole period earns amount * balance / supply, which
	// is exactly what the index paid before. If this drifts, every existing
	// vehicle's arithmetic changed and the migration is not safe.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 250)
	f.advance(365 * day)
	f.distribute(t, 1_000_000)

	// 250 of 1,000 tokens is a quarter of 1,000,000.
	require.Equal(t, math.NewInt(250_000), f.owed(t, f.alice))
}

func TestBuyingJustBeforeADeclarationEarnsAlmostNothing(t *testing.T) {
	// The anti-snipe property, and it is STRONGER here than under the index:
	// a buyer one day before a declaration earns one day of share-time out of a
	// year rather than the whole quarter's rent.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 500)
	f.advance(364 * day)

	f.send(t, f.sponsor, f.bob, 500) // bob arrives the day before
	f.advance(1 * day)
	f.distribute(t, 1_000_000)

	alice, bob := f.owed(t, f.alice), f.owed(t, f.bob)
	require.True(t, alice.GT(bob.MulRaw(100)),
		"a full year should earn vastly more than a day: alice %s, bob %s", alice, bob)
}

func TestSecondsAccruedInOnePeriodArePaidAtThatPeriodsRate(t *testing.T) {
	// A holder who settles long afterwards must still be paid at the rate of
	// the period they held through. The rate is only known when a period
	// closes, so this is what the rate history exists for.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 1_000)
	f.advance(100 * day)

	f.distribute(t, 1_000_000) // period 0 closes: alice held it all
	f.advance(100 * day)
	f.distribute(t, 10) // period 1 closes at a far lower rate

	// Alice never touched her balance across either, so she is owed both in
	// full — the second must not be applied to the first's seconds.
	require.Equal(t, math.NewInt(1_000_010), f.owed(t, f.alice))
}

func TestSettlingAcrossManyPeriodsIsPaidInFull(t *testing.T) {
	// The subtraction that replaces a loop. Alice holds through five
	// distributions without touching her balance; settling must pay all five,
	// and must do it without walking them.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 1_000)

	for i := 0; i < 5; i++ {
		f.advance(30 * day)
		f.distribute(t, 100_000)
	}
	require.Equal(t, math.NewInt(500_000), f.owed(t, f.alice))
}

/* ------------------------------------------------- what must never happen */

func TestADistributionIsNeverPaidTwiceNorShort(t *testing.T) {
	// The conservation property. Whatever the pattern of transfers, the sum of
	// what every holder is owed cannot exceed what was paid in — and should not
	// fall far short of it either, because the shortfall is money stranded in
	// the vault with no owner.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 600)
	f.send(t, f.sponsor, f.bob, 400)

	f.advance(90 * day)
	f.send(t, f.alice, f.bob, 200) // a move mid-period
	f.advance(90 * day)
	f.send(t, f.bob, f.alice, 100)
	f.advance(90 * day)

	f.distribute(t, 900_000)

	total := f.owed(t, f.alice).Add(f.owed(t, f.bob)).Add(f.owed(t, f.sponsor))
	require.True(t, total.LTE(math.NewInt(900_000)),
		"holders are owed %s of a %s distribution: money was conjured", total, math.NewInt(900_000))
	require.True(t, total.GTE(math.NewInt(899_990)),
		"holders are owed only %s of 900000: the remainder is stranded", total)
}

func TestTwoDistributionsInOneBlockDoNotDivideByZero(t *testing.T) {
	// A period of zero length. The rate would be amount / 0; the guard gives it
	// a zero rate and puts the whole amount in the cumulative figure, so the
	// money reaches holders by the old rule rather than being lost or halting
	// the chain.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 1_000)
	f.advance(10 * day)

	require.NotPanics(t, func() {
		f.distribute(t, 1_000)
		f.distribute(t, 1_000) // same block, no time between them
	})
	require.True(t, f.owed(t, f.alice).IsPositive())
}

func TestAPositionThatNeverAccruedDoesNotStartAtTheEpoch(t *testing.T) {
	// A last_touch of zero is 1970. Left unguarded, a position's first accrual
	// would be balance times fifty-six years, which would swamp every other
	// holder's share of the period.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 500)
	f.send(t, f.sponsor, f.bob, 500)
	f.advance(30 * day)
	f.distribute(t, 1_000_000)

	alice, bob := f.owed(t, f.alice), f.owed(t, f.bob)
	require.True(t, alice.Sub(bob).Abs().LTE(math.NewInt(2)),
		"two holders who arrived together must be paid alike: alice %s, bob %s", alice, bob)
}

func TestPendingSecondsAreReportedInTheirOwnUnits(t *testing.T) {
	// Not converted to money. The rate for the current period is unknown until
	// it closes, so a figure in dollars would be a forecast standing beside a
	// settled amount in the same units, and a reader cannot tell those apart.
	f := newBasisFixture(t, 1_000)
	f.send(t, f.sponsor, f.alice, 1_000)
	f.advance(10 * day)

	seconds, err := f.k.PendingTokenSeconds(
		f.env.Ctx, f.assetID, f.alice, math.NewInt(1_000), f.env.Ctx.BlockTime().Unix())
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000*10*day), seconds)

	require.True(t, f.owed(t, f.alice).IsZero(),
		"nothing has been declared, so nothing is owed yet")
}
