package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

func validGenesis() types.GenesisState {
	gs := *types.DefaultGenesis()
	gs.Jewelers = []types.Jeweler{{Id: 1, Name: "Maison Diop", Business: "b1", Staff: []string{"s1"}, Status: types.JEWELER_ACTIVE}}
	gs.Jewels = []types.Jewel{{Id: 1, JewelerId: 1, Inscription: "MD0001"}}
	gs.Benefits = []types.Benefit{{Id: 1, JewelId: 1, JewelerId: 1, Kind: "cleaning", Allowance: 1}}
	gs.UsedNonces = []types.UsedNonce{{BenefitId: 1, Nonce: "n"}}
	gs.Offers = []types.Offer{{JewelId: 1, From: "b1", To: "c1", ExpiresAt: 10}}
	gs.NextJewelerId, gs.NextJewelId, gs.NextBenefitId = 2, 2, 2
	return gs
}

func TestAGenesisTheModuleCouldHaveWrittenValidates(t *testing.T) {
	require.NoError(t, types.DefaultGenesis().Validate())
	gs := validGenesis()
	require.NoError(t, gs.Validate())
}

func TestAGenesisThatBreaksAGuaranteeIsRefused(t *testing.T) {
	for name, mutate := range map[string]func(*types.GenesisState){
		"counter behind a jeweler": func(g *types.GenesisState) { g.NextJewelerId = 1 },
		"counter behind a jewel":   func(g *types.GenesisState) { g.NextJewelId = 1 },
		"counter behind a benefit": func(g *types.GenesisState) { g.NextBenefitId = 1 },
		"jeweler zero":             func(g *types.GenesisState) { g.Jewelers[0].Id = 0 },
		"business shared": func(g *types.GenesisState) {
			g.Jewelers = append(g.Jewelers, types.Jeweler{Id: 2, Business: "b1"})
			g.NextJewelerId = 3
		},
		"staff on two jewelers": func(g *types.GenesisState) {
			g.Jewelers = append(g.Jewelers, types.Jeweler{Id: 2, Business: "b2", Staff: []string{"s1"}})
			g.NextJewelerId = 3
		},
		"jewel of an unknown jeweler": func(g *types.GenesisState) { g.Jewels[0].JewelerId = 9 },
		"inscription not normalised":  func(g *types.GenesisState) { g.Jewels[0].Inscription = "md-0001" },
		"inscription twice": func(g *types.GenesisState) {
			g.Jewels = append(g.Jewels, types.Jewel{Id: 2, JewelerId: 1, Inscription: "MD0001"})
			g.NextJewelId = 3
		},
		"offer with both targets": func(g *types.GenesisState) { g.Offers[0].ClaimKey = "ab" },
		"offer on no jewel":       func(g *types.GenesisState) { g.Offers[0].JewelId = 9 },
		"benefit on no jewel":     func(g *types.GenesisState) { g.Benefits[0].JewelId = 9 },
		"benefit of another jeweler": func(g *types.GenesisState) {
			g.Jewelers = append(g.Jewelers, types.Jeweler{Id: 2, Business: "b2"})
			g.NextJewelerId = 3
			g.Benefits[0].JewelerId = 2
		},
		"nonce of no benefit":      func(g *types.GenesisState) { g.UsedNonces[0].BenefitId = 9 },
		"reassignment of no jewel": func(g *types.GenesisState) { g.Reassignments = []types.Reassignment{{JewelId: 9}} },
	} {
		gs := validGenesis()
		mutate(&gs)
		require.Error(t, gs.Validate(), name)
	}
}
