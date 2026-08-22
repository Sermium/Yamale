package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/stablecoin/keeper"
	"yamale/blockchain/x/stablecoin/types"
)

// The jurisdictional perimeter on issuer admission.
//
// Admitting the issuer of a currency used to be a chain-wide governance vote and
// nothing else, which made every national decision everybody's business. It is
// now also a monetary authority's act, bounded by that authority's border — and
// the border is the whole of what makes the second signer safe.

// applyFor files an application for a denom by a fresh applicant recorded in a
// country, and returns the applicant.
func (f *fixture) applyFor(t *testing.T, ms types.MsgServer, denom, country string) string {
	t.Helper()
	applicant := f.perimeter.NewPlacedAddr(t, country)
	_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: applicant, Denom: denom, DisplayDenom: denom[1:], Exponent: 6,
		Name: denom, Symbol: denom, Description: denom,
	})
	require.NoError(t, err)
	return applicant
}

// A monetary authority admits an issuer at home and is refused on one abroad.
func TestAMonetaryAuthorityAdmitsIssuersInsideItsOwnBordersOnly(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	// Nigeria's central bank, and two applicants: one Nigerian, one Ghanaian.
	cbn := f.perimeter.NewPlacedAddr(t, "NG")
	f.perimeter.Grant(t, cbn, aliastypes.ROLE_MONETARY_AUTHORITY, "NG")

	nigerian := f.applyFor(t, ms, "ungn", "NG")
	f.applyFor(t, ms, "ughs", "GH")

	_, err := ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: cbn, Denom: "ughs", Approve: true,
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)
	require.ErrorContains(t, err, "ROLE_MONETARY_AUTHORITY")
	require.ErrorContains(t, err, "GH")

	// Refused, and nothing was written: the application is still pending and no
	// issuer was admitted.
	app, err := f.keeper.IssuerApplication.Get(f.ctx, "ughs")
	require.NoError(t, err)
	require.Equal(t, types.StatusPending, app.Status)
	has, err := f.keeper.ApprovedIssuer.Has(f.ctx, "ughs")
	require.NoError(t, err)
	require.False(t, has)

	// The same authority, the same message shape, at home: admitted.
	_, err = ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: cbn, Denom: "ungn", Approve: true,
	})
	require.NoError(t, err)
	admitted, err := f.keeper.ApprovedIssuer.Get(f.ctx, "ungn")
	require.NoError(t, err)
	require.Equal(t, nigerian, admitted.Issuer, "the wrong applicant was admitted")
}

// Governance keeps its own power. It is the body that grants the roles, so
// requiring it to hold one would be circular.
func TestGovernanceStillAdmitsIssuersAnywhere(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	f.applyFor(t, ms, "ughs", "GH")
	_, err := ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: f.env.AuthorityString(t), Denom: "ughs", Approve: true,
	})
	require.NoError(t, err)

	// Including for an applicant nobody has placed, which is the state a chain
	// stood up before its perimeter registry is in.
	_, unplaced := f.env.Addr(t)
	_, err = ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: unplaced, Denom: "uxof", DisplayDenom: "xof", Exponent: 6,
		Name: "Franc CFA", Symbol: "XOF", Description: "x",
	})
	require.NoError(t, err)
	_, err = ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: f.env.AuthorityString(t), Denom: "uxof", Approve: true,
	})
	require.NoError(t, err)
}

// An account that is not an authority at all is told so, rather than told
// something about the applicant it was never entitled to touch.
func TestAStrangerIsRefusedAsAnInvalidSigner(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	f.applyFor(t, ms, "ungn", "NG")

	_, stranger := f.env.Addr(t)
	_, err := ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: stranger, Denom: "ungn", Approve: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	// And an authority of the wrong kind is equally not a monetary authority.
	other := f.perimeter.NewPlacedAddr(t, "NG")
	f.perimeter.Grant(t, other, aliastypes.ROLE_PAYMENTS_AUTHORITY, "NG")
	_, err = ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: other, Denom: "ungn", Approve: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}

// An applicant the chain cannot place is refused to a national authority.
//
// Named separately from the governance case above because the two answers differ
// on purpose: governance may act on an unplaced account, a national authority may
// not, and that is the difference between the body that defines the perimeters and
// a body inside one.
func TestAnUnplacedApplicantIsRefusedToANationalAuthority(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	cbn := f.perimeter.NewPlacedAddr(t, "NG")
	f.perimeter.Grant(t, cbn, aliastypes.ROLE_MONETARY_AUTHORITY, aliastypes.ChainWide)

	_, unplaced := f.env.Addr(t)
	_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: unplaced, Denom: "uxof", DisplayDenom: "xof", Exponent: 6,
		Name: "Franc CFA", Symbol: "XOF", Description: "x",
	})
	require.NoError(t, err)

	_, err = ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: cbn, Denom: "uxof", Approve: true,
	})
	require.ErrorIs(t, err, aliastypes.ErrNoJurisdiction,
		"a chain-wide grant reached an account the chain cannot place")
}

// A chain not wired to the perimeter loses the delegated path and keeps
// governance. It does not gain an open door.
func TestAnUnwiredPerimeterLeavesOnlyGovernance(t *testing.T) {
	f := initFixture(t)

	unwired := keeper.NewKeeper(f.env.StoreService, f.env.Codec, f.env.AddressCodec,
		f.env.Authority, f.env.BankKeeper, nil)
	ms := keeper.NewMsgServerImpl(unwired)

	applicant := f.perimeter.NewPlacedAddr(t, "NG")
	f.perimeter.Grant(t, applicant, aliastypes.ROLE_MONETARY_AUTHORITY, "NG")
	_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: applicant, Denom: "ungn", DisplayDenom: "ngn", Exponent: 6,
		Name: "Naira", Symbol: "NGN", Description: "x",
	})
	require.NoError(t, err)

	_, err = ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: applicant, Denom: "ungn", Approve: true,
	})
	require.ErrorIs(t, err, aliastypes.ErrNoScopeKeeper)

	_, err = ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: f.env.AuthorityString(t), Denom: "ungn", Approve: true,
	})
	require.NoError(t, err)
}
