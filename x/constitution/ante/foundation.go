// Package ante holds the gate that keeps the foundation group the shape the
// constitution says it is.
package ante

import (
	"context"
	"fmt"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/types/query"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/cosmos/cosmos-sdk/x/group"

	"yamale/blockchain/x/constitution/types"
)

// ConstitutionKeeper is the settlement in force.
type ConstitutionKeeper interface {
	GetInvariants(ctx context.Context) (types.Invariants, error)
}

// GroupKeeper is the part of x/group this gate reads.
//
// Read-only, and deliberately the query surface rather than the keeper's
// internals: x/group is an SDK module this repository cannot edit, so the gate
// has to wrap it from outside. Anything it needed that the query service does
// not expose would be a sign this belonged somewhere else.
type GroupKeeper interface {
	GroupPolicyInfo(ctx context.Context, req *group.QueryGroupPolicyInfoRequest) (*group.QueryGroupPolicyInfoResponse, error)
	GroupMembers(ctx context.Context, req *group.QueryGroupMembersRequest) (*group.QueryGroupMembersResponse, error)
	Proposal(ctx context.Context, req *group.QueryProposalRequest) (*group.QueryProposalResponse, error)
}

// FoundationGuardDecorator refuses any transaction that would change the size
// of the foundation group, the number of signatures it needs, or who
// administers it.
//
// The rule it enforces is that a departing custodian is replaced in the same
// decision. Not because a group of four is unworkable, but because it is not
// what anybody agreed to: five custodians of whom three must sign is sixty per
// cent; four of whom three must sign is seventy-five, and the people who happen
// to remain hold more authority than the ceremony gave them. At three it is
// unanimity, where one custodian who cannot be reached freezes the account the
// chain is still sending seized property into. Nobody proposes that outcome —
// it is reached by two reasonable decisions taken months apart.
//
// x/group's MsgUpdateGroupMembers already carries adds and removes in one
// message, so the swap is the native operation. This makes it the only one.
//
// # What this cannot see
//
// An ante decorator is only ever shown messages that arrived as transactions,
// and x/group's Exec dispatches a proposal's contents through the message
// router. Every route to that still starts with a transaction — see check() —
// with two exceptions, and they are stated here rather than in a runbook
// because a rule people are asked to remember is not a rule.
//
//  1. **Interchain accounts.** An ICA executes through the router as well, so
//     an ICA sending group.MsgExec would not pass here. The host is disabled in
//     this chain's genesis and enabling it is a governance decision that has to
//     come with an explicit allow-list; the same caveat applies to
//     x/validatorgov's gate and for the same reason.
//
//  2. **A group proxying for another group.** x/group's Exec authorises nobody
//     — any address may execute an accepted proposal — so a proposal belonging
//     to some other group can carry group.MsgExec for a foundation proposal,
//     and that MsgExec reaches the router without passing here.
//
// The second is worth being precise about rather than alarmed by. Submission is
// checked, so no proposal that would shrink the group can be created in the
// first place. The gap is narrower: two swaps, each valid when submitted, that
// interact — both naming the same incoming custodian, say — so that executing
// the second produces four members. The exec-time re-check below catches that
// on the direct route and the proxy route evades it.
//
// Reaching it requires three of five custodians to pass both proposals and then
// deliberately route around a refusal they have already been shown. Anybody who
// can do that already holds the three signatures that move every asset in the
// account, so the bypass grants no authority they did not have. It is a drift
// guard, and every way the group drifts by accident is refused loudly.
// Recovery is also ordinary: a group left at four is repaired by an update that
// adds one, which produces five and is allowed.
type FoundationGuardDecorator struct {
	constitution ConstitutionKeeper
	group        GroupKeeper
}

func NewFoundationGuardDecorator(constitution ConstitutionKeeper, groupKeeper GroupKeeper) FoundationGuardDecorator {
	return FoundationGuardDecorator{constitution: constitution, group: groupKeeper}
}

// maxNesting bounds how deep the gate will unwrap, for the same reason
// x/validatorgov's does: a transaction can nest MsgExec arbitrarily, and
// following it without a bound is unbounded work for one fee. Deeper than this
// is refused outright, so the bound cannot become the way around the check.
const maxNesting = 6

func (d FoundationGuardDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	// The foundation group is created by the genesis file itself — see the key
	// ceremony guide — so at height zero there is nothing to protect and the
	// group does not exist yet to be looked up.
	if ctx.BlockHeight() == 0 {
		return next(ctx, tx, simulate)
	}

	// Nothing is read from the store unless the transaction could possibly
	// reach a group message. This decorator sits in the chain every transaction
	// on the network passes through, and resolving the foundation costs two
	// store reads — a constitution lookup and a group policy lookup. Paid on
	// every bank send, every swap and every payment, that is a tax on the whole
	// chain for a rule that applies to a handful of message types.
	if !mightReachGroup(tx.GetMsgs()) {
		return next(ctx, tx, simulate)
	}

	foundation, err := d.foundation(ctx)
	if err != nil {
		return ctx, err
	}
	// The recovery destination is not a group policy. That is a legitimate
	// state — a chain whose foundation is still an ordinary account, or one
	// mid-migration — and this gate has nothing to say about it. Refusing here
	// would stop such a chain from processing any transaction at all, which is
	// a far worse failure than the one being guarded against.
	if foundation == nil {
		return next(ctx, tx, simulate)
	}

	if err := d.check(ctx, tx.GetMsgs(), foundation, 0); err != nil {
		return ctx, err
	}
	return next(ctx, tx, simulate)
}

// mightReachGroup is a cheap pre-filter over the transaction's top-level
// messages, deciding whether the full check is worth any store reads.
//
// It must over-approximate, and it does: anything in x/group, plus the two
// carriers that dispatch other people's messages through the router, plus
// anything else this decorator descends into. A message type missing from here
// is a message type the rule does not apply to, so it errs towards the type
// prefix rather than an exact list — `/cosmos.group.v1.` catches a message added
// to x/group by an SDK upgrade, which an enumeration would not.
func mightReachGroup(msgs []sdk.Msg) bool {
	for _, msg := range msgs {
		typeURL := sdk.MsgTypeURL(msg)
		switch {
		case strings.HasPrefix(typeURL, "/cosmos.group.v1."),
			typeURL == sdk.MsgTypeURL(&authztypes.MsgExec{}),
			typeURL == sdk.MsgTypeURL(&govv1.MsgSubmitProposal{}):
			return true
		}
	}
	return false
}

// foundationGroup is what the constitution says the account seized assets go to
// must look like, resolved to the group that actually holds it.
type foundationGroup struct {
	policyAddress string
	groupID       uint64
	count         uint32
	threshold     uint32
}

func (d FoundationGuardDecorator) foundation(ctx sdk.Context) (*foundationGroup, error) {
	invariants, err := d.constitution.GetInvariants(ctx)
	if err != nil {
		return nil, err
	}

	// Resolved from the address rather than stored a second time. The
	// constitution already names the account; the group id is a fact about the
	// chain's state, and a copy of it in the invariants would be one more value
	// that could disagree with reality after a genesis is hand-edited.
	info, err := d.group.GroupPolicyInfo(ctx, &group.QueryGroupPolicyInfoRequest{
		Address: invariants.EnforcementRecoveryDestination,
	})
	if err != nil || info == nil || info.Info == nil {
		return nil, nil
	}

	return &foundationGroup{
		policyAddress: info.Info.Address,
		groupID:       info.Info.GroupId,
		count:         invariants.FoundationCustodianCount,
		threshold:     invariants.FoundationSignatureThreshold,
	}, nil
}

// check walks a transaction's messages, descending into the ones that carry
// others and resolving the ones that name a proposal.
//
// Both halves are load-bearing and for different reasons.
//
// Descending covers x/authz and x/gov, which dispatch what they carry through
// the message router after the ante chain has already run — the same hole
// x/validatorgov's gate had to close.
//
// Resolving covers x/group itself, and it is the more important half here.
// The foundation group is its own admin, so a MsgUpdateGroupMembers for it can
// only ever be signed by the group policy address, which no private key
// produces. The only way it can execute is inside a group proposal, and group
// proposals execute through the router too. Every route to that still begins
// with a transaction — MsgSubmitProposal carries the messages inline, MsgExec
// and a try-executing MsgVote name a proposal that is already in the store — so
// checking all three catches it before the router ever sees it.
//
// Checking execution as well as submission is not belt and braces. Two swaps
// can each be valid when submitted and invalid together: both name the same
// incoming custodian, the first executes and admits them, and the second then
// removes somebody and "adds" a member who is already there, leaving four. Only
// a check at execution time, against the members in the store at that moment,
// sees it.
func (d FoundationGuardDecorator) check(ctx sdk.Context, msgs []sdk.Msg, foundation *foundationGroup, depth int) error {
	if depth > maxNesting {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
			"transaction nests messages more than %d deep", maxNesting)
	}

	for _, msg := range msgs {
		switch m := msg.(type) {
		case *authztypes.MsgExec:
			inner, err := m.GetMessages()
			if err != nil {
				// Contents that cannot be decoded cannot be shown to be safe.
				return errorsmod.Wrap(err, "cannot inspect the messages inside MsgExec")
			}
			if err := d.check(ctx, inner, foundation, depth+1); err != nil {
				return err
			}

		case *govv1.MsgSubmitProposal:
			inner, err := m.GetMsgs()
			if err != nil {
				return errorsmod.Wrap(err, "cannot inspect the messages inside a governance proposal")
			}
			if err := d.check(ctx, inner, foundation, depth+1); err != nil {
				return err
			}

		case *group.MsgSubmitProposal:
			inner, err := m.GetMsgs()
			if err != nil {
				return errorsmod.Wrap(err, "cannot inspect the messages inside a group proposal")
			}
			if err := d.check(ctx, inner, foundation, depth+1); err != nil {
				return err
			}

		case *group.MsgExec:
			if err := d.checkProposal(ctx, m.ProposalId, foundation, depth); err != nil {
				return err
			}

		case *group.MsgVote:
			// Only a try-executing vote can move anything in this transaction.
			// An ordinary vote is followed by a MsgExec, which is caught above;
			// checking here as well would cost a store read on every vote cast
			// on the chain.
			if m.Exec == group.Exec_EXEC_TRY {
				if err := d.checkProposal(ctx, m.ProposalId, foundation, depth); err != nil {
					return err
				}
			}

		default:
			if err := d.checkLeaf(ctx, msg, foundation); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkProposal inspects the messages a stored proposal would execute.
func (d FoundationGuardDecorator) checkProposal(ctx sdk.Context, proposalID uint64, foundation *foundationGroup, depth int) error {
	response, err := d.group.Proposal(ctx, &group.QueryProposalRequest{ProposalId: proposalID})
	// A proposal that does not exist is x/group's problem, not this gate's: it
	// will fail on its own terms. Refusing here would turn a stale proposal id
	// into an error blamed on the constitution.
	if err != nil || response == nil || response.Proposal == nil {
		return nil
	}
	inner, err := response.Proposal.GetMsgs()
	if err != nil {
		return errorsmod.Wrap(err, "cannot inspect the messages inside the proposal being executed")
	}
	return d.check(ctx, inner, foundation, depth+1)
}

// checkLeaf is where the rule is actually applied.
func (d FoundationGuardDecorator) checkLeaf(ctx sdk.Context, msg sdk.Msg, foundation *foundationGroup) error {
	switch m := msg.(type) {
	case *group.MsgUpdateGroupMembers:
		if m.GroupId != foundation.groupID {
			return nil
		}
		resulting, err := d.resultingMembers(ctx, foundation.groupID, m.MemberUpdates)
		if err != nil {
			return err
		}
		if resulting != int(foundation.count) {
			return errorsmod.Wrapf(types.ErrConstitutionalInvariant,
				"this update would leave the foundation group with %d custodians; the constitution fixes it at %d. "+
					"A departing custodian is replaced in the same message: set their weight to 0 and add the incoming "+
					"custodian's address in the same member_updates list. Removing one on the understanding that a "+
					"replacement will be added later is how three-of-five quietly becomes three-of-four and then "+
					"unanimity, and the account it guards is the one every seizure is sent to",
				resulting, foundation.count)
		}

	case *group.MsgLeaveGroup:
		if m.GroupId != foundation.groupID {
			return nil
		}
		// The one message that shrinks the group with nobody's agreement but
		// the departing member's. A custodian resigning is a decision for the
		// custodians, because it changes the weight every remaining one holds.
		return errorsmod.Wrap(types.ErrConstitutionalInvariant,
			"a custodian cannot leave the foundation group on their own signature. "+
				"Leaving changes how much authority everybody else holds, so it is a decision the custodians take "+
				"together: propose a MsgUpdateGroupMembers that removes the departing custodian and adds their "+
				"replacement in the same message")

	case *group.MsgUpdateGroupPolicyDecisionPolicy:
		if m.GroupPolicyAddress != foundation.policyAddress {
			return nil
		}
		policy, err := m.GetDecisionPolicy()
		if err != nil {
			return errorsmod.Wrap(err, "cannot inspect the decision policy this would install")
		}
		threshold, ok := policy.(*group.ThresholdDecisionPolicy)
		if !ok {
			// A percentage policy would make the threshold follow the
			// membership, which is exactly the drift the fixed count exists to
			// stop; anything else is a policy the constitution cannot express.
			return errorsmod.Wrapf(types.ErrConstitutionalInvariant,
				"the foundation group must use a threshold decision policy of %d signatures, not %T",
				foundation.threshold, policy)
		}
		want := fmt.Sprintf("%d", foundation.threshold)
		if threshold.Threshold != want {
			return errorsmod.Wrapf(types.ErrConstitutionalInvariant,
				"this would set the foundation's signature threshold to %s; the constitution fixes it at %s. "+
					"Changing it is a constitutional amendment — three weeks in public and a four-fifths "+
					"ratification — not a proposal by the custodians it constrains",
				threshold.Threshold, want)
		}

	case *group.MsgUpdateGroupAdmin:
		if m.GroupId != foundation.groupID {
			return nil
		}
		// The group being its own admin is what makes the membership
		// unchangeable except by the custodians themselves. Moving the admin
		// elsewhere is one transaction away from a single key that can rewrite
		// the whole arrangement — which is the key this design exists to
		// abolish, reintroduced through a message that reads like maintenance.
		return errorsmod.Wrap(types.ErrConstitutionalInvariant,
			"the foundation group's admin cannot be moved. It administers itself, which is what makes its membership "+
				"changeable only by the custodians and never by a single key")

	case *group.MsgUpdateGroupPolicyAdmin:
		if m.GroupPolicyAddress != foundation.policyAddress {
			return nil
		}
		return errorsmod.Wrap(types.ErrConstitutionalInvariant,
			"the foundation group policy's admin cannot be moved. It administers itself, which is what makes its "+
				"decision policy changeable only by the custodians and never by a single key")
	}

	return nil
}

// resultingMembers is how many custodians the group would have afterwards.
//
// Computed against the members actually in the store rather than against the
// message alone, because MsgUpdateGroupMembers is a set of edits and not a
// replacement: "remove one" and "add one" are the same field, distinguished by
// a weight of zero, and re-adding somebody who is already a member changes
// nothing at all.
func (d FoundationGuardDecorator) resultingMembers(ctx sdk.Context, groupID uint64, updates []group.MemberRequest) (int, error) {
	// The limit is explicit, and it is not decoration. x/group's member query
	// pages, and asked with no page request it returns at most a hundred
	// members — so a group larger than that would be silently undercounted and
	// every legitimate change to it refused. types.MaxFoundationCustodians caps
	// the constitutional count well below this, and asking for one more than
	// the cap turns any disagreement between the two into the refusal below
	// rather than a wrong answer.
	const limit = types.MaxFoundationCustodians + 1
	response, err := d.group.GroupMembers(ctx, &group.QueryGroupMembersRequest{
		GroupId:    groupID,
		Pagination: &query.PageRequest{Limit: limit},
	})
	if err != nil {
		return 0, errorsmod.Wrap(err, "cannot read the foundation group's members")
	}
	if len(response.Members) >= limit {
		return 0, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
			"the foundation group has at least %d members, more than the constitution can describe; refusing rather than counting a page of it",
			limit)
	}

	members := make(map[string]bool, len(response.Members)+len(updates))
	for _, member := range response.Members {
		if member.Member != nil {
			members[member.Member.Address] = true
		}
	}

	for _, update := range updates {
		weight, err := sdkmath.LegacyNewDecFromStr(update.Weight)
		if err != nil {
			// x/group would reject this too, but it must not be counted as a
			// member here: an unparseable weight that this function guessed at
			// could make a removal look like an addition.
			return 0, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
				"member %s has an unreadable weight %q", update.Address, update.Weight)
		}
		if weight.IsZero() {
			delete(members, update.Address)
			continue
		}
		members[update.Address] = true
	}

	return len(members), nil
}
