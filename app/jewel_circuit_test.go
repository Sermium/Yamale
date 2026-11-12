//go:build !settlement

package app

import (
	"testing"

	"cosmossdk.io/x/nft"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	"github.com/stretchr/testify/require"

	jeweltypes "yamale/blockchain/x/jewel/types"
)

// x/nft's own send skips every rule x/jewel exists for. Disabling it in
// x/circuit must stop it at the router, which is where authz and interchain
// accounts also arrive; a check in the ante chain would see neither.
//
// The proof is the difference between two errors. With the send open, the
// router hands the message to x/nft, which answers that the sender does not
// own the token. With it disabled, the router refuses before x/nft looks.
func TestABareNFTSendIsRefusedAtTheRouterEvenInsideAuthz(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	owner := sdk.AccAddress([]byte("jewel-owner-________"))
	ownerStr, err := app.AuthKeeper.AddressCodec().BytesToString(owner)
	require.NoError(t, err)
	send := &nft.MsgSend{ClassId: "jewel-1", Id: "jewel-1", Sender: ownerStr, Receiver: ownerStr}

	handler := app.MsgServiceRouter().Handler(send)
	require.NotNil(t, handler)
	const nftAnswer = "is not the owner of nft"
	// baseapp/msg_service_router.go: "circuit breaker disables execution of this message: <url>"
	const breakerAnswer = "circuit breaker disables execution of this message"
	_, err = handler(ctx, send)
	require.ErrorContains(t, err, nftAnswer, "control: with the send open, x/nft itself answers")

	require.NoError(t, app.CircuitBreakerKeeper.DisableList.Set(ctx, jeweltypes.NFTSendTypeURL))

	_, err = handler(ctx, send)
	require.ErrorContains(t, err, breakerAnswer, "the router must refuse before x/nft runs")
	require.NotContains(t, err.Error(), nftAnswer)

	exec := authz.NewMsgExec(owner, []sdk.Msg{send})
	_, err = app.MsgServiceRouter().Handler(&exec)(ctx, &exec)
	require.ErrorContains(t, err, breakerAnswer, "authz must be refused by the router too")
	require.NotContains(t, err.Error(), nftAnswer, "authz must not reach x/nft either")
}
