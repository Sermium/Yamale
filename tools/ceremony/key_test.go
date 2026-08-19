package main

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"yamale/blockchain/app"
)

// vectorPhrase is the BIP-39 test vector for all-zero entropy. Using a
// published vector rather than a freshly generated phrase means this test
// compares against something an auditor can reproduce with any other wallet.
const vectorPhrase = "abandon abandon abandon abandon abandon abandon abandon abandon abandon " +
	"abandon abandon abandon abandon abandon abandon abandon abandon abandon " +
	"abandon abandon abandon abandon abandon art"

// The address and pubkey the chain's own keyring produces for that phrase:
//
//	blockchaind keys add v --recover --keyring-backend test
//
// Hardcoded from that command's output rather than recomputed here, so this is
// a comparison between two implementations instead of a function agreeing with
// itself. A tool that derived addresses even slightly differently from the node
// would hand five custodians keys that look right and control nothing, and
// nobody would find out until the first seizure.
const (
	vectorAddress = "yml1r5v5srda7xfth3hn2s26txvrcrntldjuray0xk"
	vectorPubKey  = "ArpmqEz3g5rxcqE+f8n15wCMuLyhWF+PO6+zA57aPB/d"
)

func TestAddressConfigMatchesTheChain(t *testing.T) {
	if accountPrefix != app.AccountAddressPrefix {
		t.Fatalf("prefix %q does not match the chain's %q", accountPrefix, app.AccountAddressPrefix)
	}
	if coinType != app.ChainCoinType {
		t.Fatalf("coin type %d does not match the chain's %d", coinType, app.ChainCoinType)
	}
}

func TestDerivedAddressMatchesTheChainsKeyring(t *testing.T) {
	configureAddresses()

	s, err := secretFromInput(vectorPhrase)
	if err != nil {
		t.Fatal(err)
	}
	defer s.zero()

	priv, path, err := s.derive(0)
	if err != nil {
		t.Fatal(err)
	}
	if path != "m/44'/118'/0'/0/0" {
		t.Fatalf("hd path = %q", path)
	}

	id, err := identityOf("vector", roleCustodian, priv, path, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if id.Address != vectorAddress {
		t.Fatalf("address = %q, want %q", id.Address, vectorAddress)
	}
	if id.PubKey.Key != vectorPubKey {
		t.Fatalf("pubkey = %q, want %q", id.PubKey.Key, vectorPubKey)
	}
	// A custodian is not a validator and must not be handed an operator
	// address: given one, the obvious conclusion is that the foundation key is
	// also a staking key.
	if id.Valoper != "" {
		t.Fatalf("a custodian identity carries a valoper address: %q", id.Valoper)
	}
}

func TestValidatorIdentityCarriesAnOperatorAddress(t *testing.T) {
	configureAddresses()

	s, err := secretFromInput(vectorPhrase)
	if err != nil {
		t.Fatal(err)
	}
	defer s.zero()

	priv, path, err := s.derive(0)
	if err != nil {
		t.Fatal(err)
	}
	id, err := identityOf("operator", roleValidator, priv, path, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id.Valoper, accountPrefix+"valoper1") {
		t.Fatalf("valoper = %q", id.Valoper)
	}
	// Same key, two encodings. If these ever stop being the same key the
	// operator would self-bond from an account the chain does not associate
	// with their validator.
	if id.Address != vectorAddress {
		t.Fatalf("account address = %q, want %q", id.Address, vectorAddress)
	}
}

func TestMisTranscribedPhraseIsRefusedRatherThanDerived(t *testing.T) {
	// Each of these is one real BIP-39 word swapped for another real BIP-39
	// word, which is exactly what a hand-copied sheet with a mistake on it
	// looks like: right word count, every word in the list, wrong key.
	//
	// This is the case bip39.IsMnemonicValid does NOT catch — it checks the
	// count and the wordlist and never verifies the checksum, so all four of
	// these pass it. Pinned as a test because the name of that function makes
	// the opposite assumption very easy to write, and a restore drill built on
	// it would confirm a sheet that recovers nothing.
	for _, broken := range []string{
		strings.Replace(vectorPhrase, "art", "zoo", 1),
		strings.Replace(vectorPhrase, "art", "young", 1),
		strings.Replace(vectorPhrase, "abandon", "ability", 1),
		strings.Replace(vectorPhrase, "abandon", "zone", 1),
	} {
		if _, err := secretFromInput(broken); err == nil {
			t.Fatalf("a phrase with a broken checksum was accepted: %q", broken)
		}
	}

	if _, err := secretFromInput("not a mnemonic at all"); err == nil {
		t.Fatal("arbitrary text was accepted as a phrase")
	}
	// A word dropped off the end is the other half of the same failure, and
	// the reason the transcription check always asks for the last word.
	short := strings.TrimSuffix(vectorPhrase, " art")
	if _, err := secretFromInput(short); err == nil {
		t.Fatal("a 23-word phrase was accepted")
	}
}

func TestNewSecretProducesTwentyFourDistinctWords(t *testing.T) {
	s, err := newSecret()
	if err != nil {
		t.Fatal(err)
	}
	defer s.zero()

	if got := s.wordCount(); got != 24 {
		t.Fatalf("word count = %d, want 24", got)
	}

	other, err := newSecret()
	if err != nil {
		t.Fatal(err)
	}
	defer other.zero()

	if string(s.buf) == string(other.buf) {
		t.Fatal("two generated phrases are identical, so the entropy source is not one")
	}
}

func TestZeroActuallyOverwritesThePhrase(t *testing.T) {
	s, err := newSecret()
	if err != nil {
		t.Fatal(err)
	}
	// Held deliberately: zero() nils the field, so without a second reference
	// there would be nothing left to inspect and this test would pass against
	// an implementation that only dropped the pointer.
	buf := s.buf
	s.zero()

	for i, b := range buf {
		if b != 0 {
			t.Fatalf("byte %d of the phrase buffer survived zeroing: %q", i, b)
		}
	}
	if s.buf != nil {
		t.Fatal("the secret still holds a buffer after being zeroed")
	}
}

func TestWordAccess(t *testing.T) {
	s, err := secretFromInput("  alpha   bravo charlie  ")
	if err == nil {
		t.Fatal("a three-word phrase passed the BIP-39 checksum, which cannot be right")
	}
	// secretFromInput refuses it, so the word splitting is exercised directly.
	s = &secret{buf: []byte("alpha bravo charlie")}
	if got := s.wordCount(); got != 3 {
		t.Fatalf("word count = %d", got)
	}
	if got := string(s.word(1)); got != "alpha" {
		t.Fatalf("word 1 = %q", got)
	}
	if got := string(s.word(3)); got != "charlie" {
		t.Fatalf("word 3 = %q", got)
	}
	// Out of range is nil, not a panic and not a wrapped index: askBack
	// compares against this, and a wrapped index would silently accept the
	// wrong word.
	if s.word(0) != nil || s.word(4) != nil {
		t.Fatal("out-of-range word access returned something")
	}
}

func TestFingerprintIsStableDistinctAndReadable(t *testing.T) {
	configureAddresses()

	s, err := secretFromInput(vectorPhrase)
	if err != nil {
		t.Fatal(err)
	}
	defer s.zero()

	pub, err := base64.StdEncoding.DecodeString(vectorPubKey)
	if err != nil {
		t.Fatal(err)
	}

	first := fingerprint(pub)
	if first != fingerprint(pub) {
		t.Fatal("the fingerprint is not stable, so a sheet could never be checked against it")
	}
	if len(first) != 9 || first[4] != '-' {
		t.Fatalf("fingerprint %q is not XXXX-XXXX", first)
	}
	// Crockford's alphabet: the four characters people confuse on paper must
	// never appear, or a fingerprint read aloud is ambiguous.
	if strings.ContainsAny(first, "ILOU") {
		t.Fatalf("fingerprint %q contains a character that is confusable on paper", first)
	}

	// A different key must give a different fingerprint, or a swapped envelope
	// would not be detectable — which is the only thing this value is for.
	other := make([]byte, len(pub))
	copy(other, pub)
	other[len(other)-1] ^= 0x01
	if fingerprint(other) == first {
		t.Fatal("two different public keys share a fingerprint")
	}
}

func TestSlug(t *testing.T) {
	for input, want := range map[string]string{
		"A. Okafor":         "a-okafor",
		"Banque Nationale":  "banque-nationale",
		"  Trailing --  ":   "trailing",
		"Zürich Custodian":  "z-rich-custodian",
		"multiple   spaces": "multiple-spaces",
	} {
		if got := slug(input); got != want {
			t.Errorf("slug(%q) = %q, want %q", input, got, want)
		}
	}
}
