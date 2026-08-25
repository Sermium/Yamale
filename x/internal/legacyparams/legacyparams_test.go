package legacyparams_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/internal/legacyparams"
)

// A wire scanner is a thing worth testing against the bytes it will actually
// meet and against the bytes that would make it lie.
//
// The failure mode it exists to prevent is not a panic: it is a plausible string
// read out of the wrong offset. So the cases below are mostly about the fields
// it must SKIP correctly — a scanner that mis-measured a varint or a fixed-width
// value would return whatever fell out of the misalignment, and a migration
// would grant an authority to it.

// tag builds a protobuf field tag byte for a single-byte field number.
func tag(field, wire byte) byte { return field<<3 | wire }

// str encodes a length-delimited field. Only short values, which is all an
// address is.
func str(field byte, value string) []byte {
	out := []byte{tag(field, 2), byte(len(value))}
	return append(out, value...)
}

func TestStringsFindsEveryOccurrenceOfOneField(t *testing.T) {
	raw := []byte{tag(1, 0), 8} // payload_length = 8
	raw = append(raw, str(2, "first")...)
	raw = append(raw, str(2, "second")...)
	raw = append(raw, str(3, "not this one")...)

	found, err := legacyparams.Strings(raw, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, found)
}

// The four wire types it can measure, each carrying a value it must step over
// without reading. A scanner that skipped any of them by the wrong number of
// bytes would find the target field at an offset inside somebody else's value.
func TestStringsSkipsEveryWireTypeItClaimsTo(t *testing.T) {
	raw := []byte{tag(1, 0), 0xAC, 0x02} // a two-byte varint
	raw = append(raw, tag(4, 1))         // fixed64
	raw = append(raw, 1, 2, 3, 4, 5, 6, 7, 8)
	raw = append(raw, tag(5, 5)) // fixed32
	raw = append(raw, 9, 10, 11, 12)
	raw = append(raw, str(6, "somebody else's string")...)
	raw = append(raw, str(8, "the one being looked for")...)

	found, err := legacyparams.Strings(raw, 8)
	require.NoError(t, err)
	require.Equal(t, []string{"the one being looked for"}, found)
}

// Nothing is not an error. A chain whose parameters were never written has no
// retired value to carry, and a scanner that refused it would refuse on every
// fresh chain in every test.
func TestStringsAcceptsNothing(t *testing.T) {
	for _, raw := range [][]byte{nil, {}} {
		found, err := legacyparams.Strings(raw, 2)
		require.NoError(t, err)
		require.Empty(t, found)
	}
}

// Every way the bytes can be wrong is an error rather than a guess. A length
// prefix trusted out of a truncated record is the standard way a decoder like
// this becomes a panic, and a wire type guessed at is how it starts reading
// whatever alignment falls out.
func TestStringsRefusesBytesItCannotMeasure(t *testing.T) {
	for name, raw := range map[string][]byte{
		"length longer than what remains": {tag(2, 2), 40, 'a', 'b'},
		"truncated fixed64":               {tag(4, 1), 1, 2, 3},
		"truncated fixed32":               {tag(5, 5), 1, 2},
		"wire type 3, a group":            {tag(6, 3)},
		"wire type 4, a group end":        {tag(6, 4)},
		"varint that never terminates": {
			tag(1, 0), 0x80, 0x80, 0x80,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := legacyparams.Strings(raw, 2)
			require.Error(t, err)
		})
	}
}

// Last is what a conformant decoder would have produced for a singular proto3
// string, which is the last occurrence rather than the first.
//
// Nothing this repository writes emits one twice. Matching the rule anyway is
// the point: a migration must read what a decoder would have read, not what a
// reasonable person would have written.
func TestLastTakesTheLastOccurrenceAndReportsAbsence(t *testing.T) {
	raw := append(str(8, "an older address"), str(8, "the one in force")...)
	value, found, err := legacyparams.Last(raw, 8)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "the one in force", value)

	// Absent and empty are different states, which is the whole reason the
	// boolean is separate from the string: the difference between them decides
	// whether an authority existed.
	_, found, err = legacyparams.Last(raw, 9)
	require.NoError(t, err)
	require.False(t, found)

	_, found, err = legacyparams.Last(str(8, ""), 8)
	require.NoError(t, err)
	require.True(t, found, "a field that was set to the empty string was still set")
}
