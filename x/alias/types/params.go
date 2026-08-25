package types

import "fmt"

// DefaultParams returns the module's default parameters.
func DefaultParams() Params {
	return Params{PayloadLength: PayloadLength}
}

// NewParams constructs Params.
//
// It used to take the foundation administrators as a variadic tail. They are a
// role grant now — see ROLE_FOUNDATION_ADMINISTRATOR — so this takes the one
// parameter the module still has, and a caller that used to pass a list gets a
// compile error rather than a silently ignored argument.
func NewParams(payloadLength uint32) Params {
	return Params{PayloadLength: payloadLength}
}

// Validate bounds the identifier length.
//
// Below eight characters the space stops being large enough to be unguessable;
// above sixteen nobody can read one aloud, which is the entire point of the
// module. Both ends are refused rather than clamped: a governance proposal that
// silently became something other than what was voted on is worse than one that
// fails.
//
// It no longer validates a list of foundation administrators, and the three
// things that check used to do are now done by the grant registry rather than
// by this function — which is most of the reason for moving them:
//
//   - the cap is enforced by GrantRole and by GenesisState.Validate, over the
//     chain-wide grants of ROLE_FOUNDATION_ADMINISTRATOR. See
//     MaxFoundationAdministrators.
//   - duplicates are impossible: a grant is keyed by (holder, role,
//     jurisdiction), so granting the same triple twice writes one record.
//   - an entry that is not an address is refused, which this function never
//     managed. It checked for an empty string and nothing else, so a mistyped
//     address passed a governance vote, occupied one of the eight places and
//     granted the exemption to nobody. GrantRole decodes the holder.
func (p Params) Validate() error {
	if p.PayloadLength < MinPayloadLen || p.PayloadLength > MaxPayloadLen {
		return fmt.Errorf("payload_length must be between %d and %d, got %d",
			MinPayloadLen, MaxPayloadLen, p.PayloadLength)
	}
	return nil
}
