package types

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dcrsecp "github.com/decred/dcrd/dcrec/secp256k1/v4"
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

// CardClaimSignBytes is what an ownership card's secret signs to file a claim.
// It names the claimer, so a claim copied off the network cannot be refiled
// for anyone else, and the card's key, so a signature made with a card the
// shop has since replaced fails. cardKeyHex is the lower-case hex form the
// chain stores.
func CardClaimSignBytes(chainID string, jewelID uint64, cardKeyHex, claimer string) []byte {
	return []byte(fmt.Sprintf("yamale-jewel-card/v1:%s:%d:%s:%s", chainID, jewelID, cardKeyHex, claimer))
}

// NormaliseCardKey returns a card key in the form the chain stores, lower-case
// hex, or an error if it is not a compressed secp256k1 public key: 33 bytes,
// a point on the curve. A key off the curve could never sign a claim, so the
// card printed with it would be dead from the start.
func NormaliseCardKey(k string) (string, error) {
	k = strings.ToLower(k)
	b, err := hex.DecodeString(k)
	if err != nil || len(b) != secp256k1.PubKeySize {
		return "", ErrBadCardKey
	}
	if _, err := dcrsecp.ParsePubKey(b); err != nil {
		return "", ErrBadCardKey
	}
	return k, nil
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
