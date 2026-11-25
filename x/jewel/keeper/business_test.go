package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// moveBusiness proposes and accepts a move of the shop's business account.
func (f *fixture) moveBusiness(t *testing.T, jid uint64, from, to string) uint64 {
	t.Helper()
	_, err := f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: from, JewelerId: jid, NewBusiness: to})
	require.NoError(t, err)
	res, err := f.srv.AcceptBusinessMove(f.env.Ctx, &types.MsgAcceptBusinessMove{NewBusiness: to, JewelerId: jid})
	require.NoError(t, err)
	return res.Jewels
}

// The case that made this message: a person whose own account is the shop's
// business account hands the shop to an account of its own, stays staff, and
// can then own one of the shop's pieces like any customer.
func TestTheShopMovesToItsOwnAccountAndThePersonBecomesACustomer(t *testing.T) {
	f := setup(t)
	jid, person, staff := f.admit(t, "Maison Diop")
	_, err := f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: person, JewelerId: jid, Add: []string{person}})
	require.NoError(t, err)

	claimed := f.register(t, staff, "MD-0001")
	offered := f.register(t, staff, "MD-0002")
	kept := f.register(t, staff, "MD-0003")
	card := f.issue(t, staff, claimed)
	_, buyer := f.env.Addr(t)
	f.claimFiled(t, card, claimed, buyer)
	_, someone := f.env.Addr(t)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: offered, To: someone})
	require.NoError(t, err)

	_, shop := f.env.Addr(t)
	require.Equal(t, uint64(3), f.moveBusiness(t, jid, person, shop))

	for _, id := range []uint64{claimed, offered, kept} {
		require.Equal(t, shop, f.owner(t, id), "the stock moved with the business account")
		require.Equal(t, uint32(0), f.jewel(t, id).OwnerCount, "moving the shop is not a sale")
	}
	j, err := f.k.Jeweler.Get(f.env.Ctx, jid)
	require.NoError(t, err)
	require.Equal(t, shop, j.Business)
	require.Empty(t, j.PendingBusiness)
	require.Zero(t, j.PendingBusinessUntil)
	has, err := f.k.ByBusiness.Has(f.env.Ctx, person)
	require.NoError(t, err)
	require.False(t, has)
	got, err := f.k.ByBusiness.Get(f.env.Ctx, shop)
	require.NoError(t, err)
	require.Equal(t, jid, got)
	got, err = f.k.ByStaff.Get(f.env.Ctx, person)
	require.NoError(t, err)
	require.Equal(t, jid, got, "the person stays staff")

	// The open offer was made by the old account and went with it; the card
	// and the claim belong to the piece and stay.
	has, err = f.k.Offer.Has(f.env.Ctx, offered)
	require.NoError(t, err)
	require.False(t, has)
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: person, JewelId: claimed})
	require.NoError(t, err)
	require.Equal(t, buyer, f.owner(t, claimed))

	// And the person can now buy from their own shop.
	mine := f.issue(t, staff, kept)
	f.claimFiled(t, mine, kept, person)
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: kept})
	require.NoError(t, err)
	require.Equal(t, person, f.owner(t, kept))

	require.True(t, f.eventTypes()["blockchain.jewel.v1.EventBusinessMoved"])
}

func TestAMoveNeedsTheBusinessToProposeAndTheNamedAccountToAccept(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	_, x := f.env.Addr(t)
	_, y := f.env.Addr(t)

	_, err := f.srv.AcceptBusinessMove(f.env.Ctx, &types.MsgAcceptBusinessMove{NewBusiness: x, JewelerId: jid})
	require.ErrorIs(t, err, types.ErrNoBusinessMove, "nothing proposed")

	_, err = f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: staff, JewelerId: jid, NewBusiness: x})
	require.ErrorIs(t, err, types.ErrNotBusiness, "staff cannot give the shop away")
	_, err = f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: business, JewelerId: jid, NewBusiness: business})
	require.ErrorIs(t, err, types.ErrMoveToSelf)

	_, err = f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: business, JewelerId: jid, NewBusiness: x})
	require.NoError(t, err)
	_, err = f.srv.AcceptBusinessMove(f.env.Ctx, &types.MsgAcceptBusinessMove{NewBusiness: y, JewelerId: jid})
	require.ErrorIs(t, err, types.ErrNoBusinessMove, "only the named account accepts")

	// Withdrawn: the named account can no longer take it.
	_, err = f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: business, JewelerId: jid})
	require.NoError(t, err)
	_, err = f.srv.AcceptBusinessMove(f.env.Ctx, &types.MsgAcceptBusinessMove{NewBusiness: x, JewelerId: jid})
	require.ErrorIs(t, err, types.ErrNoBusinessMove)

	// Lapsed after offer_ttl.
	_, err = f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: business, JewelerId: jid, NewBusiness: x})
	require.NoError(t, err)
	f.advance(time.Duration(types.DefaultParams().OfferTtl+1) * time.Second)
	_, err = f.srv.AcceptBusinessMove(f.env.Ctx, &types.MsgAcceptBusinessMove{NewBusiness: x, JewelerId: jid})
	require.ErrorIs(t, err, types.ErrBusinessMoveLapsed)

	// Its own staff may take the shop over.
	require.Equal(t, uint64(0), f.moveBusiness(t, jid, business, staff))
}

func TestAMoveIsRefusedToAnAccountTiedToAnotherShopOrHoldingThisOnesPieces(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	_, otherBusiness, otherStaff := f.admit(t, "Atelier Sow")

	for _, taken := range []string{otherBusiness, otherStaff} {
		_, err := f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: business, JewelerId: jid, NewBusiness: taken})
		require.ErrorIs(t, err, types.ErrAccountTied)
	}

	// Named while free, then hired elsewhere before accepting.
	_, late := f.env.Addr(t)
	_, err := f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: business, JewelerId: jid, NewBusiness: late})
	require.NoError(t, err)
	_, err = f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: otherBusiness, JewelerId: 2, Add: []string{late}})
	require.NoError(t, err)
	_, err = f.srv.AcceptBusinessMove(f.env.Ctx, &types.MsgAcceptBusinessMove{NewBusiness: late, JewelerId: jid})
	require.ErrorIs(t, err, types.ErrAccountTied)

	// A customer of this shop would see their own piece turn into stock.
	id := f.register(t, staff, "MD-0001")
	card := f.issue(t, staff, id)
	_, customer := f.env.Addr(t)
	f.claimFiled(t, card, id, customer)
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: business, JewelerId: jid, NewBusiness: customer})
	require.NoError(t, err)
	_, err = f.srv.AcceptBusinessMove(f.env.Ctx, &types.MsgAcceptBusinessMove{NewBusiness: customer, JewelerId: jid})
	require.ErrorIs(t, err, types.ErrBusinessOwnsPieces)
	require.Equal(t, customer, f.owner(t, id))
}

func TestASuspendedShopCannotMoveItsBusinessAccount(t *testing.T) {
	f := setup(t)
	jid, business, _ := f.admit(t, "Maison Diop")
	_, x := f.env.Addr(t)
	_, err := f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: business, JewelerId: jid, NewBusiness: x})
	require.NoError(t, err)
	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "inquiry"})
	require.NoError(t, err)
	_, err = f.srv.AcceptBusinessMove(f.env.Ctx, &types.MsgAcceptBusinessMove{NewBusiness: x, JewelerId: jid})
	require.ErrorIs(t, err, types.ErrJewelerSuspended)
	_, err = f.srv.ProposeBusinessMove(f.env.Ctx, &types.MsgProposeBusinessMove{Business: business, JewelerId: jid, NewBusiness: x})
	require.ErrorIs(t, err, types.ErrJewelerSuspended)
}
