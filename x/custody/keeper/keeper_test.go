package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/custody/keeper"
	module "yamale/blockchain/x/custody/module"
	"yamale/blockchain/x/custody/types"
)

// Bank is real here, and that is the point. This module mints claims and burns
// them, so a stubbed bank would test that it can write a record rather than
// that the record corresponds to any money. Every assertion below is on supply
// or on balances.

const denom = "yeth"

func setup(t *testing.T) (*integration.Env, keeper.Keeper, types.MsgServer, types.QueryServer) {
	t.Helper()
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(
		env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), env.BankKeeper,
	)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))
	return env, k, keeper.NewMsgServerImpl(k), keeper.NewQueryServerImpl(k)
}

// register an asset and two attestors, which is the minimum working custodian.
func ready(t *testing.T, env *integration.Env, ms types.MsgServer) (string, string) {
	t.Helper()
	_, err := ms.RegisterAsset(env.Ctx, &types.MsgRegisterAsset{
		Authority: env.AuthorityString(t), Denom: denom,
		SourceChain: "ethereum", Symbol: "yETH", Exponent: 18,
	})
	require.NoError(t, err)

	_, a1 := env.Addr(t)
	_, a2 := env.Addr(t)
	for _, a := range []string{a1, a2} {
		_, err := ms.SetAttestor(env.Ctx, &types.MsgSetAttestor{
			Authority: env.AuthorityString(t), Attestor: a, Active: true,
		})
		require.NoError(t, err)
	}
	return a1, a2
}

func TestOneAttestorCannotMint(t *testing.T) {
	env, _, ms, _ := setup(t)
	a1, _ := ready(t, env, ms)
	_, recipient := env.Addr(t)

	res, err := ms.AttestDeposit(env.Ctx, &types.MsgAttestDeposit{
		Attestor: a1, Denom: denom, Recipient: recipient,
		Amount: math.NewInt(1_000), ExternalRef: "0xdeadbeef",
	})
	require.NoError(t, err)
	require.False(t, res.Credited, "one attestor credited a deposit on its own")

	// The whole security argument: nothing exists until the threshold.
	require.True(t, env.BankKeeper.GetSupply(env.Ctx, denom).Amount.IsZero(),
		"supply was minted before the attestation threshold")
}

func TestThresholdMints(t *testing.T) {
	env, _, ms, _ := setup(t)
	a1, a2 := ready(t, env, ms)
	addr, recipient := env.Addr(t)

	amount := math.NewInt(1_000_000)
	for _, a := range []string{a1, a2} {
		_, err := ms.AttestDeposit(env.Ctx, &types.MsgAttestDeposit{
			Attestor: a, Denom: denom, Recipient: recipient,
			Amount: amount, ExternalRef: "0xabc",
		})
		require.NoError(t, err)
	}

	// Supply equals the deposit in full — not the net of fee. The claim
	// outstanding must equal the asset held, or the solvency comparison is
	// wrong by the fee on every deposit ever made.
	require.Equal(t, amount, env.BankKeeper.GetSupply(env.Ctx, denom).Amount)

	// The recipient gets the deposit less the fee. Default is 10 bps.
	got := env.BankKeeper.GetBalance(env.Ctx, addr, denom).Amount
	require.Equal(t, amount.Sub(math.NewInt(1_000)), got)
}

func TestTheSameDepositCannotBeCreditedTwice(t *testing.T) {
	env, _, ms, _ := setup(t)
	a1, a2 := ready(t, env, ms)
	_, recipient := env.Addr(t)

	attest := func(a string) error {
		_, err := ms.AttestDeposit(env.Ctx, &types.MsgAttestDeposit{
			Attestor: a, Denom: denom, Recipient: recipient,
			Amount: math.NewInt(500), ExternalRef: "0xreplay",
		})
		return err
	}
	require.NoError(t, attest(a1))
	require.NoError(t, attest(a2))
	after := env.BankKeeper.GetSupply(env.Ctx, denom).Amount

	// Attesting the same source transaction again must not mint a second time.
	// This is the failure that empties bridges, and it needs somebody to try it
	// rather than showing up in ordinary use.
	require.ErrorIs(t, attest(a1), types.ErrDuplicateRef)
	require.ErrorIs(t, attest(a2), types.ErrDuplicateRef)
	require.Equal(t, after, env.BankKeeper.GetSupply(env.Ctx, denom).Amount,
		"a replayed external reference minted again")
}

func TestOneAttestorCannotAttestTwiceToReachTheThreshold(t *testing.T) {
	env, _, ms, _ := setup(t)
	a1, _ := ready(t, env, ms)
	_, recipient := env.Addr(t)

	msg := &types.MsgAttestDeposit{
		Attestor: a1, Denom: denom, Recipient: recipient,
		Amount: math.NewInt(10), ExternalRef: "0xsolo",
	}
	_, err := ms.AttestDeposit(env.Ctx, msg)
	require.NoError(t, err)

	_, err = ms.AttestDeposit(env.Ctx, msg)
	require.ErrorIs(t, err, types.ErrAlreadyAttested)
	require.True(t, env.BankKeeper.GetSupply(env.Ctx, denom).Amount.IsZero())
}

func TestAttestorsMustAgreeOnTheAmount(t *testing.T) {
	env, _, ms, _ := setup(t)
	a1, a2 := ready(t, env, ms)
	_, recipient := env.Addr(t)

	_, err := ms.AttestDeposit(env.Ctx, &types.MsgAttestDeposit{
		Attestor: a1, Denom: denom, Recipient: recipient,
		Amount: math.NewInt(100), ExternalRef: "0xdisagree",
	})
	require.NoError(t, err)

	// A second attestor naming a larger amount is refused rather than counted.
	// Otherwise one attestor could nudge a figure upward and have the
	// disagreement recorded as consensus.
	_, err = ms.AttestDeposit(env.Ctx, &types.MsgAttestDeposit{
		Attestor: a2, Denom: denom, Recipient: recipient,
		Amount: math.NewInt(100_000), ExternalRef: "0xdisagree",
	})
	require.Error(t, err)
	require.True(t, env.BankKeeper.GetSupply(env.Ctx, denom).Amount.IsZero())
}

func TestNonAttestorIsRefused(t *testing.T) {
	env, _, ms, _ := setup(t)
	ready(t, env, ms)
	_, stranger := env.Addr(t)
	_, recipient := env.Addr(t)

	_, err := ms.AttestDeposit(env.Ctx, &types.MsgAttestDeposit{
		Attestor: stranger, Denom: denom, Recipient: recipient,
		Amount: math.NewInt(1), ExternalRef: "0xnope",
	})
	require.ErrorIs(t, err, types.ErrNotAttestor)
}

func TestRedemptionBurnsImmediatelyAndWaitsOutTheDelay(t *testing.T) {
	env, _, ms, _ := setup(t)
	a1, a2 := ready(t, env, ms)
	addr, recipient := env.Addr(t)

	amount := math.NewInt(1_000_000)
	for _, a := range []string{a1, a2} {
		_, err := ms.AttestDeposit(env.Ctx, &types.MsgAttestDeposit{
			Attestor: a, Denom: denom, Recipient: recipient,
			Amount: amount, ExternalRef: "0xdep",
		})
		require.NoError(t, err)
	}
	held := env.BankKeeper.GetBalance(env.Ctx, addr, denom).Amount

	res, err := ms.RequestRedemption(env.Ctx, &types.MsgRequestRedemption{
		Redeemer: recipient, Denom: denom, Amount: held, Destination: "0xdest",
	})
	require.NoError(t, err)

	// Burned at request, not at payout. Leaving the claim in circulation while
	// the asset is being sent would let it be spent again and redeemed twice.
	require.True(t, env.BankKeeper.GetBalance(env.Ctx, addr, denom).Amount.IsZero())

	// Settling inside the window is refused on chain, not merely discouraged in
	// a client — a window that can be skipped by calling the chain directly is
	// not a window.
	_, err = ms.SettleRedemption(env.Ctx, &types.MsgSettleRedemption{
		Attestor: a1, RedemptionId: res.RedemptionId, SettledRef: "0xpaid",
	})
	require.ErrorIs(t, err, types.ErrNotPayableYet)

	// Past the delay it settles.
	ctx := sdk.UnwrapSDKContext(env.Ctx).WithBlockHeight(res.PayableAtHeight)
	_, err = ms.SettleRedemption(ctx, &types.MsgSettleRedemption{
		Attestor: a1, RedemptionId: res.RedemptionId, SettledRef: "0xpaid",
	})
	require.NoError(t, err)

	// And not twice.
	_, err = ms.SettleRedemption(ctx, &types.MsgSettleRedemption{
		Attestor: a2, RedemptionId: res.RedemptionId, SettledRef: "0xagain",
	})
	require.ErrorIs(t, err, types.ErrAlreadySettled)
}

func TestSolvencyIsComputedNotAsserted(t *testing.T) {
	env, _, ms, qs := setup(t)
	a1, a2 := ready(t, env, ms)
	_, recipient := env.Addr(t)

	// Never attested is not solvent. Reporting a brand-new asset as healthy
	// until somebody deposits into it would make the query useless exactly when
	// it is first consulted.
	s, err := qs.Solvency(env.Ctx, &types.QuerySolvencyRequest{})
	require.NoError(t, err)
	require.Len(t, s.Solvency, 1)
	require.False(t, s.Solvency[0].Solvent)

	amount := math.NewInt(1_000_000)
	for _, a := range []string{a1, a2} {
		_, err := ms.AttestDeposit(env.Ctx, &types.MsgAttestDeposit{
			Attestor: a, Denom: denom, Recipient: recipient,
			Amount: amount, ExternalRef: "0xsolv",
		})
		require.NoError(t, err)
	}

	// Under-reported reserve: issued exceeds held, so insolvent.
	_, err = ms.ReportReserve(env.Ctx, &types.MsgReportReserve{
		Attestor: a1, Denom: denom, Held: amount.Sub(math.OneInt()),
	})
	require.NoError(t, err)
	s, err = qs.Solvency(env.Ctx, &types.QuerySolvencyRequest{})
	require.NoError(t, err)
	require.False(t, s.Solvency[0].Solvent, "issued exceeds held but reported solvent")

	// Fully backed.
	_, err = ms.ReportReserve(env.Ctx, &types.MsgReportReserve{
		Attestor: a1, Denom: denom, Held: amount,
	})
	require.NoError(t, err)
	s, err = qs.Solvency(env.Ctx, &types.QuerySolvencyRequest{})
	require.NoError(t, err)
	require.True(t, s.Solvency[0].Solvent)
	require.Equal(t, amount, s.Solvency[0].Issued)
}

func TestThresholdBelowTwoIsRefused(t *testing.T) {
	env, _, ms, _ := setup(t)

	// One attestor is not a threshold, it is a single point of unlimited
	// issuance — refused rather than merely discouraged.
	_, err := ms.UpdateParams(env.Ctx, &types.MsgUpdateParams{
		Authority: env.AuthorityString(t), Params: types.NewParams(1, 100, 10),
	})
	require.ErrorIs(t, err, types.ErrInvalidParams)

	_, notGov := env.Addr(t)
	_, err = ms.UpdateParams(env.Ctx, &types.MsgUpdateParams{
		Authority: notGov, Params: types.NewParams(3, 100, 10),
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}

func TestGenesisRoundTrips(t *testing.T) {
	env, k, ms, _ := setup(t)
	a1, a2 := ready(t, env, ms)
	_, recipient := env.Addr(t)

	for _, ref := range []string{"0x1", "0x2"} {
		for _, a := range []string{a1, a2} {
			_, err := ms.AttestDeposit(env.Ctx, &types.MsgAttestDeposit{
				Attestor: a, Denom: denom, Recipient: recipient,
				Amount: math.NewInt(1_000), ExternalRef: ref,
			})
			require.NoError(t, err)
		}
	}
	_, err := ms.ReportReserve(env.Ctx, &types.MsgReportReserve{
		Attestor: a1, Denom: denom, Held: math.NewInt(2_000),
	})
	require.NoError(t, err)

	exported, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())

	// A second environment, so the import lands in an empty store. A keeper
	// built over `env.StoreService` shares its state with the one above, which
	// would leave the replay guard already populated by the attestations that
	// built the state — and the assertion below would then hold whether
	// InitGenesis rebuilt it or not.
	other := integration.New(t, types.ModuleName, module.AppModule{})
	fresh := keeper.NewKeeper(other.Codec, other.AddressCodec, other.StoreService,
		log.NewNopLogger(), other.AuthorityString(t), other.BankKeeper)
	require.NoError(t, fresh.InitGenesis(other.Ctx, *exported))

	again, err := fresh.ExportGenesis(other.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, again)

	// The replay guard is derived on import rather than carried, so it has to
	// be asserted directly. Going through AttestDeposit cannot see it: the
	// deposit record *is* carried, so an already-credited one is refused by its
	// own status before the guard is ever consulted, and the call returns
	// ErrDuplicateRef whether the index was rebuilt or not.
	credited := 0
	for _, d := range again.Deposits {
		if d.Status != types.DepositStatus_DEPOSIT_STATUS_CREDITED {
			continue
		}
		credited++
		has, err := fresh.ExternalRefs.Has(other.Ctx, collections.Join(d.Denom, d.ExternalRef))
		require.NoError(t, err)
		require.Truef(t, has, "the replay guard was not rebuilt for %s/%s", d.Denom, d.ExternalRef)
	}
	require.NotZero(t, credited, "no credited deposit survived the export, so the guard is untested")

	// And it still behaves: a credited deposit stays unrepeatable.
	freshMs := keeper.NewMsgServerImpl(fresh)
	_, err = freshMs.AttestDeposit(other.Ctx, &types.MsgAttestDeposit{
		Attestor: a1, Denom: denom, Recipient: recipient,
		Amount: math.NewInt(1_000), ExternalRef: "0x1",
	})
	require.ErrorIs(t, err, types.ErrDuplicateRef,
		"the replay guard did not survive a genesis round trip")
}
