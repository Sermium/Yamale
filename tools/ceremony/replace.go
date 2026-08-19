package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/group"
)

// buildReplacement produces the one message a custodian's departure is allowed
// to be: the outgoing custodian removed and the incoming one added, together.
//
// There is no function here that removes a custodian on their own, and that is
// the whole design. x/group's MsgUpdateGroupMembers takes adds and removes in
// the same list, so the swap is native rather than a convention — but "native"
// is not "enforced", and a tool that offered a bare removal as a convenience
// would be the thing an operator reaches for at four in the afternoon when the
// replacement has not been appointed yet.
//
// What goes wrong if they do is quiet and it ratchets. Three of five is sixty
// per cent; three of four is seventy-five, so everyone who stayed now holds
// more of the authority than the ceremony gave them. Lose another and it is
// three of three, where any one custodian can veto and one who cannot be
// reached freezes the account the chain is still sending seized property into.
// Nobody decides on that. It is arrived at by two individually reasonable
// decisions taken months apart.
//
// The chain refuses a bare removal too — see x/constitution's ante gate — so
// this is the second of two locks rather than the only one. Both exist because
// the two fail differently: the gate cannot explain itself at the moment
// somebody is drafting the proposal, and this cannot stop a proposal drafted by
// hand.
func buildReplacement(
	custodians []identity,
	outgoing identity,
	incoming identity,
	policyAddress string,
	groupID uint64,
) ([]byte, []byte, error) {
	if incoming.Role != roleCustodian {
		return nil, nil, fmt.Errorf(
			"%s is recorded as %q; an incoming custodian's key comes from the same ceremony as everybody else's",
			incoming.Name, incoming.Role)
	}
	if _, err := sdk.AccAddressFromBech32(incoming.Address); err != nil {
		return nil, nil, fmt.Errorf("%s has an address this chain cannot read: %w", incoming.Name, err)
	}

	var found bool
	for _, custodian := range custodians {
		if custodian.Address == outgoing.Address {
			found = true
		}
		if custodian.Address == incoming.Address {
			return nil, nil, fmt.Errorf(
				"%s is already a custodian, so this is a removal wearing a swap's clothes: the group would end up one member short",
				incoming.Name)
		}
	}
	if !found {
		return nil, nil, fmt.Errorf(
			"%s (%s) is not in this group, so there is nobody to replace",
			outgoing.Name, outgoing.Address)
	}

	update := &group.MsgUpdateGroupMembers{
		Admin:   policyAddress,
		GroupId: groupID,
		MemberUpdates: []group.MemberRequest{
			// Weight zero is x/group's removal. Ordered outgoing first purely
			// so the proposal reads the way the decision was taken.
			{Address: outgoing.Address, Weight: "0"},
			{Address: incoming.Address, Weight: "1", Metadata: fmt.Sprintf("%s (%s)", incoming.Name, incoming.Fingerprint)},
		},
	}

	registry := codectypes.NewInterfaceRegistry()
	group.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	proposal, err := group.NewMsgSubmitProposal(
		policyAddress,
		// The proposer is filled in by whichever custodian submits it: the CLI
		// takes the first proposer as --from, and naming somebody here would
		// mean the document only works if that person is the one at a keyboard.
		[]string{"<proposing custodian's address>"},
		[]sdk.Msg{update},
		fmt.Sprintf("%s replaces %s as a custodian", incoming.Name, outgoing.Name),
		group.Exec_EXEC_UNSPECIFIED,
		fmt.Sprintf("Replace custodian %s with %s", outgoing.Name, incoming.Name),
		fmt.Sprintf(
			"%s is stepping down as a custodian of the foundation account. %s takes their place, "+
				"with a key generated through the same ceremony and fingerprint %s. "+
				"Removal and replacement are one decision: the group is always exactly %d custodians.",
			outgoing.Name, incoming.Name, incoming.Fingerprint, len(custodians)),
	)
	if err != nil {
		return nil, nil, err
	}

	updateJSON, err := cdc.MarshalJSON(update)
	if err != nil {
		return nil, nil, err
	}
	proposalJSON, err := cdc.MarshalJSON(proposal)
	if err != nil {
		return nil, nil, err
	}
	return updateJSON, proposalJSON, nil
}

func runReplace(args []string) error {
	flags := flag.NewFlagSet("replace-custodian", flag.ExitOnError)
	outgoingPath := flags.String("outgoing", "", "the departing custodian's public record")
	incomingPath := flags.String("incoming", "", "the incoming custodian's public record, from their own ceremony")
	groupID := flags.Uint64("group-id", 1, "the foundation group's id")
	seq := flags.Uint64("seq", 1, "group policy sequence number, for deriving the policy address")
	out := flags.String("out", ".", "directory for the proposal")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *outgoingPath == "" || *incomingPath == "" {
		return fmt.Errorf(
			"--outgoing and --incoming are both required.\n" +
				"A custodian's departure and their replacement are one decision, so this command has no\n" +
				"way to express only half of it. If the replacement has not been appointed yet, the\n" +
				"departure has not been decided yet either")
	}
	if flags.NArg() == 0 {
		return fmt.Errorf("give the current custodians' public record files as well, so the group can be checked")
	}

	custodians, err := readIdentities(flags.Args())
	if err != nil {
		return err
	}
	outgoing, err := readIdentities([]string{*outgoingPath})
	if err != nil {
		return err
	}
	incoming, err := readIdentities([]string{*incomingPath})
	if err != nil {
		return err
	}

	policyAddr, err := policyAddress(*seq)
	if err != nil {
		return err
	}

	update, proposal, err := buildReplacement(custodians, outgoing[0], incoming[0], policyAddr, *groupID)
	if err != nil {
		return err
	}

	files := map[string][]byte{
		"replace-custodian-update.json":   update,
		"replace-custodian-proposal.json": proposal,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(*out, name), append(data, '\n'), 0o644); err != nil {
			return err
		}
	}

	c := stdConsole()
	c.printf("=== replacing a custodian in group %d ===\n\n", *groupID)
	c.printf("  out  %s\n", outgoing[0].describe())
	c.printf("  in   %s\n", incoming[0].describe())
	c.println()
	c.printf("  the group stays at %d custodians\n", len(custodians))
	c.printf("  policy %s\n", policyAddr)
	c.println()
	c.println("Written replace-custodian-proposal.json. Fill in the proposer's address and submit:")
	c.println()
	c.println("    blockchaind tx group submit-proposal replace-custodian-proposal.json")
	c.println()
	c.println("Both halves are in one message on purpose, and the chain enforces it: a proposal")
	c.println("that only removed somebody is refused by the ante gate, because it would move the")
	c.println("rule from three of five to three of four without anybody deciding to.")
	c.println()
	c.println("For the ceremony record, under notes:")
	c.printf("  %s\n", describeSwap(outgoing[0], incoming[0]))
	return nil
}

// describeSwap is what the ceremony record says about a replacement. Kept
// beside the builder so the wording and the message cannot drift apart.
func describeSwap(outgoing, incoming identity) string {
	return strings.TrimSpace(fmt.Sprintf(
		"%s (%s) replaced %s (%s) as a custodian; the group remained the same size throughout.",
		incoming.Name, incoming.Fingerprint, outgoing.Name, outgoing.Fingerprint,
	))
}
