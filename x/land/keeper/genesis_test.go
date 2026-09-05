package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/land/keeper"
	"yamale/blockchain/x/land/types"
)

// A registry that does not survive export and import is a registry that cannot
// be upgraded, and this chain upgrades by exporting state. The counters are the
// part most likely to be got wrong, so they are asserted explicitly rather than
// left to the deep-equal.
func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	id := f.register(t, "survey-A", "REF-001")
	other := f.register(t, "survey-B", "REF-002")

	// One live authorisation and one withdrawn, because the withdrawn one is
	// the entry an export is most tempted to drop — and dropping it would
	// import a registry in which a permission the office took away is live
	// again.
	_, err := f.srv.AuthoriseFractionalisation(f.env.Ctx,
		&types.MsgAuthoriseFractionalisation{
			Creator: f.office, ParcelId: id, Right: "exploitation",
			MaxShareBps: 4000, ExpiresAt: farFuture,
		})
	require.NoError(t, err)
	_, err = f.srv.AuthoriseFractionalisation(f.env.Ctx,
		&types.MsgAuthoriseFractionalisation{
			Creator: f.office, ParcelId: other, Right: "lease",
			MaxShareBps: 2500, ExpiresAt: farFuture,
		})
	require.NoError(t, err)
	_, err = f.srv.AuthoriseFractionalisation(f.env.Ctx,
		&types.MsgAuthoriseFractionalisation{
			Creator: f.office, ParcelId: other, Withdraw: true,
		})
	require.NoError(t, err)

	// One parcel frozen, released and frozen again, so the export carries both
	// a lifted entry with every field set and a live one. A freeze list that
	// survives the round trip only while it holds a single unlifted entry is a
	// list that breaks on the first parcel with a history.
	_, err = f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: other, Reason: "court order 2026/114",
	})
	require.NoError(t, err)
	_, err = f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: other, Unfreeze: true,
		Reason: "inquiry closed, no finding against the holder",
	})
	require.NoError(t, err)
	_, err = f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: other, Reason: "appeal lodged 2026/221",
	})
	require.NoError(t, err)

	f.propose(t, id)

	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())

	require.Len(t, exported.Parcels, 2)
	require.Len(t, exported.Transfers, 1)
	// Asserted here as well as in the byte comparison below, because a freeze
	// history dropped on export would still round-trip byte-for-byte: the
	// import would faithfully reproduce the loss.
	require.Len(t, exported.Parcels[1].Freezes, 2)
	require.True(t, exported.Parcels[1].Freezes[0].Lifted)
	require.False(t, exported.Parcels[1].Freezes[1].Lifted)
	require.Equal(t, "court order 2026/114", exported.Parcels[1].Freezes[0].Reason)
	require.Equal(t, "inquiry closed, no finding against the holder",
		exported.Parcels[1].Freezes[0].LiftReason)
	require.Len(t, exported.Authorities, 4)
	require.Len(t, exported.FractionalisationAuthorities, 2)
	// Parcels 1 and 2, so the next id is 3 — not "the highest id present".
	// Zero is skipped: see RegisterParcel for why a parcel 0 cannot exist.
	require.Equal(t, uint64(1), exported.Parcels[0].Id)
	require.Equal(t, uint64(3), exported.NextParcelId)
	// One transfer proposed, so the next id is 2 — zero is skipped here for the
	// same reason it is skipped for parcels, and a client that omits
	// transfer_id from a MsgValidateTransfer must be refused rather than
	// addressing the first transfer this registry ever recorded.
	require.Equal(t, uint64(1), exported.Transfers[0].Id)
	require.Equal(t, uint64(2), exported.NextTransferId)

	// Import into an empty registry and export again: the two must agree, or an
	// upgrade silently changes the state it claims to be preserving.
	//
	// Empty rather than another setup(): a registry that has already admitted
	// four offices of its own would export those on top of the imported ones,
	// and the comparison would be measuring the fixture rather than the
	// round trip.
	genv := integration.New(t, types.ModuleName)
	g := keeper.NewKeeper(genv.StoreService, genv.Codec, genv.AddressCodec, genv.Authority,
		nil, nil)
	require.NoError(t, g.InitGenesis(genv.Ctx, *exported))

	again, err := g.ExportGenesis(genv.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported.Parcels, again.Parcels)
	require.Equal(t, exported.Transfers, again.Transfers)
	require.Equal(t, exported.NextParcelId, again.NextParcelId)
	require.Equal(t, exported.NextTransferId, again.NextTransferId)
	require.Equal(t, exported.Params, again.Params)
	require.Equal(t, exported.FractionalisationAuthorities, again.FractionalisationAuthorities)

	// Byte for byte, not field by field. Comparing the encodings is the only
	// assertion that catches a field added to the record and forgotten in
	// InitGenesis or ExportGenesis — the two states would still look equal in
	// every property anybody thought to check.
	first, err := exported.Marshal()
	require.NoError(t, err)
	second, err := again.Marshal()
	require.NoError(t, err)
	require.Equal(t, first, second)
}

// The uniqueness guarantee has to hold across an import too — otherwise it can
// be bypassed by writing a genesis file rather than by sending a message.
func TestGenesisRefusesDuplicateGround(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.NextParcelId = 2
	gs.Parcels = []types.Parcel{
		{Id: 0, GeometryHash: "same", CadastralRef: "A"},
		{Id: 1, GeometryHash: "same", CadastralRef: "B"},
	}
	require.ErrorContains(t, gs.Validate(), "share a survey hash")
}

// A counter behind an existing id would hand that id out a second time.
func TestGenesisRefusesRewoundCounter(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.NextParcelId = 1
	gs.Parcels = []types.Parcel{{Id: 5, GeometryHash: "g", CadastralRef: "r"}}
	require.ErrorContains(t, gs.Validate(), "re-issue")
}
