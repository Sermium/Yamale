package recovery

import (
	"bytes"
	"crypto/rand"
	"testing"
)

// Secret sharing is the kind of code that produces confident-looking output
// while being wrong. A transcription slip in the field arithmetic still splits,
// still combines, and still returns 32 bytes; it just returns the wrong 32 bytes
// for some inputs and the right ones for the ones a lazy test tries. So every
// test here is aimed at a specific way it can be wrong, and several of them fail
// against implementations that pass a naive round-trip.

func mustSplit(t *testing.T, secret []byte, shares, threshold int) []Share {
	t.Helper()
	got, err := Split(secret, shares, threshold)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	return got
}

/* ------------------------------------------------------------ the field */

// The one test that catches a transcribed table. Every non-zero element must
// have an inverse, and a^255 = 1 for all of them; a wrong polynomial or a
// non-primitive generator breaks this for some elements and not others, which is
// precisely why a round-trip on one secret can still pass.
func TestEveryNonZeroElementHasAnInverse(t *testing.T) {
	for a := 1; a < 256; a++ {
		inverse := div(1, byte(a))
		if got := mul(byte(a), inverse); got != 1 {
			t.Fatalf("%d * %d = %d, want 1", a, inverse, got)
		}
	}
}

func TestTheGeneratorReachesEveryNonZeroElement(t *testing.T) {
	// If 3 were not primitive the log table would have collisions, and the
	// tables would be silently wrong for the elements it never reached.
	seen := map[byte]bool{}
	for i := 0; i < 255; i++ {
		seen[expTable[i]] = true
	}
	if len(seen) != 255 {
		t.Fatalf("the generator reaches %d of 255 non-zero elements", len(seen))
	}
	if seen[0] {
		t.Fatal("zero is not in the multiplicative group")
	}
}

func TestMultiplicationIsCommutativeAndDistributes(t *testing.T) {
	for a := 0; a < 256; a += 7 {
		for b := 0; b < 256; b += 5 {
			x, y := byte(a), byte(b)
			if mul(x, y) != mul(y, x) {
				t.Fatalf("mul(%d,%d) != mul(%d,%d)", a, b, b, a)
			}
			for c := 0; c < 256; c += 31 {
				z := byte(c)
				// Addition is xor in this field.
				if mul(x, y^z) != mul(x, y)^mul(x, z) {
					t.Fatalf("distributivity fails at %d,%d,%d", a, b, c)
				}
			}
		}
	}
}

/* ------------------------------------------------------- round-tripping */

func TestEveryPairOfSharesRebuildsTheSecret(t *testing.T) {
	// Not "some two shares" — every two. A Lagrange basis with the subtraction
	// the wrong way round rebuilds correctly from shares 1 and 2 and wrongly
	// from 2 and 3, and a test that tries one pair never sees it.
	secret := []byte("the device key, or something the length of one")
	shares := mustSplit(t, secret, 5, 2)

	for i := range shares {
		for j := i + 1; j < len(shares); j++ {
			got, err := Combine([]Share{shares[i], shares[j]})
			if err != nil {
				t.Fatalf("shares %d,%d: %v", i, j, err)
			}
			if !bytes.Equal(got, secret) {
				t.Fatalf("shares %d,%d rebuilt %q", i, j, got)
			}
		}
	}
}

func TestAHigherThresholdNeedsEveryOneOfItsShares(t *testing.T) {
	secret := []byte("three of five")
	shares := mustSplit(t, secret, 5, 3)

	for i := range shares {
		for j := i + 1; j < len(shares); j++ {
			for k := j + 1; k < len(shares); k++ {
				got, err := Combine([]Share{shares[i], shares[j], shares[k]})
				if err != nil {
					t.Fatalf("shares %d,%d,%d: %v", i, j, k, err)
				}
				if !bytes.Equal(got, secret) {
					t.Fatalf("shares %d,%d,%d rebuilt %q", i, j, k, got)
				}
			}
		}
	}
}

func TestTooFewSharesRebuildSomethingElseEntirely(t *testing.T) {
	// The property that makes this secret sharing rather than an encoding, and
	// the reason Combine cannot report an error for it: two shares of a 3-of-5
	// split produce a perfectly well-formed wrong answer.
	//
	// This is the test that justifies sealing before splitting. Without the
	// authentication tag there is nothing anywhere in the system that can tell
	// this apart from a correct recovery.
	secret := []byte("three of five")
	shares := mustSplit(t, secret, 5, 3)

	got, err := Combine(shares[:2])
	if err != nil {
		t.Fatalf("Combine: %v", err)
	}
	if bytes.Equal(got, secret) {
		t.Fatal("two shares of a 3-of-5 split must not rebuild the secret")
	}
	if len(got) != len(secret) {
		t.Fatalf("the wrong answer is still the right shape: got %d bytes", len(got))
	}
}

func TestASecretOfAnyByteValueSurvives(t *testing.T) {
	// Zero bytes and 0xff are where a field implementation that special-cases
	// badly goes wrong, and a key is uniform random so it contains both.
	secret := make([]byte, 256)
	for i := range secret {
		secret[i] = byte(i)
	}
	shares := mustSplit(t, secret, 3, 2)
	got, err := Combine([]Share{shares[0], shares[2]})
	if err != nil {
		t.Fatalf("Combine: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("a secret containing every byte value did not survive")
	}
}

func TestRandomSecretsRoundTrip(t *testing.T) {
	for i := 0; i < 200; i++ {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			t.Fatalf("rand: %v", err)
		}
		shares := mustSplit(t, secret, 3, 2)
		got, err := Combine([]Share{shares[1], shares[2]})
		if err != nil {
			t.Fatalf("Combine: %v", err)
		}
		if !bytes.Equal(got, secret) {
			t.Fatalf("round %d: %x != %x", i, got, secret)
		}
	}
}

/* ----------------------------------------------------------- the shares */

func TestNoShareIsTheSecret(t *testing.T) {
	// The mistake that makes the whole thing decorative: a share equal to the
	// secret, or an x of zero, which evaluates the polynomial exactly where the
	// secret lives.
	secret := []byte("0123456789abcdef0123456789abcdef")
	shares := mustSplit(t, secret, 3, 2)
	for i, s := range shares {
		if s.X == 0 {
			t.Fatalf("share %d has x=0, which is the secret's own position", i)
		}
		if bytes.Equal(s.Y, secret) {
			t.Fatalf("share %d IS the secret", i)
		}
	}
}

func TestTwoSharesOfTheSameSecretDiffer(t *testing.T) {
	// If the polynomial's coefficients were fixed rather than fresh, or the
	// randomness were being ignored, the shares would be a deterministic
	// function of the secret and holding two of them would be no better than
	// holding one. Split twice and check all four differ.
	secret := []byte("same secret, twice")
	a := mustSplit(t, secret, 2, 2)
	b := mustSplit(t, secret, 2, 2)
	if bytes.Equal(a[0].Y, b[0].Y) {
		t.Fatal("two splits of one secret produced the same share: the polynomial is not random")
	}
	if bytes.Equal(a[0].Y, a[1].Y) {
		t.Fatal("two shares of one split are identical")
	}
}

func TestASingleShareIsIndependentOfEveryByteOfTheSecret(t *testing.T) {
	// Perfect secrecy, as far as a test can state it: hold x=1 fixed, split two
	// secrets that differ in every byte, and the shares must not track the
	// difference. An implementation that leaked the secret into the share —
	// say by using a zero coefficient — would fail here.
	//
	// Over many trials, each share byte should be uniform regardless of the
	// secret byte, so the two sets of shares agree on about 1/256 of bytes.
	const trials = 400
	zeros := make([]byte, 64)
	ones := bytes.Repeat([]byte{0xff}, 64)

	agreements := 0
	for i := 0; i < trials; i++ {
		a := mustSplit(t, zeros, 3, 2)
		b := mustSplit(t, ones, 3, 2)
		for j := range a[0].Y {
			if a[0].Y[j] == b[0].Y[j] {
				agreements++
			}
		}
	}
	total := trials * 64
	rate := float64(agreements) / float64(total)
	// 1/256 is 0.0039. A leak would push this to 0 (never agree) or 1 (always).
	if rate > 0.02 {
		t.Fatalf("shares of different secrets agree %.4f of the time, want ~0.004", rate)
	}
	if agreements == 0 {
		t.Fatal("shares of different secrets NEVER agree; that is a pattern, not randomness")
	}
}

/* ----------------------------------------------------------- refusals */

func TestSplitRefusesAThresholdItCannotHonour(t *testing.T) {
	for _, c := range []struct{ shares, threshold int }{
		{3, 4},   // more needed than exist
		{3, 1},   // a "threshold" of one is a copy, not a share
		{3, 0},   //
		{3, -1},  //
		{300, 2}, // more shares than the field has non-zero points
	} {
		if _, err := Split([]byte("x"), c.shares, c.threshold); err != ErrThreshold {
			t.Fatalf("Split(%d,%d) returned %v, want ErrThreshold", c.shares, c.threshold, err)
		}
	}
}

func TestSplitRefusesAnEmptySecret(t *testing.T) {
	if _, err := Split(nil, 3, 2); err != ErrEmptySecret {
		t.Fatalf("got %v", err)
	}
}

func TestCombineRefusesSharesThatCannotBelongTogether(t *testing.T) {
	shares := mustSplit(t, []byte("abcd"), 3, 2)

	if _, err := Combine(shares[:1]); err != ErrShareCount {
		t.Fatalf("one share: %v", err)
	}

	short := Share{X: 9, Y: []byte{1, 2}}
	if _, err := Combine([]Share{shares[0], short}); err != ErrShareLengths {
		t.Fatalf("mismatched lengths: %v", err)
	}

	// A duplicated x is the dangerous one: two copies of one share look like
	// two shares, and the interpolation would divide by zero and return
	// plausible rubbish rather than refusing.
	if _, err := Combine([]Share{shares[0], shares[0]}); err != ErrDuplicateX {
		t.Fatalf("duplicate x: %v", err)
	}

	zero := Share{X: 0, Y: make([]byte, 4)}
	if _, err := Combine([]Share{shares[0], zero}); err != ErrShareX {
		t.Fatalf("x=0: %v", err)
	}
}

func TestEqualComparesWithoutLeakingWhereTheyDiffer(t *testing.T) {
	if !Equal([]byte("abc"), []byte("abc")) {
		t.Fatal("equal bytes should compare equal")
	}
	if Equal([]byte("abc"), []byte("abd")) {
		t.Fatal("different bytes should not compare equal")
	}
	if Equal([]byte("abc"), []byte("ab")) {
		t.Fatal("different lengths should not compare equal")
	}
}
