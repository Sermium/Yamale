package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/treasury/types"
)

// Escrow's whole claim is where the money sits between the deal and its end:
// the module account, reachable by nobody except through the four handlers.
// Every assertion below is on a balance, because a claim about custody that is
// tested against a record rather than against money is not tested at all.

const escrowDenom = "uyml"

func escrowParties(t *testing.T, env *integration.Env) (buyer, seller, moderator string) {
	t.Helper()
	_, buyer = env.Addr(t)
	_, seller = env.Addr(t)
	_, moderator = env.Addr(t)
	addr, err := env.AddressCodec.StringToBytes(buyer)
	require.NoError(t, err)
	env.Fund(t, addr, sdk.NewCoins(sdk.NewCoin(escrowDenom, math.NewInt(1_000_000))))
	return
}

func TestEscrowHoldsFundsInTheModuleAccount(t *testing.T) {
	f := initFixture(t)
	env, ms := f.env, f.ms
	buyer, seller, moderator := escrowParties(t, env)

	buyerAddr, err := env.AddressCodec.StringToBytes(buyer)
	require.NoError(t, err)
	before := env.Balance(buyerAddr, escrowDenom)

	res, err := ms.OpenEscrow(env.Ctx, &types.MsgOpenEscrow{
		Depositor: buyer, Beneficiary: seller, Moderator: moderator,
		Amount: sdk.NewCoin(escrowDenom, math.NewInt(50_000)), Memo: "two bags of cement",
	})
	require.NoError(t, err)

	// The buyer has paid, the seller has not been paid, and the chain holds it.
	require.Equal(t, before.SubRaw(50_000), env.Balance(buyerAddr, escrowDenom))
	sellerAddr, err := env.AddressCodec.StringToBytes(seller)
	require.NoError(t, err)
	require.True(t, env.Balance(sellerAddr, escrowDenom).IsZero())
	require.Equal(t, math.NewInt(50_000), env.ModuleBalance(types.ModuleName, escrowDenom))

	// And only the buyer can end that.
	_, err = ms.ReleaseEscrow(env.Ctx, &types.MsgReleaseEscrow{Depositor: seller, LockId: res.LockId})
	require.ErrorIs(t, err, types.ErrNotDepositor)
	_, err = ms.ReleaseEscrow(env.Ctx, &types.MsgReleaseEscrow{Depositor: moderator, LockId: res.LockId})
	require.ErrorIs(t, err, types.ErrNotDepositor)

	_, err = ms.ReleaseEscrow(env.Ctx, &types.MsgReleaseEscrow{Depositor: buyer, LockId: res.LockId})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(50_000), env.Balance(sellerAddr, escrowDenom))
	require.True(t, env.ModuleBalance(types.ModuleName, escrowDenom).IsZero())
}

// The moderator's only power is deciding a case somebody opened. A moderator
// who could touch a quiet escrow would be a custodian under another name, which
// is precisely what this design removes.
func TestModeratorCannotTouchAQuietEscrow(t *testing.T) {
	f := initFixture(t)
	env, ms := f.env, f.ms
	buyer, seller, moderator := escrowParties(t, env)

	res, err := ms.OpenEscrow(env.Ctx, &types.MsgOpenEscrow{
		Depositor: buyer, Beneficiary: seller, Moderator: moderator,
		Amount: sdk.NewCoin(escrowDenom, math.NewInt(10_000)),
	})
	require.NoError(t, err)

	_, err = ms.ResolveEscrow(env.Ctx, &types.MsgResolveEscrow{
		Moderator: moderator, LockId: res.LockId, PayBeneficiary: true,
	})
	require.ErrorIs(t, err, types.ErrNoOpenCase)
	require.Equal(t, math.NewInt(10_000), env.ModuleBalance(types.ModuleName, escrowDenom))
}

// The seller escalating is what removes the need for a deadline: a buyer who
// goes quiet cannot strand the money, because the other side can move it to a
// decision.
func TestSellerCanEscalateAgainstASilentBuyer(t *testing.T) {
	f := initFixture(t)
	env, ms := f.env, f.ms
	buyer, seller, moderator := escrowParties(t, env)

	res, err := ms.OpenEscrow(env.Ctx, &types.MsgOpenEscrow{
		Depositor: buyer, Beneficiary: seller, Moderator: moderator,
		Amount: sdk.NewCoin(escrowDenom, math.NewInt(30_000)),
	})
	require.NoError(t, err)

	_, err = ms.DisputeEscrow(env.Ctx, &types.MsgDisputeEscrow{
		Party: seller, LockId: res.LockId, Reason: "delivered, buyer has not replied for three weeks",
	})
	require.NoError(t, err)

	// Frozen: the buyer cannot quietly release around an open case.
	_, err = ms.ReleaseEscrow(env.Ctx, &types.MsgReleaseEscrow{Depositor: buyer, LockId: res.LockId})
	require.ErrorIs(t, err, types.ErrEscrowDisputed)

	// A stranger cannot open or decide one either.
	_, other := env.Addr(t)
	// Rejected as a stranger rather than told the case state — somebody with no
	// standing should not learn whether a dispute exists.
	_, err = ms.DisputeEscrow(env.Ctx, &types.MsgDisputeEscrow{Party: other, LockId: res.LockId, Reason: "x"})
	require.ErrorIs(t, err, types.ErrNotParty)

	// The buyer, who is a party, is told.
	_, err = ms.DisputeEscrow(env.Ctx, &types.MsgDisputeEscrow{Party: buyer, LockId: res.LockId, Reason: "x"})
	require.ErrorIs(t, err, types.ErrAlreadyDisputed)
	_, err = ms.ResolveEscrow(env.Ctx, &types.MsgResolveEscrow{Moderator: other, LockId: res.LockId, PayBeneficiary: true})
	require.ErrorIs(t, err, types.ErrNotModerator)

	_, err = ms.ResolveEscrow(env.Ctx, &types.MsgResolveEscrow{
		Moderator: moderator, LockId: res.LockId, PayBeneficiary: true,
	})
	require.NoError(t, err)

	sellerAddr, err := env.AddressCodec.StringToBytes(seller)
	require.NoError(t, err)
	require.Equal(t, math.NewInt(30_000), env.Balance(sellerAddr, escrowDenom))
}

// Refunding is the other half of the moderator's decision, and the money must
// come back whole — a refund that loses a unit is a refund somebody disputes.
func TestModeratorCanRefundTheBuyer(t *testing.T) {
	f := initFixture(t)
	env, ms := f.env, f.ms
	buyer, seller, moderator := escrowParties(t, env)

	buyerAddr, err := env.AddressCodec.StringToBytes(buyer)
	require.NoError(t, err)
	before := env.Balance(buyerAddr, escrowDenom)

	res, err := ms.OpenEscrow(env.Ctx, &types.MsgOpenEscrow{
		Depositor: buyer, Beneficiary: seller, Moderator: moderator,
		Amount: sdk.NewCoin(escrowDenom, math.NewInt(25_000)),
	})
	require.NoError(t, err)
	_, err = ms.DisputeEscrow(env.Ctx, &types.MsgDisputeEscrow{
		Party: buyer, LockId: res.LockId, Reason: "never arrived",
	})
	require.NoError(t, err)
	_, err = ms.ResolveEscrow(env.Ctx, &types.MsgResolveEscrow{
		Moderator: moderator, LockId: res.LockId, PayBeneficiary: false,
	})
	require.NoError(t, err)

	require.Equal(t, before, env.Balance(buyerAddr, escrowDenom))
	require.True(t, env.ModuleBalance(types.ModuleName, escrowDenom).IsZero())
}

// A settled escrow is finished. Without this a moderator could resolve a case
// twice and pay the same money out of the module account again — which the bank
// would refuse only once the account ran dry, meaning somebody else's escrow
// would be the one that failed.
func TestSettledEscrowCannotBeSettledAgain(t *testing.T) {
	f := initFixture(t)
	env, ms := f.env, f.ms
	buyer, seller, moderator := escrowParties(t, env)

	res, err := ms.OpenEscrow(env.Ctx, &types.MsgOpenEscrow{
		Depositor: buyer, Beneficiary: seller, Moderator: moderator,
		Amount: sdk.NewCoin(escrowDenom, math.NewInt(5_000)),
	})
	require.NoError(t, err)
	_, err = ms.ReleaseEscrow(env.Ctx, &types.MsgReleaseEscrow{Depositor: buyer, LockId: res.LockId})
	require.NoError(t, err)

	_, err = ms.ReleaseEscrow(env.Ctx, &types.MsgReleaseEscrow{Depositor: buyer, LockId: res.LockId})
	require.ErrorIs(t, err, types.ErrLockClosed)
	_, err = ms.DisputeEscrow(env.Ctx, &types.MsgDisputeEscrow{Party: seller, LockId: res.LockId, Reason: "again"})
	require.ErrorIs(t, err, types.ErrLockClosed)
}

// Guards on the shape of the deal itself.
func TestEscrowRefusesNonsenseArrangements(t *testing.T) {
	f := initFixture(t)
	env, ms := f.env, f.ms
	buyer, seller, moderator := escrowParties(t, env)
	amount := sdk.NewCoin(escrowDenom, math.NewInt(1_000))

	_, err := ms.OpenEscrow(env.Ctx, &types.MsgOpenEscrow{
		Depositor: buyer, Beneficiary: buyer, Moderator: moderator, Amount: amount,
	})
	require.ErrorIs(t, err, types.ErrSelfEscrow)

	// A moderator who is one of the parties is not a moderator.
	_, err = ms.OpenEscrow(env.Ctx, &types.MsgOpenEscrow{
		Depositor: buyer, Beneficiary: seller, Moderator: seller, Amount: amount,
	})
	require.ErrorIs(t, err, types.ErrModeratorIsParty)

	_, err = ms.OpenEscrow(env.Ctx, &types.MsgOpenEscrow{
		Depositor: buyer, Beneficiary: seller, Moderator: buyer, Amount: amount,
	})
	require.ErrorIs(t, err, types.ErrModeratorIsParty)

	// A case with no statement gets decided on whoever complains loudest.
	res, err := ms.OpenEscrow(env.Ctx, &types.MsgOpenEscrow{
		Depositor: buyer, Beneficiary: seller, Moderator: moderator, Amount: amount,
	})
	require.NoError(t, err)
	_, err = ms.DisputeEscrow(env.Ctx, &types.MsgDisputeEscrow{Party: buyer, LockId: res.LockId, Reason: ""})
	require.ErrorIs(t, err, types.ErrNoReason)
}
