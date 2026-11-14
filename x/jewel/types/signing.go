package types

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// VoucherSignBytes is what an owner signs to consent to one benefit use. The
// chain id is in it so a voucher from a test chain cannot be spent here.
func VoucherSignBytes(chainID string, v Voucher) []byte {
	return []byte(fmt.Sprintf("yamale-jewel-voucher/v1:%s:%d:%d:%s:%d",
		chainID, v.BenefitId, v.JewelId, v.Nonce, v.ExpiresAt))
}

// ClaimSignBytes is what a claim key signs. It names the claimer, which makes
// a claim copied out of the mempool useless to anyone else, and the offer's
// expiry, which makes a signature for one offer useless for any later offer
// made to the same key. Two offers made in the same block have the same expiry,
// so the expiry separates offers only across block times; whoever holds the
// claim key can sign afresh in any case.
func ClaimSignBytes(chainID string, jewelID uint64, offerExpiresAt int64, claimer string) []byte {
	return []byte(fmt.Sprintf("yamale-jewel-claim/v2:%s:%d:%d:%s", chainID, jewelID, offerExpiresAt, claimer))
}

// VerifySecp256k1 checks a signature by a compressed secp256k1 key and
// returns the key's account address.
func VerifySecp256k1(pubKey, msg, sig []byte) (sdk.AccAddress, bool) {
	if len(pubKey) != secp256k1.PubKeySize {
		return nil, false
	}
	pk := &secp256k1.PubKey{Key: pubKey}
	if !pk.VerifySignature(msg, sig) {
		return nil, false
	}
	return sdk.AccAddress(pk.Address()), true
}
