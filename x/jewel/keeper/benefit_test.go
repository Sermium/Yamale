package keeper_test

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// sellTo sells a jewel from stock to the account of a known key, so the test
// can sign vouchers as the owner.
func (f *fixture) sellTo(t *testing.T, staff string, id uint64, priv *secp256k1.PrivKey) string {
	t.Helper()
	buyer, err := f.env.AddressCodec.BytesToString(sdk.AccAddress(priv.PubKey().Address()))
	require.NoError(t, err)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.NoError(t, err)
	return buyer
}

// voucher builds the redemption a staff member submits after scanning the
// owner's code.
func (f *fixture) voucher(t *testing.T, staff string, priv *secp256k1.PrivKey, benefitID, jewelID uint64, nonce string, ttl time.Duration) *types.MsgRedeemBenefit {
	t.Helper()
	v := types.Voucher{BenefitId: benefitID, JewelId: jewelID, Nonce: nonce,
		ExpiresAt: f.env.Ctx.BlockTime().Add(ttl).Unix()}
	sig, err := priv.Sign(types.VoucherSignBytes(f.env.Ctx.ChainID(), v))
	require.NoError(t, err)
	return &types.MsgRedeemBenefit{Staff: staff, Voucher: v, OwnerPubKey: priv.PubKey().Bytes(), OwnerSignature: sig}
}

func (f *fixture) cleaning(t *testing.T, staff string, jewelID uint64, allowance uint32, period time.Duration) uint64 {
	t.Helper()
	res, err := f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: jewelID,
		Kind: "cleaning", Allowance: allowance, Period: int64(period / time.Second)})
	require.NoError(t, err)
	return res.Id
}

func TestTheOwnerConsentsToEachUse(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	bid := f.cleaning(t, staff, id, 2, 365*24*time.Hour)

	_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "n1", 4*time.Minute))
	require.NoError(t, err)
	b, _ := f.k.Benefit.Get(f.env.Ctx, bid)
	require.Len(t, b.Uses, 1)
}

func TestStaffCannotSpendABenefitWithoutTheOwner(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.sellTo(t, staff, id, secp256k1.GenPrivKey())
	bid := f.cleaning(t, staff, id, 2, 0)

	impostor := secp256k1.GenPrivKey()
	_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, impostor, bid, id, "n1", time.Minute))
	require.ErrorIs(t, err, types.ErrBadVoucher)
}

func TestAVoucherIsSpentOnceAndLivesFiveMinutesAtMost(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	bid := f.cleaning(t, staff, id, 5, 0)

	v := f.voucher(t, staff, owner, bid, id, "n1", time.Minute)
	_, err := f.srv.RedeemBenefit(f.env.Ctx, v)
	require.NoError(t, err)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, v)
	require.ErrorIs(t, err, types.ErrVoucherUsed)

	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "n2", 10*time.Minute))
	require.ErrorIs(t, err, types.ErrBadVoucher, "a voucher valid for ten minutes is refused")

	old := f.voucher(t, staff, owner, bid, id, "n3", time.Minute)
	f.advance(2 * time.Minute)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, old)
	require.ErrorIs(t, err, types.ErrBadVoucher, "an expired voucher is refused")
}

func TestTheAllowanceIsPerRollingPeriod(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	year := 365 * 24 * time.Hour
	bid := f.cleaning(t, staff, id, 2, year)

	for _, n := range []string{"a", "b"} {
		_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, n, time.Minute))
		require.NoError(t, err)
	}
	_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "c", time.Minute))
	require.ErrorIs(t, err, types.ErrAllowanceUsed)

	f.advance(year + time.Hour)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "d", time.Minute))
	require.NoError(t, err)
}

// The benefit follows the jewel: the new owner's voucher works, the old
// owner's does not.
func TestABenefitPassesToTheNextOwner(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	first := secp256k1.GenPrivKey()
	firstAddr := f.sellTo(t, staff, id, first)
	bid := f.cleaning(t, staff, id, 5, 0)

	second := secp256k1.GenPrivKey()
	secondAddr, _ := f.env.AddressCodec.BytesToString(sdk.AccAddress(second.PubKey().Address()))
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: firstAddr, JewelId: id, To: secondAddr})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: secondAddr, JewelId: id})
	require.NoError(t, err)

	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, first, bid, id, "old", time.Minute))
	require.ErrorIs(t, err, types.ErrBadVoucher)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, second, bid, id, "new", time.Minute))
	require.NoError(t, err)
}

func TestASuspendedJewelersBenefitsAreNotHonoured(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	bid := f.cleaning(t, staff, id, 5, 0)
	_, err := f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "closed"})
	require.NoError(t, err)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "n", time.Minute))
	require.ErrorIs(t, err, types.ErrJewelerSuspended)
}

func TestOnlyTheIssuingJewelerAttachesBenefits(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	_, _, other := f.admit(t, "Atelier Ba")
	id := f.register(t, staff, "MD-0001")
	_, err := f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: other, JewelId: id, Kind: "cleaning", Allowance: 1})
	require.ErrorIs(t, err, types.ErrNotStaff)
	_, err = f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: id, Kind: "", Allowance: 1})
	require.ErrorIs(t, err, types.ErrInvalidBenefit)
	_, err = f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: id, Kind: "cleaning", Allowance: 0})
	require.ErrorIs(t, err, types.ErrInvalidBenefit)
}

func TestAnExpiredBenefitIsRefused(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	res, err := f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: id, Kind: "resizing",
		Allowance: 1, ExpiresAt: f.env.Ctx.BlockTime().Add(time.Hour).Unix()})
	require.NoError(t, err)
	f.advance(2 * time.Hour)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, res.Id, id, "n", time.Minute))
	require.ErrorIs(t, err, types.ErrBenefitExpired)
}

// Bech32 accepts an all-upper-case spelling of the same account. The staff
// address must be canonicalised before the lookup, so the upper-case spelling
// is the same staff member and the benefit it creates is theirs.
func TestAnUpperCaseStaffAddressIsTheSameStaffMember(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)

	upper := strings.ToUpper(staff)
	res, err := f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: upper, JewelId: id, Kind: "cleaning", Allowance: 2})
	require.NoError(t, err)
	b, err := f.k.Benefit.Get(f.env.Ctx, res.Id)
	require.NoError(t, err)
	require.Equal(t, id, b.JewelId)
	require.Equal(t, jid, b.JewelerId)

	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, upper, owner, res.Id, id, "n1", time.Minute))
	require.NoError(t, err)
	b, _ = f.k.Benefit.Get(f.env.Ctx, res.Id)
	require.Len(t, b.Uses, 1)
}
