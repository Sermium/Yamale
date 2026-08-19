package main

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	bip39 "github.com/cosmos/go-bip39"
)

// secret is a recovery phrase held in a buffer this program owns.
//
// The claim this type supports is narrow and worth stating precisely, because
// the runbook depends on it and the report says the same thing:
//
// What it guarantees. Nothing in this program writes the phrase to a file, a
// log, an environment variable or a command line, and it is never a function
// argument outside this file. The only sink is console.print, which refuses to
// run when its output is not a terminal.
//
// What it cannot guarantee. bip39.NewMnemonic returns a Go string, and a Go
// string is immutable: that copy stays wherever the allocator put it until the
// garbage collector reuses the page, and no code in Go can shorten that. The
// same is true of the string handed to hd.Secp256k1.Derive, which is the
// chain's own derivation path and the reason this tool does not reimplement it
// — a wallet that derived addresses even slightly differently would produce
// keys that look correct and control nothing. So: every buffer this program
// allocates is zeroed, and two short-lived immutable copies are not. The
// mitigations for those are a machine with no swap file, no hibernation and no
// crash dumps, and a power cycle at the end — which is why the runbook makes
// all four ceremony steps rather than recommendations.
type secret struct {
	buf []byte
}

// newSecret generates a fresh phrase.
func newSecret() (*secret, error) {
	entropy, err := bip39.NewEntropy(entropyBits)
	if err != nil {
		return nil, err
	}
	defer zero(entropy)

	phrase, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, err
	}
	// Copied into a buffer immediately so that everything downstream of this
	// line is zeroable. See the type comment for what this does not fix.
	return &secret{buf: []byte(phrase)}, nil
}

// secretFromInput takes a phrase somebody has typed back in, for the restore
// drill.
func secretFromInput(line string) (*secret, error) {
	buf := []byte(strings.Join(strings.Fields(line), " "))

	// MnemonicToByteArray, not IsMnemonicValid.
	//
	// This is not a stylistic choice and the difference is the whole point of
	// the drill. bip39.IsMnemonicValid — the obvious-looking call, and the one
	// tools/wallet used — checks only that there are 12/15/18/21/24 words and
	// that each one appears in the wordlist. It does not verify the checksum.
	// Every realistic transcription error passes it: somebody copying by hand
	// writes another real BIP-39 word, so the word count is right, every word
	// is in the list, and the phrase validates while deriving a completely
	// different, empty account.
	//
	// MnemonicToByteArray recomputes the checksum, which is the four bits at
	// the end of a 24-word phrase whose entire job is catching exactly this.
	entropy, err := bip39.MnemonicToByteArray(string(buf))
	if err != nil {
		zero(buf)
		return nil, errors.New(
			"that is not a valid recovery phrase — the checksum does not match.\n" +
				"One of the words is wrong, or two are swapped. Check it against the sheet word\n" +
				"by word. Do not correct a guess and try again: a phrase with one wrong word\n" +
				"derives a different, empty account rather than failing, which is why the last\n" +
				"word of a BIP-39 phrase carries a checksum and why this is checked here")
	}
	zero(entropy)
	return &secret{buf: buf}, nil
}

// zero overwrites the phrase. Call it with defer, at the point the secret is
// created, every time.
func (s *secret) zero() {
	if s == nil {
		return
	}
	zero(s.buf)
	s.buf = nil
}

// wordCount is how many words the phrase has.
func (s *secret) wordCount() int {
	return len(splitWords(s.buf))
}

// word returns the i-th word, one-based, as a slice into the phrase buffer.
//
// A slice rather than a string so that displaying and checking the phrase
// allocates no copies of it that zero() would then miss.
func (s *secret) word(i int) []byte {
	words := splitWords(s.buf)
	if i < 1 || i > len(words) {
		return nil
	}
	return words[i-1]
}

func splitWords(buf []byte) [][]byte {
	var words [][]byte
	start := -1
	for i := 0; i <= len(buf); i++ {
		if i == len(buf) || buf[i] == ' ' {
			if start >= 0 {
				words = append(words, buf[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return words
}

// zero overwrites a buffer and keeps it alive across the loop.
//
// runtime.KeepAlive is not decoration. Without it the compiler is free to
// notice that nothing reads the buffer afterwards and drop the stores, which
// would leave the phrase in memory while this function claims to have removed
// it. Go has no explicit_bzero; this is the closest thing available.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}

// derive turns the phrase into a key, through the chain's own derivation path.
//
// The caller owns the returned private key and must zero it. The one string
// conversion here is the unavoidable copy described on the secret type.
func (s *secret) derive(index uint32) (*secp256k1.PrivKey, string, error) {
	path := hd.NewFundraiserParams(0, coinType, index).String()

	derived, err := hd.Secp256k1.Derive()(string(s.buf), "", path)
	if err != nil {
		return nil, "", err
	}
	defer zero(derived)

	priv, ok := hd.Secp256k1.Generate()(derived).(*secp256k1.PrivKey)
	if !ok {
		return nil, "", errors.New("the chain's key generator returned an unexpected key type")
	}
	return priv, path, nil
}

// role is what a key is being generated for. It changes the warnings, the
// derived addresses shown, and what the ceremony record calls the holder.
type role string

const (
	roleCustodian role = "custodian"
	roleValidator role = "validator-operator"
)

// pubKeyJSON is the pubkey in the form every SDK message and query uses, so it
// can be pasted into a validator.json or a group members file without being
// re-encoded by hand.
type pubKeyJSON struct {
	Type string `json:"@type"`
	Key  string `json:"key"`
}

// identity is everything this ceremony publishes about one key. All of it is
// public: an address, a pubkey and a digest of the pubkey. Nothing here helps
// anybody spend anything.
type identity struct {
	Name        string     `json:"name"`
	Role        role       `json:"role"`
	Address     string     `json:"address"`
	Valoper     string     `json:"valoper,omitempty"`
	PubKey      pubKeyJSON `json:"pubkey"`
	Fingerprint string     `json:"fingerprint"`
	HDPath      string     `json:"hd_path"`
	GeneratedAt string     `json:"generated_at"`
	Ceremony    string     `json:"ceremony,omitempty"`
}

// identityOf assembles the public record for a derived key.
func identityOf(name string, r role, priv *secp256k1.PrivKey, path string, now time.Time) (identity, error) {
	pub := priv.PubKey()
	addressBytes := pub.Address().Bytes()

	address, err := bech32.ConvertAndEncode(accountPrefix, addressBytes)
	if err != nil {
		return identity{}, err
	}

	id := identity{
		Name: name,
		Role: r,
		PubKey: pubKeyJSON{
			Type: "/cosmos.crypto.secp256k1.PubKey",
			Key:  base64.StdEncoding.EncodeToString(pub.Bytes()),
		},
		Address:     address,
		Fingerprint: fingerprint(pub.Bytes()),
		HDPath:      path,
		GeneratedAt: now.UTC().Format(time.RFC3339),
	}

	// A validator's operator address is the same key in the valoper prefix.
	// Shown only for that role, because a custodian who was given one would
	// reasonably conclude the foundation key is also a staking key.
	if r == roleValidator {
		id.Valoper, err = bech32.ConvertAndEncode(accountPrefix+"valoper", addressBytes)
		if err != nil {
			return identity{}, err
		}
	}
	return id, nil
}

// fingerprintDomain separates this digest from every other digest of the same
// pubkey. Without it, a value printed here could be mistaken for — or replayed
// as — an address hash or a transaction hash somewhere else.
const fingerprintDomain = "yamale-ceremony-fingerprint-v1"

// fingerprint is the short string a custodian writes on their own sheet.
//
// It is what makes a swapped or mis-filed envelope detectable. Five years from
// now, an envelope labelled "custodian 3" either recovers to a key whose
// fingerprint matches the one on the ceremony record or it does not, and that
// check needs no network, no node and nothing anybody has to be trusted about.
//
// Crockford's base32 alphabet, so the four characters people confuse on paper —
// I, L, O, U — never appear, and 1/l, 0/O cannot be misread into a different
// valid value. Forty bits in two groups of four: short enough to be read aloud
// across a room and compared by eye, long enough that two of the five
// custodians colliding is not a thing that happens.
func fingerprint(pubkey []byte) string {
	sum := sha256.Sum256(append([]byte(fingerprintDomain), pubkey...))
	encoder := base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)
	encoded := encoder.EncodeToString(sum[:5])
	return encoded[:4] + "-" + encoded[4:8]
}

// describe is the human-readable line the record and the console both use.
func (id identity) describe() string {
	return fmt.Sprintf("%s  %s  %s", id.Fingerprint, id.Address, id.Name)
}
