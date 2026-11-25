package keeper_test

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// issue prints a fresh card for a stock jewel and returns its secret, as the
// Atelier does when it prints a card.
func (f *fixture) issue(t *testing.T, staff string, id uint64) *secp256k1.PrivKey {
	t.Helper()
	card := secp256k1.GenPrivKey()
	_, err := f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: staff, JewelId: id,
		CardKey: hex.EncodeToString(card.PubKey().Bytes())})
	require.NoError(t, err)
	return card
}

// claimMsg is what the buyer's phone files after scanning the card: the
// card's secret signs the claimer's address and the card's own key.
func (f *fixture) claimMsg(t *testing.T, card *secp256k1.PrivKey, id uint64, claimer string) *types.MsgFileClaim {
	t.Helper()
	pub := card.PubKey().Bytes()
	sig, err := card.Sign(types.CardClaimSignBytes(f.env.Ctx.ChainID(), id, hex.EncodeToString(pub), claimer))
	require.NoError(t, err)
	return &types.MsgFileClaim{Claimer: claimer, JewelId: id, CardPubKey: pub, CardSignature: sig}
}

// claimFiled files a claim and expects it to succeed.
func (f *fixture) claimFiled(t *testing.T, card *secp256k1.PrivKey, id uint64, claimer string) {
	t.Helper()
	_, err := f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, card, id, claimer))
	require.NoError(t, err)
}

// requireSpent checks that the jewel has no card, no claim and no entry in
// its jeweler's claim list: what every change of owner must leave behind.
func (f *fixture) requireSpent(t *testing.T, jewelerID, id uint64) {
	t.Helper()
	has, err := f.k.Card.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.False(t, has, "the card is spent")
	has, err = f.k.Claim.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.False(t, has, "the claim is gone")
	has, err = f.k.ClaimsByJeweler.Has(f.env.Ctx, collections.Join(jewelerID, id))
	require.NoError(t, err)
	require.False(t, has, "the claim left the jeweler's list")
}

func (f *fixture) eventTypes() map[string]bool {
	seen := map[string]bool{}
	for _, e := range f.env.Ctx.EventManager().Events() {
		seen[e.Type] = true
	}
	return seen
}

func TestACardClaimApprovedByTheShopMovesTheJewel(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	card := f.issue(t, staff, id)
	_, buyer := f.env.Addr(t)

	f.claimFiled(t, card, id, buyer)
	require.Equal(t, business, f.owner(t, id), "a claim moves nothing by itself")

	// The verification page and the Atelier both see it.
	res, err := f.qs.Jewel(f.env.Ctx, &types.QueryJewelRequest{Id: id})
	require.NoError(t, err)
	require.True(t, res.HasCard)
	require.NotNil(t, res.Claim)
	require.Equal(t, buyer, res.Claim.Claimer)
	require.Equal(t, hex.EncodeToString(card.PubKey().Bytes()), res.Claim.CardKey)
	require.Equal(t, f.env.Ctx.BlockTime().Unix(), res.Claim.FiledAt)
	list, err := f.qs.ClaimsByJeweler(f.env.Ctx, &types.QueryClaimsByJewelerRequest{JewelerId: jid})
	require.NoError(t, err)
	require.Len(t, list.Claims, 1)
	require.Equal(t, id, list.Claims[0].Jewel.Id)
	require.Equal(t, buyer, list.Claims[0].Claim.Claimer)

	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: id})
	require.NoError(t, err)
	require.Equal(t, buyer, f.owner(t, id))
	require.Equal(t, uint32(1), f.jewel(t, id).OwnerCount)
	f.requireSpent(t, jid, id)
	require.Equal(t, []types.EventJewelTransferred{
		{JewelId: id, From: business, To: buyer, Kind: types.TransferCard},
	}, f.transfers(t))
	seen := f.eventTypes()
	for _, want := range []string{"EventCardIssued", "EventClaimFiled", "EventClaimApproved"} {
		require.True(t, seen["blockchain.jewel.v1."+want], want)
	}

	res, err = f.qs.Jewel(f.env.Ctx, &types.QueryJewelRequest{Id: id})
	require.NoError(t, err)
	require.False(t, res.HasCard)
	require.Nil(t, res.Claim)
	list, err = f.qs.ClaimsByJeweler(f.env.Ctx, &types.QueryClaimsByJewelerRequest{JewelerId: jid})
	require.NoError(t, err)
	require.Empty(t, list.Claims)

	// A card moves a jewel once: filing again with it finds no card.
	_, other := f.env.Addr(t)
	_, err = f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, card, id, other))
	require.ErrorIs(t, err, types.ErrNoCard)
}

func TestTheBusinessAccountItselfIssuesAndApproves(t *testing.T) {
	f := setup(t)
	_, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	card := f.issue(t, business, id)
	_, buyer := f.env.Addr(t)
	f.claimFiled(t, card, id, buyer)
	_, err := f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: business, JewelId: id})
	require.NoError(t, err)
	require.Equal(t, buyer, f.owner(t, id))
}

func TestIssuingACardIsRefusedOutsideTheShopsStock(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	key := hex.EncodeToString(secp256k1.GenPrivKey().PubKey().Bytes())

	// A stranger.
	id := f.register(t, staff, "MD-0001")
	_, stranger := f.env.Addr(t)
	_, err := f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: stranger, JewelId: id, CardKey: key})
	require.ErrorIs(t, err, types.ErrNotOwner)

	// Another jeweler's staff.
	_, _, otherStaff := f.admit(t, "Atelier Ba")
	_, err = f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: otherStaff, JewelId: id, CardKey: key})
	require.ErrorIs(t, err, types.ErrNotOwner)

	// A jewel that left the shop: its owner is no longer the shop, and staff
	// no longer act for it.
	sold := f.register(t, staff, "MD-0002")
	buyer := f.sell(t, staff, sold)
	_, err = f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: buyer, JewelId: sold, CardKey: key})
	require.ErrorIs(t, err, types.ErrNotStock)
	_, err = f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: staff, JewelId: sold, CardKey: key})
	require.ErrorIs(t, err, types.ErrNotOwner)

	// A jewel reported stolen.
	stolen := f.register(t, staff, "MD-0003")
	_, err = f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: staff, JewelId: stolen})
	require.NoError(t, err)
	_, err = f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: staff, JewelId: stolen, CardKey: key})
	require.ErrorIs(t, err, types.ErrStolen)

	// A key that is not a compressed secp256k1 public key.
	for name, bad := range map[string]string{
		"empty":         "",
		"not hex":       strings.Repeat("zz", 33),
		"too short":     key[:64],
		"uncompressed":  "04" + strings.Repeat("11", 64),
		"off the curve": "02" + strings.Repeat("ff", 32),
		"bad prefix":    "05" + key[2:],
	} {
		_, err = f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: staff, JewelId: id, CardKey: bad})
		require.ErrorIs(t, err, types.ErrBadCardKey, name)
	}

	// A suspended jeweler hands over nothing: its staff stop acting, and its
	// business account is refused as suspended.
	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: staff, JewelId: id, CardKey: key})
	require.ErrorIs(t, err, types.ErrNotOwner)
	_, err = f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: business, JewelId: id, CardKey: key})
	require.ErrorIs(t, err, types.ErrJewelerSuspended)

	has, err := f.k.Card.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.False(t, has, "no refusal left a card behind")
}

// The card key is stored in the one form the sign bytes use, whatever case the
// app sent, and the staff member is recorded canonically.
func TestAnUpperCaseCardKeyIsStoredInLowerCase(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	card := secp256k1.GenPrivKey()
	key := hex.EncodeToString(card.PubKey().Bytes())
	_, err := f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: strings.ToUpper(staff), JewelId: id,
		CardKey: strings.ToUpper(key)})
	require.NoError(t, err)
	c, err := f.k.Card.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, key, c.Key)
	require.Equal(t, staff, c.IssuedBy)
	require.Equal(t, f.env.Ctx.BlockTime().Unix(), c.IssuedAt)

	_, buyer := f.env.Addr(t)
	f.claimFiled(t, card, id, buyer)
}

func TestAClaimIsRefusedWithoutTheCurrentCardAndItsSignature(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, buyer := f.env.Addr(t)

	// No card yet.
	_, err := f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, secp256k1.GenPrivKey(), id, buyer))
	require.ErrorIs(t, err, types.ErrNoCard)

	card := f.issue(t, staff, id)

	// Another key, signing correctly for itself.
	_, err = f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, secp256k1.GenPrivKey(), id, buyer))
	require.ErrorIs(t, err, types.ErrCardNotLive)

	// A claim copied off the network and refiled for someone else.
	_, thief := f.env.Addr(t)
	copied := f.claimMsg(t, card, id, buyer)
	copied.Claimer = thief
	_, err = f.srv.FileClaim(f.env.Ctx, copied)
	require.ErrorIs(t, err, types.ErrBadCardClaim)

	// A signature made for another chain, or another jewel.
	pub := card.PubKey().Bytes()
	sig, err := card.Sign(types.CardClaimSignBytes("yamale-other-1", id, hex.EncodeToString(pub), buyer))
	require.NoError(t, err)
	_, err = f.srv.FileClaim(f.env.Ctx, &types.MsgFileClaim{Claimer: buyer, JewelId: id, CardPubKey: pub, CardSignature: sig})
	require.ErrorIs(t, err, types.ErrBadCardClaim)
	sig, err = card.Sign(types.CardClaimSignBytes(f.env.Ctx.ChainID(), id+1, hex.EncodeToString(pub), buyer))
	require.NoError(t, err)
	_, err = f.srv.FileClaim(f.env.Ctx, &types.MsgFileClaim{Claimer: buyer, JewelId: id, CardPubKey: pub, CardSignature: sig})
	require.ErrorIs(t, err, types.ErrBadCardClaim)

	// No signature at all.
	_, err = f.srv.FileClaim(f.env.Ctx, &types.MsgFileClaim{Claimer: buyer, JewelId: id, CardPubKey: pub})
	require.ErrorIs(t, err, types.ErrBadCardClaim)

	has, err := f.k.Claim.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.False(t, has)

	// The upper-case spelling of the buyer is the same buyer, and the
	// signature over the canonical spelling verifies.
	msg := f.claimMsg(t, card, id, buyer)
	msg.Claimer = strings.ToUpper(buyer)
	_, err = f.srv.FileClaim(f.env.Ctx, msg)
	require.NoError(t, err)
	c, err := f.k.Claim.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, buyer, c.Claimer)
}

func TestOneClaimAtATimeAndNeverByTheShop(t *testing.T) {
	f := setup(t)
	_, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	card := f.issue(t, staff, id)

	_, err := f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, card, id, business))
	require.ErrorIs(t, err, types.ErrShopSelfClaim)

	_, first := f.env.Addr(t)
	_, second := f.env.Addr(t)
	f.claimFiled(t, card, id, first)
	_, err = f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, card, id, second))
	require.ErrorIs(t, err, types.ErrClaimPending)
	_, err = f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, card, id, first))
	require.ErrorIs(t, err, types.ErrClaimPending, "filing twice is still one claim")
}

func TestNoClaimOnAStolenJewelOrASuspendedJewelersStock(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	card := f.issue(t, staff, id)
	_, buyer := f.env.Addr(t)

	_, err := f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: staff, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, card, id, buyer))
	require.ErrorIs(t, err, types.ErrStolen)
	_, err = f.srv.ClearStolen(f.env.Ctx, &types.MsgClearStolen{Owner: staff, JewelId: id})
	require.NoError(t, err)

	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, card, id, buyer))
	require.ErrorIs(t, err, types.ErrJewelerSuspended)

	_, err = f.srv.ReinstateJeweler(f.env.Ctx, &types.MsgReinstateJeweler{Office: f.office, JewelerId: jid})
	require.NoError(t, err)
	f.claimFiled(t, card, id, buyer)
}

// Every change of owner spends the card, whatever caused it. The claim with
// it: a card can never move a jewel twice.
func TestEveryKindOfTransferSpendsTheCardAndTheClaim(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	_, claimer := f.env.Addr(t)

	t.Run("an accepted offer", func(t *testing.T) {
		id := f.register(t, staff, "MD-0001")
		f.claimFiled(t, f.issue(t, staff, id), id, claimer)
		f.sell(t, staff, id)
		f.requireSpent(t, jid, id)
	})
	t.Run("a claimed offer", func(t *testing.T) {
		id := f.register(t, staff, "MD-0002")
		f.claimFiled(t, f.issue(t, staff, id), id, claimer)
		claimKey := secp256k1.GenPrivKey()
		_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id,
			ClaimKey: hex.EncodeToString(claimKey.PubKey().Bytes())})
		require.NoError(t, err)
		_, friend := f.env.Addr(t)
		_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: friend, JewelId: id,
			ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: f.claimSig(t, claimKey, id, friend)})
		require.NoError(t, err)
		f.requireSpent(t, jid, id)
	})
	t.Run("a completed reassignment", func(t *testing.T) {
		id := f.register(t, staff, "MD-0003")
		f.claimFiled(t, f.issue(t, staff, id), id, claimer)
		_, heir := f.env.Addr(t)
		_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
			To: heir, Reason: "the business lost its key"})
		require.NoError(t, err)
		f.advance(month + time.Second)
		_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: heir, JewelId: id})
		require.NoError(t, err)
		f.requireSpent(t, jid, id)
	})
	t.Run("an approved claim", func(t *testing.T) {
		id := f.register(t, staff, "MD-0004")
		f.claimFiled(t, f.issue(t, staff, id), id, claimer)
		_, err := f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: id})
		require.NoError(t, err)
		f.requireSpent(t, jid, id)
	})
}

// A transfer spends a card even with no claim on it.
func TestASaleSpendsAnUnclaimedCard(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	card := f.issue(t, staff, id)
	f.sell(t, staff, id)
	f.requireSpent(t, jid, id)
	_, someone := f.env.Addr(t)
	_, err := f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, card, id, someone))
	require.ErrorIs(t, err, types.ErrNoCard)
}

func TestReprintingACardCancelsTheOldOneAndItsClaim(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	old := f.issue(t, staff, id)
	_, stranger := f.env.Addr(t)
	f.claimFiled(t, old, id, stranger)

	// Reissuing the same key keeps the claim made with it.
	_, err := f.srv.IssueCard(f.env.Ctx, &types.MsgIssueCard{Staff: staff, JewelId: id,
		CardKey: hex.EncodeToString(old.PubKey().Bytes())})
	require.NoError(t, err)
	has, err := f.k.Claim.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.True(t, has, "the same card, so the same claim")

	fresh := f.issue(t, staff, id)
	has, err = f.k.Claim.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.False(t, has, "a claim made with the old card is dropped")
	has, err = f.k.ClaimsByJeweler.Has(f.env.Ctx, collections.Join(jid, id))
	require.NoError(t, err)
	require.False(t, has)

	_, err = f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, old, id, stranger))
	require.ErrorIs(t, err, types.ErrCardNotLive, "the old card is cancelled")
	_, buyer := f.env.Addr(t)
	f.claimFiled(t, fresh, id, buyer)
}

func TestATheftReportClosesTheClaimAndKeepsTheCard(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.claimFiled(t, f.issue(t, staff, id), id, func() string { _, a := f.env.Addr(t); return a }())
	_, err := f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: staff, JewelId: id})
	require.NoError(t, err)
	has, err := f.k.Claim.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.False(t, has)
	has, err = f.k.ClaimsByJeweler.Has(f.env.Ctx, collections.Join(jid, id))
	require.NoError(t, err)
	require.False(t, has)
	has, err = f.k.Card.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.True(t, has, "the card stays; the shop reprints it once the jewel is back")
}

func TestARejectedClaimLeavesTheCardForTheRealBuyer(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	card := f.issue(t, staff, id)

	_, err := f.srv.RejectClaim(f.env.Ctx, &types.MsgRejectClaim{Staff: staff, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoClaim)

	_, stranger := f.env.Addr(t)
	f.claimFiled(t, card, id, stranger)
	_, err = f.srv.RejectClaim(f.env.Ctx, &types.MsgRejectClaim{Staff: stranger, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner, "the claimer cannot reject, only withdraw")

	_, err = f.srv.RejectClaim(f.env.Ctx, &types.MsgRejectClaim{Staff: staff, JewelId: id})
	require.NoError(t, err)
	require.True(t, f.eventTypes()["blockchain.jewel.v1.EventClaimRejected"])
	has, err := f.k.ClaimsByJeweler.Has(f.env.Ctx, collections.Join(jid, id))
	require.NoError(t, err)
	require.False(t, has)

	_, buyer := f.env.Addr(t)
	f.claimFiled(t, card, id, buyer)
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: id})
	require.NoError(t, err)
	require.Equal(t, buyer, f.owner(t, id))
}

func TestOnlyTheClaimerWithdraws(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	card := f.issue(t, staff, id)
	_, buyer := f.env.Addr(t)

	_, err := f.srv.WithdrawClaim(f.env.Ctx, &types.MsgWithdrawClaim{Claimer: buyer, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoClaim)

	f.claimFiled(t, card, id, buyer)
	_, other := f.env.Addr(t)
	_, err = f.srv.WithdrawClaim(f.env.Ctx, &types.MsgWithdrawClaim{Claimer: other, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotClaimer)
	_, err = f.srv.WithdrawClaim(f.env.Ctx, &types.MsgWithdrawClaim{Claimer: staff, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotClaimer, "staff reject; they do not withdraw")

	_, err = f.srv.WithdrawClaim(f.env.Ctx, &types.MsgWithdrawClaim{Claimer: strings.ToUpper(buyer), JewelId: id})
	require.NoError(t, err)
	require.True(t, f.eventTypes()["blockchain.jewel.v1.EventClaimWithdrawn"])
	has, err := f.k.ClaimsByJeweler.Has(f.env.Ctx, collections.Join(jid, id))
	require.NoError(t, err)
	require.False(t, has)
	has, err = f.k.Card.Has(f.env.Ctx, id)
	require.NoError(t, err)
	require.True(t, has)
}

func TestApprovalIsRefusedWithoutAClaimOrAnActiveShop(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	card := f.issue(t, staff, id)

	_, err := f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoClaim)

	_, buyer := f.env.Addr(t)
	f.claimFiled(t, card, id, buyer)

	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: buyer, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner, "a claimer cannot approve their own claim")
	_, _, otherStaff := f.admit(t, "Atelier Ba")
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: otherStaff, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner)

	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner)
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: business, JewelId: id})
	require.ErrorIs(t, err, types.ErrJewelerSuspended)
	require.Equal(t, business, f.owner(t, id))

	_, err = f.srv.ReinstateJeweler(f.env.Ctx, &types.MsgReinstateJeweler{Office: f.office, JewelerId: jid})
	require.NoError(t, err)
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: id})
	require.NoError(t, err, "the claim waited out the suspension")
	require.Equal(t, buyer, f.owner(t, id))
}

// No handler leaves a card or a claim on a jewel that has left the shop, so
// these refusals guard against a state the handlers cannot reach. The state is
// written directly to prove the guards are there.
func TestACardOrClaimOnAJewelThatLeftTheShopIsInert(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	card := secp256k1.GenPrivKey()
	key := hex.EncodeToString(card.PubKey().Bytes())
	require.NoError(t, f.k.Card.Set(f.env.Ctx, id, types.Card{Key: key, IssuedBy: staff}))

	_, someone := f.env.Addr(t)
	_, err := f.srv.FileClaim(f.env.Ctx, f.claimMsg(t, card, id, someone))
	require.ErrorIs(t, err, types.ErrNotStock)

	require.NoError(t, f.k.Claim.Set(f.env.Ctx, id, types.Claim{Claimer: someone, CardKey: key}))
	require.NoError(t, f.k.ClaimsByJeweler.Set(f.env.Ctx, collections.Join(jid, id)))
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: owner, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotStock)
	_, err = f.srv.ApproveClaim(f.env.Ctx, &types.MsgApproveClaim{Staff: staff, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner)
	require.Equal(t, owner, f.owner(t, id))
}

func TestClaimsAreListedPerJeweler(t *testing.T) {
	f := setup(t)
	a, _, staffA := f.admit(t, "Maison Diop")
	b, _, staffB := f.admit(t, "Atelier Ba")
	var mine []uint64
	for _, code := range []string{"MD-0001", "MD-0002", "MD-0003"} {
		id := f.register(t, staffA, code)
		_, c := f.env.Addr(t)
		f.claimFiled(t, f.issue(t, staffA, id), id, c)
		mine = append(mine, id)
	}
	theirs := f.register(t, staffB, "AB-0001")
	_, c := f.env.Addr(t)
	f.claimFiled(t, f.issue(t, staffB, theirs), theirs, c)
	f.issue(t, staffA, f.register(t, staffA, "MD-0004")) // a card, no claim: not listed

	res, err := f.qs.ClaimsByJeweler(f.env.Ctx, &types.QueryClaimsByJewelerRequest{JewelerId: a})
	require.NoError(t, err)
	var got []uint64
	for _, e := range res.Claims {
		got = append(got, e.Jewel.Id)
		require.Equal(t, a, e.Jewel.JewelerId)
	}
	require.Equal(t, mine, got)

	res, err = f.qs.ClaimsByJeweler(f.env.Ctx, &types.QueryClaimsByJewelerRequest{JewelerId: b})
	require.NoError(t, err)
	require.Len(t, res.Claims, 1)
	require.Equal(t, theirs, res.Claims[0].Jewel.Id)

	_, err = f.qs.ClaimsByJeweler(f.env.Ctx, &types.QueryClaimsByJewelerRequest{JewelerId: 99})
	require.ErrorIs(t, err, types.ErrNoJeweler)
}
