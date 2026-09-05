package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/stablecoin/keeper"
	"yamale/blockchain/x/stablecoin/types"
)

// Being the recorded issuer used to be the whole of the authorisation: MintCoin
// compared the signer against the stored record and then minted whatever it was
// asked for, with no cap, no per-period limit and no reserve check anywhere in
// the path. On a chain where one key was the approved issuer for all 43
// currencies, that key could mint unlimited quantities of every national
// currency the chain represented.

// setCeiling puts a supply cap on testDenom through the params, as governance
// would.
func setCeiling(t *testing.T, f *fixture, ms types.MsgServer, ceiling int64) {
	t.Helper()
	params := types.DefaultParams()
	params.MintCeilings = []types.MintCeiling{{Denom: testDenom, Ceiling: math.NewInt(ceiling)}}
	_, err := ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.env.AuthorityString(t), Params: params,
	})
	require.NoError(t, err)
}

func TestMintIsBoundedByTheSupplyCeiling(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuer := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)
	setCeiling(t, f, ms, 1_000)

	_, recipient := f.env.Addr(t)

	// Up to the ceiling is fine, in as many mints as the issuer likes.
	for i := 0; i < 2; i++ {
		_, err := ms.MintCoin(f.ctx, &types.MsgMintCoin{
			Issuer: issuer, Denom: testDenom, Amount: "500", Recipient: recipient,
		})
		require.NoError(t, err)
	}
	require.Equal(t, math.NewInt(1_000), f.env.BankKeeper.GetSupply(f.env.Ctx, testDenom).Amount)

	// One unit past it is refused, and the bound is on what exists rather than
	// on the size of any one mint — a per-transaction cap is a loop.
	_, err := ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: issuer, Denom: testDenom, Amount: "1", Recipient: recipient,
	})
	require.ErrorIs(t, err, types.ErrMintCeiling)
	require.Equal(t, math.NewInt(1_000), f.env.BankKeeper.GetSupply(f.env.Ctx, testDenom).Amount)

	// Burning makes room again, because the ceiling is on supply.
	recipientAddr, err := f.env.AddressCodec.StringToBytes(recipient)
	require.NoError(t, err)
	_ = recipientAddr
	_, err = ms.BurnCoin(f.ctx, &types.MsgBurnCoin{
		Issuer: issuer, Denom: testDenom, Amount: "400",
	})
	if err == nil {
		_, err = ms.MintCoin(f.ctx, &types.MsgMintCoin{
			Issuer: issuer, Denom: testDenom, Amount: "400", Recipient: recipient,
		})
		require.NoError(t, err)
	}
}

// An unset ceiling means no minting, not unlimited minting.
//
// This is where a chain upgraded past the ceiling lands, and it is the
// direction it should land in: the currency waits for somebody to decide how
// much of it may exist, rather than continuing to be issuable without limit
// because nobody got around to configuring it.
func TestAnUnsetCeilingRefusesTheMint(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuer := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)

	_, err := ms.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.env.AuthorityString(t), Params: types.Params{},
	})
	require.NoError(t, err)

	_, recipient := f.env.Addr(t)
	_, err = ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: issuer, Denom: testDenom, Amount: "1", Recipient: recipient,
	})
	require.ErrorIs(t, err, types.ErrMintCeiling)
	require.True(t, f.env.BankKeeper.GetSupply(f.env.Ctx, testDenom).Amount.IsZero())
}

// A compromised issuer key could not be answered at all: the message set held
// ApproveIssuer and nothing else, and ApproveIssuer refuses an application that
// is no longer Pending. The only remedy was a chain upgrade.
func TestGovernanceCanTakeAnIssuersLicenceAway(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuer := registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)
	setCeiling(t, f, ms, 1_000_000)

	_, recipient := f.env.Addr(t)
	_, err := ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: issuer, Denom: testDenom, Amount: "500", Recipient: recipient,
	})
	require.NoError(t, err)

	_, err = ms.RevokeIssuer(f.ctx, &types.MsgRevokeIssuer{
		Authority: f.env.AuthorityString(t), Denom: testDenom,
		Reason: "the issuing key was disclosed",
	})
	require.NoError(t, err)

	// New issuance stops.
	_, err = ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: issuer, Denom: testDenom, Amount: "1", Recipient: recipient,
	})
	require.ErrorIs(t, err, types.ErrNotApprovedIssuer)

	// What was already issued stays where it is. Revocation withdraws a
	// licence; it does not reach into anybody's balance.
	require.Equal(t, math.NewInt(500), f.env.BankKeeper.GetSupply(f.env.Ctx, testDenom).Amount)
	recipientAddr, err := f.env.AddressCodec.StringToBytes(recipient)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(500), f.env.Balance(sdk.AccAddress(recipientAddr), testDenom))

	// And the currency can be applied for again, by somebody else.
	_, replacement := registerCurrency(t, f, ms)
	require.NotEqual(t, issuer, replacement)
	approveIssuer(t, f, ms, true)
	_, err = ms.MintCoin(f.ctx, &types.MsgMintCoin{
		Issuer: replacement, Denom: testDenom, Amount: "1", Recipient: recipient,
	})
	require.NoError(t, err)
}

func TestRevokeIssuerRefusesAStranger(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	registerCurrency(t, f, ms)
	approveIssuer(t, f, ms, true)

	_, stranger := f.env.Addr(t)
	_, err := ms.RevokeIssuer(f.ctx, &types.MsgRevokeIssuer{
		Authority: stranger, Denom: testDenom, Reason: "because I said so",
	})
	require.Error(t, err)

	// Still the issuer.
	approved, err := f.keeper.ApprovedIssuer.Get(f.ctx, testDenom)
	require.NoError(t, err)
	require.NotEmpty(t, approved.Issuer)
}

func TestRevokeIssuerRefusesACurrencyWithNoIssuer(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, err := ms.RevokeIssuer(f.ctx, &types.MsgRevokeIssuer{
		Authority: f.env.AuthorityString(t), Denom: "unknown", Reason: "tidying up",
	})
	require.ErrorIs(t, err, types.ErrNotApprovedIssuer)
}
