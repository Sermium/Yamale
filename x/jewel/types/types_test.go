package types_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// An inscription is read off metal through a loupe and typed by a person, so
// the same code arrives with spaces, dashes and lower case. Normalising is
// what makes "one inscription, one jewel" mean anything.
func TestInscriptionsNormaliseToOneForm(t *testing.T) {
	for _, in := range []string{"gia 2141-438-1", "GIA2141.438.1", " gia/2141_438 1 "} {
		got, err := types.NormaliseInscription(in)
		require.NoError(t, err, in)
		require.Equal(t, "GIA21414381", got, in)
	}
}

func TestInscriptionsThatCannotBeEngravedAreRefused(t *testing.T) {
	for _, in := range []string{"", "AB", "MAISON-É-2024", "AB#1234", "A123456789012345678901234567890123456789X"} {
		_, err := types.NormaliseInscription(in)
		require.ErrorIs(t, err, types.ErrInvalidInscription, in)
	}
}

func TestARecordMustBeAFingerprintAndALocation(t *testing.T) {
	good := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	require.NoError(t, types.ValidateRecord(good, "https://loyalty.yamaletech.com/r/"+good))
	require.ErrorIs(t, types.ValidateRecord("9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08", "x"), types.ErrInvalidRecord)
	require.ErrorIs(t, types.ValidateRecord(good[:63], "x"), types.ErrInvalidRecord)
	require.ErrorIs(t, types.ValidateRecord(good, ""), types.ErrInvalidRecord)
}

func TestTheDefaultsAreTheSpecsNumbers(t *testing.T) {
	p := types.DefaultParams()
	require.Equal(t, int64(14*24*3600), p.OfferTtl)
	require.Equal(t, int64(300), p.VoucherMaxTtl)
	require.Equal(t, int64(30*24*3600), p.ReassignmentWindow)
	require.Empty(t, p.Office)
	require.NoError(t, p.Validate())
}

func TestParamsThatDisableAProtectionAreRefused(t *testing.T) {
	for _, mutate := range []func(*types.Params){
		func(p *types.Params) { p.OfferTtl = 0 },
		func(p *types.Params) { p.VoucherMaxTtl = 0 },
		func(p *types.Params) { p.VoucherMaxTtl = 3601 },
		func(p *types.Params) { p.ReassignmentWindow = 0 },
		// An owner travelling for a week must still be able to object.
		func(p *types.Params) { p.ReassignmentWindow = 7*24*3600 - 1 },
	} {
		p := types.DefaultParams()
		mutate(&p)
		require.Error(t, p.Validate())
	}
	p := types.DefaultParams()
	p.ReassignmentWindow = 7 * 24 * 3600
	require.NoError(t, p.Validate(), "a week is the shortest window allowed")
}

// A signature by the right key over the right bytes verifies, and names the
// key's address. Anything else does not verify.
func TestSignaturesVerifyOnlyOverTheirOwnBytes(t *testing.T) {
	priv := secp256k1.GenPrivKey()
	msg := types.ClaimSignBytes("yamale-local-1", 7, 1790000000, "yml1claimer")
	sig, err := priv.Sign(msg)
	require.NoError(t, err)

	addr, ok := types.VerifySecp256k1(priv.PubKey().Bytes(), msg, sig)
	require.True(t, ok)
	require.Equal(t, sdk.AccAddress(priv.PubKey().Address()), addr)

	_, ok = types.VerifySecp256k1(priv.PubKey().Bytes(), types.ClaimSignBytes("yamale-local-1", 7, 1790000000, "yml1thief"), sig)
	require.False(t, ok, "a claim signed for one address must not verify for another")
	_, ok = types.VerifySecp256k1(priv.PubKey().Bytes(), types.ClaimSignBytes("other-chain", 7, 1790000000, "yml1claimer"), sig)
	require.False(t, ok, "a claim signed on one chain must not verify on another")
	_, ok = types.VerifySecp256k1(priv.PubKey().Bytes(), types.ClaimSignBytes("yamale-local-1", 7, 1790000001, "yml1claimer"), sig)
	require.False(t, ok, "a claim signed for one offer must not verify for another")
	_, ok = types.VerifySecp256k1([]byte{1, 2, 3}, msg, sig)
	require.False(t, ok)
}

func TestTokenIdsNameTheirJeweler(t *testing.T) {
	require.Equal(t, "jewel-3", types.ClassID(3))
	require.Equal(t, "jewel-41", types.TokenID(41))
}
