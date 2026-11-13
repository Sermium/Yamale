package main

// Requests that change anything are signed by an account key.
//
// The API keeps no accounts and no passwords. A person is whoever holds the
// key, and what they may do is whatever the chain says that key is: staff of
// an active jeweler, its business, or simply an account. One format, shared
// with clients/sdk/src/loyaltyauth.ts and pinned by testdata/vectors.json:
//
//	yamale-loyalty/v1:<purpose>:<chain id>:<subject>:<issued, unix seconds>
//
// signed as Cosmos signs (SHA-256, low-S, 64 bytes r‖s), and sent as three
// headers. The subject binds the signature to this request's content, so a
// captured signature cannot be replayed onto another body; the five-minute
// window bounds how long a replay of the same body works, and every signed
// request here is idempotent.

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"

	"yamale/blockchain/x/jewel/types"
)

const (
	headerKey       = "X-Yamale-Key"
	headerSignature = "X-Yamale-Signature"
	headerIssued    = "X-Yamale-Issued"
	maxClockSkew    = 5 * time.Minute
)

func signBytes(purpose, chainID, subject string, issued int64) []byte {
	return []byte(fmt.Sprintf("yamale-loyalty/v1:%s:%s:%s:%d", purpose, chainID, subject, issued))
}

// signer returns the account that signed r for purpose. subject is a function
// of that account because one purpose, sponsorship, is about the signer's own
// address.
func (s *server) signer(r *http.Request, purpose string, subject func(address string) string) (string, error) {
	pub, err := base64.StdEncoding.DecodeString(r.Header.Get(headerKey))
	if err != nil || len(pub) != secp256k1.PubKeySize {
		return "", errors.New("the request is not signed: " + headerKey + " must be a 33-byte public key in base64")
	}
	sig, err := base64.StdEncoding.DecodeString(r.Header.Get(headerSignature))
	if err != nil || len(sig) != 64 {
		return "", errors.New("the request is not signed: " + headerSignature + " must be 64 bytes in base64")
	}
	issued, err := strconv.ParseInt(r.Header.Get(headerIssued), 10, 64)
	if err != nil {
		return "", errors.New(headerIssued + " must be unix seconds")
	}
	if d := s.now().Sub(time.Unix(issued, 0)); d > maxClockSkew || d < -maxClockSkew {
		return "", errors.New("the signature is more than five minutes from this server's clock; check the device's time and sign again")
	}
	address, err := bech32.ConvertAndEncode("yml", (&secp256k1.PubKey{Key: pub}).Address())
	if err != nil {
		return "", err
	}
	if _, ok := types.VerifySecp256k1(pub, signBytes(purpose, s.chainID, subject(address), issued), sig); !ok {
		return "", errors.New("the signature does not match this request")
	}
	return address, nil
}
