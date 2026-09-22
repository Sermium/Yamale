// Package chain is the half of the recovery scheme that knows what chain this
// is: turning a secp256k1 public key into a cosmos-sdk key type and a bech32
// address.
//
// Separate from the recovery package itself for one blunt reason, inherited
// from the threshold code this replaces: importing cosmos-sdk drags in the
// whole store stack — pebble, goleveldb — none of which compiles for GOOS=js,
// and the device's half has to run in a browser. Keeping the sealing and
// sharing free of it is what makes a WebAssembly build possible at all.
package chain

import (
	"errors"
	"fmt"

	sdksecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// PubKey converts a compressed secp256k1 public key into the type the chain's
// ante handler verifies against.
//
// What comes out is an ordinary secp256k1.PubKey. An account recovered through
// this package is indistinguishable on chain from an account whose owner keeps
// a seed phrase in a drawer — same address format, same signature verification,
// same 65-byte signature. Nothing about the account announces how it is held,
// which matters because the alternative would have made the whole customer base
// countable by a stranger reading the chain.
func PubKey(compressed []byte) (cryptotypes.PubKey, error) {
	if len(compressed) != secp256k1.PubKeyBytesLenCompressed {
		return nil, errors.New("a compressed secp256k1 public key is 33 bytes")
	}
	// Parsed rather than trusted. An arbitrary 33 bytes is not a point on the
	// curve, and an address derived from one would be an address nothing can
	// ever sign for — which is indistinguishable, afterwards, from an account
	// whose key was lost.
	if _, err := secp256k1.ParsePubKey(compressed); err != nil {
		return nil, errors.New("that is not a point on secp256k1")
	}
	return &sdksecp256k1.PubKey{Key: compressed}, nil
}

// Address is the bech32 account address a public key controls.
//
// Derived from the key alone, so every party computes it and none of them has
// to be trusted to report it. A device that took the address from a service's
// word for it would be a device that can be told to fund somebody else's
// account.
func Address(compressed []byte) (string, error) {
	pk, err := PubKey(compressed)
	if err != nil {
		return "", err
	}
	return sdk.AccAddress(pk.Address()).String(), nil
}

// CheckAddress reports whether a string is a well-formed account address on
// this chain.
//
// Used where an address arrives from an operator rather than from a key — an
// import, principally. It confirms the bech32 and the prefix and nothing more:
// there is no way to tell from an address alone whether anybody holds the key
// for it, so this catches a typo and not a mistake.
func CheckAddress(address string) error {
	if address == "" {
		return errors.New("no address")
	}
	if _, err := sdk.AccAddressFromBech32(address); err != nil {
		return fmt.Errorf("%q is not an account address on this chain: %w", address, err)
	}
	return nil
}
