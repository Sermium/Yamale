package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/validatorgov/types"
)

// A genesis round trip, into a second environment.
//
// Into a *second* one because three round-trip tests in this repository were
// found to be vacuous: the "fresh" keeper shared a store with the exporter, so
// the assertion was that a store equals itself and it would have passed against
// an InitGenesis that did nothing at all.
func TestGenesisRoundTripsWithDeclarationsAndAPendingDemotion(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, 2_500, noCeiling, 1)

	_, growerStr := f.admit(t, "ENTITY-BIG", "OWNER-BIG", "CH", 3)
	for i := 0; i < 4; i++ {
		f.admit(t, "ENTITY-"+string(rune('A'+i)), "OWNER-"+string(rune('A'+i)), "ZA", 1)
	}

	f.epoch(t, 1)
	require.True(t, f.demoted(t, growerStr), "the export is only interesting if there is a demotion in it")

	exported, err := f.keeper.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate(), "an export the chain would refuse to import is not an export")
	require.Len(t, exported.Demotions, 1)
	require.Len(t, exported.ApprovedValidatorMap, 5)
	for _, approved := range exported.ApprovedValidatorMap {
		require.NotEmpty(t, approved.Declaration.LegalEntityId)
		require.NotEmpty(t, approved.Declaration.BeneficialOwnerId)
		require.NotEmpty(t, approved.Declaration.Jurisdiction)
	}

	// A different chain entirely: its own environment, its own store.
	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.env.Ctx, *exported))

	reexported, err := g.keeper.ExportGenesis(g.env.Ctx)
	require.NoError(t, err)

	require.Equal(t,
		f.env.Codec.MustMarshalJSON(exported),
		g.env.Codec.MustMarshalJSON(reexported),
		"a genesis that does not survive a round trip cannot survive an upgrade",
	)
}

// A demotion is not derivable from anything else in the genesis: the validator
// it names is jailed, contributes no power, and recomputing the ceilings from
// the exported state would find no breach and conclude there was never anything
// to hold down. So dropping it would hand every demoted validator its seats
// back at the next upgrade, silently.
func TestADroppedDemotionWouldSilentlyRestoreEverything(t *testing.T) {
	f := initFixture(t)
	f.caps(t, 2_000, 2_500, noCeiling, 1)

	_, growerStr := f.admit(t, "ENTITY-BIG", "OWNER-BIG", "CH", 3)
	for i := 0; i < 4; i++ {
		f.admit(t, "ENTITY-"+string(rune('A'+i)), "OWNER-"+string(rune('A'+i)), "ZA", 1)
	}
	f.epoch(t, 1)

	exported, err := f.keeper.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)

	// Import the same genesis with the demotions stripped, which is what an
	// export that forgot them would produce.
	stripped := *exported
	stripped.Demotions = nil

	g := initFixture(t)
	require.NoError(t, g.keeper.InitGenesis(g.env.Ctx, stripped))

	has, err := g.keeper.Demotion.Has(g.env.Ctx, growerStr)
	require.NoError(t, err)
	require.False(t, has,
		"this is what the loss looks like: the chain no longer knows it is holding anybody down")
}

// A demotion naming a validator no allowlist carries would be a validator
// jailed by a rule nobody could look up, and nothing would ever restore it.
func TestGenesisRefusesADemotionWithoutItsValidator(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.Demotions = []types.Demotion{{
		Operator: "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
		Cap:      types.CONCENTRATION_CAP_ENTITY,
		Group:    "ENTITY-A",
	}}

	require.ErrorContains(t, genesis.Validate(), "is not an approved validator")
}

// Every approved validator has to be declared, the founding set included. A
// genesis is the one place a whole validator set can be admitted with no
// declaration at once, which is exactly the founding bias the ceilings exist to
// bound.
func TestGenesisRefusesAnUndeclaredApprovedValidator(t *testing.T) {
	genesis := types.DefaultGenesis()
	genesis.ApprovedValidatorMap = []types.ApprovedValidator{{Candidate: "cosmos1abc", Approved: "true"}}

	require.ErrorContains(t, genesis.Validate(), "legal_entity_id is required")
}
