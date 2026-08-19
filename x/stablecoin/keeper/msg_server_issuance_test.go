package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/stablecoin/keeper"
	"yamale/blockchain/x/stablecoin/types"
)

const testDenom = "uchf"

// registerCurrency files an application for testDenom and returns the
// applicant's address.
func registerCurrency(t *testing.T, f *fixture, ms types.MsgServer) (sdk.AccAddress, string) {
	t.Helper()

	applicant, applicantStr := f.env.Addr(t)
	_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator:      applicantStr,
		Denom:        testDenom,
		DisplayDenom: "chf",
		Exponent:     6,
		Name:         "Swiss Franc",
		Symbol:       "CHF",
		Description:  "A mock franc-pegged stablecoin",
	})
	require.NoError(t, err)
	return applicant, applicantStr
}

// approveIssuer runs the governance approval for testDenom.
func approveIssuer(t *testing.T, f *fixture, ms types.MsgServer, approve bool) {
	t.Helper()

	_, err := ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: f.env.AuthorityString(t),
		Denom:     testDenom,
		Approve:   approve,
	})
	require.NoError(t, err)
}

func TestRegisterCurrencyRecordsPendingApplication(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, applicantStr := registerCurrency(t, f, ms)

	app, err := f.keeper.IssuerApplication.Get(f.ctx, testDenom)
	require.NoError(t, err)
	require.Equal(t, types.StatusPending, app.Status)
	require.Equal(t, applicantStr, app.Creator)
	require.Equal(t, "Swiss Franc", app.Name)

	// Registering does not by itself grant issuing rights.
	has, err := f.keeper.ApprovedIssuer.Has(f.ctx, testDenom)
	require.NoError(t, err)
	require.False(t, has)
}

func TestRegisterCurrencyRejectsDuplicates(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	registerCurrency(t, f, ms)

	// A second application for the same denom, even from someone else.
	_, otherStr := f.env.Addr(t)
	_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: otherStr, Denom: testDenom, DisplayDenom: "chf", Exponent: 6,
		Name: "Swiss Franc", Symbol: "CHF",
	})
	require.ErrorIs(t, err, types.ErrCurrencyExists)

	// And still rejected once the first one is approved.
	approveIssuer(t, f, ms, true)
	_, err = ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: otherStr, Denom: testDenom, DisplayDenom: "chf", Exponent: 6,
		Name: "Swiss Franc", Symbol: "CHF",
	})
	require.ErrorIs(t, err, types.ErrCurrencyExists)
}

func TestRegisterCurrencyRejectsInvalidCreator(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: "not-an-address", Denom: testDenom,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid creator address")
}

func TestApproveIssuerRequiresGovAuthority(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, applicantStr := registerCurrency(t, f, ms)

	// The applicant cannot approve their own application.
	_, err := ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: applicantStr, Denom: testDenom, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	has, err := f.keeper.ApprovedIssuer.Has(f.ctx, testDenom)
	require.NoError(t, err)
	require.False(t, has, "a non-gov signer must not be able to grant issuing rights")
}

func TestApproveIssuerGrantsRightsAndPublishesMetadata(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, applicantStr := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)

	approved, err := f.keeper.ApprovedIssuer.Get(f.ctx, testDenom)
	require.NoError(t, err)
	require.Equal(t, applicantStr, approved.Issuer)

	app, err := f.keeper.IssuerApplication.Get(f.ctx, testDenom)
	require.NoError(t, err)
	require.Equal(t, types.StatusApproved, app.Status)

	// The denom is now discoverable through x/bank, which wallets rely on to
	// render the currency correctly.
	meta, found := f.env.BankKeeper.GetDenomMetaData(f.env.Ctx, testDenom)
	require.True(t, found)
	require.Equal(t, "Swiss Franc", meta.Name)
	require.Equal(t, "CHF", meta.Symbol)
	require.Equal(t, testDenom, meta.Base)
	require.Equal(t, "chf", meta.Display)
	require.Len(t, meta.DenomUnits, 2)
	require.Equal(t, uint32(0), meta.DenomUnits[0].Exponent)
	require.Equal(t, uint32(6), meta.DenomUnits[1].Exponent)
}

func TestApproveIssuerRejection(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, false)

	app, err := f.keeper.IssuerApplication.Get(f.ctx, testDenom)
	require.NoError(t, err)
	require.Equal(t, types.StatusRejected, app.Status)

	has, err := f.keeper.ApprovedIssuer.Has(f.ctx, testDenom)
	require.NoError(t, err)
	require.False(t, has)

	// No metadata is published for a rejected currency.
	_, found := f.env.BankKeeper.GetDenomMetaData(f.env.Ctx, testDenom)
	require.False(t, found)
}

func TestApproveIssuerRejectsNonPendingApplication(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)

	// A second approval of an already-decided application must not go through.
	_, err := ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: f.env.AuthorityString(t), Denom: testDenom, Approve: true,
	})
	require.ErrorIs(t, err, types.ErrApplicationNotPending)
}

func TestApproveIssuerUnknownDenom(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, err := ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: f.env.AuthorityString(t), Denom: "unknown", Approve: true,
	})
	require.ErrorIs(t, err, types.ErrApplicationNotFound)
}

func TestMintCoinByApprovedIssuer(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuerStr := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)

	recipient, recipientStr := f.env.Addr(t)

	_, err := ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: issuerStr, Denom: testDenom, Amount: "1000000", Recipient: recipientStr,
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(recipient, testDenom))
	require.Equal(t, math.NewInt(1_000_000), f.env.Supply(testDenom))

	// Nothing is stranded in the module account.
	require.True(t, f.env.ModuleBalance(types.ModuleName, testDenom).IsZero())
}

func TestMintCoinRejectsUnapprovedIssuer(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuerStr := registerCurrency(t, f, ms)
	_, recipientStr := f.env.Addr(t)

	// Before governance approves, not even the applicant may mint.
	_, err := ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: issuerStr, Denom: testDenom, Amount: "1000000", Recipient: recipientStr,
	})
	require.ErrorIs(t, err, types.ErrNotApprovedIssuer)
	require.True(t, f.env.Supply(testDenom).IsZero())

	// After approval, a different address still may not mint that denom.
	approveIssuer(t, f, ms, true)
	_, impostorStr := f.env.Addr(t)
	_, err = ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: impostorStr, Denom: testDenom, Amount: "1000000", Recipient: recipientStr,
	})
	require.ErrorIs(t, err, types.ErrNotApprovedIssuer)
	require.True(t, f.env.Supply(testDenom).IsZero())
}

func TestMintCoinRejectsBadInput(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuerStr := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)
	_, recipientStr := f.env.Addr(t)

	testCases := []struct {
		name   string
		msg    *types.MsgMintCoin
		errStr string
	}{
		{
			name:   "invalid issuer address",
			msg:    &types.MsgMintCoin{Issuer: "nope", Denom: testDenom, Amount: "1", Recipient: recipientStr},
			errStr: "invalid issuer address",
		},
		{
			name:   "invalid recipient address",
			msg:    &types.MsgMintCoin{Issuer: issuerStr, Denom: testDenom, Amount: "1", Recipient: "nope"},
			errStr: "invalid recipient address",
		},
		{
			name:   "zero amount",
			msg:    &types.MsgMintCoin{Issuer: issuerStr, Denom: testDenom, Amount: "0", Recipient: recipientStr},
			errStr: "invalid mint amount",
		},
		{
			name:   "negative amount",
			msg:    &types.MsgMintCoin{Issuer: issuerStr, Denom: testDenom, Amount: "-5", Recipient: recipientStr},
			errStr: "invalid mint amount",
		},
		{
			name:   "unregistered denom",
			msg:    &types.MsgMintCoin{Issuer: issuerStr, Denom: "ujpy", Amount: "1", Recipient: recipientStr},
			errStr: "has no approved issuer",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.MintCoin(f.ctx, tc.msg)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errStr)
		})
	}
}

func TestBurnCoinByApprovedIssuer(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	issuer, issuerStr := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)

	// Mint to the issuer themself, then burn part of it.
	_, err := ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: issuerStr, Denom: testDenom, Amount: "1000000", Recipient: issuerStr,
	})
	require.NoError(t, err)

	_, err = ms.BurnCoin(f.ctx, &types.MsgBurnCoin{
		Issuer: issuerStr, Denom: testDenom, Amount: "400000",
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(600_000), f.env.Balance(issuer, testDenom))
	require.Equal(t, math.NewInt(600_000), f.env.Supply(testDenom))
	require.True(t, f.env.ModuleBalance(types.ModuleName, testDenom).IsZero())
}

// The issuer can only burn what they hold, not another holder's balance.
func TestBurnCoinLimitedToIssuerBalance(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuerStr := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)

	holder, holderStr := f.env.Addr(t)
	_, err := ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: issuerStr, Denom: testDenom, Amount: "1000000", Recipient: holderStr,
	})
	require.NoError(t, err)

	_, err = ms.BurnCoin(f.ctx, &types.MsgBurnCoin{
		Issuer: issuerStr, Denom: testDenom, Amount: "1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient funds")

	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(holder, testDenom))
	require.Equal(t, math.NewInt(1_000_000), f.env.Supply(testDenom))
}

func TestBurnCoinRejectsUnapprovedIssuer(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuerStr := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)

	impostor, impostorStr := f.env.Addr(t)
	_, err := ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: issuerStr, Denom: testDenom, Amount: "1000000", Recipient: impostorStr,
	})
	require.NoError(t, err)

	// Holding the coins does not make you their issuer.
	_, err = ms.BurnCoin(f.ctx, &types.MsgBurnCoin{
		Issuer: impostorStr, Denom: testDenom, Amount: "1000",
	})
	require.ErrorIs(t, err, types.ErrNotApprovedIssuer)
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(impostor, testDenom))
}

// Two currencies must be independently permissioned: an issuer approved for
// one denom has no rights over the other.
func TestIssuerRightsAreScopedToTheirDenom(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, chfIssuerStr := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)

	_, jpyIssuerStr := f.env.Addr(t)
	_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: jpyIssuerStr, Denom: "ujpy", DisplayDenom: "jpy", Exponent: 6,
		Name: "Japanese Yen", Symbol: "JPY",
	})
	require.NoError(t, err)
	_, err = ms.ApproveIssuer(f.ctx, &types.MsgApproveIssuer{
		Authority: f.env.AuthorityString(t), Denom: "ujpy", Approve: true,
	})
	require.NoError(t, err)

	recipient, recipientStr := f.env.Addr(t)

	// The CHF issuer may not mint JPY.
	_, err = ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: chfIssuerStr, Denom: "ujpy", Amount: "1000", Recipient: recipientStr,
	})
	require.ErrorIs(t, err, types.ErrNotApprovedIssuer)

	// The JPY issuer may.
	_, err = ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: jpyIssuerStr, Denom: "ujpy", Amount: "1000", Recipient: recipientStr,
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000), f.env.Balance(recipient, "ujpy"))
	require.True(t, f.env.Balance(recipient, testDenom).IsZero())
}
