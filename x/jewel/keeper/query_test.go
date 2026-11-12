package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

func TestTheVerificationPageGetsEverythingInOneQuery(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, err := f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: id, Kind: "cleaning", Allowance: 2})
	require.NoError(t, err)

	res, err := f.qs.Jewel(f.env.Ctx, &types.QueryJewelRequest{Id: id})
	require.NoError(t, err)
	require.Equal(t, "MD0001", res.Jewel.Inscription)
	require.Equal(t, owner, res.Owner)
	require.Nil(t, res.Offer)
	require.Nil(t, res.Reassignment)
	require.Len(t, res.Benefits, 1)

	_, err = f.qs.Jewel(f.env.Ctx, &types.QueryJewelRequest{Id: 999})
	require.ErrorIs(t, err, types.ErrNoJewel)
}

func TestASaleVoidsTheReassignmentShownOnTheVerificationPage(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, c := f.env.Addr(t)
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: c, Reason: "r"})
	require.NoError(t, err)
	res, err := f.qs.Jewel(f.env.Ctx, &types.QueryJewelRequest{Id: id})
	require.NoError(t, err)
	require.NotNil(t, res.Reassignment)

	f.sellFrom(t, owner, id)
	res, err = f.qs.Jewel(f.env.Ctx, &types.QueryJewelRequest{Id: id})
	require.NoError(t, err)
	require.Nil(t, res.Reassignment)
}

func TestASearchByInscriptionFindsEveryJewelerUsingIt(t *testing.T) {
	f := setup(t)
	_, _, a := f.admit(t, "Maison Diop")
	_, _, b := f.admit(t, "Atelier Ba")
	f.register(t, a, "RING-0001")
	f.register(t, b, "ring 0001")
	f.register(t, b, "RING-0002")

	res, err := f.qs.JewelsByInscription(f.env.Ctx, &types.QueryJewelsByInscriptionRequest{Inscription: "ring-0001"})
	require.NoError(t, err)
	require.Len(t, res.Jewels, 2)

	_, err = f.qs.JewelsByInscription(f.env.Ctx, &types.QueryJewelsByInscriptionRequest{Inscription: "#"})
	require.ErrorIs(t, err, types.ErrInvalidInscription)
}

func TestJewelersAreListedSuspendedOnesIncluded(t *testing.T) {
	f := setup(t)
	id, _, _ := f.admit(t, "Maison Diop")
	f.admit(t, "Atelier Ba")
	_, err := f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: id, Reason: "r"})
	require.NoError(t, err)

	res, err := f.qs.Jewelers(f.env.Ctx, &types.QueryJewelersRequest{})
	require.NoError(t, err)
	require.Len(t, res.Jewelers, 2)

	one, err := f.qs.Jeweler(f.env.Ctx, &types.QueryJewelerRequest{Id: id})
	require.NoError(t, err)
	require.Equal(t, types.JEWELER_SUSPENDED, one.Jeweler.Status)
}
