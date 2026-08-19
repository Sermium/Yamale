package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/validatorgov/keeper"
	"yamale/blockchain/x/validatorgov/types"
)

// keeperMsgServer is the module's own message server over the fixture's keeper.
func keeperMsgServer(f *fixture) types.MsgServer {
	return keeper.NewMsgServerImpl(f.keeper)
}

// consAddrOf is the consensus address behind an operator account, which is what
// x/staking keys jailing by.
func (f *fixture) consAddrOf(t *testing.T, operator sdk.AccAddress) sdk.ConsAddress {
	t.Helper()
	validator, err := f.staking.GetValidator(f.env.Ctx, sdk.ValAddress(operator))
	require.NoError(t, err)
	consAddr, err := validator.GetConsAddr()
	require.NoError(t, err)
	return consAddr
}
