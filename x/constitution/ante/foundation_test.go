package ante_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/cosmos/cosmos-sdk/x/group"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"

	constitutionante "yamale/blockchain/x/constitution/ante"
	"yamale/blockchain/x/constitution/types"
)

const (
	foundationID = uint64(1)
	otherGroupID = uint64(2)
)

// foundationPolicy is a 32-byte address, the length x/group derives a policy
// account at. Built from the global bech32 config rather than written out, so
// the tests do not depend on which prefix happens to be configured — what is
// under test is the rule, not the encoding.
var foundationPolicy = sdk.AccAddress([]byte("foundation-group-policy-32-bytes")).String()

func custodianAddress(i int) string {
	return sdk.AccAddress(fmt.Sprintf("custodian-%02d-------", i)).String()
}

// --- stubs -------------------------------------------------------------

type stubConstitution struct {
	invariants types.Invariants
	err        error
}

func (s stubConstitution) GetInvariants(context.Context) (types.Invariants, error) {
	return s.invariants, s.err
}

type stubGroup struct {
	policies  map[string]*group.GroupPolicyInfo
	members   map[uint64][]*group.GroupMember
	proposals map[uint64]*group.Proposal
}

func (s stubGroup) GroupPolicyInfo(_ context.Context, req *group.QueryGroupPolicyInfoRequest) (*group.QueryGroupPolicyInfoResponse, error) {
	info, ok := s.policies[req.Address]
	if !ok {
		return nil, errors.New("group policy: not found")
	}
	return &group.QueryGroupPolicyInfoResponse{Info: info}, nil
}

// GroupMembers truncates to the requested page, as x/group's own query does.
// A stub that returned everything regardless would hide exactly the bug the
// explicit limit in resultingMembers exists to prevent.
func (s stubGroup) GroupMembers(_ context.Context, req *group.QueryGroupMembersRequest) (*group.QueryGroupMembersResponse, error) {
	members := s.members[req.GroupId]
	if req.Pagination != nil && req.Pagination.Limit > 0 && uint64(len(members)) > req.Pagination.Limit {
		members = members[:req.Pagination.Limit]
	}
	return &group.QueryGroupMembersResponse{Members: members}, nil
}

func (s stubGroup) Proposal(_ context.Context, req *group.QueryProposalRequest) (*group.QueryProposalResponse, error) {
	proposal, ok := s.proposals[req.ProposalId]
	if !ok {
		return nil, errors.New("load proposal: not found")
	}
	return &group.QueryProposalResponse{Proposal: proposal}, nil
}

// stubTx carries only what the gate inspects: the transaction's messages.
type stubTx struct{ msgs []sdk.Msg }

func (t stubTx) GetMsgs() []sdk.Msg                    { return t.msgs }
func (t stubTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }

// --- fixture -----------------------------------------------------------

type fixture struct {
	decorator constitutionante.FoundationGuardDecorator
	ctx       sdk.Context
	groups    stubGroup
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	members := make([]*group.GroupMember, 0, 5)
	for i := 1; i <= 5; i++ {
		members = append(members, &group.GroupMember{
			GroupId: foundationID,
			Member:  &group.Member{Address: custodianAddress(i), Weight: "1"},
		})
	}

	groups := stubGroup{
		policies: map[string]*group.GroupPolicyInfo{
			foundationPolicy: {Address: foundationPolicy, GroupId: foundationID, Admin: foundationPolicy},
		},
		members: map[uint64][]*group.GroupMember{
			foundationID: members,
			otherGroupID: {{GroupId: otherGroupID, Member: &group.Member{Address: custodianAddress(9), Weight: "1"}}},
		},
		proposals: map[uint64]*group.Proposal{},
	}

	invariants := types.DefaultInvariants()
	invariants.EnforcementRecoveryDestination = foundationPolicy

	return fixture{
		decorator: constitutionante.NewFoundationGuardDecorator(stubConstitution{invariants: invariants}, groups),
		ctx:       sdk.Context{}.WithBlockHeight(10),
		groups:    groups,
	}
}

func (f fixture) run(t *testing.T, msgs ...sdk.Msg) (bool, error) {
	t.Helper()
	reached := false
	_, err := f.decorator.AnteHandle(f.ctx, stubTx{msgs: msgs}, false, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		reached = true
		return ctx, nil
	})
	return reached, err
}

func (f fixture) storeProposal(t *testing.T, id uint64, msgs ...sdk.Msg) {
	t.Helper()
	anys := make([]*codectypes.Any, 0, len(msgs))
	for _, msg := range msgs {
		any, err := codectypes.NewAnyWithValue(msg)
		require.NoError(t, err)
		anys = append(anys, any)
	}
	f.groups.proposals[id] = &group.Proposal{
		Id: id, GroupPolicyAddress: foundationPolicy, Messages: anys,
	}
}

func removeCustodian(i int) *group.MsgUpdateGroupMembers {
	return &group.MsgUpdateGroupMembers{
		Admin:         foundationPolicy,
		GroupId:       foundationID,
		MemberUpdates: []group.MemberRequest{{Address: custodianAddress(i), Weight: "0"}},
	}
}

func swapCustodian(outgoing int, incoming string) *group.MsgUpdateGroupMembers {
	return &group.MsgUpdateGroupMembers{
		Admin:   foundationPolicy,
		GroupId: foundationID,
		MemberUpdates: []group.MemberRequest{
			{Address: custodianAddress(outgoing), Weight: "0"},
			{Address: incoming, Weight: "1"},
		},
	}
}

// --- the rule ----------------------------------------------------------

// Removing a custodian without replacing them is the whole point of this gate.
// It is not that four custodians cannot work — it is that three-of-five is 60%
// and three-of-four is 75%, so the people who remain quietly hold more of the
// authority than the ceremony gave them, and the next departure makes it
// unanimity.
func TestRemovingACustodianWithoutAReplacementIsRefused(t *testing.T) {
	f := newFixture(t)

	reached, err := f.run(t, removeCustodian(2))

	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrConstitutionalInvariant)
	require.Contains(t, err.Error(), "replaced in the same message")
	require.False(t, reached, "the transaction must not reach the rest of the ante chain")
}

func TestSwappingACustodianIsAllowed(t *testing.T) {
	f := newFixture(t)

	reached, err := f.run(t, swapCustodian(2, custodianAddress(6)))

	require.NoError(t, err)
	require.True(t, reached)
}

func TestAddingASixthCustodianIsRefused(t *testing.T) {
	// The rule is an exact count, not a floor. Six of whom three must sign is
	// 50%, which is not a supermajority of anything.
	f := newFixture(t)

	reached, err := f.run(t, &group.MsgUpdateGroupMembers{
		Admin:         foundationPolicy,
		GroupId:       foundationID,
		MemberUpdates: []group.MemberRequest{{Address: custodianAddress(6), Weight: "1"}},
	})

	require.Error(t, err)
	require.False(t, reached)
}

func TestRemovingTwoAndAddingTwoIsAllowed(t *testing.T) {
	f := newFixture(t)

	reached, err := f.run(t, &group.MsgUpdateGroupMembers{
		Admin:   foundationPolicy,
		GroupId: foundationID,
		MemberUpdates: []group.MemberRequest{
			{Address: custodianAddress(1), Weight: "0"},
			{Address: custodianAddress(2), Weight: "0"},
			{Address: custodianAddress(6), Weight: "1"},
			{Address: custodianAddress(7), Weight: "1"},
		},
	})

	require.NoError(t, err)
	require.True(t, reached)
}

func TestRemovingOneAndAddingSomebodyAlreadyInTheGroupIsRefused(t *testing.T) {
	// The swap that is not a swap: custodian 2 leaves and custodian 3, who is
	// already a member, is "added". x/group would apply it and the group would
	// be four. The count is computed against the members in the store rather
	// than from the message, which is what catches this.
	f := newFixture(t)

	reached, err := f.run(t, swapCustodian(2, custodianAddress(3)))

	require.Error(t, err)
	require.Contains(t, err.Error(), "4 custodians")
	require.False(t, reached)
}

func TestACustodianCannotLeaveOnTheirOwnSignature(t *testing.T) {
	// MsgLeaveGroup is the message the brief did not name and the one that
	// matters most: it needs no admin, no proposal and no other custodian's
	// agreement, and it changes how much authority everybody else holds.
	f := newFixture(t)

	reached, err := f.run(t, &group.MsgLeaveGroup{Address: custodianAddress(3), GroupId: foundationID})

	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrConstitutionalInvariant)
	require.False(t, reached)
}

func TestOtherGroupsAreUntouched(t *testing.T) {
	// This chain has an x/group for everybody. A gate that made every group on
	// it immutable would be a much larger change than the one asked for.
	//
	// The removal below names a *foundation* custodian's address, deliberately.
	// A gate that ignored the group id would compute four members and refuse
	// this; one that checks the id lets it through because group 2 is not the
	// foundation. Written the obvious way — removing a member of the other
	// group — this test passed with the group-id check deleted, which means it
	// was asserting nothing.
	f := newFixture(t)

	reached, err := f.run(t,
		&group.MsgLeaveGroup{Address: custodianAddress(9), GroupId: otherGroupID},
		&group.MsgUpdateGroupMembers{
			Admin:         custodianAddress(9),
			GroupId:       otherGroupID,
			MemberUpdates: []group.MemberRequest{{Address: custodianAddress(2), Weight: "0"}},
		},
	)

	require.NoError(t, err)
	require.True(t, reached)
}

// --- the routes around it ----------------------------------------------

// The foundation group is its own admin, so a MsgUpdateGroupMembers for it can
// only be signed by the group policy address — which no private key produces.
// The only way it executes is inside a group proposal. This is therefore not an
// edge case: it is the *normal* path, and a gate that only looked at top-level
// messages would catch nothing at all.
func TestARemovalInsideAGroupProposalIsRefused(t *testing.T) {
	f := newFixture(t)

	proposal, err := group.NewMsgSubmitProposal(
		foundationPolicy,
		[]string{custodianAddress(1)},
		[]sdk.Msg{removeCustodian(2)},
		"", group.Exec_EXEC_UNSPECIFIED,
		"Retire custodian 2", "Custodian 2 is stepping down.",
	)
	require.NoError(t, err)

	reached, err := f.run(t, proposal)

	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrConstitutionalInvariant)
	require.False(t, reached)
}

func TestASwapInsideAGroupProposalIsAllowed(t *testing.T) {
	f := newFixture(t)

	proposal, err := group.NewMsgSubmitProposal(
		foundationPolicy,
		[]string{custodianAddress(1)},
		[]sdk.Msg{swapCustodian(2, custodianAddress(6))},
		"", group.Exec_EXEC_UNSPECIFIED,
		"Replace custodian 2", "Custodian 2 is stepping down; custodian 6 takes their place.",
	)
	require.NoError(t, err)

	reached, err := f.run(t, proposal)

	require.NoError(t, err)
	require.True(t, reached)
}

// A proposal submitted before this gate existed — an upgrade, in other words —
// would still be sitting in the store waiting to be executed. MsgExec carries
// only a proposal id, so the gate has to resolve it.
func TestExecutingAStoredProposalThatRemovesACustodianIsRefused(t *testing.T) {
	f := newFixture(t)
	f.storeProposal(t, 7, removeCustodian(2))

	reached, err := f.run(t, &group.MsgExec{ProposalId: 7, Executor: custodianAddress(1)})

	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrConstitutionalInvariant)
	require.False(t, reached)
}

func TestExecutingAStoredSwapIsAllowed(t *testing.T) {
	f := newFixture(t)
	f.storeProposal(t, 8, swapCustodian(2, custodianAddress(6)))

	reached, err := f.run(t, &group.MsgExec{ProposalId: 8, Executor: custodianAddress(1)})

	require.NoError(t, err)
	require.True(t, reached)
}

func TestATryExecutingVoteIsResolvedToo(t *testing.T) {
	// MsgVote carries an exec field, so the vote that reaches the threshold can
	// execute the proposal in the same transaction. A gate that only watched
	// MsgExec would miss it.
	f := newFixture(t)
	f.storeProposal(t, 9, removeCustodian(2))

	reached, err := f.run(t, &group.MsgVote{
		ProposalId: 9, Voter: custodianAddress(3),
		Option: group.VOTE_OPTION_YES, Exec: group.Exec_EXEC_TRY,
	})

	require.Error(t, err)
	require.False(t, reached)
}

func TestAnOrdinaryVoteIsNotBlocked(t *testing.T) {
	// A vote that does not try to execute changes nothing on its own, and the
	// MsgExec that follows is caught. Blocking it would mean a custodian could
	// not even vote no on a proposal the gate will refuse.
	f := newFixture(t)
	f.storeProposal(t, 10, removeCustodian(2))

	reached, err := f.run(t, &group.MsgVote{
		ProposalId: 10, Voter: custodianAddress(3),
		Option: group.VOTE_OPTION_NO, Exec: group.Exec_EXEC_UNSPECIFIED,
	})

	require.NoError(t, err)
	require.True(t, reached)
}

func TestAuthzCannotCarryItPast(t *testing.T) {
	f := newFixture(t)

	leave := &group.MsgLeaveGroup{Address: custodianAddress(3), GroupId: foundationID}
	exec := authztypes.NewMsgExec(sdk.MustAccAddressFromBech32(custodianAddress(3)), []sdk.Msg{leave})

	reached, err := f.run(t, &exec)

	require.Error(t, err)
	require.False(t, reached)
}

func TestGovernanceCannotCarryItPast(t *testing.T) {
	// x/gov executes what it carries through the message router as well. The
	// admin check in x/group would reject this one on its own, but relying on
	// that would mean the gate's coverage depended on another module's rules.
	f := newFixture(t)

	proposal, err := govv1.NewMsgSubmitProposal(
		[]sdk.Msg{removeCustodian(2)}, nil, custodianAddress(1), "",
		"Retire custodian 2", "Custodian 2 is stepping down.", false,
	)
	require.NoError(t, err)

	reached, err := f.run(t, proposal)

	require.Error(t, err)
	require.False(t, reached)
}

func TestNestingBeyondTheLimitIsRefusedOutright(t *testing.T) {
	// The depth bound must not become the way around the check: past it the
	// transaction is refused rather than passed through.
	//
	// What is nested is a swap, which the gate would otherwise allow. Nesting a
	// removal instead makes this test pass whether the bound exists or not,
	// because the removal is caught on its own merits — which is exactly what
	// it did before, so it was testing the wrong thing.
	f := newFixture(t)

	signer := sdk.MustAccAddressFromBech32(custodianAddress(1))
	var msg sdk.Msg = swapCustodian(2, custodianAddress(6))
	for i := 0; i < 8; i++ {
		exec := authztypes.NewMsgExec(signer, []sdk.Msg{msg})
		msg = &exec
	}

	reached, err := f.run(t, msg)

	require.Error(t, err)
	require.Contains(t, err.Error(), "nests messages more than")
	require.False(t, reached)
}

func TestNestingWithinTheLimitStillWorks(t *testing.T) {
	// The bound has to leave room for legitimate nesting, or it is a ban on
	// authz rather than a denial-of-service guard.
	f := newFixture(t)

	signer := sdk.MustAccAddressFromBech32(custodianAddress(1))
	exec := authztypes.NewMsgExec(signer, []sdk.Msg{swapCustodian(2, custodianAddress(6))})

	reached, err := f.run(t, &exec)

	require.NoError(t, err)
	require.True(t, reached)
}

// --- the threshold and the admin ---------------------------------------

func TestLoweringTheSignatureThresholdIsRefused(t *testing.T) {
	// Without this, three custodians could vote to make it two, and the
	// arrangement a ceremony established would be changed in one proposal by
	// the people it constrains.
	f := newFixture(t)

	msg, err := group.NewMsgUpdateGroupPolicyDecisionPolicy(
		sdk.MustAccAddressFromBech32(foundationPolicy),
		sdk.MustAccAddressFromBech32(foundationPolicy),
		group.NewThresholdDecisionPolicy("2", 0, 0),
	)
	require.NoError(t, err)

	reached, err := f.run(t, msg)

	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrConstitutionalInvariant)
	require.False(t, reached)
}

func TestRestatingTheSameThresholdIsAllowed(t *testing.T) {
	// Re-installing the same policy with a different voting window is
	// housekeeping, and refusing it would leave the custodians unable to change
	// anything about how they operate.
	f := newFixture(t)

	msg, err := group.NewMsgUpdateGroupPolicyDecisionPolicy(
		sdk.MustAccAddressFromBech32(foundationPolicy),
		sdk.MustAccAddressFromBech32(foundationPolicy),
		group.NewThresholdDecisionPolicy("3", 0, 0),
	)
	require.NoError(t, err)

	reached, err := f.run(t, msg)

	require.NoError(t, err)
	require.True(t, reached)
}

func TestAPercentagePolicyIsRefused(t *testing.T) {
	// A percentage makes the threshold follow the membership, which is exactly
	// the drift the fixed count exists to stop.
	f := newFixture(t)

	msg, err := group.NewMsgUpdateGroupPolicyDecisionPolicy(
		sdk.MustAccAddressFromBech32(foundationPolicy),
		sdk.MustAccAddressFromBech32(foundationPolicy),
		group.NewPercentageDecisionPolicy("0.6", 0, 0),
	)
	require.NoError(t, err)

	reached, err := f.run(t, msg)

	require.Error(t, err)
	require.False(t, reached)
}

func TestTheAdminCannotBeMovedOffTheGroup(t *testing.T) {
	// The group administering itself is what makes its membership changeable
	// only by the custodians. Moving the admin is one transaction away from
	// reintroducing the single key this whole design abolishes.
	f := newFixture(t)

	reached, err := f.run(t, &group.MsgUpdateGroupAdmin{
		GroupId: foundationID, Admin: foundationPolicy, NewAdmin: custodianAddress(1),
	})
	require.Error(t, err)
	require.False(t, reached)

	reached, err = f.run(t, &group.MsgUpdateGroupPolicyAdmin{
		GroupPolicyAddress: foundationPolicy, Admin: foundationPolicy, NewAdmin: custodianAddress(1),
	})
	require.Error(t, err)
	require.False(t, reached)
}

// --- when the gate has nothing to say ----------------------------------

func TestAChainWhoseFoundationIsAnOrdinaryAccountIsUnaffected(t *testing.T) {
	// A recovery destination that is not a group policy is a legitimate state —
	// a chain that has not migrated yet. Refusing here would stop it processing
	// any transaction at all, which is worse than the problem being guarded
	// against.
	invariants := types.DefaultInvariants()
	invariants.EnforcementRecoveryDestination = custodianAddress(1)

	decorator := constitutionante.NewFoundationGuardDecorator(
		stubConstitution{invariants: invariants},
		stubGroup{policies: map[string]*group.GroupPolicyInfo{}},
	)

	reached := false
	_, err := decorator.AnteHandle(
		sdk.Context{}.WithBlockHeight(10),
		stubTx{msgs: []sdk.Msg{removeCustodian(2)}},
		false,
		func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { reached = true; return ctx, nil },
	)

	require.NoError(t, err)
	require.True(t, reached)
}

func TestTwoSwapsThatInteractAreCaughtAtExecution(t *testing.T) {
	// Each of these is a perfectly good swap when it is submitted. Together
	// they are not: both bring in custodian 6, so once the first has executed
	// the second removes somebody and "adds" a member who is already there,
	// leaving four. Nothing at submission time can see it — which is why the
	// gate resolves a proposal again at execution, against the members in the
	// store at that moment.
	f := newFixture(t)

	first, err := group.NewMsgSubmitProposal(foundationPolicy, []string{custodianAddress(1)},
		[]sdk.Msg{swapCustodian(2, custodianAddress(6))}, "", group.Exec_EXEC_UNSPECIFIED, "a", "a")
	require.NoError(t, err)
	second, err := group.NewMsgSubmitProposal(foundationPolicy, []string{custodianAddress(1)},
		[]sdk.Msg{swapCustodian(3, custodianAddress(6))}, "", group.Exec_EXEC_UNSPECIFIED, "b", "b")
	require.NoError(t, err)

	reached, err := f.run(t, first)
	require.NoError(t, err, "the first swap is valid when submitted")
	require.True(t, reached)

	reached, err = f.run(t, second)
	require.NoError(t, err, "the second swap is also valid when submitted")
	require.True(t, reached)

	// The first has now executed: custodian 2 is out, custodian 6 is in.
	f.groups.members[foundationID] = []*group.GroupMember{
		{GroupId: foundationID, Member: &group.Member{Address: custodianAddress(1), Weight: "1"}},
		{GroupId: foundationID, Member: &group.Member{Address: custodianAddress(3), Weight: "1"}},
		{GroupId: foundationID, Member: &group.Member{Address: custodianAddress(4), Weight: "1"}},
		{GroupId: foundationID, Member: &group.Member{Address: custodianAddress(5), Weight: "1"}},
		{GroupId: foundationID, Member: &group.Member{Address: custodianAddress(6), Weight: "1"}},
	}
	f.storeProposal(t, 20, swapCustodian(3, custodianAddress(6)))

	reached, err = f.run(t, &group.MsgExec{ProposalId: 20, Executor: custodianAddress(1)})
	require.Error(t, err, "executing the second swap would leave four custodians")
	require.Contains(t, err.Error(), "4 custodians")
	require.False(t, reached)
}

func TestAGroupTooLargeToReadIsRefusedRatherThanUndercounted(t *testing.T) {
	// x/group's member query pages. A group larger than the page this asks for
	// would be silently undercounted, and every legitimate change to it
	// refused for a reason nobody could work out.
	f := newFixture(t)

	oversized := make([]*group.GroupMember, 0, types.MaxFoundationCustodians+1)
	for i := 0; i <= types.MaxFoundationCustodians; i++ {
		oversized = append(oversized, &group.GroupMember{
			GroupId: foundationID,
			Member:  &group.Member{Address: custodianAddress(100 + i), Weight: "1"},
		})
	}
	f.groups.members[foundationID] = oversized

	_, err := f.run(t, removeCustodian(2))
	require.Error(t, err)
	require.Contains(t, err.Error(), "more than the constitution can describe")
}

func TestATransactionWithNoGroupMessageReadsNoState(t *testing.T) {
	// This decorator is in the chain every transaction on the network passes
	// through. Resolving the foundation costs two store reads, and paying them
	// on every bank send is a tax on the whole chain for a rule about a handful
	// of message types.
	counted := &countingGroup{stubGroup: newFixture(t).groups}
	invariants := types.DefaultInvariants()
	invariants.EnforcementRecoveryDestination = foundationPolicy

	decorator := constitutionante.NewFoundationGuardDecorator(
		stubConstitution{invariants: invariants}, counted)

	reached := false
	_, err := decorator.AnteHandle(
		sdk.Context{}.WithBlockHeight(10),
		stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{FromAddress: custodianAddress(1), ToAddress: custodianAddress(2)}}},
		false,
		func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { reached = true; return ctx, nil },
	)

	require.NoError(t, err)
	require.True(t, reached)
	require.Zero(t, counted.lookups, "a bank send made the gate read the constitution and the group table")

	// A group message must still be checked, or the pre-filter has become the
	// way around the gate.
	_, err = decorator.AnteHandle(
		sdk.Context{}.WithBlockHeight(10),
		stubTx{msgs: []sdk.Msg{removeCustodian(2)}},
		false,
		func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil },
	)
	require.Error(t, err)
	require.NotZero(t, counted.lookups)
}

type countingGroup struct {
	stubGroup
	lookups int
}

func (c *countingGroup) GroupPolicyInfo(ctx context.Context, req *group.QueryGroupPolicyInfoRequest) (*group.QueryGroupPolicyInfoResponse, error) {
	c.lookups++
	return c.stubGroup.GroupPolicyInfo(ctx, req)
}

func TestGenesisIsNotGated(t *testing.T) {
	// The foundation group is created by the genesis file itself, so at height
	// zero there is nothing to protect and nothing to look up.
	f := newFixture(t)
	f.ctx = sdk.Context{}.WithBlockHeight(0)

	reached, err := f.run(t, removeCustodian(2))

	require.NoError(t, err)
	require.True(t, reached)
}

func TestAnUnreadableWeightIsRefusedRatherThanGuessedAt(t *testing.T) {
	// An unparseable weight that this gate guessed at could make a removal look
	// like an addition and pass a group of four.
	f := newFixture(t)

	reached, err := f.run(t, &group.MsgUpdateGroupMembers{
		Admin:         foundationPolicy,
		GroupId:       foundationID,
		MemberUpdates: []group.MemberRequest{{Address: custodianAddress(2), Weight: "none"}},
	})

	require.Error(t, err)
	require.False(t, reached)
}

func TestAZeroWrittenAsADecimalStillRemoves(t *testing.T) {
	// x/group parses weights as decimals, so "0.0" removes a member exactly as
	// "0" does. A gate comparing the string to "0" would pass this.
	f := newFixture(t)

	reached, err := f.run(t, &group.MsgUpdateGroupMembers{
		Admin:         foundationPolicy,
		GroupId:       foundationID,
		MemberUpdates: []group.MemberRequest{{Address: custodianAddress(2), Weight: "0.0"}},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "4 custodians")
	require.False(t, reached)
}
