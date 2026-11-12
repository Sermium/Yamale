// Command jewelsign produces the two signatures x/jewel takes that a chain
// key in a keyring cannot make through the CLI: a claim key's signature over
// the claimer's address, and an owner's consent to a benefit use.
//
// It exists for scripts/devnet/seed-jewel.sh. The real app signs both in the
// browser with the account service's key.
package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"yamale/blockchain/x/jewel/types"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "claimkey":
		k := secp256k1.GenPrivKey()
		fmt.Println(hex.EncodeToString(k.Key), hex.EncodeToString(k.PubKey().Bytes()))
	case "claim":
		need(6)
		emit(claimOffer(os.Args[2], os.Args[3], os.Args[4], os.Args[5]))
	case "voucher":
		need(9)
		emit(redeemBenefit(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6], os.Args[7], os.Args[8]))
	default:
		usage()
	}
}

// claimOffer signs the claimer exactly as given. The handlers canonicalise the
// address before building the sign bytes, so scripts must pass the lower-case
// keyring address, or the claim is refused as a bad signature.
func claimOffer(privHex, chainID, jewelIDStr, claimer string) map[string]any {
	k := key(privHex)
	sig := sign(k, types.ClaimSignBytes(chainID, u64(jewelIDStr), claimer))
	return map[string]any{
		"@type": "/blockchain.jewel.v1.MsgClaimOffer", "claimer": claimer,
		"jewel_id": jewelIDStr, "claim_pub_key": b64(k.PubKey().Bytes()), "claim_signature": b64(sig),
	}
}

// redeemBenefit is the owner's voucher for one benefit use, signed over the
// same bytes the chain rebuilds from the message.
func redeemBenefit(privHex, chainID, staff, benefitIDStr, jewelIDStr, nonce, expiresStr string) map[string]any {
	k := key(privHex)
	v := types.Voucher{BenefitId: u64(benefitIDStr), JewelId: u64(jewelIDStr), Nonce: nonce, ExpiresAt: i64(expiresStr)}
	sig := sign(k, types.VoucherSignBytes(chainID, v))
	return map[string]any{
		"@type": "/blockchain.jewel.v1.MsgRedeemBenefit", "staff": staff,
		"voucher":       map[string]any{"benefit_id": benefitIDStr, "jewel_id": jewelIDStr, "nonce": nonce, "expires_at": expiresStr},
		"owner_pub_key": b64(k.PubKey().Bytes()), "owner_signature": b64(sig),
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: jewelsign claimkey | claim <priv-hex> <chain-id> <jewel-id> <claimer> | voucher <priv-hex> <chain-id> <staff> <benefit-id> <jewel-id> <nonce> <expires-at>")
	os.Exit(2)
}

func need(n int) {
	if len(os.Args) != n {
		usage()
	}
}

func key(h string) *secp256k1.PrivKey {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 32 {
		fmt.Fprintln(os.Stderr, "the private key must be 32 bytes of hex")
		os.Exit(1)
	}
	return &secp256k1.PrivKey{Key: b}
}

func sign(k *secp256k1.PrivKey, msg []byte) []byte {
	sig, err := k.Sign(msg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return sig
}

func u64(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return v
}

func i64(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return v
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func emit(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
