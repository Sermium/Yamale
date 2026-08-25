// Package legacyparams reads a field out of a stored protobuf message that the
// current Go type no longer has.
//
// It exists for exactly one situation and should never grow past it: a module
// retires a parameter, reserves its field number so nobody can reuse it, and
// then has to migrate the value the running chain still holds.
//
// The trap it answers is quiet. A reserved field is one the generated Go type
// does not have, and this repository's gogoproto build does not keep unknown
// fields either — so unmarshalling the stored parameters with the new type drops
// the retired value with no error at all, because discarding an unknown field is
// what a protobuf decoder is supposed to do. A migration written the obvious way
// therefore reads back a zero value, carries nothing across, logs success, and
// removes an authority from a live chain. That has to be read from the bytes.
//
// One implementation rather than one per module, because two hand-written wire
// scanners is two chances to skip a field wrongly, and the failure of a wire
// scanner is not a panic — it is a plausible string read out of the wrong
// offset.
package legacyparams

import (
	"encoding/binary"
	"fmt"
)

// Protobuf wire types. Only the four that carry a length this scanner can
// measure are handled; see the default branch of Strings.
const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireFixed32 = 5
)

// Strings returns every length-delimited value carried by one field number, in
// the order they appear.
//
// That covers both shapes a retired string parameter can have. A repeated string
// yields its whole list. A singular proto3 string yields nothing when it was
// never set, and — by the encoding rules rather than by an accident of this
// implementation — the LAST entry is the value a conformant decoder would have
// produced if a writer ever emitted the field twice. See Last.
//
// Nil or empty input is not an error and yields nothing. A chain whose
// parameters were never written has no retired value to carry, and a scanner
// that refused it would refuse on every fresh chain in every test.
//
// It never allocates from a length it has not checked against the bytes
// remaining, and it refuses a wire type it cannot measure rather than skipping
// one. A length prefix trusted out of a truncated record is the standard way a
// decoder like this becomes a panic, and a wire type guessed at is how it starts
// reading whatever alignment happens to fall out.
func Strings(raw []byte, field uint64) ([]string, error) {
	var found []string
	for len(raw) > 0 {
		tag, read := binary.Uvarint(raw)
		if read <= 0 {
			return nil, fmt.Errorf("malformed field tag")
		}
		raw = raw[read:]

		at, wire := tag>>3, tag&7
		switch wire {
		case wireVarint:
			_, read := binary.Uvarint(raw)
			if read <= 0 {
				return nil, fmt.Errorf("malformed varint in field %d", at)
			}
			raw = raw[read:]
		case wireFixed64:
			if len(raw) < 8 {
				return nil, fmt.Errorf("truncated 64-bit value in field %d", at)
			}
			raw = raw[8:]
		case wireBytes:
			length, read := binary.Uvarint(raw)
			if read <= 0 {
				return nil, fmt.Errorf("malformed length prefix in field %d", at)
			}
			raw = raw[read:]
			if length > uint64(len(raw)) {
				return nil, fmt.Errorf("field %d claims %d bytes and %d remain", at, length, len(raw))
			}
			value := raw[:length]
			raw = raw[length:]
			if at == field {
				found = append(found, string(value))
			}
		case wireFixed32:
			if len(raw) < 4 {
				return nil, fmt.Errorf("truncated 32-bit value in field %d", at)
			}
			raw = raw[4:]
		default:
			return nil, fmt.Errorf("field %d uses wire type %d, which this scanner cannot skip", at, wire)
		}
	}
	return found, nil
}

// Last returns the value a singular proto3 string field would have decoded to,
// and whether the field was present at all.
//
// Last rather than first, because that is what protobuf says: when a field
// appears more than once in a message, the last occurrence wins for a scalar.
// Nothing this repository writes emits one twice, and the point of matching the
// rule anyway is that a migration must read what a decoder would have read, not
// what a reasonable person would have written.
//
// The boolean is separate from the string because an empty value and an absent
// field are different states, and the whole reason this package exists is that
// the difference between those two decides whether an authority existed.
func Last(raw []byte, field uint64) (string, bool, error) {
	found, err := Strings(raw, field)
	if err != nil {
		return "", false, err
	}
	if len(found) == 0 {
		return "", false, nil
	}
	return found[len(found)-1], true, nil
}
