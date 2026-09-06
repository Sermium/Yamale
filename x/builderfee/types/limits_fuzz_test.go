package types

import (
	"strings"
	"testing"
)

// msg_type_url is a store key chosen by whoever signs a permissionless message,
// so its validator is fuzzed rather than merely unit-tested.
//
// The audit's H-6 was that this field was unbounded and unvalidated: an
// attacker-chosen key of arbitrary length, written into state for one
// transaction fee. ValidateMsgTypeURL is what now stands in front of that, and
// the properties below are the ones the store depends on — not that any
// particular string is rejected, but that nothing which survives validation can
// be a key the module cannot live with.
func FuzzValidateMsgTypeURL(f *testing.F) {
	f.Add("/blockchain.amm.v1.MsgSwap")
	f.Add("/cosmos.bank.v1beta1.MsgSend")
	f.Add("")
	f.Add("/")
	f.Add("//")
	f.Add("cosmos.bank.v1beta1.MsgSend")
	f.Add("/cosmos/bank/v1beta1/MsgSend")
	f.Add("/no_dots_here")
	f.Add("/" + strings.Repeat("a.", 500) + "Msg")
	f.Add("/a." + strings.Repeat("b", MaxMsgTypeURLLength))
	f.Add("/blockchain.amm.v1.MsgSwap\x00")
	f.Add("/blockchain.amm.v1.MsgSwap\n")
	f.Add("/ünïcode.v1.MsgSwap")
	f.Add("/a.b ")
	f.Add(" /a.b")

	f.Fuzz(func(t *testing.T, url string) {
		err := ValidateMsgTypeURL(url)
		if err != nil {
			return
		}

		// Everything below is a property of a url this module has ACCEPTED, and
		// each one is something the store or a later reader relies on.

		if len(url) > MaxMsgTypeURLLength {
			t.Fatalf("accepted a %d-character key; the bound is %d", len(url), MaxMsgTypeURLLength)
		}
		if url == "" {
			t.Fatal("accepted the empty string as a message type")
		}
		if !strings.HasPrefix(url, "/") {
			t.Fatalf("accepted %q, which does not begin with '/'", url)
		}
		// A second slash would make the key look like a path, and the module's
		// own comment is that a type URL is one dotted proto name.
		if strings.Count(url, "/") != 1 {
			t.Fatalf("accepted %q, which holds %d slashes", url, strings.Count(url, "/"))
		}
		if !strings.Contains(url[1:], ".") {
			t.Fatalf("accepted %q, which is not a qualified name", url)
		}

		// The characters. A store key carrying a NUL, a newline or a space is
		// one that renders differently everywhere it is displayed and can be
		// confused with a neighbouring key.
		for _, r := range url[1:] {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '.' || r == '_'
			if !ok {
				t.Fatalf("accepted %q, which holds the character %q", url, r)
			}
		}

		// And validation must be stable: the same input decided the same way
		// twice, or two nodes could disagree about whether a key is legal and
		// the chain would fork on it.
		if second := ValidateMsgTypeURL(url); second != nil {
			t.Fatalf("accepted %q once and refused it the second time: %v", url, second)
		}
	})
}
