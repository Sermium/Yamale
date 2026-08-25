package main

// The foundation-administrator ceremony: putting the power to correct a recorded
// country into an M-of-N group, and getting that group appointed.
//
// It is the third sibling of the foundation and country ceremonies, and it shares
// their machinery rather than copying it. The keys, the possession signatures, the
// group assembly and the fingerprint everybody reads to each other are the same
// code — `ceremony host` generates an administrator group's keys in each
// custodian's own browser exactly as it generates the foundation's. What differs
// is the far end, and the difference is the reason this file exists.
//
// # Why it ends at a governance proposal
//
// The foundation ceremony ends at a genesis fragment, because the foundation has
// to exist at height zero. The country ceremony ends at a proposal to the
// FOUNDATION's group, because the constitution names the foundation as the
// account that may admit a country.
//
// An administrator is appointed by neither. ROLE_FOUNDATION_ADMINISTRATOR is a
// role grant at the chain-wide scope, and x/alias refuses that scope from every
// signer but the governance module account — assertMayGrant refuses "*" before it
// has even consulted the constitution. The foundation's own 3-of-5 may grant a
// role inside a country and may not grant this one. So this ceremony ends at a
// document for `blockchaind tx gov submit-proposal`, decided by the whole voting
// set over the full voting period.
//
// That is not a detail of plumbing. It is the answer to "who decides who can move
// a customer out from under their regulator", and the answer is not the
// foundation.
//
// The appointment used to be a MsgUpdateParams rewriting a repeated field on
// x/alias's parameters, and the move to a grant did not move that authority by an
// inch: it was governance-only then because the parameter's authority was the
// governance module account, and it is governance-only now because the chain-wide
// scope is. What changed is everything downstream of that — see
// appointmentProposal, where an appointment stopped being a rewrite of every
// parameter x/alias has.
//
// # Why it is two phases, and why the first cannot be skipped
//
// The same reason as the country enrolment, with one worse consequence. An
// x/group policy address derives from the group policy sequence number alone —
// not from the members, not from the threshold, not from the admin. So an address
// computed offline commits to nothing whatever about who controls it.
//
// On a live run of the country ceremony a predicted address came out as the
// FOUNDATION'S OWN, because both were policy sequence 1. Carried into an
// appointment proposal, that would have been a governance vote appointing the
// foundation a foundation administrator — passing, executing, and reading as
// correct at every step, with the group this ceremony was actually held for
// holding nothing.
//
// So:
//
//  1. Create the group. `ceremony administrators group` writes the transaction and
//     nothing else. It composes no proposal and it writes no address.
//  2. Read the address back. `ceremony administrators confirm` takes the chain's
//     own answers and verifies that the policy at that address really is this
//     group: the same members, the same threshold, administering itself.
//  3. Only then compose the proposal. `ceremony administrators propose` refuses
//     outright until the address has been read back. There is no flag that
//     relaxes it and no --seq to fall back on.
//
// # Why this tool does not talk to the chain
//
// Unchanged from the country enrolment, and for the same reason: tools/ceremony
// contains no outbound network code, and the runbook makes that claim to people
// who are about to type a seed phrase in front of it. Evidence arrives as files
// from `blockchaind query ... -o json`, and is verified against what the group IS
// rather than trusted because it came over a socket.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	aliastypes "yamale/blockchain/x/alias/types"
)

// administratorsConfig is what a ceremony lead writes down before the day.
//
// Short, and note what is NOT in it: the threshold, the members, and the policy
// address. The first two come from the group file the hosted ceremony produced, so
// the config cannot disagree with what the custodians signed for; the third comes
// from the chain. A config field for any of them would be a field somebody could
// fill in wrongly, and all three decide who ends up able to correct any account's
// country.
type administratorsConfig struct {
	// Ceremony is the human name of this appointment, printed on the record and
	// recorded on chain inside the group metadata.
	Ceremony string `json:"ceremony"`

	// ChainID is which chain this is for. Required, because a proposal is only
	// meaningful against a named chain.
	ChainID string `json:"chain_id"`

	// Group is the path to the assembled group file the hosted ceremony wrote —
	// group.json from `ceremony host --out`.
	Group string `json:"group"`

	// Reason is why this group is being appointed, and it ends up in the
	// proposal's summary where the voting set will read it. Required, because
	// "appoint this address" is not a case anybody can vote on.
	Reason string `json:"reason"`
}

// administratorsDossier is the record of what has happened, carried between steps.
type administratorsDossier struct {
	Ceremony  string `json:"ceremony"`
	ChainID   string `json:"chain_id"`
	CreatedAt string `json:"created_at"`
	Reason    string `json:"reason"`

	// Threshold and Members come from the ceremony, never from the config.
	Threshold int                   `json:"threshold"`
	Members   []administratorMember `json:"members"`

	// GroupFingerprint is the value the custodians read aloud to each other. It
	// is on the record so that the group created on chain can be tied back to the
	// afternoon they spent agreeing it.
	GroupFingerprint string `json:"group_fingerprint"`

	// GroupFile is where the assembled ceremony output lives. Re-read at each
	// step rather than copied into this file, because the member set is the one
	// field that decides who holds the power.
	GroupFile string `json:"group_file"`

	// OnChain is absent until `confirm` has read the address back off the chain.
	// Its absence is what makes `propose` refuse.
	OnChain *onChainGroup `json:"on_chain,omitempty"`

	// Appointed is absent until `verify` has read the chain-wide grants back and
	// found this group holding one.
	Appointed *appointment `json:"appointed,omitempty"`
}

// administratorMember is one custodian, as the ceremony produced them.
type administratorMember struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	Fingerprint string `json:"fingerprint"`
}

// appointment is the evidence that the proposal actually took effect.
//
// Recorded because a governance proposal that PASSED and a governance proposal
// that took effect are two different states, and the gap between them is where
// this goes wrong quietly. A proposal can pass its vote and still fail at
// execution, which leaves the chain exactly as it was and reports it in a
// transaction log nobody is watching.
type appointment struct {
	PolicyAddress string `json:"policy_address"`

	// Jurisdiction is recorded even though the chain accepts only one value for
	// it. A record that left it out would read identically whether the grant this
	// ceremony asked for landed or a country-scoped one did, and the fact that the
	// chain refuses the second today is a fact about today: this record is read by
	// somebody years afterwards who has no way to know which rules were in force.
	Jurisdiction string `json:"jurisdiction"`

	// GrantedBy and GrantedAtHeight are the chain's own account of who made this
	// grant and when, and they are the part of the evidence this tool did not
	// compose. The proposal it wrote NAMES governance as the authority; these two
	// are the chain saying that is who actually signed, at a height somebody can
	// go and look at.
	GrantedBy       string `json:"granted_by"`
	GrantedAtHeight int64  `json:"granted_at_height"`

	// Administrators is every account holding the role chain-wide when this was
	// read, not just this group.
	//
	// The old reason for recording the whole set no longer applies: the
	// appointment was a MsgUpdateParams that replaced the entire parameter object,
	// so a list that had SHRUNK was the only visible trace of a proposal composed
	// from a stale read. A grant cannot drop a holder it does not name, and that
	// failure is gone.
	//
	// The set is still recorded, for the reason the cap exists rather than for
	// that one. This role is the single exception to every account on the chain
	// having a jurisdiction, so how many accounts hold it is the number a reader
	// of this record should be able to see without querying a chain that has moved
	// on since the day it was signed.
	Administrators []string `json:"administrators"`

	VerifiedAt string `json:"verified_at"`
}

// validate refuses a config that could not produce a working appointment.
func (c administratorsConfig) validate() error {
	if strings.TrimSpace(c.Ceremony) == "" {
		return errors.New("ceremony is required: it is the name this group carries on chain, permanently")
	}
	if strings.TrimSpace(c.ChainID) == "" {
		return errors.New("chain_id is required: a proposal is only meaningful against a named chain")
	}
	if strings.TrimSpace(c.Group) == "" {
		return errors.New(
			"group is required: the path to group.json from the hosted ceremony. The threshold and the members " +
				"come from there rather than from this file, so that a config paired with the wrong ceremony's " +
				"keys is a refusal rather than an appointment of the wrong group")
	}
	if strings.TrimSpace(c.Reason) == "" {
		return errors.New(
			"reason is required. It becomes the proposal's summary, which is the only explanation most of the " +
				"voting set will read — and what they are being asked to agree to is that this group may move any " +
				"account on the chain out from under the authority investigating it")
	}
	return nil
}

// administratorsDossierFor builds the dossier from the config and the ceremony.
//
// Everything that decides who holds the power is read out of the group file. The
// config contributes the name, the chain and the reason, and nothing else.
func administratorsDossierFor(config administratorsConfig, now time.Time) (administratorsDossier, error) {
	if err := config.validate(); err != nil {
		return administratorsDossier{}, err
	}

	assembled, err := readAssembledGroup(config.Group)
	if err != nil {
		return administratorsDossier{}, err
	}
	if err := requireAdministratorsCeremony(assembled.Params, config.Ceremony); err != nil {
		return administratorsDossier{}, err
	}

	members := make([]administratorMember, 0, len(assembled.Custodians))
	for _, custodian := range assembled.Custodians {
		members = append(members, administratorMember{
			Name:        custodian.Name,
			Address:     custodian.Address,
			Fingerprint: custodian.Fingerprint,
		})
	}
	// Sorted by address, so the dossier and the chain's answer can be compared
	// directly. x/group returns members in its own order.
	sort.Slice(members, func(i, j int) bool { return members[i].Address < members[j].Address })

	return administratorsDossier{
		Ceremony:         strings.TrimSpace(config.Ceremony),
		ChainID:          strings.TrimSpace(config.ChainID),
		CreatedAt:        now.UTC().Truncate(time.Second).Format(time.RFC3339),
		Reason:           strings.TrimSpace(config.Reason),
		Threshold:        assembled.Params.Threshold,
		Members:          members,
		GroupFingerprint: assembled.Fingerprint,
		GroupFile:        config.Group,
	}, nil
}

// requireAdministratorsCeremony refuses a group file that was not made for this.
//
// The symmetric check to the country enrolment's, and it matters in both
// directions. A foundation ceremony's keys turned into an administrator group
// would be recorded on chain as "Yamale foundation" — indistinguishable, in the
// one field a human reads, from the account that holds every seized asset. A
// country office's keys turned into one would appoint an office that holds a
// national perimeter to a role that exists precisely because its holder has none.
//
// Neither is caught by anything downstream: the chain matches administrators by
// address equality and does not care where the address came from.
func requireAdministratorsCeremony(params ceremonyParams, ceremony string) error {
	if params.Office != nil {
		return fmt.Errorf(
			"that group file is a ceremony for a country office in %s, not for foundation administrators.\n"+
				"An office holds authority inside one perimeter; an administrator exists because it has none, and "+
				"its identifier carries the reserved code that says so. Appointing an office here would hand a "+
				"national authority the power to move accounts out of every other country's perimeter — and the "+
				"chain would not notice, because administrators are matched by address equality",
			params.Office.Country)
	}
	if !params.Administrators {
		return fmt.Errorf(
			"that group file is the FOUNDATION's ceremony, not an administrator ceremony.\n"+
				"Its group is recorded on chain as %q, so an administrator group created from it would be "+
				"indistinguishable — in the one field a human reads to find out what a group is — from the "+
				"account that holds every seized asset on this chain. The custodians also never saw what this "+
				"key would be for: the administrator marker is inside the parameters fingerprint they read "+
				"aloud, and theirs did not carry it.\n"+
				"Run the ceremony again with the foundation-administrator box ticked",
			foundationLabel)
	}
	if !sameCeremonyName(ceremony, params.Name) {
		return fmt.Errorf(
			"the config calls this ceremony %q and the group file was made for %q. One of those is not this "+
				"group, and guessing which would appoint an address nobody chose",
			ceremony, params.Name)
	}
	return nil
}

// sameCeremonyName compares names the way sameOffice does: trimmed, case-folded.
//
// Loose on whitespace and case because a config and a ceremony form are typed by
// two people at two times, and strict on everything else because the name is what
// ties this dossier to that afternoon.
func sameCeremonyName(configured, ceremony string) bool {
	return strings.EqualFold(strings.TrimSpace(configured), strings.TrimSpace(ceremony))
}

// memberAddresses is the sorted member set, for comparing against the chain.
func (d administratorsDossier) memberAddresses() []string {
	out := make([]string, 0, len(d.Members))
	for _, m := range d.Members {
		out = append(out, m.Address)
	}
	sort.Strings(out)
	return out
}

// attested reduces the dossier to what confirmGroup checks against.
func (d administratorsDossier) attested() attestedGroup {
	return attestedGroup{
		Name:      d.Ceremony,
		Threshold: d.Threshold,
		Members:   d.memberAddresses(),
	}
}

// requireConfirmedGroup is the refusal the two-phase design exists to make.
//
// No confirmed address, no proposal. There is no flag that relaxes it, no
// fallback to a predicted address, and no path on which an unconfirmed dossier
// produces a proposal naming anything at all.
func (d administratorsDossier) requireConfirmedGroup() (string, error) {
	if d.OnChain == nil {
		return "", fmt.Errorf(
			"this group has not been read back from the chain, so this tool does not know its address and will " +
				"not guess one.\n" +
				"An x/group policy address derives from the policy sequence number alone — not from the members, " +
				"the threshold or the admin — so an address computed offline commits to nothing about who " +
				"controls it. On a live run of the country ceremony a predicted address came out as the " +
				"FOUNDATION'S OWN, because both were policy sequence 1: a proposal carrying that would have " +
				"appointed the foundation, passed, and read as correct at every step.\n" +
				"Create the group, then:\n" +
				"  ceremony administrators confirm --dossier <file> --tx tx.json --policy policy.json " +
				"--members members.json")
	}
	if strings.TrimSpace(d.OnChain.PolicyAddress) == "" {
		return "", errors.New(
			"the confirmation record carries no address, which should be impossible; do not proceed")
	}
	return d.OnChain.PolicyAddress, nil
}

// ---------------------------------------------------------------- reading and writing

func readAdministratorsDossier(path string) (administratorsDossier, error) {
	var dossier administratorsDossier
	if err := readStrictJSON(path, &dossier); err != nil {
		return administratorsDossier{}, err
	}
	if strings.TrimSpace(dossier.ChainID) == "" || len(dossier.Members) == 0 {
		return administratorsDossier{}, fmt.Errorf(
			"%s is not an appointment dossier: it names no chain or no members", path)
	}
	return dossier, nil
}

func writeAdministratorsDossier(path string, dossier administratorsDossier) error {
	blob, err := json.MarshalIndent(dossier, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), append(blob, '\n'), 0o644)
}

// ---------------------------------------------------------------- the grant

// verifyAppointment reads the chain-wide grants back and checks this group holds
// one.
//
// The step that exists because a proposal passing and a proposal taking effect are
// two different things. `tx gov` reports neither: a proposal that passed its vote
// and failed at execution leaves the chain exactly as it was, and says so only in
// a transaction log nobody is watching.
//
// It reads chain-wide-grants rather than role-grants for this one address, and
// that is deliberate: the two answer the same question about this group, and only
// the first also answers "how many accounts stand outside every perimeter" — which
// is what the record carries and what the cap is about. One query for both steps
// is also one query an operator can get wrong.
func verifyAppointment(dossier administratorsDossier, path string, now time.Time) (appointment, error) {
	address, err := dossier.requireConfirmedGroup()
	if err != nil {
		return appointment{}, err
	}

	held, err := chainWideGrantsOf(path, aliastypes.ROLE_FOUNDATION_ADMINISTRATOR)
	if err != nil {
		return appointment{}, err
	}

	holders := make([]string, 0, len(held))
	for _, grant := range held {
		holders = append(holders, grant.Holder)
	}
	for _, grant := range held {
		if grant.Holder == address {
			return appointment{
				PolicyAddress:   address,
				Jurisdiction:    aliastypes.ChainWide,
				GrantedBy:       grant.GrantedBy,
				GrantedAtHeight: grant.GrantedAtHeight,
				Administrators:  holders,
				VerifiedAt:      now.UTC().Truncate(time.Second).Format(time.RFC3339),
			}, nil
		}
	}

	return appointment{}, fmt.Errorf(
		"%s holds no %s grant at the %q scope, so it holds nothing.\n"+
			"The accounts that do are %s.\n"+
			"A governance proposal reporting code 0 was ACCEPTED and has not necessarily executed, and a "+
			"proposal that PASSED can still fail when it executes — which leaves the chain exactly as it was "+
			"and reports it in a transaction log nobody is watching. Check the proposal itself:\n"+
			"  blockchaind query gov proposal <id> -o json",
		address, aliastypes.RoleName(aliastypes.ROLE_FOUNDATION_ADMINISTRATOR), aliastypes.ChainWide,
		describeList(holders))
}

// requireAppointableCount refuses a proposal the chain would reject on arithmetic
// alone, and one it would accept and act on to no effect.
//
// The cap is eight, it is not about storage, and the chain now enforces it itself
// in GrantRole. Checking it here as well is not redundant, for the reason it was
// worth checking before the chain did: a proposal that passes its vote and fails
// at execution is worse than one refused before it is written. The first has cost
// a voting period, sits in the proposal history reading as an appointment, and
// reports its failure in a transaction log nobody is watching.
//
// aliastypes.MaxFoundationAdministrators rather than a literal, so a change to the
// chain's rule is a compile-time change here.
//
// current is the whole set of holders, and comparing its length against the cap
// matches what the keeper does because the duplicate branch above has already
// returned for the one case where the two counts differ: GrantRole counts
// EXCLUDING the holder being granted, and a holder already in this set never
// reaches the length check.
func requireAppointableCount(current []string, adding string) error {
	role := aliastypes.RoleName(aliastypes.ROLE_FOUNDATION_ADMINISTRATOR)
	for _, existing := range current {
		if existing == adding {
			return fmt.Errorf(
				"%s already holds %s chain-wide, so this proposal would change nothing.\n"+
					"The chain would ACCEPT it. GrantRole counts the cap excluding the holder being granted, "+
					"precisely so that a grant can be re-made — which is how a proposal resubmitted after a "+
					"timeout arrives, and how a required shape is added to a grant that had none. This ceremony "+
					"records no required shape, so a second grant has nothing to change: it would be a governance "+
					"vote that passes, executes, reads on the record as an appointment, and leaves the chain "+
					"exactly as it was", adding, role)
		}
	}
	if len(current) >= aliastypes.MaxFoundationAdministrators {
		return fmt.Errorf(
			"%d accounts already hold %s chain-wide and the chain caps it at %d. The cap is not about storage: "+
				"it is there so that widening the one exception to every account having a jurisdiction cannot "+
				"happen by accident. Revoke somebody first, in a proposal of its own, so both decisions are "+
				"voted on separately.\nThe holders are %s",
			len(current), role, aliastypes.MaxFoundationAdministrators, describeList(current))
	}
	return nil
}
