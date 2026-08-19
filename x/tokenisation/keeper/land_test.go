package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	landkeeper "yamale/blockchain/x/land/keeper"
	landtypes "yamale/blockchain/x/land/types"
	"yamale/blockchain/x/tokenisation/keeper"
	module "yamale/blockchain/x/tokenisation/module"
	"yamale/blockchain/x/tokenisation/types"
)

// The land registry is real here, not stubbed. Every rule under test is the
// registry's — a ceiling, an expiry, a withdrawal, a restriction, who holds
// title — so a stub would only prove this module can read back a struct the
// test itself wrote. What has to be proved is that the two modules agree.

// blockTime is a fixed, sane wall clock. The expiry rule is meaningless at the
// zero time, where every positive expiry is in the future.
var blockTime = time.Unix(1_700_000_000, 0).UTC()

type landFixture struct {
	env   *integration.Env
	landK landkeeper.Keeper
	landS landtypes.MsgServer
	tokK  keeper.Keeper
	tokS  types.MsgServer

	// office is the registry office in charge of the parcel; holder holds title
	// to it and is the account any vehicle over it must be owned by; minter is
	// the tokenisation collection's appointed authority.
	office string
	holder string
	minter string
	parcel uint64
}

func landSetup(t *testing.T) *landFixture {
	t.Helper()

	env := integration.NewWith(t,
		[]string{types.ModuleName, landtypes.ModuleName}, module.AppModule{})
	env.Ctx = env.Ctx.WithBlockTime(blockTime)

	authority, err := env.AddressCodec.StringToBytes(env.AuthorityString(t))
	require.NoError(t, err)

	// nil group keeper: these tests exercise the bridge, not office admission.
	landK := landkeeper.NewKeeper(
		env.Store(landtypes.ModuleName), env.Codec, env.AddressCodec, env.Authority, nil)
	require.NoError(t, landK.Params.Set(env.Ctx, landtypes.DefaultParams()))

	tokK := keeper.NewKeeper(
		env.Codec, env.Store(types.ModuleName), env.AddressCodec,
		authority, env.BankKeeper, module.NewLandBridge(landK), log.NewNopLogger(),
	)
	require.NoError(t, tokK.InitGenesis(env.Ctx, *types.DefaultGenesis()))

	f := &landFixture{
		env:   env,
		landK: landK,
		landS: landkeeper.NewMsgServerImpl(landK),
		tokK:  tokK,
		tokS:  keeper.NewMsgServerImpl(tokK),
	}

	_, f.office = env.Addr(t)
	require.NoError(t, landK.Authority.Set(env.Ctx, f.office, landtypes.Authority{
		Address: f.office, Name: "office", Jurisdiction: "test", Active: true,
	}))

	_, f.holder = env.Addr(t)
	_, f.minter = env.Addr(t)

	registered, err := f.landS.RegisterParcel(env.Ctx, &landtypes.MsgRegisterParcel{
		Creator: f.office, GeometryHash: "survey-A", CadastralRef: "REF-001",
		Holder: f.holder,
	})
	require.NoError(t, err)
	f.parcel = registered.Id

	_, err = f.tokS.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "deeds", Authority: f.minter,
			Verification:           types.VERIFY_GOVERNANCE,
			ChallengeWindowSeconds: 86_400,
			DisputeBondBps:         100,
		},
	})
	require.NoError(t, err)

	return f
}

// expiry is an authorisation deadline one hour past the fixture's block time.
func expiry() int64 { return blockTime.Add(time.Hour).Unix() }

func (f *landFixture) authorise(t *testing.T, maxShareBps uint32, expiresAt int64) {
	t.Helper()
	_, err := f.landS.AuthoriseFractionalisation(f.env.Ctx,
		&landtypes.MsgAuthoriseFractionalisation{
			Creator: f.office, ParcelId: f.parcel, Right: "exploitation",
			MaxShareBps: maxShareBps, ExpiresAt: expiresAt,
		})
	require.NoError(t, err)
}

func (f *landFixture) withdraw(t *testing.T) {
	t.Helper()
	_, err := f.landS.AuthoriseFractionalisation(f.env.Ctx,
		&landtypes.MsgAuthoriseFractionalisation{
			Creator: f.office, ParcelId: f.parcel, Withdraw: true,
		})
	require.NoError(t, err)
}

// forbid imposes the restriction after the authorisation has been granted,
// which is the case that matters: a restriction that could only ever precede a
// grant would never need to outrank one.
func (f *landFixture) forbid(t *testing.T) {
	t.Helper()
	_, err := f.landS.SetRestriction(f.env.Ctx, &landtypes.MsgSetRestriction{
		Creator: f.office, ParcelId: f.parcel,
		Kind: landtypes.RestrictionNoFractionalisation, Detail: "customary tenure",
	})
	require.NoError(t, err)
}

// mint records title over parcelID, owned by the parcel's holder.
func (f *landFixture) mint(t *testing.T, parcelID uint64) uint64 {
	t.Helper()
	res, err := f.tokS.MintAsset(f.env.Ctx, &types.MsgMintAsset{
		Minter: f.minter, CollectionId: "deeds", Owner: f.holder,
		Uri: "ipfs://deed", ParcelId: parcelID,
	})
	require.NoError(t, err)
	return res.AssetId
}

func (f *landFixture) fractionalise(assetID uint64, holderShareBps uint32, symbol string) error {
	_, err := f.tokS.Fractionalise(f.env.Ctx, &types.MsgFractionalise{
		Owner: f.holder, AssetId: assetID, Symbol: symbol,
		Supply: math.NewInt(1_000_000), HolderShareBps: holderShareBps,
		IncomeDenom: income,
	})
	return err
}

// The hole this whole bridge exists to close. Before it, a vehicle over land
// the registry had never approved issued shares like any other asset.
func TestFractionaliseRefusedWithoutAuthorisation(t *testing.T) {
	f := landSetup(t)
	id := f.mint(t, f.parcel)

	require.ErrorIs(t, f.fractionalise(id, 4_000, "LAND"), types.ErrNoLandAuthorisation)
	require.True(t, f.env.Supply(types.FractionDenom(id, "LAND")).IsZero())
}

// Withdrawal stops new issuance. It does not expropriate anybody who already
// holds shares — that is a taking, and it belongs to a court.
func TestFractionaliseRefusedAfterWithdrawal(t *testing.T) {
	f := landSetup(t)
	f.authorise(t, 4_000, expiry())
	f.withdraw(t)
	id := f.mint(t, f.parcel)

	require.ErrorIs(t, f.fractionalise(id, 4_000, "LAND"), types.ErrAuthorisationWithdrawn)
}

// The expiry is measured against block time. A permission granted for one
// purpose must not sit open for years.
func TestFractionaliseRefusedAfterExpiry(t *testing.T) {
	f := landSetup(t)
	f.authorise(t, 4_000, expiry())
	id := f.mint(t, f.parcel)

	// One second past the deadline: the boundary is where an off-by-one lives.
	f.env.Ctx = f.env.Ctx.WithBlockTime(time.Unix(expiry(), 0).UTC())
	require.ErrorIs(t, f.fractionalise(id, 4_000, "LAND"), types.ErrAuthorisationExpired)
}

// The ceiling caps what is sold, not what is kept.
//
// This is the test that fails if the two are read the other way round. The
// office permitted 40%: issuing 4000 is exactly that and is allowed, and
// issuing 6000 — the share a sponsor obeying the inverted reading would
// retain — is a 60% sale of land the registry capped at 40%.
func TestCeilingCapsTheIssuedShareNotTheRetainedShare(t *testing.T) {
	f := landSetup(t)
	f.authorise(t, 4_000, expiry())

	over := f.mint(t, f.parcel)
	require.ErrorIs(t, f.fractionalise(over, 6_000, "OVER"), types.ErrShareCeilingExceeded)
	require.True(t, f.env.Supply(types.FractionDenom(over, "OVER")).IsZero())

	// And one basis point over is over.
	edge := f.mint(t, f.parcel)
	require.ErrorIs(t, f.fractionalise(edge, 4_001, "EDGE"), types.ErrShareCeilingExceeded)

	at := f.mint(t, f.parcel)
	require.NoError(t, f.fractionalise(at, 4_000, "LAND"))

	holder, err := f.env.AddressCodec.StringToBytes(f.holder)
	require.NoError(t, err)
	denom := types.FractionDenom(at, "LAND")
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(holder, denom))

	asset, err := f.tokK.Assets.Get(f.env.Ctx, at)
	require.NoError(t, err)
	require.Equal(t, uint32(4_000), asset.HolderShareBps)
	require.Equal(t, f.parcel, asset.ParcelId)
}

// A ceiling checked one vehicle at a time is not a ceiling: the owner mints a
// second asset over the same ground and sells the same 40% again.
func TestCeilingBoundsTheTotalAcrossVehicles(t *testing.T) {
	f := landSetup(t)
	f.authorise(t, 4_000, expiry())

	first := f.mint(t, f.parcel)
	require.NoError(t, f.fractionalise(first, 3_000, "ONE"))

	second := f.mint(t, f.parcel)
	require.ErrorIs(t, f.fractionalise(second, 1_500, "TWO"), types.ErrShareCeilingExceeded)

	// The room that is genuinely left is still usable.
	require.NoError(t, f.fractionalise(second, 1_000, "TWO"))
}

// A restriction outranks the office's own permission, even one granted before
// the restriction was imposed. Otherwise a standing authorisation is a way to
// sell around a limit set yesterday.
func TestRestrictionOutranksTheAuthorisation(t *testing.T) {
	f := landSetup(t)
	f.authorise(t, 4_000, expiry())
	id := f.mint(t, f.parcel)
	f.forbid(t)

	require.ErrorIs(t, f.fractionalise(id, 4_000, "LAND"),
		types.ErrLandFractionalisationForbidden)
}

// Title lives in x/land and moves without this module hearing about it. A
// sponsor who has sold the land is selling rights over somebody else's ground.
func TestFractionaliseRefusedWhenOwnerIsNoLongerTheParcelHolder(t *testing.T) {
	f := landSetup(t)
	f.authorise(t, 4_000, expiry())
	id := f.mint(t, f.parcel)

	parcel, err := f.landK.Parcel.Get(f.env.Ctx, f.parcel)
	require.NoError(t, err)
	_, parcel.Holder = f.env.Addr(t)
	require.NoError(t, f.landK.Parcel.Set(f.env.Ctx, f.parcel, parcel))

	require.ErrorIs(t, f.fractionalise(id, 4_000, "LAND"), types.ErrNotParcelHolder)
}

// Refused at the mint, where it costs one read, rather than later with a buyer
// on the other side of the transaction.
func TestMintRefusesAnUnknownParcel(t *testing.T) {
	f := landSetup(t)

	_, err := f.tokS.MintAsset(f.env.Ctx, &types.MsgMintAsset{
		Minter: f.minter, CollectionId: "deeds", Owner: f.holder,
		Uri: "ipfs://deed", ParcelId: f.parcel + 99,
	})
	require.ErrorIs(t, err, types.ErrNoParcel)
}

func TestMintRefusesAnOwnerWhoIsNotTheParcelHolder(t *testing.T) {
	f := landSetup(t)
	_, stranger := f.env.Addr(t)

	_, err := f.tokS.MintAsset(f.env.Ctx, &types.MsgMintAsset{
		Minter: f.minter, CollectionId: "deeds", Owner: stranger,
		Uri: "ipfs://deed", ParcelId: f.parcel,
	})
	require.ErrorIs(t, err, types.ErrNotParcelHolder)
}

// The regression guard for everything that is not land: a warehouse receipt, a
// bond, a shipping container. parcel_id 0 means the registry is never
// consulted, and the whole shareholding may be sold.
//
// The parcel here carries a restriction forbidding fractionalisation and has no
// authorisation at all, so any of the land checks leaking into the non-land
// path would refuse this.
func TestAssetWithNoParcelIsUntouchedByTheRegistry(t *testing.T) {
	f := landSetup(t)
	f.forbid(t)

	id := f.mint(t, 0)
	require.NoError(t, f.fractionalise(id, 10_000, "WHSE"))

	holder, err := f.env.AddressCodec.StringToBytes(f.holder)
	require.NoError(t, err)
	denom := types.FractionDenom(id, "WHSE")
	require.Equal(t, math.NewInt(1_000_000), f.env.Balance(holder, denom))
	require.Equal(t, math.NewInt(1_000_000), f.env.Supply(denom))

	asset, err := f.tokK.Assets.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Zero(t, asset.ParcelId)

	// And nothing was recorded against parcel 0, which x/land never issues
	// precisely so that this sentinel can mean "not land".
	_, err = f.tokK.ParcelIssuedBps.Get(f.env.Ctx, 0)
	require.Error(t, err)
}

// The running total is derived state, and derived state that an import forgets
// to rebuild is a ceiling that has quietly reset to zero on land already sold
// in shares.
func TestImportRebuildsTheIssuedTotal(t *testing.T) {
	f := landSetup(t)
	f.authorise(t, 4_000, expiry())
	id := f.mint(t, f.parcel)
	require.NoError(t, f.fractionalise(id, 3_000, "ONE"))

	exported, err := f.tokK.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())

	g := landSetup(t)
	require.NoError(t, g.tokK.InitGenesis(g.env.Ctx, *exported))

	issued, err := g.tokK.ParcelIssuedBps.Get(g.env.Ctx, f.parcel)
	require.NoError(t, err)
	require.Equal(t, uint32(3_000), issued)

	// Export is unchanged by the rebuild: the total is derived, never written
	// into genesis, so a reimport cannot make the two states differ.
	again, err := g.tokK.ExportGenesis(g.env.Ctx)
	require.NoError(t, err)
	first, err := exported.Marshal()
	require.NoError(t, err)
	second, err := again.Marshal()
	require.NoError(t, err)
	require.Equal(t, first, second)
}

// Fails closed. A chain built without x/land must refuse an asset that names a
// parcel rather than treat every parcel as unrestricted.
func TestNamingAParcelWithNoRegistryIsRefused(t *testing.T) {
	env, _, ms := setup(t)

	_, err := ms.CreateCollection(env.Ctx, &types.MsgCreateCollection{
		Authority: env.AuthorityString(t),
		Collection: types.Collection{
			Id: "deeds", Authority: env.AuthorityString(t),
			Verification:           types.VERIFY_GOVERNANCE,
			ChallengeWindowSeconds: 86_400,
			DisputeBondBps:         100,
		},
	})
	require.NoError(t, err)

	_, owner := env.Addr(t)
	_, err = ms.MintAsset(env.Ctx, &types.MsgMintAsset{
		Minter: env.AuthorityString(t), CollectionId: "deeds", Owner: owner,
		Uri: "ipfs://deed", ParcelId: 1,
	})
	require.ErrorIs(t, err, types.ErrNoLandRegistry)
}
