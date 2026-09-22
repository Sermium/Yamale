package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/herd/keeper"
	"yamale/blockchain/x/herd/types"
)

// The register's one promise.
//
// `ByNationalID makes an identifier mean one animal. It is the key printed on a
// certificate.` That was false while the write did nothing but TrimSpace: two
// spellings of the same identifier were two animals, and neither query could
// see the other.

func TestTwoSpellingsOfOneIdentifierCanonicaliseTogether(t *testing.T) {
	// The bug, stated as the thing it broke. A serial typed without its leading
	// zeros is the same animal, and before this it was a second record with a
	// second certificate and no sign the first existed.
	withZeros, err := keeper.CanonicalNationalID("221/682/001029/L")
	require.NoError(t, err)
	without, err := keeper.CanonicalNationalID("221/682/1029/L")
	require.NoError(t, err)
	require.Equal(t, withZeros, without,
		"a serial is a fixed-width field; leading zeros carry nothing")
	require.Equal(t, "221/682/001029/L", withZeros)
}

func TestACertificateSpellingSurvivesUnchanged(t *testing.T) {
	// The common case must be a no-op, or every existing record's index key
	// moves and the fix is worse than the fault.
	for _, id := range []string{
		"221/682/000102/L", "221/682/000103/L", "221/682/021029/L",
	} {
		got, err := keeper.CanonicalNationalID(id)
		require.NoError(t, err)
		require.Equal(t, id, got, "already canonical identifiers must not move")
	}
}

func TestTheBreedLetterIsCaseInsensitive(t *testing.T) {
	upper, err := keeper.CanonicalNationalID("221/682/001029/L")
	require.NoError(t, err)
	lower, err := keeper.CanonicalNationalID("221/682/001029/l")
	require.NoError(t, err)
	require.Equal(t, upper, lower)
}

func TestASerialTooLongIsRefusedRatherThanTruncated(t *testing.T) {
	// The one outcome worse than a rejection: dropping a digit produces a
	// valid-looking identifier that belongs to a different animal.
	_, err := keeper.CanonicalNationalID("221/682/0001029/L")
	require.ErrorIs(t, err, types.ErrInvalidNationalID)
}

func TestAMalformedIdentifierIsRefused(t *testing.T) {
	for _, bad := range []string{
		"221/682/001029",     // three parts
		"221/682/001029/L/X", // five
		"221/68A/001029/L",   // a letter where digits belong
		"221//001029/L",      // an empty part
		"   ",                // nothing at all
		"not an identifier",
	} {
		_, err := keeper.CanonicalNationalID(bad)
		require.Error(t, err, "%q should be refused", bad)
	}
}

func TestAnEmptyIdentifierSaysWhichThingIsMissing(t *testing.T) {
	_, err := keeper.CanonicalNationalID("")
	require.ErrorIs(t, err, types.ErrNoNationalID)
}

/* ------------------------------------------------ ISO 11784/11785 tags */

func TestAReadersSpacingIsTheSameTransponder(t *testing.T) {
	// A reader prints groups; an operator types none; a form inserts dots. All
	// one animal, and before this all different animals.
	want := "982000000000003"
	for _, spelling := range []string{
		"982000000000003",
		"982 000 000 000 003",
		"982.000.000.000.003",
		"982-000-000-000-003",
		"  982000000000003  ",
	} {
		got, err := keeper.CanonicalTag(spelling)
		require.NoError(t, err, spelling)
		require.Equal(t, want, got, "%q is the same transponder", spelling)
	}
}

func TestATagOfTheWrongLengthIsRefused(t *testing.T) {
	// Fourteen digits is a misread, not a shorter code, and storing it would
	// index an animal under something no reader will ever return again.
	for _, bad := range []string{"98200000000000", "9820000000000034", "982"} {
		_, err := keeper.CanonicalTag(bad)
		require.ErrorIs(t, err, types.ErrInvalidTag, bad)
	}
}

func TestATagWithLettersIsRefused(t *testing.T) {
	_, err := keeper.CanonicalTag("982X00000000003")
	require.ErrorIs(t, err, types.ErrInvalidTag)
}

func TestAnAnimalWithNoTagIsNotAnError(t *testing.T) {
	// Level 1 has no transponder, and the keeper deliberately does not index an
	// empty tag — indexing it would collide every untagged animal with every
	// other.
	got, err := keeper.CanonicalTag("")
	require.NoError(t, err)
	require.Equal(t, "", got)

	got, err = keeper.CanonicalTag("   ")
	require.NoError(t, err)
	require.Equal(t, "", got)
}

func TestTheChainAgreesWithWhatTheClientCanonicalises(t *testing.T) {
	// clients/herd/src/identity.ts does the same work before signing, and the
	// two must agree: a client that canonicalises one way and a chain that
	// indexes another is a subtler version of the bug this file fixes.
	//
	// These pairs are taken from that file's own tests.
	cases := map[string]string{
		"221/682/1029/L":   "221/682/001029/L",
		"221/682/021029/l": "221/682/021029/L",
		" 221/682/12/L ":   "221/682/000012/L",
	}
	for in, want := range cases {
		got, err := keeper.CanonicalNationalID(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
}
