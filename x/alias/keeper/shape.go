package keeper

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/x/group"

	"yamale/blockchain/x/alias/types"
)

// This file answers one question about an office: how many of its members does it
// take to act, and how many members does it have?
//
// It exists because the check the module used to make was the wrong shape. It
// asked whether the holder of a role *was* an x/group policy, once, at grant
// time, and it asked nothing about the arrangement inside. Two consequences
// followed and both were live on a running chain:
//
//  1. A one-of-one group is a group policy, so it answered yes. The rule that a
//     role holder must be M-of-N was satisfied by a single key with extra steps.
//  2. An office administers itself — that is what makes its membership
//     changeable by its own members and never by a stranger — so it can vote to
//     reduce its own threshold at any time after the grant. A country could hold
//     a proper ceremony, stand up a three-of-five enforcement authority, and that
//     office could later become a one-of-one and go on freezing accounts.
//
// The failure the second one names is one this codebase already has a sentence
// for: a jurisdiction stamped once at account creation and never re-examined is
// an event, not a perimeter. An office's shape was an event.
//
// So the requirement is written on the grant and read on every action the grant
// permits. Note where that check is NOT: it is not an ante decorator, and it must
// not become one. Interchain accounts and x/authz reach the message router
// without passing through the ante chain, and they reach it for exactly the
// actions that matter most.

// officeShape is what a group actually is right now, in the two numbers a
// requirement is written in.
type officeShape struct {
	// signatures is how few members it TAKES to act — not the threshold number.
	//
	// The distinction is the whole reason this struct exists. x/group counts
	// weight, not heads, so a policy with a threshold of 3 over members weighing
	// 3, 1, 1, 1 and 1 is a one-of-five: the first member reaches the threshold
	// alone. Reading the threshold as "three signatures" would let that group
	// hold a grant requiring three, which is the failure this whole file is
	// about, one level down.
	signatures uint32

	// members is how many members hold a positive weight. A member who cannot
	// vote is a name on a list, and padding a group with weightless members is
	// the obvious way to satisfy a count while shrinking the number of people who
	// decide.
	members uint32
}

func (s officeShape) rule() string {
	return fmt.Sprintf("%d-of-%d", s.signatures, s.members)
}

// assertShape refuses when the holder no longer keeps the shape a grant records.
//
// Read the nil branches first, because they are the decisions rather than the
// arithmetic:
//
//   - want == nil: no requirement was recorded, so there is nothing to hold the
//     office to and no query is made. That is the state of every grant made
//     before required_shape existed, and it means this check costs an
//     unrequirement-carrying chain exactly nothing.
//
//   - k.groups == nil: no group keeper is wired, so the shape cannot be
//     established — and this REFUSES rather than skipping. assertGroupAccount
//     does the opposite for its own check, deliberately, and the difference is
//     which way the bypass runs. A missing group keeper there can only produce a
//     grant that should have been refused; here it would produce an ACTION that
//     should have been refused, on the path that freezes accounts and admits
//     issuers. A threshold check that waved the action through when its
//     dependency was absent would be a threshold check any wiring mistake
//     silently removes, and the one-of-one office it was meant to stop is the one
//     that benefits. Unit tests that grant with a requirement therefore have to
//     supply a group keeper; that is the intended cost.
func (k Keeper) assertShape(ctx context.Context, holder string, want *types.OfficeShape) error {
	if want == nil {
		return nil
	}
	if k.groups == nil {
		return errorsmod.Wrapf(types.ErrOfficeShape,
			"%s holds a grant requiring %s and x/group is not wired in, so the office's shape cannot be read; "+
				"refusing rather than assuming it still holds", holder, want.Rule())
	}

	have, err := k.officeShapeOf(ctx, holder)
	if err != nil {
		return err
	}
	if !want.Satisfies(have.signatures, have.members) {
		return errorsmod.Wrapf(types.ErrOfficeShape,
			"%s was granted its authority as %s and is now %s. An office may grow; it may not fall below the "+
				"shape it was granted under. Restore it with a MsgUpdateGroupMembers or a "+
				"MsgUpdateGroupPolicyDecisionPolicy voted by the office itself, or have the grant re-made",
			holder, want.Rule(), have.rule())
	}
	return nil
}

// assertShapeNotReduced refuses a re-grant that would weaken or drop the
// requirement already recorded against the same triple.
//
// Re-granting an existing triple is deliberately idempotent — it rewrites the
// attribution and the height and is what a proposal resubmitted after a timeout
// should do. That property is worth keeping and it has a sharp edge once a grant
// carries a requirement: the obvious resubmission is composed from the summary
// rather than from the stored grant, the required_shape field is left out, and
// the pin is silently removed by a proposal whose stated purpose was to change
// nothing. That is the same trap MsgUpdateParams documents — a message that
// replaces a whole record is a message whose omissions are edits — and it lands
// here on the field that decides whether an office can vote itself to one-of-one.
//
// So the requirement ratchets. Equal is fine, higher is fine, lower and absent
// are refused, and the way to genuinely relax one is to say so: a MsgRevokeRole
// and a MsgGrantRole in the same proposal, which x/group executes together or not
// at all. Two messages rather than an omission, so that relaxing an office's
// required shape is a thing somebody wrote down.
func (k Keeper) assertShapeNotReduced(
	ctx context.Context, holder string, role types.Role, scope string, want *types.OfficeShape,
) error {
	existing, err := k.RoleGrants.Get(ctx, collections.Join3(holder, int32(role), scope))
	if errors.Is(err, collections.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.RequiredShape == nil {
		// Nothing was pinned, so nothing can be reduced. Adding a requirement to a
		// grant that had none is an ordinary re-grant and the point of the field.
		return nil
	}
	if want == nil {
		return errorsmod.Wrapf(types.ErrOfficeShape,
			"%s already holds %s in %s with a required shape of %s, and this grant records none. Omitting "+
				"required_shape on a re-grant would remove the pin; to relax it deliberately, revoke the grant and "+
				"make a new one in the same proposal",
			holder, types.RoleName(role), scope, existing.RequiredShape.Rule())
	}
	if !existing.RequiredShape.Satisfies(want.Signatures, want.Members) {
		return errorsmod.Wrapf(types.ErrOfficeShape,
			"%s already holds %s in %s with a required shape of %s, and this grant would reduce it to %s. "+
				"A requirement only ratchets upward; to lower it, revoke the grant and make a new one in the same "+
				"proposal",
			holder, types.RoleName(role), scope, existing.RequiredShape.Rule(), want.Rule())
	}
	return nil
}

// officeShapeOf resolves what an address is, as an office, right now.
//
// Two queries and no store writes: the group policy, for the threshold, and the
// group's members, for the weights. Both are keeper calls into x/group — the same
// calls x/constitution's ante gate makes on the foundation's own group — so the
// cost is a handful of KV reads and a decode, not a cross-chain hop and not an
// iteration over anything unbounded.
//
// Every failure is an error and none of them is a permissive answer. An address
// that is not a policy, a decision policy this module cannot read, a weight that
// will not parse, a group larger than the module can page: all refusals. The
// failure this function exists to prevent is not being too strict.
func (k Keeper) officeShapeOf(ctx context.Context, holder string) (officeShape, error) {
	info, err := k.groups.GroupPolicyInfo(ctx, &group.QueryGroupPolicyInfoRequest{Address: holder})
	if err != nil || info == nil || info.Info == nil {
		// Reached when a role holder stops being a group policy at all. x/group
		// has no message that deletes a policy today, so on a correct chain this
		// is the same state as "never was one" — which is exactly why it must be a
		// refusal and not a nil shape compared against a requirement.
		return officeShape{}, errorsmod.Wrapf(types.ErrHolderNotGroup,
			"%s is not an x/group policy, so it has no M-of-N to check", holder)
	}

	threshold, err := k.policyThreshold(holder, info.Info)
	if err != nil {
		return officeShape{}, err
	}

	// One more than the cap, so a group at or past it is an explicit refusal
	// naming the cap rather than a count that stopped at a page boundary and read
	// as a small office. See types.MaxOfficeMembers.
	const limit = types.MaxOfficeMembers + 1
	members, err := k.groups.GroupMembers(ctx, &group.QueryGroupMembersRequest{
		GroupId:    info.Info.GroupId,
		Pagination: &query.PageRequest{Limit: limit},
	})
	if err != nil || members == nil {
		return officeShape{}, errorsmod.Wrapf(types.ErrOfficeShape,
			"the members of %s's group could not be read, so its shape cannot be checked", holder)
	}
	if len(members.Members) >= limit {
		return officeShape{}, errorsmod.Wrapf(types.ErrOfficeShape,
			"%s's group has at least %d members, more than the %d this module can read a shape from; "+
				"refusing rather than counting a page of it", holder, limit, types.MaxOfficeMembers)
	}

	weights := make([]sdkmath.LegacyDec, 0, len(members.Members))
	for _, entry := range members.Members {
		if entry == nil || entry.Member == nil {
			continue
		}
		weight, err := sdkmath.LegacyNewDecFromStr(entry.Member.Weight)
		if err != nil {
			// x/group would refuse this weight too, and it must not be guessed at
			// here: an unparseable weight treated as zero would undercount the
			// office, and treated as one would overcount it. Neither is an answer.
			return officeShape{}, errorsmod.Wrapf(types.ErrOfficeShape,
				"member %s of %s's group has an unreadable weight %q",
				entry.Member.Address, holder, entry.Member.Weight)
		}
		if !weight.IsPositive() {
			// x/group removes a member whose weight is set to zero, so this is
			// defensive rather than expected. Skipped rather than counted: a member
			// who cannot vote is not a share of the office.
			continue
		}
		weights = append(weights, weight)
	}

	signatures, err := fewestSigners(weights, threshold)
	if err != nil {
		return officeShape{}, errorsmod.Wrapf(types.ErrOfficeShape, "%s: %s", holder, err.Error())
	}

	return officeShape{signatures: signatures, members: uint32(len(weights))}, nil
}

// policyThreshold reads the absolute number of votes a policy requires.
//
// Only a threshold policy, and a percentage policy is refused rather than
// converted. The conversion is arithmetically possible — sixty per cent of a
// total weight of five is three — and it is refused anyway, for the reason
// x/constitution refuses one on the foundation's group: a percentage makes the
// threshold FOLLOW the membership, so an office could drop from five members to
// two and still satisfy its percentage while the number of people who decide has
// gone from three to two. A requirement of three signatures cannot be held
// against a policy that redefines three every time somebody leaves.
//
// Unpacked through the codec rather than through the Any's cached value, because
// the cached value is a property of how the response happened to be built. A
// keeper query populates it and a response that crossed a wire does not, and a
// check that worked in-process and refused every office over gRPC would be a
// check nobody could debug.
func (k Keeper) policyThreshold(holder string, info *group.GroupPolicyInfo) (sdkmath.LegacyDec, error) {
	if info.DecisionPolicy == nil {
		return sdkmath.LegacyDec{}, errorsmod.Wrapf(types.ErrOfficeShape,
			"%s's group policy records no decision policy, so it has no threshold to check", holder)
	}
	var policy group.DecisionPolicy
	if err := k.cdc.UnpackAny(info.DecisionPolicy, &policy); err != nil {
		return sdkmath.LegacyDec{}, errorsmod.Wrapf(types.ErrOfficeShape,
			"%s's decision policy cannot be read: %s", holder, err.Error())
	}
	threshold, ok := policy.(*group.ThresholdDecisionPolicy)
	if !ok {
		return sdkmath.LegacyDec{}, errorsmod.Wrapf(types.ErrOfficeShape,
			"%s uses a %T decision policy. A recorded shape can only be held against a threshold policy: a "+
				"percentage one moves with the membership, so an office could shed members and satisfy it forever",
			holder, policy)
	}
	value, err := sdkmath.LegacyNewDecFromStr(threshold.Threshold)
	if err != nil {
		return sdkmath.LegacyDec{}, errorsmod.Wrapf(types.ErrOfficeShape,
			"%s's threshold %q is not a number", holder, threshold.Threshold)
	}
	if !value.IsPositive() {
		return sdkmath.LegacyDec{}, errorsmod.Wrapf(types.ErrOfficeShape,
			"%s's threshold is %s, so any single member acts alone", holder, threshold.Threshold)
	}
	return value, nil
}

// fewestSigners is how few members can reach the threshold between them.
//
// The greedy answer is the right one and it is worth saying why, because "take
// the biggest weights first" looks like an approximation. The question is the
// minimum COUNT of items whose sum reaches a target, with no constraint but the
// count — so for any k, the largest achievable sum from k members is the sum of
// the k largest weights. If that sum does not reach the threshold then no k
// members can, and if it does then k members can. Sorting descending and
// accumulating therefore finds the exact minimum rather than an estimate.
//
// A count of members that cannot reach it at all is an error rather than a very
// large number. An office whose threshold exceeds its total weight is frozen: it
// can take no action, so nothing turns on whether its shape is adequate, and
// reporting "frozen" sends an operator to the group where reporting "shape too
// small" would send them to the grant.
func fewestSigners(weights []sdkmath.LegacyDec, threshold sdkmath.LegacyDec) (uint32, error) {
	sorted := make([]sdkmath.LegacyDec, len(weights))
	copy(sorted, weights)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].GT(sorted[j]) })

	total := sdkmath.LegacyZeroDec()
	for i, weight := range sorted {
		total = total.Add(weight)
		if total.GTE(threshold) {
			return uint32(i + 1), nil
		}
	}
	return 0, fmt.Errorf(
		"the group's %d members hold %s of voting weight between them and its threshold is %s, "+
			"so no set of members can act at all",
		len(sorted), total.String(), threshold.String())
}
