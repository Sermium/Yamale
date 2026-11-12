package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/jewel/types"
)

const testChain = "yamale-test-1"

func unb64(t *testing.T, m map[string]any, field string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(m[field].(string))
	if err != nil {
		t.Fatalf("%s is not base64: %v", field, err)
	}
	return b
}

// decodeMsg reads the emitted JSON the way the chain's codec does, through the
// same interface registry, so a field name or @type the chain would refuse
// fails here rather than on a live chain.
func decodeMsg(t *testing.T, m map[string]any) sdk.Msg {
	t.Helper()
	reg := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(reg)
	bz, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var msg sdk.Msg
	if err := codec.NewProtoCodec(reg).UnmarshalInterfaceJSON(bz, &msg); err != nil {
		t.Fatalf("the chain's codec refuses the JSON: %v", err)
	}
	return msg
}

func TestClaimOfferVerifiesOnlyForItsClaimer(t *testing.T) {
	priv := secp256k1.GenPrivKey()
	claimer := sdk.AccAddress(priv.PubKey().Address()).String()
	other := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address()).String()

	m := claimOffer(hex.EncodeToString(priv.Key), testChain, "7", claimer)
	pub, sig := unb64(t, m, "claim_pub_key"), unb64(t, m, "claim_signature")

	addr, ok := types.VerifySecp256k1(pub, types.ClaimSignBytes(testChain, 7, claimer), sig)
	if !ok || !bytes.Equal(addr, priv.PubKey().Address()) {
		t.Fatalf("claim does not verify for its claimer")
	}
	if _, ok := types.VerifySecp256k1(pub, types.ClaimSignBytes(testChain, 7, other), sig); ok {
		t.Fatal("claim verifies for a different claimer")
	}
	if _, ok := types.VerifySecp256k1(pub, types.ClaimSignBytes("other-chain", 7, claimer), sig); ok {
		t.Fatal("claim verifies on a different chain")
	}

	msg, ok := decodeMsg(t, m).(*types.MsgClaimOffer)
	if !ok || msg.Claimer != claimer || msg.JewelId != 7 || !bytes.Equal(msg.ClaimPubKey, pub) {
		t.Fatalf("decoded claim does not match what was signed: %+v", msg)
	}
}

func TestVoucherVerifiesOnlyOverItsFields(t *testing.T) {
	owner := secp256k1.GenPrivKey()
	staff := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address()).String()

	m := redeemBenefit(hex.EncodeToString(owner.Key), testChain, staff, "3", "7", "nonce-1", "1800000000")
	pub, sig := unb64(t, m, "owner_pub_key"), unb64(t, m, "owner_signature")

	v := types.Voucher{BenefitId: 3, JewelId: 7, Nonce: "nonce-1", ExpiresAt: 1800000000}
	if _, ok := types.VerifySecp256k1(pub, types.VoucherSignBytes(testChain, v), sig); !ok {
		t.Fatal("voucher does not verify over its own fields")
	}
	v.BenefitId = 4
	if _, ok := types.VerifySecp256k1(pub, types.VoucherSignBytes(testChain, v), sig); ok {
		t.Fatal("voucher verifies for a different benefit")
	}

	msg, ok := decodeMsg(t, m).(*types.MsgRedeemBenefit)
	if !ok || msg.Staff != staff || msg.Voucher.BenefitId != 3 || msg.Voucher.ExpiresAt != 1800000000 || msg.Voucher.Nonce != "nonce-1" {
		t.Fatalf("decoded voucher does not match what was signed: %+v", msg)
	}
	if !bytes.Equal(msg.OwnerPubKey, pub) {
		t.Fatal("decoded owner key differs")
	}
}

func TestClaimKeyHasDocumentedSizes(t *testing.T) {
	k := secp256k1.GenPrivKey()
	if len(hex.EncodeToString(k.Key)) != 64 || len(hex.EncodeToString(k.PubKey().Bytes())) != 66 {
		t.Fatal("claim key is not 32-byte private and 33-byte compressed public")
	}
}
