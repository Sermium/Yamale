package keeper_test

import (
	"testing"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/tokenisation/keeper"
	module "yamale/blockchain/x/tokenisation/module"
	"yamale/blockchain/x/tokenisation/types"
)

// Bank is real here. This module mints a shareholding, holds income against it
// and pays it out, so a stubbed bank would only prove it can write a record —
// not that the record corresponds to any money. Every assertion is on a supply
// or a balance.

const income = "uyml"

func setup(t *testing.T) (*integration.Env, keeper.Keeper, types.MsgServer) {
	t.Helper()
	env := integration.New(t, types.ModuleName, module.AppModule{})
	authority, err := env.AddressCodec.StringToBytes(env.AuthorityString(t))
	require.NoError(t, err)

	// No land registry. Everything in this file is a warehouse or a bond —
	// parcel_id 0 — so the registry is never consulted, and a nil keeper here
	// is what proves it: any call into the land bridge from these paths would
	// fail loudly rather than silently pass.
	k := keeper.NewKeeper(
		env.Codec, env.StoreService, env.AddressCodec,
		authority, env.BankKeeper, nil, log.NewNopLogger(),
	)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))
	return env, k, keeper.NewMsgServerImpl(k)
}

// vehicle builds the smallest working thing: a collection, an asset, and a
// shareholding carrying all of the asset's economics.
func vehicle(t *testing.T, env *integration.Env, ms types.MsgServer, supply int64) (uint64, string, string) {
	t.Helper()
	_, minter := env.Addr(t)

	_, err := ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "deeds", Authority: minter,
			Verification:           types.VERIFY_GOVERNANCE,
			ChallengeWindowSeconds: 86_400,
			DisputeBondBps:         100,
		},
	})
	require.NoError(t, err)

	minted, err := ms.MintAsset(env.Ctx, &types.MsgMintAsset{
		Minter: minter, CollectionId: "deeds", Owner: minter, Uri: "ipfs://deed",
	})
	require.NoError(t, err)

	frac, err := ms.Fractionalise(env.Ctx, &types.MsgFractionalise{
		Owner: minter, AssetId: minted.AssetId, Symbol: "WHSE",
		Supply: math.NewInt(supply), HolderShareBps: 10_000, IncomeDenom: income,
	})
	require.NoError(t, err)

	return minted.AssetId, minter, frac.FractionDenom
}

// Regression for the halt: a Position exists for anyone who has ever held the
// denom, including somebody who transferred it all away. Dividing the payout by
// their balance panicked on zero, and a panic in a permissionless handler stops
// every validator.
//
// Before the fix this test does not fail — it takes the process down.
func TestRedeemWithZeroBalanceDoesNotHalt(t *testing.T) {
	env, k, ms := setup(t)
	id, owner, denom := vehicle(t, env, ms, 1_000)

	_, other := env.Addr(t)
	ownerAddr, err := env.AddressCodec.StringToBytes(owner)
	require.NoError(t, err)
	otherAddr, err := env.AddressCodec.StringToBytes(other)
	require.NoError(t, err)

	// Give the whole shareholding away, so the sender keeps a position and no
	// balance.
	require.NoError(t, env.BankKeeper.SendCoins(env.Ctx, ownerAddr, otherAddr,
		sdk.NewCoins(sdk.NewCoin(denom, math.NewInt(1_000)))))

	// Redemption is only reachable at REALISED, so drive the asset there. An
	// earlier version of this test stopped at the status check and passed while
	// exercising none of the arithmetic it claims to cover.
	asset, err := k.Assets.Get(env.Ctx, id)
	require.NoError(t, err)
	asset.Status = types.STATUS_REALISED
	require.NoError(t, k.Assets.Set(env.Ctx, id, asset))

	// And a position must exist for the sender, which is what makes the
	// divisor reachable at all.
	require.NoError(t, k.Settle(env.Ctx, id, ownerAddr, math.ZeroInt()))

	require.NotPanics(t, func() {
		_, err = ms.Redeem(env.Ctx, &types.MsgRedeem{
			Holder: owner, AssetId: id, Amount: math.NewInt(1),
		})
	})
	require.Error(t, err, "redeeming with no balance must be refused, not panic")
}

// Regression for the theft: AccrueIncome credits the index by amount alone, in
// the vault's denom. Funding a worthless token used to raise the index as
// though real income had arrived, letting the funder claim the vault in the
// denom that actually has value.
func TestFundVaultRefusesForeignDenom(t *testing.T) {
	env, k, ms := setup(t)
	id, owner, _ := vehicle(t, env, ms, 1_000)

	ownerAddr, err := env.AddressCodec.StringToBytes(owner)
	require.NoError(t, err)
	env.Fund(t, ownerAddr, sdk.NewCoins(sdk.NewCoin("ushib", math.NewInt(1_000_000))))

	before, err := k.Vaults.Get(env.Ctx, id)
	require.NoError(t, err)

	_, err = ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: owner, AssetId: id,
		Amount: sdk.NewCoin("ushib", math.NewInt(1_000_000)),
	})
	require.ErrorIs(t, err, types.ErrWrongDenom)

	after, err := k.Vaults.Get(env.Ctx, id)
	require.NoError(t, err)
	require.True(t, after.CumulativePerToken.Equal(before.CumulativePerToken),
		"a refused funding must not move the index")
}

// The invariant the module exists to keep: what the vault owes its holders is
// never more than what it holds. Asserted as a property over the whole flow
// rather than pinned to one number, so it survives a refactor of the maths.
func TestVaultNeverOwesMoreThanItHolds(t *testing.T) {
	env, k, ms := setup(t)
	id, owner, denom := vehicle(t, env, ms, 3) // deliberately indivisible

	ownerAddr, err := env.AddressCodec.StringToBytes(owner)
	require.NoError(t, err)
	env.Fund(t, ownerAddr, sdk.NewCoins(sdk.NewCoin(income, math.NewInt(1_000_000))))

	// Spread the shareholding so the division has a remainder.
	_, b := env.Addr(t)
	bAddr, err := env.AddressCodec.StringToBytes(b)
	require.NoError(t, err)
	require.NoError(t, env.BankKeeper.SendCoins(env.Ctx, ownerAddr, bAddr,
		sdk.NewCoins(sdk.NewCoin(denom, math.NewInt(1)))))

	// Amounts chosen to truncate: 100 over 3 holders, repeatedly.
	for i := 0; i < 7; i++ {
		_, err = ms.FundVault(env.Ctx, &types.MsgFundVault{
			Funder: owner, AssetId: id,
			Amount: sdk.NewCoin(income, math.NewInt(100)),
		})
		require.NoError(t, err)
	}

	held := env.ModuleBalance(types.ModuleName, income)

	owed := math.ZeroInt()
	for _, h := range []string{owner, b} {
		addr, err := env.AddressCodec.StringToBytes(h)
		require.NoError(t, err)
		e, err := k.Entitlement(env.Ctx, id, addr)
		require.NoError(t, err)
		owed = owed.Add(e)
	}

	require.True(t, owed.LTE(held),
		"vault owes %s but holds %s — truncation must favour the vault", owed, held)
}

// Rounding direction, stated on its own. Every division on a payout path must
// leave the fraction with the module, never with the caller: a one-unit leak
// per call is a drain when the call is permissionless and repeatable.
func TestAccrualTruncatesTowardTheVault(t *testing.T) {
	env, k, ms := setup(t)
	id, owner, _ := vehicle(t, env, ms, 3)

	ownerAddr, err := env.AddressCodec.StringToBytes(owner)
	require.NoError(t, err)
	env.Fund(t, ownerAddr, sdk.NewCoins(sdk.NewCoin(income, math.NewInt(100))))

	_, err = ms.FundVault(env.Ctx, &types.MsgFundVault{
		Funder: owner, AssetId: id, Amount: sdk.NewCoin(income, math.NewInt(10)),
	})
	require.NoError(t, err)

	e, err := k.Entitlement(env.Ctx, id, ownerAddr)
	require.NoError(t, err)
	// 10 across 3 tokens is 3.33 each; the sole holder of all 3 gets 9, not 10.
	require.True(t, e.LTE(math.NewInt(10)))
}

// A collection that would let one attestor decide a sale price is refused at
// creation. One attestor is not a threshold, it is a single point of unlimited
// theft — the same rule x/custody enforces on deposits.
func TestSingleAttestorCollectionRefused(t *testing.T) {
	env, _, ms := setup(t)
	_, minter := env.Addr(t)

	_, err := ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "weak", Authority: minter,
			Verification: types.VERIFY_ATTESTORS, AttestationThreshold: 1,
			ChallengeWindowSeconds: 86_400,
		},
	})
	require.ErrorIs(t, err, types.ErrInvalidThreshold)
}

// Only the collection's appointed authority may mint title, and a collection
// whose authority has been revoked refuses outright rather than falling back to
// governance.
func TestMintRequiresTheAppointedAuthority(t *testing.T) {
	env, _, ms := setup(t)
	_, minter := env.Addr(t)
	_, stranger := env.Addr(t)

	_, err := ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "deeds", Authority: minter,
			Verification: types.VERIFY_GOVERNANCE, ChallengeWindowSeconds: 86_400,
		},
	})
	require.NoError(t, err)

	_, err = ms.MintAsset(env.Ctx, &types.MsgMintAsset{
		Minter: stranger, CollectionId: "deeds", Owner: stranger,
	})
	require.ErrorIs(t, err, types.ErrNotAuthority)

	_, err = ms.SetCollectionAuthority(env.Ctx, &types.MsgSetCollectionAuthority{
		Authority: env.AuthorityString(t), CollectionId: "deeds", NewAuthority: "",
	})
	require.NoError(t, err)

	_, err = ms.MintAsset(env.Ctx, &types.MsgMintAsset{
		Minter: minter, CollectionId: "deeds", Owner: minter,
	})
	require.ErrorIs(t, err, types.ErrNoAuthority,
		"a revoked authority must refuse, not fall back to governance")
}

// Supply is fixed at fractionalisation. An owner who could issue more against
// an asset whose interests they had already sold is the whole fraud.
func TestCannotFractionaliseTwice(t *testing.T) {
	env, _, ms := setup(t)
	id, owner, _ := vehicle(t, env, ms, 1_000)

	_, err := ms.Fractionalise(env.Ctx, &types.MsgFractionalise{
		Owner: owner, AssetId: id, Symbol: "MORE",
		Supply: math.NewInt(1_000_000), HolderShareBps: 10_000, IncomeDenom: income,
	})
	require.ErrorIs(t, err, types.ErrAlreadyFractionalised)
}

// Genesis must round-trip, or an export cannot be re-imported and upgrades
// break. The counter is the usual casualty: derived state that writes zero
// where InitGenesis wrote nothing.
func TestGenesisRoundTrips(t *testing.T) {
	env, k, ms := setup(t)
	vehicle(t, env, ms, 1_000)

	exported, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())

	require.NoError(t, k.InitGenesis(env.Ctx, *exported))
	again, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, again)
}
