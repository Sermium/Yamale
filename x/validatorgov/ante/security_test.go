package ante_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
)

// The gate is the only thing making this chain's validator set permissioned, so
// what matters is not that it rejects a bare MsgCreateValidator — that is
// already covered — but that it cannot be walked around.
//
// It inspected only the transaction's top-level messages. x/authz's MsgExec
// carries other messages inside it and dispatches them through the message
// router *after* the ante chain has run, so a MsgCreateValidator wrapped in one
// never met the gate at all. Anyone could grant themselves the authorisation —
// a grant to yourself is a normal, unprivileged transaction — and then join the
// validator set of a permissioned chain without a vote.
func TestTheGateCannotBeBypassedByNestingTheMessage(t *testing.T) {
	f := initGateFixture(t)

	_, candidateStr := f.env.Addr(t)
	valAddr := sdk.ValAddress(sdk.MustAccAddressFromBech32(candidateStr)).String()

	inner := &stakingtypes.MsgCreateValidator{ValidatorAddress: valAddr}
	exec := authztypes.NewMsgExec(sdk.MustAccAddressFromBech32(candidateStr), []sdk.Msg{inner})

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, stubTx{msgs: []sdk.Msg{&exec}}, false, terminalAnteHandler(&reached))

	require.Error(t, err, "a nested MsgCreateValidator must be refused like a bare one")
	require.False(t, reached, "the transaction must not reach the rest of the ante chain")
}

// Nesting it twice is the same attack with one more layer, and a check that
// only unwraps a single level would pass the test above and fail here.
func TestTheGateUnwrapsRepeatedNesting(t *testing.T) {
	f := initGateFixture(t)

	_, candidateStr := f.env.Addr(t)
	candidate := sdk.MustAccAddressFromBech32(candidateStr)
	valAddr := sdk.ValAddress(candidate).String()

	inner := &stakingtypes.MsgCreateValidator{ValidatorAddress: valAddr}
	once := authztypes.NewMsgExec(candidate, []sdk.Msg{inner})
	twice := authztypes.NewMsgExec(candidate, []sdk.Msg{&once})

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, stubTx{msgs: []sdk.Msg{&twice}}, false, terminalAnteHandler(&reached))

	require.Error(t, err)
	require.False(t, reached)
}

// An approved candidate must still be able to join, nested or not — the gate
// exists to check approval, not to ban a message shape.
func TestAnApprovedCandidateMayStillUseAuthz(t *testing.T) {
	f := initGateFixture(t)

	candidateStr, valAddr := f.approveCandidate(t)
	candidate := sdk.MustAccAddressFromBech32(candidateStr)

	inner := &stakingtypes.MsgCreateValidator{ValidatorAddress: valAddr}
	exec := authztypes.NewMsgExec(candidate, []sdk.Msg{inner})

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, stubTx{msgs: []sdk.Msg{&exec}}, false, terminalAnteHandler(&reached))

	require.NoError(t, err)
	require.True(t, reached)
}
