package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/builderfee/keeper"
	"yamale/blockchain/x/builderfee/types"
)

// A post handler runs after the message has already succeeded, so an error
// returned from it undoes work that was valid. The fee split is a distribution
// nicety; failing somebody's payment because a *builder's share* could not be
// paid is disproportionate — and the sender cannot avoid it, because the payout
// address is chosen by governance rather than by them.
//
// A builder whose address later became unpayable — the simplest case being a
// blocked module account — would otherwise have taken down every transaction
// carrying that message type.
func TestAnUnpayableBuilderDoesNotFailTheTransaction(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	params := types.DefaultParams()
	params.BuilderFeeShareBps = 5000
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	// A module account blocked from receiving ordinary transfers, which is
	// exactly the shape of an address that stops being payable after governance
	// approved it.
	blocked := f.env.AuthKeeper.GetModuleAddress(types.ModuleName)
	require.NotEmpty(t, blocked, "the module account must exist for this test to mean anything")
	f.env.Block(blocked)

	// The payout address is chosen when the builder registers; governance only
	// approves the registration.
	msgType := sdk.MsgTypeURL(&banktypes.MsgSend{})
	_, creatorStr := f.env.Addr(t)
	_, err := ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: creatorStr, MsgTypeUrl: msgType, PayoutAddress: blocked.String(),
	})
	require.NoError(t, err)
	_, err = ms.ApproveBuilder(f.ctx, &types.MsgApproveBuilder{
		Authority: f.env.AuthorityString(t), MsgTypeUrl: msgType, Approve: true,
	})
	require.NoError(t, err)

	// Fees the ante handler would already have collected.
	fee := sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(1_000)))
	f.env.FundModule(t, authtypes.FeeCollectorName, fee)

	decorator := keeper.NewFeeShareDecorator(f.keeper)
	reached := false
	_, err = decorator.PostHandle(f.env.Ctx, stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}, fee: fee}, false, true,
		func(ctx sdk.Context, _ sdk.Tx, _, _ bool) (sdk.Context, error) {
			reached = true
			return ctx, nil
		})

	require.NoError(t, err, "an unpayable builder must not fail an already-successful transaction")
	require.True(t, reached, "the post-handler chain must continue")

	// The share stays where it would have gone with no builder registered.
	require.Equal(t, math.NewInt(1_000), f.env.ModuleBalance(authtypes.FeeCollectorName, "uyml"))
}
