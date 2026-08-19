package ante_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/validatorgov/ante"
	"yamale/blockchain/x/validatorgov/types"
)

// Without this gate the concentration ceilings are advisory.
//
// A demotion is carried out by jailing, and x/staking's Jail sets no
// jailed-until time, so the ordinary unjail path is open the moment the block
// ends. A validator over its ceiling could send MsgUnjail every block and be
// back in the set until the next epoch demoted it again — forever, and at the
// price of one transaction fee.
func TestDemotedValidatorCannotUnjailItself(t *testing.T) {
	f := initGateFixture(t)
	decorator := ante.NewDemotionGateDecorator(f.keeper)

	operator, operatorStr := f.env.Addr(t)
	require.NoError(t, f.keeper.Demotion.Set(f.env.Ctx, operatorStr, types.Demotion{
		Operator: operatorStr,
		Cap:      types.CONCENTRATION_CAP_ENTITY,
		Group:    "ENTITY-BIG",
	}))

	unjail := &slashingtypes.MsgUnjail{ValidatorAddr: sdk.ValAddress(operator).String()}

	reached := false
	_, err := decorator.AnteHandle(f.env.Ctx, stubTx{msgs: []sdk.Msg{unjail}}, false, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.ErrorContains(t, err, "concentration ceiling")
	require.False(t, reached)
}

// The gate has to follow MsgExec. authz dispatches what it carries after the
// ante chain has run, and a grant to yourself is an ordinary unprivileged
// transaction — so a gate that inspected only the top level would be one
// wrapper away from being no gate at all.
func TestDemotedValidatorCannotUnjailThroughAuthz(t *testing.T) {
	f := initGateFixture(t)
	decorator := ante.NewDemotionGateDecorator(f.keeper)

	operator, operatorStr := f.env.Addr(t)
	require.NoError(t, f.keeper.Demotion.Set(f.env.Ctx, operatorStr, types.Demotion{
		Operator: operatorStr, Cap: types.CONCENTRATION_CAP_ENTITY, Group: "ENTITY-BIG",
	}))

	inner := &slashingtypes.MsgUnjail{ValidatorAddr: sdk.ValAddress(operator).String()}
	exec := authztypes.NewMsgExec(operator, []sdk.Msg{inner})

	reached := false
	_, err := decorator.AnteHandle(f.env.Ctx, stubTx{msgs: []sdk.Msg{&exec}}, false, terminalAnteHandler(&reached))
	require.Error(t, err)
	require.ErrorContains(t, err, "concentration ceiling")
	require.False(t, reached)
}

// A validator nobody has demoted unjails normally. A gate that refused
// everything would be indistinguishable from one that worked, until the first
// operator recovered from downtime.
func TestUndemotedValidatorMayUnjail(t *testing.T) {
	f := initGateFixture(t)
	decorator := ante.NewDemotionGateDecorator(f.keeper)

	operator, _ := f.env.Addr(t)
	unjail := &slashingtypes.MsgUnjail{ValidatorAddr: sdk.ValAddress(operator).String()}

	reached := false
	_, err := decorator.AnteHandle(f.env.Ctx, stubTx{msgs: []sdk.Msg{unjail}}, false, terminalAnteHandler(&reached))
	require.NoError(t, err)
	require.True(t, reached)
}
