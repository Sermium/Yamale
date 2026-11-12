package keeper_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/x/group"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/keeper"
	"yamale/blockchain/x/jewel/types"
)

func TestTheOfficeAdmitsAJewelerAndOpensItsCollection(t *testing.T) {
	f := setup(t)
	id, business, staff := f.admit(t, "Maison Diop")

	j, err := f.k.Jeweler.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, business, j.Business)
	require.Equal(t, []string{staff}, j.Staff)
	require.Equal(t, types.JEWELER_ACTIVE, j.Status)
	require.True(t, f.nft.HasClass(f.env.Ctx, types.ClassID(id)))
}

func TestOnlyTheOfficeAdmits(t *testing.T) {
	f := setup(t)
	_, someone := f.env.Addr(t)
	_, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: someone, Name: "X", Business: someone})
	require.ErrorIs(t, err, types.ErrNotOffice)
}

func TestNothingIsAdmittedBeforeAnOfficeIsAppointed(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.k.Params.Set(f.env.Ctx, types.DefaultParams()))
	_, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: f.office, Name: "X", Business: f.office})
	require.ErrorIs(t, err, types.ErrNoOffice)
}

// The guard that makes the circuit setting impossible to forget.
func TestNoJewelerIsAdmittedWhileTheBareSendIsOpen(t *testing.T) {
	f := setup(t)
	f.circuit.open = true
	_, business := f.env.Addr(t)
	_, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: f.office, Name: "X", Business: business})
	require.ErrorIs(t, err, types.ErrBareSendOpen)
}

func TestABusinessAccountBelongsToOneJeweler(t *testing.T) {
	f := setup(t)
	_, business, _ := f.admit(t, "Maison Diop")
	_, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: f.office, Name: "Copie", Business: business})
	require.ErrorIs(t, err, types.ErrBusinessTaken)
}

func TestAPersonWorksForOneJewelerAtATime(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	other, otherBusiness, _ := f.admit(t, "Atelier Ba")
	_, err := f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: otherBusiness, JewelerId: other, Add: []string{staff}})
	require.ErrorIs(t, err, types.ErrStaffTaken)
	_, err = f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: otherBusiness, JewelerId: other,
		Add: []string{strings.ToUpper(staff)}})
	require.ErrorIs(t, err, types.ErrStaffTaken, "the upper-case spelling is the same person")
}

func TestOnlyTheBusinessManagesStaff(t *testing.T) {
	f := setup(t)
	id, _, staff := f.admit(t, "Maison Diop")
	_, newcomer := f.env.Addr(t)
	_, err := f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: staff, JewelerId: id, Add: []string{newcomer}})
	require.ErrorIs(t, err, types.ErrNotBusiness)
}

func TestRemovedStaffStopActing(t *testing.T) {
	f := setup(t)
	id, business, staff := f.admit(t, "Maison Diop")
	_, err := f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: business, JewelerId: id, Remove: []string{staff}})
	require.NoError(t, err)
	has, err := f.k.ByStaff.Has(f.env.Ctx, staff)
	require.NoError(t, err)
	require.False(t, has)
	j, _ := f.k.Jeweler.Get(f.env.Ctx, id)
	require.Empty(t, j.Staff)
}

func TestSuspensionNeedsAReasonAndReinstatementUndoesIt(t *testing.T) {
	f := setup(t)
	id, _, _ := f.admit(t, "Maison Diop")
	_, err := f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: id})
	require.ErrorIs(t, err, types.ErrNoReason)

	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: id, Reason: "complaint 2026-118"})
	require.NoError(t, err)
	j, _ := f.k.Jeweler.Get(f.env.Ctx, id)
	require.Equal(t, types.JEWELER_SUSPENDED, j.Status)

	_, err = f.srv.ReinstateJeweler(f.env.Ctx, &types.MsgReinstateJeweler{Office: f.office, JewelerId: id})
	require.NoError(t, err)
	j, _ = f.k.Jeweler.Get(f.env.Ctx, id)
	require.Equal(t, types.JEWELER_ACTIVE, j.Status)
}

type fakeGroups struct{ policies map[string]bool }

func (g fakeGroups) GroupPolicyInfo(_ context.Context, req *group.QueryGroupPolicyInfoRequest) (*group.QueryGroupPolicyInfoResponse, error) {
	if g.policies[req.Address] {
		return &group.QueryGroupPolicyInfoResponse{}, nil
	}
	return nil, errors.New("not found")
}

// An office that is one key is one bribe. Governance cannot appoint one.
func TestTheOfficeMustBeAGroupAccount(t *testing.T) {
	f := setup(t)
	_, policy := f.env.Addr(t)
	_, plainKey := f.env.Addr(t)
	k := keeper.NewKeeper(f.env.StoreService, f.env.Codec, f.env.AddressCodec, f.env.Authority,
		fakeGroups{policies: map[string]bool{policy: true}}, f.nft, f.circuit)
	srv := keeper.NewMsgServerImpl(k)
	gov, err := f.env.AddressCodec.BytesToString(f.env.Authority)
	require.NoError(t, err)

	p := types.DefaultParams()
	p.Office = plainKey
	_, err = srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{Authority: gov, Params: p})
	require.ErrorIs(t, err, types.ErrOfficeNotGroup)

	p.Office = policy
	_, err = srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)

	_, err = srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{Authority: policy, Params: p})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}
