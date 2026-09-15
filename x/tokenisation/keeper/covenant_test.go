package keeper_test

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/tokenisation/types"
)

// What the operator has published.
//
// The register exists so a holder can see the undertakings that govern what
// they own without the operator choosing what to tell them. So the tests that
// matter are the ones that stop the register saying whatever is convenient:
// a filing cannot be withdrawn, a correction cannot fork, and a digest that
// matches nothing cannot be filed as though it matched something.

func covDigest(of string) []byte {
	sum := sha256.Sum256([]byte(of))
	return sum[:]
}

func accounts(body string) *types.MsgPublishCovenant {
	return &types.MsgPublishCovenant{
		Kind:        types.COVENANT_KIND_FINANCIAL_STATEMENTS,
		Title:       "Annual accounts",
		Sha256:      covDigest(body),
		Uri:         "ipfs://bafy" + body,
		PeriodStart: 1_767_225_600, // 2026-01-01
		PeriodEnd:   1_798_761_600, // 2026-12-31
	}
}

func (f *basisFixture) publish(t *testing.T, msg *types.MsgPublishCovenant) (uint64, error) {
	t.Helper()
	msg.Authority = f.authority
	msg.AssetId = f.assetID
	res, err := f.srv.PublishCovenant(f.env.Ctx, msg)
	if err != nil {
		return 0, err
	}
	return res.Id, nil
}

func TestAFilingRecordsItsDigestPeriodAndPublisher(t *testing.T) {
	f := newBasisFixture(t, 1_000)

	id, err := f.publish(t, accounts("the 2026 accounts"))
	require.NoError(t, err)
	require.NotZero(t, id, "filing 0 is never issued")

	cov, err := f.k.Covenants.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, covDigest("the 2026 accounts"), cov.Sha256)
	require.Equal(t, types.COVENANT_KIND_FINANCIAL_STATEMENTS, cov.Kind)
	require.Equal(t, f.authority, cov.PublishedBy,
		"an unattributed filing proves a document existed and nothing about who stands behind it")
	require.Equal(t, int64(1_767_225_600), cov.PeriodStart)
	require.Zero(t, cov.SupersededBy)
}

func TestOnlyTheCollectionAuthorityMayPublish(t *testing.T) {
	// A holder able to publish could file an announcement in the operator's
	// name, on a register whose value is that its filings are attributable.
	f := newBasisFixture(t, 1_000)

	aliceStr, err := f.env.AddressCodec.BytesToString(f.alice)
	require.NoError(t, err)
	msg := accounts("forged")
	msg.Authority = aliceStr
	msg.AssetId = f.assetID
	_, err = f.srv.PublishCovenant(f.env.Ctx, msg)
	require.ErrorIs(t, err, types.ErrNotAuthority)
}

func TestADeadlineIsCarriedSoItCanBeShownAsOne(t *testing.T) {
	// An assembly notice is something a holder must ACT on and can miss. A
	// register that filed it like a set of accounts makes the one with a
	// deadline as easy to overlook as the one without.
	f := newBasisFixture(t, 1_000)

	id, err := f.publish(t, &types.MsgPublishCovenant{
		Kind:   types.COVENANT_KIND_GENERAL_ASSEMBLY,
		Title:  "Notice of general assembly",
		Sha256: covDigest("agenda"),
		ActsBy: 1_800_000_000,
	})
	require.NoError(t, err)

	cov, err := f.k.Covenants.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, int64(1_800_000_000), cov.ActsBy)
}

/* ----------------------------------------------------------- the refusals */

func TestATruncatedDigestIsRefused(t *testing.T) {
	// Not a weaker proof but a forgeable one.
	f := newBasisFixture(t, 1_000)
	msg := accounts("x")
	msg.Sha256 = msg.Sha256[:16]
	_, err := f.publish(t, msg)
	require.ErrorIs(t, err, types.ErrInvalidCovenant)
}

func TestThirtyTwoZeroBytesIsNotADigest(t *testing.T) {
	// The correct length, matching nothing: what an unset field looks like once
	// something has padded it.
	f := newBasisFixture(t, 1_000)
	msg := accounts("x")
	msg.Sha256 = make([]byte, 32)
	_, err := f.publish(t, msg)
	require.ErrorIs(t, err, types.ErrInvalidCovenant)
}

func TestAFilingNeedsAKindAndATitle(t *testing.T) {
	f := newBasisFixture(t, 1_000)

	noKind := accounts("a")
	noKind.Kind = types.COVENANT_KIND_UNSPECIFIED
	_, err := f.publish(t, noKind)
	require.ErrorIs(t, err, types.ErrInvalidCovenant)

	noTitle := accounts("b")
	noTitle.Title = "   "
	_, err = f.publish(t, noTitle)
	require.ErrorIs(t, err, types.ErrInvalidCovenant)
}

func TestHalfAPeriodIsRefused(t *testing.T) {
	// One end without the other describes no interval, and a holder checking
	// whether a year is missing would be comparing against half a fact.
	f := newBasisFixture(t, 1_000)

	half := accounts("c")
	half.PeriodEnd = 0
	_, err := f.publish(t, half)
	require.ErrorIs(t, err, types.ErrInvalidCovenant)

	backwards := accounts("d")
	backwards.PeriodStart, backwards.PeriodEnd = backwards.PeriodEnd, backwards.PeriodStart
	_, err = f.publish(t, backwards)
	require.ErrorIs(t, err, types.ErrInvalidCovenant)
}

/* ------------------------------------------------------------ corrections */

func TestACorrectionGoesForwardAndTheEarlierFilingStays(t *testing.T) {
	f := newBasisFixture(t, 1_000)

	first, err := f.publish(t, accounts("first draft"))
	require.NoError(t, err)

	restated := accounts("restated")
	restated.Title = "Annual accounts (restated)"
	restated.Supersedes = first
	second, err := f.publish(t, restated)
	require.NoError(t, err)

	old, err := f.k.Covenants.Get(f.env.Ctx, first)
	require.NoError(t, err)
	require.Equal(t, second, old.SupersededBy)
	require.Equal(t, covDigest("first draft"), old.Sha256,
		"the earlier filing keeps its own digest; it is the record of what was said then")

	current, err := f.k.Covenants.Get(f.env.Ctx, second)
	require.NoError(t, err)
	require.Equal(t, first, current.Supersedes)
	require.Zero(t, current.SupersededBy)
}

func TestACorrectionChainCannotFork(t *testing.T) {
	// Two corrections of one filing leave a reader with two current versions
	// and no rule for choosing between them.
	f := newBasisFixture(t, 1_000)

	first, err := f.publish(t, accounts("first"))
	require.NoError(t, err)

	second := accounts("second")
	second.Supersedes = first
	_, err = f.publish(t, second)
	require.NoError(t, err)

	third := accounts("third")
	third.Supersedes = first
	_, err = f.publish(t, third)
	require.ErrorIs(t, err, types.ErrCovenantSuperseded)
}

func TestCorrectingWithAnIdenticalDocumentIsRefused(t *testing.T) {
	// Not a correction but an accident, and recording it would put a
	// supersession in the history that nobody can explain afterwards.
	f := newBasisFixture(t, 1_000)

	first, err := f.publish(t, accounts("same"))
	require.NoError(t, err)

	same := accounts("same")
	same.Supersedes = first
	_, err = f.publish(t, same)
	require.ErrorIs(t, err, types.ErrInvalidCovenant)
}

/* --------------------------------------------------------- reading it back */

func TestFilingsComeBackOldestFirstAndCurrentOnlyHidesCorrections(t *testing.T) {
	f := newBasisFixture(t, 1_000)

	first, err := f.publish(t, accounts("2026"))
	require.NoError(t, err)

	restated := accounts("2026 restated")
	restated.Supersedes = first
	second, err := f.publish(t, restated)
	require.NoError(t, err)

	all, err := f.k.AssetCovenants(f.env.Ctx, f.assetID, false)
	require.NoError(t, err)
	require.Len(t, all, 2, "the default shows history, because history is the question")
	require.Equal(t, first, all[0].Id, "ids are issued in order, so oldest first is the order it happened in")

	current, err := f.k.AssetCovenants(f.env.Ctx, f.assetID, true)
	require.NoError(t, err)
	require.Len(t, current, 1)
	require.Equal(t, second, current[0].Id)
}

func TestATokenisationWithNoFilingsAnswersWithNoneRatherThanAnError(t *testing.T) {
	// An absence, not a failure. The distinction is the difference between "this
	// operator has published nothing" and "the chain is broken".
	f := newBasisFixture(t, 1_000)

	out, err := f.k.AssetCovenants(f.env.Ctx, f.assetID, false)
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestFilingsSurviveAnExportImportRound(t *testing.T) {
	f := newBasisFixture(t, 1_000)

	first, err := f.publish(t, accounts("one"))
	require.NoError(t, err)
	restated := accounts("two")
	restated.Supersedes = first
	_, err = f.publish(t, restated)
	require.NoError(t, err)

	gs, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.Len(t, gs.Covenants, 2)
	// Ids start at 1 because zero is never issued, so two filings leave the
	// counter at 2 — and a counter derived from the highest id present would
	// reissue one the moment a filing was removed.
	require.Equal(t, uint64(2), gs.NextCovenantId)
}
