// Package cosmos is the half of threshold signing that knows what chain this
// is: turning the joint public key into a cosmos-sdk key type and a bech32
// address.
//
// Separate from mpc itself for one blunt reason: importing cosmos-sdk drags in
// the whole store stack — pebble, goleveldb — none of which compiles for
// GOOS=js, and the device's share has to run in a browser. Keeping the protocol
// free of it is what makes a WebAssembly build possible at all.
package cosmos

import (
	"crypto/ecdsa"
	"errors"

	sdksecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// CosmosPubKey converts the threshold key into the type the chain's ante
// handler verifies against.
//
// This is the point of the whole exercise: what comes out is an ordinary
// secp256k1.PubKey. An account holding one is indistinguishable on chain from
// an account whose owner keeps a seed phrase in a drawer — same address format,
// same signature verification, same 65-byte signature. Nothing about the
// account announces that it is held in three shares, which matters because the
// alternative design would have published the operator's key inside every
// consumer account and made the whole customer base countable by a stranger.
func CosmosPubKey(pub *ecdsa.PublicKey) (cryptotypes.PubKey, error) {
	if pub == nil || pub.X == nil || pub.Y == nil {
		return nil, errors.New("no public key")
	}
	var x, y secp256k1.FieldVal
	if overflow := x.SetByteSlice(pub.X.Bytes()); overflow {
		return nil, errors.New("public key X is not a field element")
	}
	if overflow := y.SetByteSlice(pub.Y.Bytes()); overflow {
		return nil, errors.New("public key Y is not a field element")
	}
	compressed := secp256k1.NewPublicKey(&x, &y).SerializeCompressed()
	return &sdksecp256k1.PubKey{Key: compressed}, nil
}

// Address is the bech32 account address the shares jointly control.
//
// Derived from the public key alone, so every party can compute it and none of
// them needs to be trusted to report it. A device that took the address from
// the custodian's word for it would be a device that can be told to fund
// somebody else's account.
func Address(pub *ecdsa.PublicKey) (string, error) {
	pk, err := CosmosPubKey(pub)
	if err != nil {
		return "", err
	}
	return sdk.AccAddress(pk.Address()).String(), nil
}
