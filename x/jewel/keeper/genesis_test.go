package keeper_test

import (
	"encoding/hex"
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
// byte, or the import is not the same register. The fixture covers every
// record kind, including an open offer to an account, an open claim-key offer,
// a stolen jewel and non-default params, so each export path is exercised.
func TestTheRegisterSurvivesExportAndImport(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
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

	// (a) an open offer to an account, on a jewel of its own.
	offered := f.register(t, staff, "MD-0003")
	_, toAcct := f.env.Addr(t)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: offered, To: toAcct})
	require.NoError(t, err)
	// (b) an open claim-key offer, on a jewel of its own.
	claimed := f.register(t, staff, "MD-0004")
	claimKey := hex.EncodeToString(secp256k1.GenPrivKey().PubKey().Bytes())
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: claimed, ClaimKey: claimKey})
	require.NoError(t, err)
	// (c) a jewel reported stolen by its owner; ReportStolen closes offers, so
	// it has none.
	stolen := f.register(t, staff, "MD-0005")
	thief := f.sell(t, staff, stolen)
	_, err = f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: thief, JewelId: stolen})
	require.NoError(t, err)
	// Suspended last, because a suspended jeweler registers nothing further.
	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "r"})
	require.NoError(t, err)
	// Non-default params, so the params round trip is not a default-to-default.
	p := types.DefaultParams()
	p.Office = f.office
	p.OfferTtl = 7 * 24 * 3600
	p.VoucherMaxTtl = 120
	p.ReassignmentWindow = 10 * 24 * 3600
	require.NoError(t, f.k.Params.Set(f.env.Ctx, p))

	first, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, first.Validate())
	require.Len(t, first.Offers, 2, "both open offers must be in the export")
	require.Len(t, first.Reassignments, 1)
	firstJSON, err := f.env.Codec.MarshalJSON(first)
	require.NoError(t, err)

	g := setup(t)
	k2 := keeper.NewKeeper(g.env.StoreService, g.env.Codec, g.env.AddressCodec, g.env.Authority, nil, g.nft, g.circuit)
	require.NoError(t, k2.InitGenesis(g.env.Ctx, *first))
	second, err := k2.ExportGenesis(g.env.Ctx)
	require.NoError(t, err)
	secondJSON, err := g.env.Codec.MarshalJSON(second)
	require.NoError(t, err)
	require.Equal(t, string(firstJSON), string(secondJSON), "the second export must be byte-identical")

	// Every index was rebuilt, not just the records.
	has := func(name string, ok bool, err error) {
		t.Helper()
		require.NoError(t, err, name)
		require.True(t, ok, name)
	}
	ok, err := k2.ByInscription.Has(g.env.Ctx, collectionsJoin(jid, "MD0001"))
	has("ByInscription", ok, err)
	ok, err = k2.ByStaff.Has(g.env.Ctx, staff)
	has("ByStaff", ok, err)
	ok, err = k2.ByBusiness.Has(g.env.Ctx, business)
	has("ByBusiness", ok, err)
	ok, err = k2.Search.Has(g.env.Ctx, collections.Join("MD0001", id))
	has("Search", ok, err)
	ok, err = k2.BenefitsByJewel.Has(g.env.Ctx, collections.Join(id, bid))
	has("BenefitsByJewel", ok, err)

	// The offers and the stolen flag survive, with their targets.
	o, err := k2.Offer.Get(g.env.Ctx, offered)
	require.NoError(t, err)
	require.Equal(t, toAcct, o.To)
	c, err := k2.Offer.Get(g.env.Ctx, claimed)
	require.NoError(t, err)
	require.Equal(t, claimKey, c.ClaimKey)
	sj, err := k2.Jewel.Get(g.env.Ctx, stolen)
	require.NoError(t, err)
	require.True(t, sj.Stolen)
	gotParams, err := k2.Params.Get(g.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, p, gotParams)

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
