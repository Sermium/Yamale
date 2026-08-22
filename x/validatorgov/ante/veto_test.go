package ante_test

import (
	"testing"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txsigning "github.com/cosmos/cosmos-sdk/types/tx/signing"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"

	"yamale/blockchain/testutil/integration"
	constitutiontestutil "yamale/blockchain/x/constitution/testutil"
	constitutiontypes "yamale/blockchain/x/constitution/types"
	"yamale/blockchain/x/validatorgov/ante"
	"yamale/blockchain/x/validatorgov/keeper"
	module "yamale/blockchain/x/validatorgov/module"
	vgtestutil "yamale/blockchain/x/validatorgov/testutil"
	"yamale/blockchain/x/validatorgov/types"
)

// signedTx carries what the veto decorator inspects: who signed. The messages
// are deliberately irrelevant — the rule is about the signature, not about what
// was signed, and a test that used a validatorgov message would not show that.
type signedTx struct {
	msgs    []sdk.Msg
	signers [][]byte
}

func (t signedTx) GetMsgs() []sdk.Msg                                { return t.msgs }
func (t signedTx) GetMsgsV2() ([]protov2.Message, error)             { return nil, nil }
func (t signedTx) GetSigners() ([][]byte, error)                     { return t.signers, nil }
func (t signedTx) GetPubKeys() ([]cryptotypes.PubKey, error)         { return nil, nil }
func (t signedTx) GetSignaturesV2() ([]txsigning.SignatureV2, error) { return nil, nil }

type vetoFixture struct {
	env       *integration.Env
	keeper    keeper.Keeper
	msgServer types.MsgServer
	staking   *vgtestutil.StakingKeeper
	decorator ante.OperatorVetoDecorator
}

func initVetoFixture(t *testing.T) *vetoFixture {
	t.Helper()

	env := integration.NewWith(t, []string{types.ModuleName, constitutiontypes.ModuleName}, module.AppModule{})
	staking := vgtestutil.NewStakingKeeper()
	_, destination := env.Addr(t)
	constitution := constitutiontestutil.Init(t, env, staking, constitutiontestutil.Invariants(destination))
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority,
		staking, vgtestutil.NewAuthzKeeper(), env.AuthKeeper, env.BankKeeper, constitution, nil)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))

	env.Ctx = env.Ctx.WithBlockHeight(1)

	return &vetoFixture{
		env:       env,
		keeper:    k,
		msgServer: keeper.NewMsgServerImpl(k),
		staking:   staking,
		decorator: ante.NewOperatorVetoDecorator(k),
	}
}

// openApprovedRecovery admits an operator, gives it a validator, and drives a
// recovery against it all the way to an approved, running challenge window.
func (f *vetoFixture) openApprovedRecovery(t *testing.T) (operator sdk.AccAddress, operatorStr string, rotationID uint64) {
	t.Helper()

	operator, operatorStr = f.env.Addr(t)
	_, err := f.msgServer.ApplyValidator(f.env.Ctx, &types.MsgApplyValidator{Creator: operatorStr, LegalEntityId: "LEI-TEST", BeneficialOwnerId: "OWNER-TEST", Jurisdiction: "CH"})
	require.NoError(t, err)
	_, err = f.msgServer.ApproveValidator(f.env.Ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: operatorStr, Approve: true,
	})
	require.NoError(t, err)
	f.staking.AddValidator(operator)

	_, incoming := f.env.Addr(t)
	proposed, err := f.msgServer.ProposeOperatorRecovery(f.env.Ctx, &types.MsgProposeOperatorRecovery{
		Creator: incoming, CurrentOperator: operatorStr, NewOperator: incoming, Reason: "key lost",
	})
	require.NoError(t, err)
	_, err = f.msgServer.ApproveOperatorRecovery(f.env.Ctx, &types.MsgApproveOperatorRecovery{
		Authority: f.env.AuthorityString(t), RotationId: proposed.RotationId, Approve: true,
	})
	require.NoError(t, err)

	return operator, operatorStr, proposed.RotationId
}

func (f *vetoFixture) status(t *testing.T, id uint64) types.RotationStatus {
	t.Helper()
	rotation, err := f.keeper.Rotation.Get(f.env.Ctx, id)
	require.NoError(t, err)
	return rotation.Status
}

func TestVetoDecoratorCancelsRecoveryOnAnyTransaction(t *testing.T) {
	f := initVetoFixture(t)
	operator, operatorStr, id := f.openApprovedRecovery(t)
	require.True(t, f.staking.IsJailed(operator))

	// An ordinary send. Nothing about it concerns validatorgov.
	reached := false
	tx := signedTx{
		msgs:    []sdk.Msg{&banktypes.MsgSend{FromAddress: operatorStr}},
		signers: [][]byte{operator},
	}

	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.NoError(t, err)
	require.True(t, reached, "the veto must not block the transaction that triggered it")

	require.Equal(t, types.ROTATION_STATUS_VETOED, f.status(t, id))
	require.False(t, f.staking.IsJailed(operator), "the pause lifts with the recovery")
}

func TestVetoDecoratorIgnoresOtherSigners(t *testing.T) {
	f := initVetoFixture(t)
	operator, _, id := f.openApprovedRecovery(t)

	stranger, strangerStr := f.env.Addr(t)
	reached := false
	tx := signedTx{
		msgs:    []sdk.Msg{&banktypes.MsgSend{FromAddress: strangerStr}},
		signers: [][]byte{stranger},
	}

	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.NoError(t, err)
	require.True(t, reached)

	require.Equal(t, types.ROTATION_STATUS_PENDING, f.status(t, id))
	require.True(t, f.staking.IsJailed(operator))
}

// A simulated transaction has not had its signature verified, so it is not
// evidence that anybody holds anything. Vetoing on one would let anybody keep a
// validator unrecoverable for free, without ever paying a fee.
func TestVetoDecoratorIgnoresSimulation(t *testing.T) {
	f := initVetoFixture(t)
	operator, operatorStr, id := f.openApprovedRecovery(t)

	reached := false
	tx := signedTx{
		msgs:    []sdk.Msg{&banktypes.MsgSend{FromAddress: operatorStr}},
		signers: [][]byte{operator},
	}

	_, err := f.decorator.AnteHandle(f.env.Ctx, tx, true, terminalAnteHandler(&reached))
	require.NoError(t, err)
	require.True(t, reached)

	require.Equal(t, types.ROTATION_STATUS_PENDING, f.status(t, id))
}

// CheckTx runs against a state that is discarded at the next commit. A veto
// recorded there would exist in the mempool's view of the chain and nowhere
// else, so the recovery would still complete on the state that counts.
func TestVetoDecoratorIgnoresCheckTx(t *testing.T) {
	f := initVetoFixture(t)
	operator, operatorStr, id := f.openApprovedRecovery(t)

	reached := false
	tx := signedTx{
		msgs:    []sdk.Msg{&banktypes.MsgSend{FromAddress: operatorStr}},
		signers: [][]byte{operator},
	}

	_, err := f.decorator.AnteHandle(f.env.Ctx.WithIsCheckTx(true), tx, false, terminalAnteHandler(&reached))
	require.NoError(t, err)
	require.True(t, reached)

	require.Equal(t, types.ROTATION_STATUS_PENDING, f.status(t, id))
}

func TestVetoDecoratorLeavesPlannedRotationsAlone(t *testing.T) {
	f := initVetoFixture(t)

	operator, operatorStr := f.env.Addr(t)
	_, err := f.msgServer.ApplyValidator(f.env.Ctx, &types.MsgApplyValidator{Creator: operatorStr, LegalEntityId: "LEI-TEST", BeneficialOwnerId: "OWNER-TEST", Jurisdiction: "CH"})
	require.NoError(t, err)
	_, err = f.msgServer.ApproveValidator(f.env.Ctx, &types.MsgApproveValidator{
		Authority: f.env.AuthorityString(t), Candidate: operatorStr, Approve: true,
	})
	require.NoError(t, err)

	_, incoming := f.env.Addr(t)
	planned, err := f.msgServer.RotateOperator(f.env.Ctx, &types.MsgRotateOperator{
		Creator: operatorStr, NewOperator: incoming,
	})
	require.NoError(t, err)

	reached := false
	tx := signedTx{
		msgs:    []sdk.Msg{&banktypes.MsgSend{FromAddress: operatorStr}},
		signers: [][]byte{operator},
	}
	_, err = f.decorator.AnteHandle(f.env.Ctx, tx, false, terminalAnteHandler(&reached))
	require.NoError(t, err)

	require.Equal(t, types.ROTATION_STATUS_PENDING, f.status(t, planned.RotationId),
		"an operator's own transactions must not cancel the rotation they asked for")
}
