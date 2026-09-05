package keeper_test

import (
	"strings"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"

	"yamale/blockchain/x/builderfee/keeper"
	"yamale/blockchain/x/builderfee/types"
)

const feeDenom = "uyml"

// stubTx is the minimum sdk.FeeTx the fee-share decorator inspects: the fee
// paid and the messages carried.
type stubTx struct {
	msgs []sdk.Msg
	fee  sdk.Coins
}

func (t stubTx) GetMsgs() []sdk.Msg                    { return t.msgs }
func (t stubTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }
func (t stubTx) GetGas() uint64                        { return 200000 }
func (t stubTx) GetFee() sdk.Coins                     { return t.fee }
func (t stubTx) FeePayer() []byte                      { return nil }
func (t stubTx) FeeGranter() []byte                    { return nil }

// noopTx satisfies sdk.Tx but not sdk.FeeTx, standing in for a transaction
// type the decorator cannot read a fee from.
type noopTx struct{ msgs []sdk.Msg }

func (t noopTx) GetMsgs() []sdk.Msg                    { return t.msgs }
func (t noopTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }

// terminalPostHandler ends the post-handler chain.
func terminalPostHandler(ctx sdk.Context, _ sdk.Tx, _, _ bool) (sdk.Context, error) {
	return ctx, nil
}

func feeCoins(amount int64) sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin(feeDenom, math.NewInt(amount)))
}

// approveBuilder registers and approves a builder for msgTypeURL, returning
// the payout address.
func approveBuilder(t *testing.T, f *fixture, ms types.MsgServer, msgTypeURL string) (sdk.AccAddress, string) {
	t.Helper()

	payout, payoutStr := f.env.Addr(t)
	_, creatorStr := f.env.Addr(t)

	_, err := ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: creatorStr, MsgTypeUrl: msgTypeURL, PayoutAddress: payoutStr,
	})
	require.NoError(t, err)

	_, err = ms.ApproveBuilder(f.ctx, &types.MsgApproveBuilder{
		Authority: f.env.AuthorityString(t), MsgTypeUrl: msgTypeURL, Approve: true,
	})
	require.NoError(t, err)

	return payout, payoutStr
}

// sendMsgTypeURL is a stand-in for any message a builder can register against.
var sendMsgTypeURL = sdk.MsgTypeURL(&banktypes.MsgSend{})

func TestFeeShareSplitsToApprovedBuilder(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	payout, _ := approveBuilder(t, f, ms, sendMsgTypeURL)

	// The ante handler has already moved the fee to the fee collector.
	f.env.FundModule(t, authtypes.FeeCollectorName, feeCoins(1_000))

	dec := keeper.NewFeeShareDecorator(f.keeper)
	tx := stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}, fee: feeCoins(1_000)}

	_, err := dec.PostHandle(f.env.Ctx, tx, false, true, terminalPostHandler)
	require.NoError(t, err)

	// Default share is 3000 bps = 30%.
	require.Equal(t, math.NewInt(300), f.env.Balance(payout, feeDenom))
	require.Equal(t, math.NewInt(700), f.env.ModuleBalance(authtypes.FeeCollectorName, feeDenom),
		"the remainder must stay with the fee collector for validator distribution")
}

func TestFeeShareRespectsConfiguredBps(t *testing.T) {
	testCases := []struct {
		name      string
		bps       uint64
		fee       int64
		expPayout int64
	}{
		{name: "zero share pays nothing", bps: 0, fee: 1_000, expPayout: 0},
		{name: "one percent", bps: 100, fee: 1_000, expPayout: 10},
		{name: "full share", bps: 10_000, fee: 1_000, expPayout: 1_000},
		{name: "rounds down", bps: 3_000, fee: 3, expPayout: 0},
		{name: "rounds down to a whole unit", bps: 3_000, fee: 7, expPayout: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := initFixture(t)
			ms := keeper.NewMsgServerImpl(f.keeper)

			require.NoError(t, f.keeper.Params.Set(f.ctx, types.NewParams(tc.bps)))
			payout, _ := approveBuilder(t, f, ms, sendMsgTypeURL)
			f.env.FundModule(t, authtypes.FeeCollectorName, feeCoins(tc.fee))

			dec := keeper.NewFeeShareDecorator(f.keeper)
			tx := stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}, fee: feeCoins(tc.fee)}

			_, err := dec.PostHandle(f.env.Ctx, tx, false, true, terminalPostHandler)
			require.NoError(t, err)

			require.Equal(t, math.NewInt(tc.expPayout), f.env.Balance(payout, feeDenom))
			require.Equal(t, math.NewInt(tc.fee-tc.expPayout),
				f.env.ModuleBalance(authtypes.FeeCollectorName, feeDenom))
		})
	}
}

func TestFeeShareSkipped(t *testing.T) {
	testCases := []struct {
		name     string
		simulate bool
		success  bool
		tx       sdk.Tx
	}{
		{
			name:    "failed transaction",
			success: false,
			tx:      stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}, fee: feeCoins(1_000)},
		},
		{
			name:     "simulation",
			simulate: true,
			success:  true,
			tx:       stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}, fee: feeCoins(1_000)},
		},
		{
			name:    "zero fee",
			success: true,
			tx:      stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}, fee: sdk.NewCoins()},
		},
		{
			name:    "transaction carries no fee information",
			success: true,
			tx:      noopTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := initFixture(t)
			ms := keeper.NewMsgServerImpl(f.keeper)

			payout, _ := approveBuilder(t, f, ms, sendMsgTypeURL)
			f.env.FundModule(t, authtypes.FeeCollectorName, feeCoins(1_000))

			dec := keeper.NewFeeShareDecorator(f.keeper)
			_, err := dec.PostHandle(f.env.Ctx, tc.tx, tc.simulate, tc.success, terminalPostHandler)
			require.NoError(t, err)

			require.True(t, f.env.Balance(payout, feeDenom).IsZero())
			require.Equal(t, math.NewInt(1_000), f.env.ModuleBalance(authtypes.FeeCollectorName, feeDenom))
		})
	}
}

func TestFeeShareIgnoresUnregisteredMessageTypes(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	// A builder is approved, but for a different message type. It has to be a
	// real one: an application naming a type the chain cannot route is now
	// refused, because a fee against a message nobody can send is a
	// reservation rather than an application.
	payout, _ := approveBuilder(t, f, ms, "/cosmos.bank.v1beta1.MsgMultiSend")
	f.env.FundModule(t, authtypes.FeeCollectorName, feeCoins(1_000))

	dec := keeper.NewFeeShareDecorator(f.keeper)
	tx := stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}, fee: feeCoins(1_000)}

	_, err := dec.PostHandle(f.env.Ctx, tx, false, true, terminalPostHandler)
	require.NoError(t, err)

	require.True(t, f.env.Balance(payout, feeDenom).IsZero())
	require.Equal(t, math.NewInt(1_000), f.env.ModuleBalance(authtypes.FeeCollectorName, feeDenom))
}

// A pending, not-yet-approved application must not receive a payout.
func TestFeeShareIgnoresUnapprovedBuilder(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	payout, payoutStr := f.env.Addr(t)
	_, creatorStr := f.env.Addr(t)
	_, err := ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: creatorStr, MsgTypeUrl: sendMsgTypeURL, PayoutAddress: payoutStr,
	})
	require.NoError(t, err)

	f.env.FundModule(t, authtypes.FeeCollectorName, feeCoins(1_000))

	dec := keeper.NewFeeShareDecorator(f.keeper)
	tx := stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}, fee: feeCoins(1_000)}

	_, err = dec.PostHandle(f.env.Ctx, tx, false, true, terminalPostHandler)
	require.NoError(t, err)

	require.True(t, f.env.Balance(payout, feeDenom).IsZero())
	require.Equal(t, math.NewInt(1_000), f.env.ModuleBalance(authtypes.FeeCollectorName, feeDenom))
}

// Only the first message with an approved builder is paid, even when a tx
// carries several messages that each have one.
func TestFeeSharePaysOnlyTheFirstMatchingBuilder(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	sendPayout, _ := approveBuilder(t, f, ms, sendMsgTypeURL)
	multiSendURL := sdk.MsgTypeURL(&banktypes.MsgMultiSend{})
	multiPayout, _ := approveBuilder(t, f, ms, multiSendURL)

	f.env.FundModule(t, authtypes.FeeCollectorName, feeCoins(1_000))

	dec := keeper.NewFeeShareDecorator(f.keeper)
	tx := stubTx{
		msgs: []sdk.Msg{&banktypes.MsgSend{}, &banktypes.MsgMultiSend{}},
		fee:  feeCoins(1_000),
	}

	_, err := dec.PostHandle(f.env.Ctx, tx, false, true, terminalPostHandler)
	require.NoError(t, err)

	require.Equal(t, math.NewInt(300), f.env.Balance(sendPayout, feeDenom))
	require.True(t, multiPayout != nil && f.env.Balance(multiPayout, feeDenom).IsZero())
	require.Equal(t, math.NewInt(700), f.env.ModuleBalance(authtypes.FeeCollectorName, feeDenom))
}

// A tx paying its fee in several denoms splits each of them.
func TestFeeShareSplitsEveryFeeDenom(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	payout, _ := approveBuilder(t, f, ms, sendMsgTypeURL)

	fee := sdk.NewCoins(
		sdk.NewCoin(feeDenom, math.NewInt(1_000)),
		sdk.NewCoin("uusd", math.NewInt(2_000)),
	)
	f.env.FundModule(t, authtypes.FeeCollectorName, fee)

	dec := keeper.NewFeeShareDecorator(f.keeper)
	_, err := dec.PostHandle(f.env.Ctx, stubTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}, fee: fee}, false, true, terminalPostHandler)
	require.NoError(t, err)

	require.Equal(t, math.NewInt(300), f.env.Balance(payout, feeDenom))
	require.Equal(t, math.NewInt(600), f.env.Balance(payout, "uusd"))
}

func TestRegisterBuilderRejectsDuplicates(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, payoutStr := f.env.Addr(t)
	_, creatorStr := f.env.Addr(t)

	_, err := ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: creatorStr, MsgTypeUrl: sendMsgTypeURL, PayoutAddress: payoutStr,
	})
	require.NoError(t, err)

	_, err = ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: creatorStr, MsgTypeUrl: sendMsgTypeURL, PayoutAddress: payoutStr,
	})
	require.ErrorIs(t, err, types.ErrBuilderExists)
}

func TestRegisterBuilderRejectsBadAddresses(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, validStr := f.env.Addr(t)

	_, err := ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: "nope", MsgTypeUrl: sendMsgTypeURL, PayoutAddress: validStr,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid creator address")

	_, err = ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: validStr, MsgTypeUrl: sendMsgTypeURL, PayoutAddress: "nope",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid payout address")
}

func TestApproveBuilderRequiresGovAuthority(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, payoutStr := f.env.Addr(t)
	_, creatorStr := f.env.Addr(t)
	_, err := ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: creatorStr, MsgTypeUrl: sendMsgTypeURL, PayoutAddress: payoutStr,
	})
	require.NoError(t, err)

	_, err = ms.ApproveBuilder(f.ctx, &types.MsgApproveBuilder{
		Authority: creatorStr, MsgTypeUrl: sendMsgTypeURL, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	has, err := f.keeper.ApprovedBuilder.Has(f.ctx, sendMsgTypeURL)
	require.NoError(t, err)
	require.False(t, has)
}

func TestApproveBuilderRejection(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, payoutStr := f.env.Addr(t)
	_, creatorStr := f.env.Addr(t)
	_, err := ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: creatorStr, MsgTypeUrl: sendMsgTypeURL, PayoutAddress: payoutStr,
	})
	require.NoError(t, err)

	_, err = ms.ApproveBuilder(f.ctx, &types.MsgApproveBuilder{
		Authority: f.env.AuthorityString(t), MsgTypeUrl: sendMsgTypeURL, Approve: false,
	})
	require.NoError(t, err)

	has, err := f.keeper.ApprovedBuilder.Has(f.ctx, sendMsgTypeURL)
	require.NoError(t, err)
	require.False(t, has)

	// And the message type is claimable again.
	//
	// A rejected record used to stay behind under the same key RegisterBuilder
	// refuses a duplicate on, so a rejection permanently barred that message
	// type to everybody — one transaction fee, no way back.
	has, err = f.keeper.BuilderApplication.Has(f.ctx, sendMsgTypeURL)
	require.NoError(t, err)
	require.False(t, has, "the rejected application still holds the message type")

	_, err = ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
		Creator: creatorStr, MsgTypeUrl: sendMsgTypeURL, PayoutAddress: payoutStr,
	})
	require.NoError(t, err)
}

// The store key here is chosen by whoever signs a permissionless message, so
// it is bounded in shape, in length, and against the interface registry.
func TestRegisterBuilderRefusesRubbishTypeURLs(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, payoutStr := f.env.Addr(t)
	_, creatorStr := f.env.Addr(t)

	for _, url := range []string{
		"",
		"cosmos.bank.v1beta1.MsgSend",          // no leading slash
		"/cosmos/bank/v1beta1/MsgSend",         // a path, not a proto name
		"/no_dots_here",                        // not a qualified name
		"/cosmos.bank.v1beta1.MsgSend\x00evil", // outside the character set
		"/" + strings.Repeat("a.", 200) + "MsgSend", // longer than the bound
		"/blockchain.nosuch.v1.MsgInvented",         // shaped right, does not exist
	} {
		_, err := ms.RegisterBuilder(f.ctx, &types.MsgRegisterBuilder{
			Creator: creatorStr, MsgTypeUrl: url, PayoutAddress: payoutStr,
		})
		require.ErrorIs(t, err, types.ErrInvalidMsgTypeURL, "accepted %q", url)

		has, err := f.keeper.BuilderApplication.Has(f.ctx, url)
		require.NoError(t, err)
		require.False(t, has, "wrote a store key for %q", url)
	}
}

func TestApproveBuilderUnknownApplication(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, err := ms.ApproveBuilder(f.ctx, &types.MsgApproveBuilder{
		Authority: f.env.AuthorityString(t), MsgTypeUrl: "/nothing.here.v1.Msg", Approve: true,
	})
	require.ErrorIs(t, err, types.ErrApplicationNotFound)
}
