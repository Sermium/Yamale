package keeper_test

import (
	"strings"
	"testing"
	"time"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/keeper"
	"yamale/blockchain/x/jewel/types"
)

// The register exported and imported again must export identically, byte for
// byte, or the import is not the same register.
func TestTheRegisterSurvivesExportAndImport(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	f.admit(t, "Atelier Ba")
	id := f.register(t, staff, "MD-0001")
	f.register(t, staff, "MD-0002")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	bid := f.cleaning(t, staff, id, 2, 0)
	_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "n1", time.Minute))
	require.NoError(t, err)
	_, other := f.env.Addr(t)
	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id, To: other, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "r"})
	require.NoError(t, err)

	first, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, first.Validate())
	firstJSON, err := f.env.Codec.MarshalJSON(first)
	require.NoError(t, err)

	g := setup(t)
	k2 := keeper.NewKeeper(g.env.StoreService, g.env.Codec, g.env.AddressCodec, g.env.Authority, nil, g.nft, g.circuit)
	require.NoError(t, k2.InitGenesis(g.env.Ctx, *first))
	second, err := k2.ExportGenesis(g.env.Ctx)
	require.NoError(t, err)
	secondJSON, err := g.env.Codec.MarshalJSON(second)
	require.NoError(t, err)
	require.JSONEq(t, string(firstJSON), string(secondJSON))

	// The indexes were rebuilt, not just the records.
	got, err := k2.ByInscription.Has(g.env.Ctx, collectionsJoin(jid, "MD0001"))
	require.NoError(t, err)
	require.True(t, got)
	has, err := k2.ByStaff.Has(g.env.Ctx, staff)
	require.NoError(t, err)
	require.True(t, has)

	// And the counters continue rather than restart.
	next, err := k2.NextJewelID.Peek(g.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, first.NextJewelId, next)
}

func collectionsJoin(a uint64, b string) collections.Pair[uint64, string] {
	return collections.Join(a, b)
}

// importInto imports gs into a fresh fixture's store and returns InitGenesis's error.
func importInto(t *testing.T, gs types.GenesisState) error {
	t.Helper()
	g := setup(t)
	k2 := keeper.NewKeeper(g.env.StoreService, g.env.Codec, g.env.AddressCodec, g.env.Authority, nil, g.nft, g.circuit)
	return k2.InitGenesis(g.env.Ctx, gs)
}

// Genesis writes addresses without going through a handler, so InitGenesis
// must hold them to the canonical form the handlers enforce.
func TestAGenesisWithAnUpperCaseOfficeIsRefused(t *testing.T) {
	f := setup(t)
	f.admit(t, "Maison Diop")
	gs, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, importInto(t, *gs), "the untouched export must import")

	gs.Params.Office = strings.ToUpper(f.office)
	require.Error(t, importInto(t, *gs))
}

func TestAGenesisWithAnUpperCaseStaffAddressIsRefused(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	gs, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, importInto(t, *gs), "the untouched export must import")

	require.Len(t, gs.Jewelers, 1)
	require.Equal(t, []string{staff}, gs.Jewelers[0].Staff)
	gs.Jewelers[0].Staff[0] = strings.ToUpper(staff)
	require.Error(t, importInto(t, *gs))
}
