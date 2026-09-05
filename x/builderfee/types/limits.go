package types

import (
	"strings"

	errors "cosmossdk.io/errors"
)

// MaxMsgTypeURLLength bounds the one attacker-chosen string in this module.
//
// msg_type_url is the store key for both BuilderApplication and
// ApprovedBuilder, and RegisterBuilder is permissionless, so an unbounded field
// here is an unbounded store key anybody can write. The longest type URL this
// chain actually has is well under a hundred characters; 256 leaves room for
// any module that follows without leaving room for a wall of text.
const MaxMsgTypeURLLength = 256

// ErrInvalidMsgTypeURL is registered in errors.go alongside the rest.

// ValidateMsgTypeURL refuses anything that is not shaped like a message type
// URL this chain could route.
//
// Shape only — whether the type is actually registered is a question for the
// keeper, which holds the interface registry. Checking the shape here means the
// obvious rubbish never reaches state at all.
func ValidateMsgTypeURL(url string) error {
	if url == "" {
		return errors.Wrap(ErrInvalidMsgTypeURL, "a message type URL is required")
	}
	if len(url) > MaxMsgTypeURLLength {
		return errors.Wrapf(ErrInvalidMsgTypeURL,
			"a message type URL may be at most %d characters, this one is %d", MaxMsgTypeURLLength, len(url))
	}
	if !strings.HasPrefix(url, "/") {
		return errors.Wrapf(ErrInvalidMsgTypeURL,
			"a message type URL begins with '/', as in /blockchain.amm.v1.MsgSwap: %q", url)
	}
	body := url[1:]
	if !strings.Contains(body, ".") || strings.Contains(body, "/") {
		return errors.Wrapf(ErrInvalidMsgTypeURL,
			"a message type URL is one dotted proto name, as in /blockchain.amm.v1.MsgSwap: %q", url)
	}
	for _, r := range body {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_':
		default:
			return errors.Wrapf(ErrInvalidMsgTypeURL,
				"a message type URL holds only letters, digits, '.' and '_': %q", url)
		}
	}
	return nil
}
