package keeper_test

import (
	"encoding/hex"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// transfers returns every EventJewelTransferred emitted so far, in order.
func (f *fixture) transfers(t *testing.T) []types.EventJewelTransferred {
	t.Helper()
	var out []types.EventJewelTransferred
	for _, e := range f.env.Ctx.EventManager().Events() {
		if e.Type != "blockchain.jewel.v1.EventJewelTransferred" {
			continue
		}
		msg, err := sdk.ParseTypedEvent(abci.Event(e))
		require.NoError(t, err)
		out = append(out, *msg.(*types.EventJewelTransferred))
	}
	return out
}

// The loyalty app's backend indexes changes of owner from events. Each one
// must say which jewel moved, between whom, and how.
func TestEveryChangeOfOwnerIsAnnounced(t *testing.T) {
	f := setup(t)
	_, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	require.Empty(t, f.transfers(t), "registering is not a change of owner")

	buyer := f.sell(t, staff, id)
	require.Equal(t, []types.EventJewelTransferred{
		{JewelId: id, From: business, To: buyer, Kind: types.TransferAccept},
	}, f.transfers(t))

	claimKey := secp256k1.GenPrivKey()
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: buyer, JewelId: id,
		ClaimKey: hex.EncodeToString(claimKey.PubKey().Bytes())})
	require.NoError(t, err)
	_, friend := f.env.Addr(t)
	sig := f.claimSig(t, claimKey, id, friend)
	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: friend, JewelId: id,
		ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: sig})
	require.NoError(t, err)

	_, heir := f.env.Addr(t)
	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: heir, Reason: "identity checked"})
	require.NoError(t, err)
	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: heir, JewelId: id})
	require.NoError(t, err)

	require.Equal(t, []types.EventJewelTransferred{
		{JewelId: id, From: business, To: buyer, Kind: types.TransferAccept},
		{JewelId: id, From: buyer, To: friend, Kind: types.TransferClaim},
		{JewelId: id, From: friend, To: heir, Kind: types.TransferReassign},
	}, f.transfers(t))
}

// The public page shows the number of owners and the date of each change.
func TestEachChangeOfOwnerIsDated(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	j := f.jewel(t, id)
	require.Len(t, j.OwnerSince, int(j.OwnerCount), "in stock: no owner yet, no date")

	soldAt := f.env.Ctx.BlockTime().Unix()
	a := f.sell(t, staff, id)
	j = f.jewel(t, id)
	require.Equal(t, uint32(1), j.OwnerCount)
	require.Equal(t, []int64{soldAt}, j.OwnerSince)

	f.advance(48 * time.Hour)
	resoldAt := f.env.Ctx.BlockTime().Unix()
	f.sellFrom(t, a, id)
	j = f.jewel(t, id)
	require.Equal(t, uint32(2), j.OwnerCount)
	require.Equal(t, []int64{soldAt, resoldAt}, j.OwnerSince)
}

// Every handler that changes the register says so. A watcher that misses one
// kind of change has a register that silently drifts from the chain's.
func TestEveryHandlerEmitsItsEvent(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, err := f.srv.AmendJewel(f.env.Ctx, &types.MsgAmendJewel{Staff: staff, JewelId: id,
		RecordHash: recordHash, RecordUri: "u2", Note: "carat weight"})
	require.NoError(t, err)
	_, someone := f.env.Addr(t)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: someone})
	require.NoError(t, err)
	_, err = f.srv.CancelOffer(f.env.Ctx, &types.MsgCancelOffer{Owner: staff, JewelId: id})
	require.NoError(t, err)
	owner := secp256k1.GenPrivKey()
	ownerStr := f.sellTo(t, staff, id, owner)
	bid := f.cleaning(t, staff, id, 2, 0)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "n1", time.Minute))
	require.NoError(t, err)
	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: someone, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.VetoReassignment(f.env.Ctx, &types.MsgVetoReassignment{Owner: ownerStr, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: ownerStr, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.ClearStolen(f.env.Ctx, &types.MsgClearStolen{Owner: ownerStr, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: business, JewelerId: jid, Remove: []string{staff}})
	require.NoError(t, err)
	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.ReinstateJeweler(f.env.Ctx, &types.MsgReinstateJeweler{Office: f.office, JewelerId: jid})
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, e := range f.env.Ctx.EventManager().Events() {
		seen[e.Type] = true
	}
	for _, want := range []string{
		"EventJewelerAdmitted", "EventJewelerSuspended", "EventJewelerReinstated", "EventStaffChanged",
		"EventJewelRegistered", "EventJewelAmended", "EventOfferMade", "EventOfferCancelled",
		"EventJewelTransferred", "EventStolenReported", "EventStolenCleared", "EventBenefitAttached",
		"EventBenefitRedeemed", "EventReassignmentProposed", "EventReassignmentVetoed",
	} {
		require.True(t, seen["blockchain.jewel.v1."+want], want)
	}
}
