package ante_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/validatorgov/ante"
	"yamale/blockchain/x/validatorgov/keeper"
	module "yamale/blockchain/x/validatorgov/module"
	vgtestutil "yamale/blockchain/x/validatorgov/testutil"
	"yamale/blockchain/x/validatorgov/types"
)

// stubTx carries only what the gate decorator inspects: the tx's messages.
type stubTx struct{ msgs []sdk.Msg }

func (t stubTx) GetMsgs() []sdk.Msg                    { return t.msgs }
func (t stubTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }

// terminalAnteHandler ends the ante chain, recording that it was reached.
func terminalAnteHandler(reached *bool) sdk.AnteHandler {
	return func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		*reached = true
		return ctx, nil
	}
}

type gateFixture struct {
	env       *integration.Env
	keeper    keeper.Keeper
	msgServer types.MsgServer
	decorator ante.ValidatorGateDecorator
}

func initGateFixture(t *testing.T) *gateFixture {
	t.Helper()

	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority,
		vgtestutil.NewStakingKeeper(), vgtestutil.NewAuthzKeeper())
	require.NoError(t, k.Params.Set(env.Ctx, types.DefaultParams()))

	// The gate runs on every block after genesis.
	env.Ctx = env.Ctx.WithBlockHeight(1)

	return &gateFixture{
		env:       env,
		keeper:    k,
		msgServer: keeper.NewMsgServerImpl(k),
		decorator: ante.NewValidatorGateDecorator(k),
	}
}

// approveCandidate runs the full apply-then-govern flow and returns the
// candidate's account and operator (valoper) addresses.
func (f *gateFixture) approveCandidate(t *testing.T) (accStr, valoperStr string) {
	t.Helper()

	acc, accStr := f.env.Addr(t)
	_, err := f.msgServer.ApplyValidator(f.env.Ctx, &types.MsgApplyValidator{Creator: accStr})
	require.NoError(t, err)
	_, err = f.msgServer.ApproveValidator(f.env.Ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: accStr, Approve: true,
	})
	require.NoError(t, err)

	return accStr, sdk.ValAddress(acc).String()
}

func TestGateBlocksUnapprovedCandidate(t *testing.T) {
	f := initGateFixture(t)

	acc, _ := f.env.Addr(t)
	tx := stubTx{msgs: []sdk.Msg{
		&stakingtypes.MsgCreateValidator{ValidatorAddress: sdk.ValAddress(acc).String()},
	}}

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not approved by governance")
	require.False(t, reached, "the ante chain must stop at the gate")
}

// Applying is not enough — only a passed governance vote opens the gate.
func TestGateBlocksPendingApplicant(t *testing.T) {
	f := initGateFixture(t)

	acc, accStr := f.env.Addr(t)
	_, err := f.msgServer.ApplyValidator(f.env.Ctx, &types.MsgApplyValidator{Creator: accStr})
	require.NoError(t, err)

	tx := stubTx{msgs: []sdk.Msg{
		&stakingtypes.MsgCreateValidator{ValidatorAddress: sdk.ValAddress(acc).String()},
	}}

	reached := false
	_, err = f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.False(t, reached)
}

func TestGateBlocksRejectedCandidate(t *testing.T) {
	f := initGateFixture(t)

	acc, accStr := f.env.Addr(t)
	_, err := f.msgServer.ApplyValidator(f.env.Ctx, &types.MsgApplyValidator{Creator: accStr})
	require.NoError(t, err)
	_, err = f.msgServer.ApproveValidator(f.env.Ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: accStr, Approve: false,
	})
	require.NoError(t, err)

	tx := stubTx{msgs: []sdk.Msg{
		&stakingtypes.MsgCreateValidator{ValidatorAddress: sdk.ValAddress(acc).String()},
	}}

	reached := false
	_, err = f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.False(t, reached)
}

func TestGateAllowsApprovedCandidate(t *testing.T) {
	f := initGateFixture(t)

	_, valoperStr := f.approveCandidate(t)
	tx := stubTx{msgs: []sdk.Msg{&stakingtypes.MsgCreateValidator{ValidatorAddress: valoperStr}}}

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.NoError(t, err)
	require.True(t, reached)
}

// Approving one candidate must not open the gate for anyone else.
func TestGateApprovalIsPerCandidate(t *testing.T) {
	f := initGateFixture(t)

	f.approveCandidate(t)

	other, _ := f.env.Addr(t)
	tx := stubTx{msgs: []sdk.Msg{
		&stakingtypes.MsgCreateValidator{ValidatorAddress: sdk.ValAddress(other).String()},
	}}

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.False(t, reached)
}

// Genesis validators are onboarded through the gentx ceremony, so the gate
// does not apply at height 0.
func TestGateBypassedAtGenesis(t *testing.T) {
	f := initGateFixture(t)

	acc, _ := f.env.Addr(t)
	tx := stubTx{msgs: []sdk.Msg{
		&stakingtypes.MsgCreateValidator{ValidatorAddress: sdk.ValAddress(acc).String()},
	}}

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx.WithBlockHeight(0), tx, false, terminalAnteHandler(&reached))
	require.NoError(t, err)
	require.True(t, reached)
}

func TestGateIgnoresUnrelatedMessages(t *testing.T) {
	f := initGateFixture(t)

	tx := stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}, &stakingtypes.MsgDelegate{}}}

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.NoError(t, err)
	require.True(t, reached)
}

// An unapproved MsgCreateValidator bundled behind innocuous messages must
// still be caught.
func TestGateInspectsEveryMessageInTheTx(t *testing.T) {
	f := initGateFixture(t)

	acc, _ := f.env.Addr(t)
	tx := stubTx{msgs: []sdk.Msg{
		&banktypes.MsgSend{},
		&stakingtypes.MsgCreateValidator{ValidatorAddress: sdk.ValAddress(acc).String()},
	}}

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.False(t, reached)
}

// A tx mixing an approved and an unapproved candidate is rejected as a whole.
func TestGateRejectsMixedApprovedAndUnapproved(t *testing.T) {
	f := initGateFixture(t)

	_, approvedValoper := f.approveCandidate(t)
	other, _ := f.env.Addr(t)

	tx := stubTx{msgs: []sdk.Msg{
		&stakingtypes.MsgCreateValidator{ValidatorAddress: approvedValoper},
		&stakingtypes.MsgCreateValidator{ValidatorAddress: sdk.ValAddress(other).String()},
	}}

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.False(t, reached)
}

func TestGateRejectsMalformedValidatorAddress(t *testing.T) {
	f := initGateFixture(t)

	tx := stubTx{msgs: []sdk.Msg{&stakingtypes.MsgCreateValidator{ValidatorAddress: "not-a-valoper"}}}

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid validator operator address")
	require.False(t, reached)
}

// The gate runs in simulation too, so gas estimation for an unapproved
// candidate fails the same way the real submission would.
func TestGateAppliesDuringSimulation(t *testing.T) {
	f := initGateFixture(t)

	acc, _ := f.env.Addr(t)
	tx := stubTx{msgs: []sdk.Msg{
		&stakingtypes.MsgCreateValidator{ValidatorAddress: sdk.ValAddress(acc).String()},
	}}

	reached := false
	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, true, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.False(t, reached)
}
