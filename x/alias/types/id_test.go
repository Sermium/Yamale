package types_test

import (
	"fmt"
	"testing"

	"yamale/blockchain/x/alias/types"
)

func TestAlphabetIsCrockford(t *testing.T) {
	if len(types.Alphabet) != 32 {
		t.Fatalf("alphabet is %d symbols, want 32", len(types.Alphabet))
	}
	for _, banned := range []rune{'I', 'L', 'O', 'U'} {
		for _, c := range types.Alphabet {
			if c == banned {
				t.Errorf("alphabet contains %q, which is exactly the character it must not", banned)
			}
		}
	}
	seen := map[rune]bool{}
	for _, c := range types.Alphabet {
		if seen[c] {
			t.Errorf("alphabet repeats %q", c)
		}
		seen[c] = true
	}
}

func TestDeriveIsDeterministic(t *testing.T) {
	const addr = "yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p"
	first := types.Derive("NG", addr, 0, types.PayloadLength)
	for i := 0; i < 100; i++ {
		if got := types.Derive("NG", addr, 0, types.PayloadLength); got != first {
			t.Fatalf("derive is not deterministic: %s then %s", first, got)
		}
	}
	// Every validator must agree, so this is the property the whole module
	// rests on; a nondeterministic id would halt the chain.
	if !types.Valid(first) {
		t.Fatalf("derived id %q does not pass its own check", first)
	}
}

func TestDeriveVariesWithAddressNonceAndCountry(t *testing.T) {
	a := types.Derive("NG", "yml1aaa", 0, 8)
	b := types.Derive("NG", "yml1bbb", 0, 8)
	c := types.Derive("NG", "yml1aaa", 1, 8)
	d := types.Derive("GH", "yml1aaa", 0, 8)
	if a == b {
		t.Error("different addresses produced the same id")
	}
	if a == c {
		t.Error("the nonce did not change the id — collision resolution would not terminate")
	}
	// The country is in the pre-image, not only the prefix. Otherwise an account
	// whose jurisdiction is corrected gets its own old payload back under a new
	// prefix, and the retired handle and the live one are one glance apart.
	if a[types.CountryLength:] == d[types.CountryLength:] {
		t.Errorf("a corrected country reused the payload: %s and %s", a, d)
	}
}

func TestDerivedIdentifierCarriesItsCountry(t *testing.T) {
	for _, c := range []string{"NG", "GH", "CI", "SL", "ZZ"} {
		id := types.Derive(c, "yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p", 0, 8)
		if types.Country(id) != c {
			t.Errorf("Derive(%s, …) = %q, whose country reads %q", c, id, types.Country(id))
		}
		if !types.Valid(id) {
			t.Errorf("derived id %q does not pass its own check", id)
		}
	}
}

// CI and CL, SI and SL. Crockford folds I and L onto 1, so a prefix put through
// the payload's normalisation would make Côte d'Ivoire and Chile the same
// perimeter, and Slovenia and Sierra Leone another. This is the test that the
// prefix is left alone.
func TestConfusableCountriesStayDistinct(t *testing.T) {
	const addr = "yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p"
	seen := map[string]string{}
	for _, c := range []string{"CI", "CL", "SI", "SL", "IN", "1N", "ML", "MO", "M0"} {
		id := types.Derive(c, addr, 0, 8)
		if prev, dup := seen[types.Normalise(id)[:types.CountryLength]]; dup {
			t.Errorf("countries %s and %s collapsed onto the same prefix", prev, c)
		}
		seen[types.Normalise(id)[:types.CountryLength]] = c
	}
}

// The country typo the check character exists to catch. NG and NE are
// neighbours in the list and on the map; ML and MZ are one keystroke apart.
// Without the prefix in the sum, every one of these would validate, reach the
// chain, and come back "no such account" — which reads as "that person does not
// exist" rather than "you typed the country wrong".
func TestCheckCatchesAWrongCountry(t *testing.T) {
	const addr = "yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p"
	id := types.Derive("NG", addr, 0, 8)

	for _, wrong := range []string{"NE", "NA", "NI", "MG", "GH", "ZZ"} {
		corrupted := wrong + id[types.CountryLength:]
		if types.Valid(corrupted) {
			t.Errorf("%s misread as %s passed its check: %q", "NG", wrong, corrupted)
		}
	}
}

func TestValidRefusesAPrefixThatIsNotACountry(t *testing.T) {
	const addr = "yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p"
	// A prefix nobody assigned is a perimeter no authority holds, so it is
	// refused before anything looks at the check character.
	for _, nonsense := range []string{"NX", "QK", "XA", "00", "N1"} {
		if types.Valid(types.Derive(nonsense, addr, 0, 8)) {
			t.Errorf("%q was accepted as a country prefix", nonsense)
		}
	}
}

func TestCheckCatchesEverySingleCharacterError(t *testing.T) {
	id := types.Derive("NG", "yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p", 0, 8)

	// Every symbol either alphabet can produce, so the country positions are
	// tested against all 26 letters and not only Crockford's 32.
	symbols := types.Alphabet + "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for pos := 0; pos < len(id); pos++ {
		for i := 0; i < len(symbols); i++ {
			wrong := symbols[i]
			if wrong == id[pos] {
				continue
			}
			corrupted := []byte(id)
			corrupted[pos] = wrong
			if types.Valid(string(corrupted)) {
				t.Errorf("a single wrong character passed: %s -> %s", id, corrupted)
			}
		}
	}
}

func TestCheckCatchesAdjacentTranspositions(t *testing.T) {
	// The error somebody makes reading an id down a phone line, and the reason
	// the check is Luhn rather than a plain weighted sum.
	missed := 0
	for seed := 0; seed < 200; seed++ {
		id := types.Derive("NG", fmt.Sprintf("yml1%d", seed), 0, 8)
		for pos := 0; pos+1 < len(id); pos++ {
			if id[pos] == id[pos+1] {
				continue // swapping equal characters is not an error
			}
			swapped := []byte(id)
			swapped[pos], swapped[pos+1] = swapped[pos+1], swapped[pos]
			if types.Valid(string(swapped)) {
				missed++
			}
		}
	}
	// Luhn mod N misses transpositions of characters differing by exactly half
	// the base; that is a known, bounded gap rather than a bug. Assert it stays
	// small so a regression that breaks the check outright is caught.
	if missed > 200 {
		t.Errorf("check missed %d adjacent transpositions — far more than Luhn's known gap", missed)
	}
	t.Logf("adjacent transpositions missed: %d", missed)
}

func TestNormaliseFoldsTheConfusableCharacters(t *testing.T) {
	id := types.Derive("NG", "yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p", 0, 8)
	formatted := types.Format(id)

	for _, variant := range []string{
		formatted,             // with the hyphens
		id,                    // without
		lower(id),             // typed in lower case
		" " + formatted + " ", // pasted with whitespace
	} {
		if !types.Valid(variant) {
			t.Errorf("valid id rejected in the form %q", variant)
		}
		if types.Normalise(variant) != id {
			t.Errorf("normalise(%q) = %q, want %q", variant, types.Normalise(variant), id)
		}
	}
}

func TestFormatGroupsForReading(t *testing.T) {
	got := types.Format("NGK3M97QRTB")
	if got != "NG-K3M9-7QRT-B" {
		t.Errorf("Format = %q, want NG-K3M9-7QRT-B", got)
	}
}

func TestValidRejectsWrongShapes(t *testing.T) {
	for _, bad := range []string{
		"",
		"NGK3M9",                        // too short
		"NGK3M97QRT",                    // payload with no check character
		"NGK3M97QR!B",                   // symbol outside the alphabet
		"K3M97QRTB",                     // the pre-jurisdiction form, no longer issuable
		"NGK3M97QRTBK3M97QRTBK3M97QRTB", // longer than the maximum
	} {
		if types.Valid(bad) {
			t.Errorf("%q was accepted and should not have been", bad)
		}
	}
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

// The tombstones the migration left behind still have to be recognisable, or
// genesis validation would push an operator to delete them to make the file
// load — freeing a handle somebody memorised to be issued to a stranger.
func TestLegacyIdentifiersAreStillRecognisable(t *testing.T) {
	legacy := "K3M97QRTY"
	if !types.ValidLegacy(legacy) {
		t.Fatalf("the check character for %q is not what the module used to compute", legacy)
	}
	if types.Valid(legacy) {
		t.Errorf("%q is prefixless and must not pass as an issuable identifier", legacy)
	}
	if types.ValidLegacy("K3M97QRTZ") {
		t.Error("a corrupted legacy identifier passed its check")
	}
}
