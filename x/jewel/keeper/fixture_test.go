package keeper_test

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/x/nft"
	nftkeeper "cosmossdk.io/x/nft/keeper"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/jewel/keeper"
	"yamale/blockchain/x/jewel/types"
)

// fakeCircuit stands in for x/circuit. Closed by default, because a chain
// running this module is supposed to have the bare send disabled; the test
// that proves the module checks opens it.
type fakeCircuit struct{ open bool }

func (c *fakeCircuit) IsAllowed(_ context.Context, url string) (bool, error) {
	if url == types.NFTSendTypeURL {
		return c.open, nil
	}
	return true, nil
}

const recordHash = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

type fixture struct {
	env     *integration.Env
	k       keeper.Keeper
	srv     types.MsgServer
	qs      types.QueryServer
	nft     nftkeeper.Keeper
	circuit *fakeCircuit
	office  string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := integration.NewWith(t, []string{types.ModuleName, nft.ModuleName})
	env.Ctx = env.Ctx.WithBlockTime(time.Unix(1_788_000_000, 0)).WithBlockHeight(1).WithChainID("yamale-test-1")

	nk := nftkeeper.NewKeeper(env.Store(nft.ModuleName), env.Codec, env.AuthKeeper, env.BankKeeper)
	circuit := &fakeCircuit{}
	// nil group keeper: admission is tested by writing the office into the
	// params, which is what a successful appointment produces. The group
	// check itself has its own test with a fake.
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, nil, nk, circuit)

	_, office := env.Addr(t)
	p := types.DefaultParams()
	p.Office = office
	require.NoError(t, k.Params.Set(env.Ctx, p))

	return &fixture{env: env, k: k, srv: keeper.NewMsgServerImpl(k), qs: keeper.NewQueryServerImpl(k),
		nft: nk, circuit: circuit, office: office}
}

// admit admits a jeweler with one staff member and returns its id, its
// business account and the staff member.
func (f *fixture) admit(t *testing.T, name string) (uint64, string, string) {
	t.Helper()
	_, business := f.env.Addr(t)
	res, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: f.office, Name: name, Business: business})
	require.NoError(t, err)
	_, staff := f.env.Addr(t)
	_, err = f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: business, JewelerId: res.Id, Add: []string{staff}})
	require.NoError(t, err)
	return res.Id, business, staff
}

// register registers a jewel through the given staff member.
func (f *fixture) register(t *testing.T, staff, inscription string) uint64 {
	t.Helper()
	res, err := f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: staff, Inscription: inscription, RecordHash: recordHash,
		RecordUri: "https://traceability.yamaletech.com/r/" + recordHash,
	})
	require.NoError(t, err)
	return res.Id
}

func (f *fixture) jewel(t *testing.T, id uint64) types.Jewel {
	t.Helper()
	j, err := f.k.Jewel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	return j
}

func (f *fixture) owner(t *testing.T, id uint64) string {
	t.Helper()
	o, err := f.k.Owner(f.env.Ctx, f.jewel(t, id))
	require.NoError(t, err)
	return o
}

// claimSig signs a claim over the jewel's open offer, as the buyer's phone
// does: the bytes name the offer's expiry, so a signature made for one offer
// never claims another.
func (f *fixture) claimSig(t *testing.T, key *secp256k1.PrivKey, id uint64, claimer string) []byte {
	t.Helper()
	o, err := f.k.Offer.Get(f.env.Ctx, id)
	require.NoError(t, err)
	sig, err := key.Sign(types.ClaimSignBytes("yamale-test-1", id, o.ExpiresAt, claimer))
	require.NoError(t, err)
	return sig
}

func (f *fixture) advance(d time.Duration) {
	f.env.Ctx = f.env.Ctx.WithBlockTime(f.env.Ctx.BlockTime().Add(d))
}
