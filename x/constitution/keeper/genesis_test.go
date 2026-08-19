package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/constitution/keeper"
	module "yamale/blockchain/x/constitution/module"
	"yamale/blockchain/x/constitution/types"
)

// A genesis that leaves an invariant unset is refused before the chain starts,
// not caught later by whichever handler happens to read it.
//
// The list is every field, because the failure this module exists to prevent
// was one field: recovery_destination was empty on a running chain for weeks,
// and a seizure carried by two thirds of the validator set would have had
// nowhere to send what it took.
func TestInitGenesisRefusesAnUnsetInvariant(t *testing.T) {
	complete := func() types.Invariants {
		inv := types.DefaultInvariants()
		inv.EnforcementRecoveryDestination = testRecoveryDestination
		return inv
	}

	cases := map[string]struct {
		mutate func(*types.Invariants)
		expect string
	}{
		"entity ceiling": {
			func(inv *types.Invariants) { inv.MaxEntityPowerBps = 0 },
			"max_entity_power_bps must be set",
		},
		"beneficial owner ceiling": {
			func(inv *types.Invariants) { inv.MaxBeneficialOwnerPowerBps = 0 },
			"max_beneficial_owner_power_bps must be set",
		},
		"jurisdiction ceiling": {
			func(inv *types.Invariants) { inv.MaxJurisdictionPowerBps = 0 },
			"max_jurisdiction_power_bps must be set",
		},
		"epoch length": {
			func(inv *types.Invariants) { inv.ConcentrationEpochBlocks = 0 },
			"concentration_epoch_blocks must be positive",
		},
		"validator floor": {
			func(inv *types.Invariants) { inv.MinActiveValidators = 0 },
			"min_active_validators must be positive",
		},
		"seizure threshold": {
			func(inv *types.Invariants) { inv.EnforcementThresholdBps = 0 },
			"enforcement_threshold_bps must exceed 5000",
		},
		"recovery destination": {
			func(inv *types.Invariants) { inv.EnforcementRecoveryDestination = "" },
			"recovery_destination must name the foundation account",
		},
		"voting period": {
			func(inv *types.Invariants) { inv.EnforcementVotingPeriodBlocks = 0 },
			"enforcement_voting_period_blocks must be positive",
		},
		"provisional freeze": {
			func(inv *types.Invariants) { inv.EnforcementProvisionalFreezeBlocks = 0 },
			"enforcement_provisional_freeze_blocks must be positive",
		},
		"amendment delay": {
			func(inv *types.Invariants) { inv.AmendmentDelayBlocks = 0 },
			"below the floor",
		},
		"amendment threshold": {
			func(inv *types.Invariants) { inv.AmendmentThresholdBps = 0 },
			"must exceed enforcement_threshold_bps",
		},
		"custodian count": {
			func(inv *types.Invariants) { inv.FoundationCustodianCount = 0 },
			"foundation_custodian_count must be set",
		},
		"signature threshold": {
			func(inv *types.Invariants) { inv.FoundationSignatureThreshold = 0 },
			"foundation_signature_threshold must be set",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := integration.New(t, types.ModuleName, module.AppModule{})
			k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, newStubStaking())

			genesis := types.DefaultGenesis()
			genesis.Invariants = complete()
			tc.mutate(&genesis.Invariants)

			err := k.InitGenesis(env.Ctx, *genesis)
			require.Error(t, err, "a chain must not start with this unset")
			require.ErrorContains(t, err, tc.expect)

			_, err = k.GetInvariants(env.Ctx)
			require.ErrorIs(t, err, types.ErrNoInvariants,
				"a refused genesis must write nothing at all")
		})
	}
}

// The default template is deliberately not startable, because the one value it
// cannot supply is the one that named a real institution.
func TestDefaultGenesisIsNotStartable(t *testing.T) {
	require.ErrorContains(t, types.DefaultGenesis().Validate(), "recovery_destination")
}

// A settlement whose ceilings cannot be satisfied at its own floor is a
// contradiction rather than a state a chain grows out of, so it is refused
// rather than enforced into a halt.
func TestInitGenesisRefusesAnUnsatisfiableCeiling(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, newStubStaking())

	genesis := types.DefaultGenesis()
	genesis.Invariants.EnforcementRecoveryDestination = testRecoveryDestination
	genesis.Invariants.MinActiveValidators = 3
	genesis.Invariants.MaxEntityPowerBps = 2_000 // one of three is 3334

	require.ErrorContains(t, k.InitGenesis(env.Ctx, *genesis),
		"no set this small could ever satisfy it")
}

// The foundation group's shape is refused when it is legal in x/group and
// wrong for this account. Each case here is a group x/group would happily
// create and that no chain should be sending seized property to.
func TestInitGenesisRefusesAnUnworkableFoundation(t *testing.T) {
	cases := map[string]struct {
		count, threshold uint32
		expect           string
	}{
		"threshold above the membership": {
			5, 6, "no set of custodians could ever act",
		},
		"a bare majority is not one": {
			// 3 of 6 is exactly half: two disjoint halves could each pass a
			// different proposal, and "the custodians agreed" would mean
			// nothing.
			6, 3, "not more than half",
		},
		"a minority": {
			5, 2, "not more than half",
		},
		"unanimity": {
			// The setting that looks safest and is the least safe available.
			5, 5, "would freeze the account seized assets are sent to",
		},
		"a single custodian": {
			1, 1, "would freeze the account seized assets are sent to",
		},
		"more custodians than the gate can read": {
			// x/group's member query pages. Past the cap the ante gate reads a
			// page instead of the group and would refuse every legitimate
			// change to it — so the constitution refuses the count instead.
			types.MaxFoundationCustodians + 1, 60, "above the ceiling",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := integration.New(t, types.ModuleName, module.AppModule{})
			k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, newStubStaking())

			genesis := types.DefaultGenesis()
			genesis.Invariants.EnforcementRecoveryDestination = testRecoveryDestination
			genesis.Invariants.FoundationCustodianCount = tc.count
			genesis.Invariants.FoundationSignatureThreshold = tc.threshold

			err := k.InitGenesis(env.Ctx, *genesis)
			require.Error(t, err)
			require.ErrorContains(t, err, tc.expect)
		})
	}
}

// Three of five is what the ceremony produces, so a genesis carrying it has to
// start. A validation that refused the arrangement the runbook builds would be
// found on launch day.
func TestTheCeremonysThreeOfFiveIsAcceptable(t *testing.T) {
	inv := types.DefaultInvariants()
	inv.EnforcementRecoveryDestination = testRecoveryDestination

	require.Equal(t, uint32(5), inv.FoundationCustodianCount)
	require.Equal(t, uint32(3), inv.FoundationSignatureThreshold)
	require.NoError(t, inv.Validate())
}

// Round-tripped into a second environment, not the same one. Three round-trip
// tests in this repository were once vacuous because the "fresh" keeper shared
// a store with the exporter, so it was asserting that a store equals itself.
func TestGenesisRoundTripsIntoASecondChain(t *testing.T) {
	f := initFixture(t)
	account := f.addValidator(t, 1)

	id := proposeLowerThreshold(t, f, 100)
	_, err := f.ms.RatifyAmendment(f.at(200), &types.MsgRatifyAmendment{Validator: account, AmendmentId: id})
	require.NoError(t, err)

	exported, err := f.keeper.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate(), "an export the chain would refuse to import is not an export")
	require.Len(t, exported.Amendments, 1)
	require.Len(t, exported.Ratifications, 1)

	env := integration.New(t, types.ModuleName, module.AppModule{})
	g := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, newStubStaking())
	require.NoError(t, g.InitGenesis(env.Ctx, *exported))

	reexported, err := g.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.Equal(t, f.env.Codec.MustMarshalJSON(exported), env.Codec.MustMarshalJSON(reexported),
		"a genesis that does not survive a round trip cannot survive an upgrade")
}

// A pending amendment has to keep its own clock across the import. One that
// arrived with no effective height would enact in the first block after the
// upgrade, which is the outcome the public delay exists to make impossible.
func TestPendingAmendmentSurvivesTheRoundTripWithItsDelayIntact(t *testing.T) {
	f := initFixture(t)
	f.addValidator(t, 1)
	id := proposeLowerThreshold(t, f, 100)

	exported, err := f.keeper.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)

	env := integration.New(t, types.ModuleName, module.AppModule{})
	g := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, newStubStaking())
	require.NoError(t, g.InitGenesis(env.Ctx, *exported))

	// The block after the import must not enact it.
	require.NoError(t, g.EndBlocker(env.Ctx.WithBlockHeight(101)))
	amendment, err := g.Amendment.Get(env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.AMENDMENT_STATUS_PENDING, amendment.Status)
	require.Equal(t, int64(100+types.DefaultAmendmentDelayBlocks), amendment.EffectiveAtHeight)
}
