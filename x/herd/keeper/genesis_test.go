package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/herd/keeper"
	"yamale/blockchain/x/herd/types"
)

// A registry that renumbers on import is a different registry.
//
// This is the check that catches the whole class of derived-state bug at once:
// the indexes are rebuilt from the records rather than exported, so an index
// that disagrees with its record is invisible until an animal cannot be found
// years later — and the counters are exported explicitly, because deriving them
// from the highest id present changes the next id whenever the highest-numbered
// record has been retired.
func TestGenesisRoundTripsByteForByte(t *testing.T) {
	tf := setupTransfer(t)

	// A registry with something of everything in it: two herds, a certified
	// line, a health campaign, a completed sale and one still open.
	dam := tf.animal(t, "221/682/200001/L", "982000000200001", "2021-02-11", types.SEX_FEMALE, "single")
	lamb := tf.animal(t, "221/682/200002/L", "982000000200002", "2025-06-01", types.SEX_FEMALE, "single")
	require.NoError(t, tf.certify(t, lamb, tf.animalID, dam))

	_, err := tf.srv.RecordHealthEvent(tf.env.Ctx, &types.MsgRecordHealthEvent{
		Agent: tf.office, HerdId: tf.herd,
		Disease: "PPR", Batch: "PPR-2026-0431", Treated: 38,
	})
	require.NoError(t, err)

	done := tf.propose(t)
	tf.accept(t, done)
	require.NoError(t, tf.attest(t, done, tf.attestorA))
	require.NoError(t, tf.attest(t, done, tf.attestorB))
	tf.advance(8)
	_, err = tf.srv.CompleteTransfer(tf.env.Ctx, &types.MsgCompleteTransfer{
		Creator: tf.buyer, TransferId: done,
	})
	require.NoError(t, err)

	// A retired animal, which is the case the tag index treats specially: a
	// retired animal releases its tag so a recovered transponder can lawfully
	// be refitted, and an import that reindexed it would refuse that refitting.
	retired := tf.animal(t, "221/682/200003/L", "982000000200003", "2019-09-09", types.SEX_MALE, "single")
	_, err = tf.srv.RetireAnimal(tf.env.Ctx, &types.MsgRetireAnimal{
		Creator: tf.office, AnimalId: retired, Reason: "slaughtered at Tambacounda",
	})
	require.NoError(t, err)

	exported, err := tf.k.ExportGenesis(tf.env.Ctx)
	require.NoError(t, err)

	// A second, empty keeper on its own store, so nothing survives from the
	// first by accident — importing over live state would let a stale index
	// masquerade as a rebuilt one.
	fresh := integration.NewWith(t, []string{types.ModuleName})
	k2 := keeper.NewKeeper(
		fresh.StoreService, fresh.Codec, fresh.AddressCodec, fresh.Authority, nil)
	require.NoError(t, k2.InitGenesis(fresh.Ctx, *exported))

	reexported, err := k2.ExportGenesis(fresh.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, reexported)
}

// The counters, specifically. Exporting them is the difference between a
// restored registry and one that hands out an id somebody already holds.
func TestTheNextIdSurvivesARetiredHighestRecord(t *testing.T) {
	tf := setupTransfer(t)

	last := tf.animal(t, "221/682/300001/L", "982000000300001", "2022-05-05", types.SEX_MALE, "single")
	_, err := tf.srv.RetireAnimal(tf.env.Ctx, &types.MsgRetireAnimal{
		Creator: tf.office, AnimalId: last, Reason: "died",
	})
	require.NoError(t, err)

	exported, err := tf.k.ExportGenesis(tf.env.Ctx)
	require.NoError(t, err)
	require.Greater(t, exported.NextAnimalId, last,
		"the counter must be past the retired animal, or the next import reissues its id")

	fresh := integration.NewWith(t, []string{types.ModuleName})
	k2 := keeper.NewKeeper(
		fresh.StoreService, fresh.Codec, fresh.AddressCodec, fresh.Authority, nil)
	require.NoError(t, k2.InitGenesis(fresh.Ctx, *exported))

	next, err := k2.NextAnimalID.Peek(fresh.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported.NextAnimalId, next)
}

// The indexes are not exported, so the only proof they were rebuilt correctly
// is to use them on the imported registry. An index that silently came back
// empty would leave every record present and every lookup failing — which is
// exactly how it would be discovered in production, and far too late.
func TestTheIndexesAreRebuiltOnImportAndActuallyWork(t *testing.T) {
	tf := setupTransfer(t)
	lamb := tf.animal(t, "221/682/400001/L", "982000000400001", "2025-03-03", types.SEX_FEMALE, "single")

	exported, err := tf.k.ExportGenesis(tf.env.Ctx)
	require.NoError(t, err)

	fresh := integration.NewWith(t, []string{types.ModuleName})
	k2 := keeper.NewKeeper(
		fresh.StoreService, fresh.Codec, fresh.AddressCodec, fresh.Authority, nil)
	require.NoError(t, k2.InitGenesis(fresh.Ctx, *exported))

	q := keeper.NewQueryServerImpl(k2)

	// The certificate lookup: the identifier printed on the document.
	byID, err := q.AnimalByNationalID(fresh.Ctx, &types.QueryAnimalByNationalIDRequest{
		NationalId: "221/682/400001/L",
	})
	require.NoError(t, err)
	require.Equal(t, lamb, byID.Animal.Id)

	// The transponder an agent's reader returns in a field with no signal.
	byTag, err := q.AnimalByTag(fresh.Ctx, &types.QueryAnimalByTagRequest{
		Tag: "982000000400001",
	})
	require.NoError(t, err)
	require.Equal(t, lamb, byTag.Animal.Id)

	// The paper reference an officer searches by.
	byRef, err := q.HerdByRef(fresh.Ctx, &types.QueryHerdByRefRequest{
		Reference: "SN-DK-H-000147",
	})
	require.NoError(t, err)
	require.Equal(t, tf.herd, byRef.Herd.Id)

	// And the prefix walk that makes "list this herd" affordable on a national
	// register rather than a scan nobody runs twice.
	animals, err := q.Animals(fresh.Ctx, &types.QueryAnimalsRequest{HerdId: tf.herd})
	require.NoError(t, err)
	require.Len(t, animals.Animals, len(exported.Animals))
}

// An empty registry has to import too. It is the state every chain starts in,
// and a module that only survives a round trip once it has records is a module
// that breaks on the first genesis.
func TestAnEmptyRegistryRoundTrips(t *testing.T) {
	env := integration.NewWith(t, []string{types.ModuleName})
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, nil)

	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))
	exported, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.Equal(t, types.DefaultGenesis(), exported)
}
