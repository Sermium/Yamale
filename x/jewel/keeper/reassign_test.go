package keeper_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

const month = 30 * 24 * time.Hour

func TestALostKeyIsReassignedAfterAPublicWindow(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.sell(t, staff, id)
	_, newAccount := f.env.Addr(t)
	_, anyone := f.env.Addr(t)

	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id, To: newAccount})
	require.ErrorIs(t, err, types.ErrNoReason)
	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: newAccount, Reason: "identity checked at Maison Diop, 2026-11-20"})
	require.NoError(t, err)

	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: anyone, JewelId: id})
	require.ErrorIs(t, err, types.ErrWindowOpen)

	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: anyone, JewelId: id})
	require.NoError(t, err, "anyone completes it, so the office cannot hold it hostage")
	require.Equal(t, newAccount, f.owner(t, id))
	require.Equal(t, uint32(2), f.jewel(t, id).OwnerCount)
}

func TestTheOwnerVetoesAReassignment(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, claimant := f.env.Addr(t)
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: claimant, Reason: "claimant says the key is lost"})
	require.NoError(t, err)

	_, err = f.srv.VetoReassignment(f.env.Ctx, &types.MsgVetoReassignment{Owner: claimant, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner)
	_, err = f.srv.VetoReassignment(f.env.Ctx, &types.MsgVetoReassignment{Owner: owner, JewelId: id})
	require.NoError(t, err)

	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: claimant, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoReassignment)
	require.Equal(t, owner, f.owner(t, id))
}

func TestASaleDuringTheWindowVoidsTheReassignment(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, claimant := f.env.Addr(t)
	_, buyer := f.env.Addr(t)
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: claimant, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: id, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.NoError(t, err)

	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: claimant, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoReassignment, "the sale voided the proposal")

	// A voided proposal does not block a fresh one.
	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: claimant, Reason: "r2"})
	require.NoError(t, err)
}

func TestOnlyTheOfficeProposes(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: staff, JewelId: id,
		To: staff, Reason: "r"})
	require.ErrorIs(t, err, types.ErrNotOffice)
}

// Bech32 accepts an all-upper-case spelling of the same account. Comparing
// raw strings would let a proposal to the current owner through, and a veto
// signed in upper case would be refused although it is the owner's.
func TestAnUpperCaseSpellingIsTheSameAccount(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, claimant := f.env.Addr(t)

	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: strings.ToUpper(owner), Reason: "r"})
	require.ErrorIs(t, err, types.ErrSelfOffer)

	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: claimant, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.VetoReassignment(f.env.Ctx, &types.MsgVetoReassignment{Owner: strings.ToUpper(owner), JewelId: id})
	require.NoError(t, err)
}

// A proposal made while A holds the jewel must not come back to life when A
// holds it again: the sales in between void it, and its window is long spent.
func TestAProposalDoesNotReviveWhenTheJewelReturnsToItsOwner(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	a := f.sell(t, staff, id)
	_, c := f.env.Addr(t)
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: c, Reason: "r"})
	require.NoError(t, err)

	b := f.sellFrom(t, a, id)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: b, JewelId: id, To: a})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: a, JewelId: id})
	require.NoError(t, err)
	require.Equal(t, a, f.owner(t, id))

	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: c, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoReassignment)
	require.Equal(t, a, f.owner(t, id))
}

// sellFrom has the owner offer the jewel to a fresh account, which accepts it.
func (f *fixture) sellFrom(t *testing.T, owner string, id uint64) string {
	t.Helper()
	_, buyer := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: id, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.NoError(t, err)
	return buyer
}

func TestACannotProposeAStolenJewel(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, err := f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: owner, JewelId: id})
	require.NoError(t, err)
	_, c := f.env.Addr(t)

	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: c, Reason: "r"})
	require.ErrorIs(t, err, types.ErrStolen)
}

// Reporting stolen is the owner's refusal to consent: a proposal made before
// the report must not complete after it, even once the window has passed.
func TestAStolenReportStopsAPendingReassignment(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, thief := f.env.Addr(t)
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: thief, Reason: "claimant says the key is lost"})
	require.NoError(t, err)
	_, err = f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: owner, JewelId: id})
	require.NoError(t, err)

	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: thief, JewelId: id})
	require.ErrorIs(t, err, types.ErrStolen)
	require.Equal(t, owner, f.owner(t, id))
}

// The window is the least time the owner has, not the most. A veto stays
// possible after it closes, until someone completes the reassignment: an owner
// who turns up late still keeps their jewel.
func TestTheOwnerMayStillVetoAfterTheWindowUntilItIsCompleted(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, claimant := f.env.Addr(t)
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: claimant, Reason: "claimant says the key is lost"})
	require.NoError(t, err)

	f.advance(month + time.Second)
	_, err = f.srv.VetoReassignment(f.env.Ctx, &types.MsgVetoReassignment{Owner: owner, JewelId: id})
	require.NoError(t, err, "late, but nobody had completed it")
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: claimant, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoReassignment)
	require.Equal(t, owner, f.owner(t, id))

	// Once it is completed, there is nothing left to veto.
	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: claimant, Reason: "second request"})
	require.NoError(t, err)
	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: claimant, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.VetoReassignment(f.env.Ctx, &types.MsgVetoReassignment{Owner: owner, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoReassignment)
	require.Equal(t, claimant, f.owner(t, id))
}
