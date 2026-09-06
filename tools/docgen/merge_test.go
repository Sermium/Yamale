package main

import (
	"strings"
	"testing"
)

// A message is documented twice — on the rpc that carries it and on the message
// itself — and this generator published only one of them for as long as it has
// existed. The bug was invisible while most rpcs had no comment: adding the
// one-line comments buf lint asks for was what made a hundred and thirty-eight
// sentences of written documentation disappear from docs/reference in a single
// regeneration, and reversing the preference deleted the other side instead.
//
// These are the cases that decision has to get right.
func TestMergeDocs(t *testing.T) {
	for _, tc := range []struct {
		name          string
		rpc, msg      string
		wants, avoids []string
		exact         string
	}{
		{
			name:  "neither comment is discarded for the other",
			rpc:   "Sweep collects whatever a passed seizure can now reach. Permissionless and repeatable: funds that were staked arrive later.",
			msg:   "MsgSweep collects what a passed seizure can reach right now.",
			wants: []string{"MsgSweep collects", "Permissionless and repeatable"},
		},
		{
			name:   "an opening that restates the message is dropped, what follows is kept",
			rpc:    "ReleaseEscrow pays the beneficiary. Only the depositor may, and only while no dispute is open.",
			msg:    "MsgReleaseEscrow pays the beneficiary. Only the depositor may send it.",
			wants:  []string{"only while no dispute is open"},
			avoids: []string{"ReleaseEscrow pays the beneficiary. Only the depositor may, and"},
		},
		{
			name: "a second telling of the whole message comment is dropped entire",
			rpc: "UpdateParams sets the module parameters. Governance only. " +
				"It replaces the whole Params object rather than one field. " +
				"A proposal composed without reading the current parameters drops them. " +
				"Nothing catches that, because a shorter list is a valid list.",
			msg: "MsgUpdateParams sets the module parameters. Governance only. " +
				"It replaces the whole Params object rather than one field. " +
				"A proposal composed without reading the current parameters drops them. " +
				"Nothing catches that, because a shorter list is a valid list. " +
				"Read Query/Params first and carry every field across.",
			wants: []string{"Read Query/Params first"},
			// Nothing of the rpc telling survives, and nothing of it should:
			// trimming it to the sentences that happen not to match produces
			// prose that begins in the middle of an argument.
			exact: "MsgUpdateParams sets the module parameters. Governance only. " +
				"It replaces the whole Params object rather than one field. " +
				"A proposal composed without reading the current parameters drops them. " +
				"Nothing catches that, because a shorter list is a valid list. " +
				"Read Query/Params first and carry every field across.",
		},
		{
			// Trimming a scaffolded sentence out of the middle of a comment
			// would leave the remainder opening on an "It" with nothing to
			// refer to, so a comment is only disqualified when it is entirely
			// scaffolding.
			name:   "a scaffolded message comment does not displace a written rpc one",
			rpc:    "ApproveBuilder defines the ApproveBuilder RPC. It is authority-gated and approves a pending registration.",
			msg:    "MsgApproveBuilder is the Msg/ApproveBuilder request type.",
			wants:  []string{"It is authority-gated and approves a pending registration"},
			avoids: []string{"is the Msg/ApproveBuilder request type"},
		},
		{
			// A message whose only documentation is the sentence its generator
			// wrote is an undocumented message, and the reference now says so
			// by printing nothing rather than a line that restates the heading.
			name:  "a comment that is only scaffolding is not documentation",
			rpc:   "CreatePool defines the CreatePool RPC.",
			msg:   "",
			exact: "",
		},
		{
			name:  "a message with no rpc comment keeps its own",
			rpc:   "",
			msg:   "MsgSwap exchanges one denom for another.",
			wants: []string{"MsgSwap exchanges one denom for another."},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeDocs(tc.rpc, tc.msg)
			if tc.exact != "" || len(tc.wants) == 0 {
				if got != tc.exact {
					t.Errorf("want exactly:\n%q\ngot:\n%q", tc.exact, got)
				}
			}
			for _, want := range tc.wants {
				if !strings.Contains(got, want) {
					t.Errorf("lost documentation.\nwant to contain: %q\ngot: %q", want, got)
				}
			}
			for _, avoid := range tc.avoids {
				if strings.Contains(got, avoid) {
					t.Errorf("published a restatement.\nshould not contain: %q\ngot: %q", avoid, got)
				}
			}
		})
	}
}

// Prose wrapped at eighty columns is rejoined; a bullet, a numbered item or a
// heading is not. Joining those onto the sentence above them turned three
// numbered consequences into one run-on paragraph in the published reference.
func TestCleanCommentKeepsMarkdownStructure(t *testing.T) {
	got := cleanComment(strings.Join([]string{
		"Two signers, and which one is allowed depends on the scope being",
		"granted:",
		"",
		"- a country scope may be granted by governance or by the foundation;",
		"- the chain-wide scope may be granted by governance and nobody else.",
		"",
		"# Why the foundation, when this used to be governance alone",
		"",
		"1. It can grant one office the same role in every country, one grant",
		"   at a time.",
		"2. It can revoke a country grant governance made.",
	}, "\n"))

	for _, want := range []string{
		"depends on the scope being granted:", // rejoined
		"\n- a country scope",                 // its own line
		"\n- the chain-wide scope",            // and not glued to the one above
		"\n#### Why the foundation",           // demoted below the "###" a message gets
		"1. It can grant one office the same role in every country, one grant at a time.",
		"\n2. It can revoke a country grant governance made.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want to contain %q\ngot:\n%s", want, got)
		}
	}

	// A heading in a comment must never outrank the page it is printed on.
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "####") {
			t.Errorf("heading not demoted below the message it sits under: %q", line)
		}
	}
}
