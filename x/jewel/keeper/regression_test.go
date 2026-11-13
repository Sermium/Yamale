package keeper_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// An offer of stock is made by a staff member on the business's behalf. It
// must not outlive that person's role: a removed employee, or one whose
// jeweler the office has suspended, must not be able to hand stock to an
// accomplice through an offer they opened beforehand.
func TestAStockOfferDiesWithTheRoleOfWhoeverMadeIt(t *testing.T) {
	t.Run("staff removed, offer to an account", func(t *testing.T) {
		f := setup(t)
		jid, business, staff := f.admit(t, "Maison Diop")
		id := f.register(t, staff, "MD-0001")
		_, accomplice := f.env.Addr(t)
		_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: accomplice})
		require.NoError(t, err)
		_, err = f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: business, JewelerId: jid, Remove: []string{staff}})
		require.NoError(t, err)

		_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: accomplice, JewelId: id})
		require.ErrorIs(t, err, types.ErrNotStaff)
		require.Equal(t, business, f.owner(t, id), "the stock stays with the business")
	})

	t.Run("jeweler suspended, offer to a claim key", func(t *testing.T) {
		f := setup(t)
		jid, business, staff := f.admit(t, "Maison Diop")
		id := f.register(t, staff, "MD-0001")
		claimKey := secp256k1.GenPrivKey()
		_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id,
			ClaimKey: hex.EncodeToString(claimKey.PubKey().Bytes())})
		require.NoError(t, err)
		_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "fraud"})
		require.NoError(t, err)

		_, accomplice := f.env.Addr(t)
		sig := f.claimSig(t, claimKey, id, accomplice)
		_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: accomplice, JewelId: id,
			ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: sig})
		require.ErrorIs(t, err, types.ErrJewelerSuspended)
		require.Equal(t, business, f.owner(t, id))
	})

	t.Run("an offer the owner made outlives nobody's role", func(t *testing.T) {
		f := setup(t)
		jid, _, staff := f.admit(t, "Maison Diop")
		id := f.register(t, staff, "MD-0001")
		owner := f.sell(t, staff, id)
		_, buyer := f.env.Addr(t)
		_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: id, To: buyer})
		require.NoError(t, err)
		_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "r"})
		require.NoError(t, err)

		_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
		require.NoError(t, err, "a private owner's sale does not depend on the jeweler")
		require.Equal(t, buyer, f.owner(t, id))
	})
}

// An offer speaks for the owner who made it. If the jewel reaches someone
// else by any path that does not close the offer, the new owner never agreed
// to it, and it must not be accepted.
func TestAnOfferIsRefusedOnceTheJewelHasAnotherOwner(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	a := f.sell(t, staff, id)
	_, c := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: a, JewelId: id, To: c})
	require.NoError(t, err)

	// What a bare x/nft send would do if the circuit were ever reopened.
	bAddr, b := f.env.Addr(t)
	require.NoError(t, f.nft.Transfer(f.env.Ctx, types.ClassID(f.jewel(t, id).JewelerId), types.TokenID(id), bAddr))

	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: c, JewelId: id})
	require.ErrorIs(t, err, types.ErrOfferStale)
	require.Equal(t, b, f.owner(t, id))
}

// Amendments live inside the jewel's record, which the owner's every move
// rewrites. Staff must not be able to grow it without bound.
func TestAmendmentsAreBounded(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	amend := func(note string) error {
		_, err := f.srv.AmendJewel(f.env.Ctx, &types.MsgAmendJewel{Staff: staff, JewelId: id,
			RecordHash: recordHash, RecordUri: "u", Note: note})
		return err
	}

	require.ErrorIs(t, amend(strings.Repeat("x", types.MaxAmendmentNote+1)), types.ErrNoteTooLong)
	require.Empty(t, f.jewel(t, id).Amendments)
	require.NoError(t, amend(strings.Repeat("x", types.MaxAmendmentNote)))

	for i := 1; i < types.MaxAmendments; i++ {
		require.NoError(t, amend("correction"))
	}
	require.Len(t, f.jewel(t, id).Amendments, types.MaxAmendments)
	require.ErrorIs(t, amend("one too many"), types.ErrTooManyAmendments)
	require.Len(t, f.jewel(t, id).Amendments, types.MaxAmendments)
}
