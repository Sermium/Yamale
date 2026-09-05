package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	ammkeeper "yamale/blockchain/x/amm/keeper"
	ammmodule "yamale/blockchain/x/amm/module"
	ammtypes "yamale/blockchain/x/amm/types"
	tokkeeper "yamale/blockchain/x/tokenisation/keeper"
	tokmodule "yamale/blockchain/x/tokenisation/module"
	toktypes "yamale/blockchain/x/tokenisation/types"
)

// K-1, end to end, against both real modules on one real bank.
//
// This is the offensive assessment's proof, reproduced from its description
// rather than its file: pool a fraction denom, realise the vehicle, and every
// LP's counter-asset is locked in the pool forever. It is the composition two
// module-by-module reviews could not see, because each module is correct alone
// and the defect lives in the seam — the tokenisation send restriction, read by
// the AMM's own author as a thing that stops laundering, is the trap on the way
// out.
//
// The setup is the real one: the real SendRestrictionFn on the bank keeper, the
// real AMM paying both reserve legs in one send. Only the wiring line that would
// prevent the pool is withheld in the first test and present in the second, so
// the two tests are the before and after of the fix.

type k1 struct {
	env      *integration.Env
	amm      ammtypes.MsgServer
	ammK     ammkeeper.Keeper
	tok      toktypes.MsgServer
	tokK     tokkeeper.Keeper
	fraction string // the vehicle's fraction denom
	assetID  uint64
	sponsor  string
}

const income = "uyml" // the vehicle's income denom, and the pool's other leg

// setupK1 stands up both modules and a fractionalised, ACTIVE vehicle whose
// fraction tokens are held by the sponsor. wireGuard decides whether the AMM
// has been told which denoms are fraction denoms — the one line the fix adds.
func setupK1(t *testing.T, wireGuard bool) *k1 {
	t.Helper()
	env := integration.NewWith(t,
		[]string{ammtypes.ModuleName, toktypes.ModuleName},
		ammmodule.AppModule{}, tokmodule.AppModule{})

	authority, err := env.AddressCodec.StringToBytes(env.AuthorityString(t))
	require.NoError(t, err)

	tokK := tokkeeper.NewKeeper(
		env.Codec, env.Store(toktypes.ModuleName), env.AddressCodec,
		authority, env.BankKeeper, nil, log.NewNopLogger())
	require.NoError(t, tokK.InitGenesis(env.Ctx, *toktypes.DefaultGenesis()))
	tokMs := tokkeeper.NewMsgServerImpl(tokK)

	// The real restriction, on the real bank. This is the piece that makes the
	// trap a trap, and it is registered exactly as app.go registers it.
	env.BankKeeper.AppendSendRestriction(tokK.SendRestrictionFn)

	ammK := ammkeeper.NewKeeper(
		env.StoreService, env.Codec, env.AddressCodec, authority, env.BankKeeper)
	require.NoError(t, ammK.Params.Set(env.Ctx, ammtypes.DefaultParams()))
	if wireGuard {
		ammK.SetRestrictedDenomKeeper(tokK)
	}
	ammMs := ammkeeper.NewMsgServerImpl(ammK)

	_, sponsor := env.Addr(t)
	_, err = tokMs.CreateCollection(env.Ctx, &toktypes.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: toktypes.Collection{
			Id: "warehouses", Authority: sponsor,
			Verification: toktypes.VERIFY_GOVERNANCE, ChallengeWindowSeconds: 86_400,
		},
	})
	require.NoError(t, err)

	minted, err := tokMs.MintAsset(env.Ctx, &toktypes.MsgMintAsset{
		Minter: sponsor, CollectionId: "warehouses", Owner: sponsor, Uri: "ipfs://deed",
	})
	require.NoError(t, err)
	frac, err := tokMs.Fractionalise(env.Ctx, &toktypes.MsgFractionalise{
		Owner: sponsor, AssetId: minted.AssetId, Symbol: "WHSE",
		Supply: math.NewInt(2_000_000), HolderShareBps: 10_000, IncomeDenom: income,
	})
	require.NoError(t, err)

	return &k1{
		env: env, amm: ammMs, ammK: ammK, tok: tokMs, tokK: tokK,
		fraction: frac.FractionDenom, assetID: minted.AssetId, sponsor: sponsor,
	}
}

func (k *k1) addr(t *testing.T) (sdk.AccAddress, string) { return k.env.Addr(t) }

// realise drives the vehicle to STATUS_REALISED, which is what arms the trap.
// The proceeds have to be paid first — the C-2 fix requires it — which the
// sponsor running the rug is perfectly able to do.
func (k *k1) realise(t *testing.T) {
	t.Helper()
	_, err := k.tok.ReportSale(k.env.Ctx, &toktypes.MsgReportSale{
		Reporter: k.sponsor, AssetId: k.assetID, Price: sdk.NewCoin(income, math.NewInt(1_000)),
	})
	require.NoError(t, err)

	k.env.Fund(t, mustBytesK1(t, k, k.sponsor), sdk.NewCoins(sdk.NewCoin(income, math.NewInt(1_000))))
	_, err = k.tok.PaySaleProceeds(k.env.Ctx, &toktypes.MsgPaySaleProceeds{
		Payer: k.sponsor, AssetId: k.assetID, Amount: sdk.NewCoin(income, math.NewInt(1_000)),
	})
	require.NoError(t, err)

	k.env.Ctx = k.env.Ctx.WithBlockTime(k.env.Ctx.BlockTime().Add(86_401 * time.Second))
	_, err = k.tok.FinaliseSale(k.env.Ctx, &toktypes.MsgFinaliseSale{Caller: k.sponsor, AssetId: k.assetID})
	require.NoError(t, err)

	asset, err := k.tokK.Assets.Get(k.env.Ctx, k.assetID)
	require.NoError(t, err)
	require.Equal(t, toktypes.STATUS_REALISED, asset.Status)
}

func mustBytesK1(t *testing.T, k *k1, s string) sdk.AccAddress {
	t.Helper()
	b, err := k.env.AddressCodec.StringToBytes(s)
	require.NoError(t, err)
	return b
}

// The vulnerability, on an AMM that has not been told what a fraction denom is.
// This is the state of the shipped chain, and it fails on the fix.
func TestAuditRealisedFractionLocksThePoolsCounterAsset(t *testing.T) {
	k := setupK1(t, false) // the guard is NOT wired: the pre-fix chain

	// The sponsor seeds a pool of the fraction token against real YML.
	k.env.Fund(t, mustBytesK1(t, k, k.sponsor), sdk.NewCoins(sdk.NewCoin(income, math.NewInt(1_000_000))))
	_, err := k.amm.CreatePool(k.env.Ctx, &ammtypes.MsgCreatePool{
		Creator: k.sponsor,
		DenomA:  k.fraction, AmountA: "1000000",
		DenomB: income, AmountB: "1000000",
		SwapFeeBps: 30,
	})
	require.NoError(t, err, "pre-fix: nothing stops a fraction denom going into a pool")

	// An honest LP adds liquidity — real YML in, against the fraction token
	// they were handed a slice of to make it look like a market.
	lp, lpStr := k.addr(t)
	k.env.Fund(t, lp, sdk.NewCoins(sdk.NewCoin(income, math.NewInt(500_000))))
	// Give the LP the matching fraction tokens to deposit (a real join needs
	// both legs); the sponsor transfers them some. Pre-realisation, transfers
	// settle normally.
	require.NoError(t, k.env.BankKeeper.SendCoins(k.env.Ctx,
		mustBytesK1(t, k, k.sponsor), lp, sdk.NewCoins(sdk.NewCoin(k.fraction, math.NewInt(500_000)))))
	_, err = k.amm.JoinPool(k.env.Ctx, &ammtypes.MsgJoinPool{
		Sender: lpStr, PoolId: 0, AmountA: "500000", AmountB: "500000",
	})
	require.NoError(t, err)

	lpYmlBefore := k.env.Balance(lp, income)

	// The rug: realise the vehicle.
	k.realise(t)

	// The LP tries to leave. The payout is both legs in one send; the fraction
	// leg is now halted, so the whole exit reverts and the LP's YML stays in the
	// pool. Not stolen — destroyed. Nobody recovers it, including the sponsor.
	_, err = k.amm.ExitPool(k.env.Ctx, &ammtypes.MsgExitPool{
		Sender: lpStr, PoolId: 0, Shares: "500000",
	})
	require.ErrorIs(t, err, toktypes.ErrTradingHalted,
		"the realised fraction leg should refuse the payout and trap the counter-asset")

	require.Equal(t, lpYmlBefore, k.env.Balance(lp, income),
		"the LP got nothing back")
	require.True(t, k.env.ModuleBalance(ammtypes.ModuleName, income).GTE(math.NewInt(1_500_000)),
		"the YML is still locked in the pool")
}

// The fix: the AMM knows what a fraction denom is, and refuses to pool one, so
// the trap can never be armed. Same setup, one wiring line different.
func TestTheFixRefusesToPoolAFractionDenom(t *testing.T) {
	k := setupK1(t, true) // the guard IS wired: the fixed chain

	k.env.Fund(t, mustBytesK1(t, k, k.sponsor), sdk.NewCoins(sdk.NewCoin(income, math.NewInt(1_000_000))))
	_, err := k.amm.CreatePool(k.env.Ctx, &ammtypes.MsgCreatePool{
		Creator: k.sponsor,
		DenomA:  k.fraction, AmountA: "1000000",
		DenomB: income, AmountB: "1000000",
		SwapFeeBps: 30,
	})
	require.ErrorIs(t, err, ammtypes.ErrRestrictedDenom,
		"a fraction denom must be refused at pool creation, the one point the pool does not yet exist")

	// The fiat pairs the chain actually pools are untouched: only fraction
	// denoms are restricted, and uyml against uusd is neither.
	k.env.Fund(t, mustBytesK1(t, k, k.sponsor),
		sdk.NewCoins(sdk.NewCoin(income, math.NewInt(1_000_000)), sdk.NewCoin("uusd", math.NewInt(1_000_000))))
	_, err = k.amm.CreatePool(k.env.Ctx, &ammtypes.MsgCreatePool{
		Creator: k.sponsor,
		DenomA:  income, AmountA: "1000000",
		DenomB: "uusd", AmountB: "1000000",
		SwapFeeBps: 30,
	})
	require.NoError(t, err, "an ordinary pair is still permissionless")
}
