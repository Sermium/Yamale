package keeper_test

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// sell offers a jewel from stock to a fresh account, which accepts it.
func (f *fixture) sell(t *testing.T, staff string, id uint64) string {
	t.Helper()
	_, buyer := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.NoError(t, err)
	return buyer
}

func TestStaffSellStockAndTheBuyerBecomesTheFirstOwner(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	buyer := f.sell(t, staff, id)

	require.Equal(t, buyer, f.owner(t, id))
	require.Equal(t, uint32(1), f.jewel(t, id).OwnerCount)
	has, _ := f.k.Offer.Has(f.env.Ctx, id)
	require.False(t, has, "accepting closes the offer")
}

func TestAResaleNeedsTheOwnerNotTheJeweler(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.sell(t, staff, id)
	_, someone := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: someone})
	require.ErrorIs(t, err, types.ErrNotOwner, "once sold, the jeweler's staff are strangers to it")
}

// Bech32 accepts an all-upper-case spelling of the same account. It must not
// be a second account: not a way to offer a jewel to its owner, and not a
// recipient who cannot accept.
func TestAnUpperCaseSpellingOfTheOwnerIsStillASelfOffer(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)

	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: id, To: strings.ToUpper(owner)})
	require.ErrorIs(t, err, types.ErrSelfOffer)
	require.Equal(t, uint32(1), f.jewel(t, id).OwnerCount, "no fake owner in the provenance")
}

func TestAnUpperCaseRecipientIsTheSameRecipient(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, buyer := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: strings.ToUpper(buyer)})
	require.NoError(t, err)

	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.NoError(t, err)
	require.Equal(t, buyer, f.owner(t, id))
	require.Equal(t, uint32(1), f.jewel(t, id).OwnerCount)
}

func TestAnOfferIsForTheNamedRecipientAndLapses(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, buyer := f.env.Addr(t)
	_, thief := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: buyer})
	require.NoError(t, err)

	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: thief, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotRecipient)

	f.advance(14*24*time.Hour + time.Second)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.ErrorIs(t, err, types.ErrOfferExpired)
}

func TestOneOpenOfferAtATimeButALapsedOneIsReplaced(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, a := f.env.Addr(t)
	_, b := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: a})
	require.NoError(t, err)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: b})
	require.ErrorIs(t, err, types.ErrOfferOpen)

	f.advance(15 * 24 * time.Hour)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: b})
	require.NoError(t, err)
}

func TestAnOfferNamesAnAccountOrAClaimKeyNotBoth(t *testing.T) {
	f := setup(t)
	_, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, a := f.env.Addr(t)
	key := hex.EncodeToString(secp256k1.GenPrivKey().PubKey().Bytes())

	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id})
	require.ErrorIs(t, err, types.ErrOfferTarget)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: a, ClaimKey: key})
	require.ErrorIs(t, err, types.ErrOfferTarget)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, ClaimKey: "abcd"})
	require.ErrorIs(t, err, types.ErrOfferTarget)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: business})
	require.ErrorIs(t, err, types.ErrSelfOffer)
}

func TestTheSellerCancelsBeforeAcceptance(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, buyer := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.CancelOffer(f.env.Ctx, &types.MsgCancelOffer{Owner: staff, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoOffer)
}

// The email path. The link carries a one-time key; the claimer signs their
// own address with it. A claim copied out of the mempool names someone else
// and is refused.
func TestAClaimKeyHandsTheJewelToWhoeverSignsWithIt(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	claimKey := secp256k1.GenPrivKey()
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id,
		ClaimKey: hex.EncodeToString(claimKey.PubKey().Bytes())})
	require.NoError(t, err)

	_, customer := f.env.Addr(t)
	_, thief := f.env.Addr(t)
	sig := f.claimSig(t, claimKey, id, customer)

	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: thief, JewelId: id,
		ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: sig})
	require.ErrorIs(t, err, types.ErrBadClaim, "a copied signature names the customer, not the thief")

	wrongKey := secp256k1.GenPrivKey()
	wrongSig := f.claimSig(t, wrongKey, id, thief)
	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: thief, JewelId: id,
		ClaimPubKey: wrongKey.PubKey().Bytes(), ClaimSignature: wrongSig})
	require.ErrorIs(t, err, types.ErrBadClaim)

	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: customer, JewelId: id,
		ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: sig})
	require.NoError(t, err)
	require.Equal(t, customer, f.owner(t, id))
}

// A seller who reuses a claim key for a later offer must not revive the
// earlier one's signature: the signed bytes name the offer by its expiry.
func TestASignatureForAnEarlierOfferDoesNotClaimALaterOne(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	claimKey := secp256k1.GenPrivKey()
	offer := &types.MsgOfferJewel{Owner: staff, JewelId: id, ClaimKey: hex.EncodeToString(claimKey.PubKey().Bytes())}
	_, err := f.srv.OfferJewel(f.env.Ctx, offer)
	require.NoError(t, err)
	_, customer := f.env.Addr(t)
	stale := f.claimSig(t, claimKey, id, customer)

	_, err = f.srv.CancelOffer(f.env.Ctx, &types.MsgCancelOffer{Owner: staff, JewelId: id})
	require.NoError(t, err)
	f.advance(time.Second)
	_, err = f.srv.OfferJewel(f.env.Ctx, offer)
	require.NoError(t, err)

	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: customer, JewelId: id,
		ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: stale})
	require.ErrorIs(t, err, types.ErrBadClaim, "the signature names the first offer, which is gone")

	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: customer, JewelId: id,
		ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: f.claimSig(t, claimKey, id, customer)})
	require.NoError(t, err)
	require.Equal(t, customer, f.owner(t, id))
}

func TestAStolenJewelCannotBeOffered(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, buyer := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: id, To: buyer})
	require.NoError(t, err)

	_, err = f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: owner, JewelId: id})
	require.NoError(t, err)
	require.True(t, f.jewel(t, id).Stolen)
	has, _ := f.k.Offer.Has(f.env.Ctx, id)
	require.False(t, has, "reporting a theft closes the open offer")

	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: id, To: buyer})
	require.ErrorIs(t, err, types.ErrStolen)

	_, err = f.srv.ClearStolen(f.env.Ctx, &types.MsgClearStolen{Owner: owner, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.ClearStolen(f.env.Ctx, &types.MsgClearStolen{Owner: owner, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotStolen)
}

func TestOnlyTheOwnerReportsATheft(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.sell(t, staff, id)
	_, stranger := f.env.Addr(t)
	_, err := f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: stranger, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner)
}
