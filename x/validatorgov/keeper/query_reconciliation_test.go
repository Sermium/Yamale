package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aliaskeeper "yamale/blockchain/x/alias/keeper"
	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/validatorgov/keeper"
	"yamale/blockchain/x/validatorgov/types"
)

// The expected values in this file are written as literals, never recomputed
// from the same expressions the query uses. A test that derived "NG" from the
// declaration it had just stored would pass against an implementation that
// compared a field with itself.

// approveDeclaring puts a validator on the approval list with a declared
// country.
//
// Written into the map rather than sent through MsgApproveValidator, because
// approval is governance-gated and how a validator reached the list is not what
// is under test here. What must be real is the other side of the comparison, and
// that comes from x/alias's own keeper through its own message server.
func approveDeclaring(t *testing.T, f *fixture, candidate, declared string) {
	t.Helper()
	require.NoError(t, f.keeper.ApprovedValidator.Set(f.env.Ctx, candidate, types.ApprovedValidator{
		Candidate: candidate,
		Approved:  f.env.AuthorityString(t),
		Declaration: types.Declaration{
			LegalEntityId:     "LEI-000",
			BeneficialOwnerId: "UBO-000",
			Jurisdiction:      declared,
		},
	}))
}

func TestJurisdictionReconciliationAgrees(t *testing.T) {
	f := initFixture(t)
	f.env.Ctx = f.env.Ctx.WithBlockHeight(77)
	qs := keeper.NewQueryServerImpl(f.keeper)

	_, addr := f.env.Addr(t)
	f.perimeter.Place(t, addr, "NG")
	approveDeclaring(t, f, addr, "NG")

	res, err := qs.JurisdictionReconciliation(f.env.Ctx, &types.QueryJurisdictionReconciliationRequest{})
	require.NoError(t, err)
	require.Len(t, res.Records, 1)

	row := res.Records[0]
	require.Equal(t, addr, row.Candidate)
	require.Equal(t, "NG", row.DeclaredJurisdiction)
	require.Equal(t, "NG", row.RecordedJurisdiction)
	require.Equal(t, f.env.AuthorityString(t), row.RecordedBy)
	require.Equal(t, int64(77), row.RecordedAtHeight)
	require.Equal(t, types.JURISDICTION_AGREEMENT_AGREE, row.Agreement)

	require.Equal(t, uint32(1), res.AgreeCount)
	require.Equal(t, uint32(0), res.DisagreeCount)
	require.Equal(t, uint32(0), res.UnrecordedCount)
}

// A validator that declared one country while the participant that onboarded it
// recorded another. Both countries come back, and so does the account that
// recorded one of them: the chain cannot say which party is wrong, so it names
// them both and leaves the finding to somebody who can check.
func TestJurisdictionReconciliationDisagrees(t *testing.T) {
	f := initFixture(t)
	f.env.Ctx = f.env.Ctx.WithBlockHeight(5)
	qs := keeper.NewQueryServerImpl(f.keeper)

	_, addr := f.env.Addr(t)
	f.perimeter.Place(t, addr, "SN")
	approveDeclaring(t, f, addr, "NG")

	res, err := qs.JurisdictionReconciliation(f.env.Ctx, &types.QueryJurisdictionReconciliationRequest{})
	require.NoError(t, err)
	require.Len(t, res.Records, 1)

	row := res.Records[0]
	require.Equal(t, types.JURISDICTION_AGREEMENT_DISAGREE, row.Agreement)
	require.Equal(t, "NG", row.DeclaredJurisdiction)
	require.Equal(t, "SN", row.RecordedJurisdiction)
	require.Equal(t, f.env.AuthorityString(t), row.RecordedBy)
	require.Equal(t, int64(5), row.RecordedAtHeight)

	require.Equal(t, uint32(0), res.AgreeCount)
	require.Equal(t, uint32(1), res.DisagreeCount)
	require.Equal(t, uint32(0), res.UnrecordedCount)
}

// No record at all is its own state. Not agreement — nobody has corroborated the
// declaration — and not an error for that row either: an approved validator that
// was never anybody's onboarded customer is an ordinary thing to find, and a
// query that failed on it would tell a supervisor nothing about the rest of the
// register.
func TestJurisdictionReconciliationUnrecorded(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)

	_, addr := f.env.Addr(t)
	approveDeclaring(t, f, addr, "NG")

	res, err := qs.JurisdictionReconciliation(f.env.Ctx, &types.QueryJurisdictionReconciliationRequest{})
	require.NoError(t, err)
	require.Len(t, res.Records, 1)

	row := res.Records[0]
	require.Equal(t, types.JURISDICTION_AGREEMENT_UNRECORDED, row.Agreement)
	require.Equal(t, "NG", row.DeclaredJurisdiction)
	require.Equal(t, "", row.RecordedJurisdiction)
	require.Equal(t, "", row.RecordedBy)
	require.Equal(t, int64(0), row.RecordedAtHeight)

	require.Equal(t, uint32(0), res.AgreeCount)
	require.Equal(t, uint32(0), res.DisagreeCount)
	require.Equal(t, uint32(1), res.UnrecordedCount)
}

// The counts are the part an operator reads, so they are checked against a
// register whose composition is known by construction rather than by
// recomputation: two agreeing, one disagreeing, three unplaced, six rows.
func TestJurisdictionReconciliationCountsAddUp(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)

	agreeing := []string{"NG", "SN"}
	for _, country := range agreeing {
		_, addr := f.env.Addr(t)
		f.perimeter.Place(t, addr, country)
		approveDeclaring(t, f, addr, country)
	}

	_, mismatched := f.env.Addr(t)
	f.perimeter.Place(t, mismatched, "GH")
	approveDeclaring(t, f, mismatched, "KE")

	for i := 0; i < 3; i++ {
		_, addr := f.env.Addr(t)
		approveDeclaring(t, f, addr, "ZA")
	}

	res, err := qs.JurisdictionReconciliation(f.env.Ctx, &types.QueryJurisdictionReconciliationRequest{})
	require.NoError(t, err)

	require.Len(t, res.Records, 6)
	require.Equal(t, uint32(2), res.AgreeCount)
	require.Equal(t, uint32(1), res.DisagreeCount)
	require.Equal(t, uint32(3), res.UnrecordedCount)
	require.Equal(t, len(res.Records),
		int(res.AgreeCount+res.DisagreeCount+res.UnrecordedCount))

	// No row carries the reserved zero value. It is the unset default and never
	// a finding, so a row that arrived holding it would be a row this module
	// failed to fill in.
	for _, row := range res.Records {
		require.NotEqual(t, types.JURISDICTION_AGREEMENT_UNSPECIFIED, row.Agreement)
	}
}

// Without the registry the query refuses to answer. This is the case the whole
// design of the query turns on: unwired, every validator would come back
// UNRECORDED — a plausible six-row answer that describes the wiring rather than
// the chain — and a supervisor reading it would conclude nothing is corroborated
// when in fact nothing was compared.
func TestJurisdictionReconciliationWithoutRegistryFailsClosed(t *testing.T) {
	f := initFixture(t)
	f.env.Ctx = f.env.Ctx.WithBlockHeight(9)

	_, addr := f.env.Addr(t)
	f.perimeter.Place(t, addr, "NG")
	approveDeclaring(t, f, addr, "NG")

	unwired := keeper.NewKeeper(
		f.env.StoreService,
		f.env.Codec,
		f.env.AddressCodec,
		f.env.Authority,
		f.staking,
		f.authz,
		f.env.AuthKeeper,
		f.env.BankKeeper,
		f.constitution,
		nil,
	)

	res, err := keeper.NewQueryServerImpl(unwired).
		JurisdictionReconciliation(f.env.Ctx, &types.QueryJurisdictionReconciliationRequest{})
	require.Error(t, err)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Nil(t, res)

	// And the same state, read through the wired keeper, does have an answer —
	// so the refusal above is the missing registry and not an empty register.
	wired, err := keeper.NewQueryServerImpl(f.keeper).
		JurisdictionReconciliation(f.env.Ctx, &types.QueryJurisdictionReconciliationRequest{})
	require.NoError(t, err)
	require.Len(t, wired.Records, 1)
	require.Equal(t, types.JURISDICTION_AGREEMENT_AGREE, wired.Records[0].Agreement)
}

// The foundation's reserved code, decided deliberately.
//
// x/alias answers "which country does this account belong to" for a named
// foundation administrator with the reserved code, and that is correct for the
// question the perimeter checks ask. This query asks a different question — what
// has been recorded, and by whom — and the reserved code is not a record: it is
// derived from a parameter list, with no recorder behind it and no height dating
// it. So a foundation administrator nobody has placed reads UNRECORDED here, and
// the recorded fields stay empty rather than being filled with a provenance that
// does not exist.
//
// The other half of the decision is that the reserved code can never appear as a
// recorded country at all, which is asserted below: x/alias refuses to record it.
func TestJurisdictionReconciliationFoundationCodeIsNotARecord(t *testing.T) {
	f := initFixture(t)
	f.env.Ctx = f.env.Ctx.WithBlockHeight(88)
	qs := keeper.NewQueryServerImpl(f.keeper)

	// The account is a foundation administrator, which is what makes x/alias
	// answer ZZ for it. That used to be an entry in x/alias's parameters and is a
	// chain-wide role grant now; GrantForUpgrade is the write with no signer,
	// which is what a fixture wants.
	_, addr := f.env.Addr(t)
	require.NoError(t, f.perimeter.Keeper.GrantForUpgrade(f.env.Ctx, aliastypes.RoleGrant{
		Holder:       addr,
		Role:         aliastypes.ROLE_FOUNDATION_ADMINISTRATOR,
		Jurisdiction: aliastypes.ChainWide,
	}))
	approveDeclaring(t, f, addr, "NG")

	// x/alias's own reading of the same account.
	country, err := f.perimeter.Keeper.CountryOf(f.env.Ctx, addr)
	require.NoError(t, err)
	require.Equal(t, "ZZ", country)

	// This query's reading of it.
	res, err := qs.JurisdictionReconciliation(f.env.Ctx, &types.QueryJurisdictionReconciliationRequest{})
	require.NoError(t, err)
	require.Len(t, res.Records, 1)

	row := res.Records[0]
	require.Equal(t, types.JURISDICTION_AGREEMENT_UNRECORDED, row.Agreement)
	require.Equal(t, "NG", row.DeclaredJurisdiction)
	require.Equal(t, "", row.RecordedJurisdiction)
	require.Equal(t, "", row.RecordedBy)
	require.Equal(t, int64(0), row.RecordedAtHeight)
	require.Equal(t, uint32(1), res.UnrecordedCount)

	// And recording the reserved code is refused at the source, so no row can
	// ever report it as the recorded side.
	_, err = aliaskeeper.NewMsgServerImpl(f.perimeter.Keeper).SetJurisdiction(f.env.Ctx,
		&aliastypes.MsgSetJurisdiction{
			Recorder: f.env.AuthorityString(t),
			Account:  addr,
			Country:  "ZZ",
		})
	require.Error(t, err)
}

func TestJurisdictionReconciliationNilRequest(t *testing.T) {
	f := initFixture(t)
	qs := keeper.NewQueryServerImpl(f.keeper)

	_, err := qs.JurisdictionReconciliation(f.env.Ctx, nil)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}
