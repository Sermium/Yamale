package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

// The jurisdictional perimeter on participant admission.
//
// Licensing a payment service provider was a chain-wide governance vote and
// nothing else, which made every national licence everybody's business. It is now
// also a payments authority's act, bounded by that authority's border.

// applicant files an application from a fresh account recorded in a country.
func (f *fixture) applicant(t *testing.T, ms types.MsgServer, code, country string) string {
	t.Helper()
	addr := f.perimeter.NewPlacedAddr(t, country)
	_, err := ms.ApplyParticipant(f.ctx, &types.MsgApplyParticipant{
		Creator: addr, Code: code, Name: code,
	})
	require.NoError(t, err)
	return addr
}

// A payments authority admits at home and is refused abroad.
func TestAPaymentsAuthorityAdmitsParticipantsInsideItsOwnBordersOnly(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	regulator := f.perimeter.NewPlacedAddr(t, "GH")
	f.perimeter.Grant(t, regulator, aliastypes.ROLE_PAYMENTS_AUTHORITY, "GH")

	ghanaian := f.applicant(t, ms, "GHBANK", "GH")
	nigerian := f.applicant(t, ms, "NGBANK", "NG")

	_, err := ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: regulator, Participant: nigerian, Approve: true,
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)
	require.ErrorContains(t, err, "ROLE_PAYMENTS_AUTHORITY")
	require.ErrorContains(t, err, "NG")

	// Refused, and nothing written: the application is still pending and the
	// institution is not on the rail.
	app, err := f.keeper.ParticipantApplication.Get(f.ctx, nigerian)
	require.NoError(t, err)
	require.Equal(t, types.StatusPending, app.Status)
	admitted, err := f.keeper.ApprovedParticipantExists(f.ctx, nigerian)
	require.NoError(t, err)
	require.False(t, admitted)

	// The same authority, at home.
	_, err = ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: regulator, Participant: ghanaian, Approve: true,
	})
	require.NoError(t, err)
	admitted, err = f.keeper.ApprovedParticipantExists(f.ctx, ghanaian)
	require.NoError(t, err)
	require.True(t, admitted)
}

// Governance keeps its own power, an applicant nobody has placed included.
func TestGovernanceStillAdmitsParticipantsAnywhere(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, unplaced := f.env.Addr(t)
	_, err := ms.ApplyParticipant(f.ctx, &types.MsgApplyParticipant{
		Creator: unplaced, Code: "ANYBANK", Name: "Any Bank",
	})
	require.NoError(t, err)

	_, err = ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: f.env.AuthorityString(t), Participant: unplaced, Approve: true,
	})
	require.NoError(t, err)
}

// An account that is not a payments authority is told that, rather than told
// something about the applicant.
func TestAStrangerCannotAdmitAParticipant(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	applicant := f.applicant(t, ms, "GHBANK", "GH")

	_, stranger := f.env.Addr(t)
	_, err := ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: stranger, Participant: applicant, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	// An authority of the wrong kind is equally not a payments authority. Holding
	// a role is not holding this role, which is why the grant is a triple.
	monetary := f.perimeter.NewPlacedAddr(t, "GH")
	f.perimeter.Grant(t, monetary, aliastypes.ROLE_MONETARY_AUTHORITY, "GH")
	_, err = ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: monetary, Participant: applicant, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}

// A chain not wired to the perimeter loses the delegated path and keeps
// governance.
//
// This is the wiring mistake the setter makes possible, and the assertion is that
// it fails closed. A keeper built here and never handed the perimeter is exactly
// what app.go would produce if the SetScopeKeeper line were deleted.
func TestAnUnwiredPerimeterLeavesOnlyGovernance(t *testing.T) {
	f := initFixture(t)

	unwired := keeper.NewKeeper(f.env.StoreService, f.env.Codec, f.env.AddressCodec,
		f.env.Authority, f.env.BankKeeper)
	ms := keeper.NewMsgServerImpl(unwired)

	regulator := f.perimeter.NewPlacedAddr(t, "GH")
	f.perimeter.Grant(t, regulator, aliastypes.ROLE_PAYMENTS_AUTHORITY, "GH")
	applicant := f.applicant(t, ms, "GHBANK", "GH")

	_, err := ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: regulator, Participant: applicant, Approve: true,
	})
	require.ErrorIs(t, err, aliastypes.ErrNoScopeKeeper)

	_, err = ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: f.env.AuthorityString(t), Participant: applicant, Approve: true,
	})
	require.NoError(t, err)
}

// The setter reaches the handler the message server actually runs.
//
// The keeper is copied by value into the AppModule that builds the message
// server, so a plain field assigned after construction would be set on one copy
// and read on another — the perimeter would be wired and the check would still
// refuse. This is that failure, tested directly.
func TestSettingThePerimeterReachesTheRunningHandler(t *testing.T) {
	f := initFixture(t)

	unwired := keeper.NewKeeper(f.env.StoreService, f.env.Codec, f.env.AddressCodec,
		f.env.Authority, f.env.BankKeeper)
	// The message server is built from a copy taken *before* the perimeter is
	// handed over, which is the order app.go uses.
	ms := keeper.NewMsgServerImpl(unwired)
	unwired.SetScopeKeeper(f.perimeter.Keeper)

	regulator := f.perimeter.NewPlacedAddr(t, "GH")
	f.perimeter.Grant(t, regulator, aliastypes.ROLE_PAYMENTS_AUTHORITY, "GH")
	applicant := f.applicant(t, ms, "GHBANK", "GH")

	_, err := ms.ApproveParticipant(f.ctx, &types.MsgApproveParticipant{
		Authority: regulator, Participant: applicant, Approve: true,
	})
	require.NoError(t, err, "the perimeter was set but the handler did not see it")
}
