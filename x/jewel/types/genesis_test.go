package types_test

import (
	"strings"
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
	gs.Offers = []types.Offer{{JewelId: 1, From: "b1", Maker: "s1", To: "c1", ExpiresAt: 10}}
	gs.Cards = []types.GenesisCard{{JewelId: 1, Card: types.Card{Key: cardKeyG, IssuedBy: "s1", IssuedAt: 5}}}
	gs.Claims = []types.GenesisClaim{{JewelId: 1, Claim: types.Claim{Claimer: "c9", CardKey: cardKeyG, FiledAt: 6}}}
	gs.NextJewelerId, gs.NextJewelId, gs.NextBenefitId = 2, 2, 2
	return gs
}

// cardKeyG is the secp256k1 generator point, compressed: a valid public key
// whose secret (1) nobody would print on a card.
const cardKeyG = "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"

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

		// Two records under one id: the second import would silently
		// overwrite the first.
		"jeweler id twice": func(g *types.GenesisState) {
			g.Jewelers = append(g.Jewelers, types.Jeweler{Id: 1, Name: "Atelier Ba", Business: "b2"})
		},
		"jewel id twice": func(g *types.GenesisState) {
			g.Jewels = append(g.Jewels, types.Jewel{Id: 1, JewelerId: 1, Inscription: "MD0002"})
		},
		"benefit id twice": func(g *types.GenesisState) {
			g.Benefits = append(g.Benefits, types.Benefit{Id: 1, JewelId: 1, JewelerId: 1, Kind: "resize", Allowance: 1})
		},
		"two offers on one jewel": func(g *types.GenesisState) {
			g.Offers = append(g.Offers, types.Offer{JewelId: 1, From: "b1", Maker: "s1", To: "c2", ExpiresAt: 10})
		},
		"two reassignments of one jewel": func(g *types.GenesisState) {
			g.Reassignments = []types.Reassignment{{JewelId: 1, From: "b1", To: "c1"}, {JewelId: 1, From: "b1", To: "c2"}}
		},
		"a used voucher twice": func(g *types.GenesisState) {
			g.UsedNonces = append(g.UsedNonces, types.UsedNonce{BenefitId: 1, Nonce: "n"})
		},

		// States the handlers never write.
		"offer on a stolen jewel": func(g *types.GenesisState) { g.Jewels[0].Stolen = true },
		"claim key in upper case": func(g *types.GenesisState) {
			g.Offers[0].To, g.Offers[0].ClaimKey = "", "02"+strings.Repeat("AB", 32)
		},
		"claim key of 32 bytes": func(g *types.GenesisState) {
			g.Offers[0].To, g.Offers[0].ClaimKey = "", strings.Repeat("ab", 32)
		},
		"claim key not hex": func(g *types.GenesisState) {
			g.Offers[0].To, g.Offers[0].ClaimKey = "", strings.Repeat("zz", 33)
		},
		"reassignment to nobody": func(g *types.GenesisState) {
			g.Reassignments = []types.Reassignment{{JewelId: 1, From: "b1"}}
		},
		"offer without its maker": func(g *types.GenesisState) { g.Offers[0].Maker = "" },
		"amendment note too long": func(g *types.GenesisState) {
			g.Jewels[0].Amendments = []types.Amendment{{Note: strings.Repeat("x", types.MaxAmendmentNote+1)}}
		},
		"owners without their dates": func(g *types.GenesisState) { g.Jewels[0].OwnerCount = 1 },
		"a date without its owner": func(g *types.GenesisState) {
			g.Jewels[0].OwnerSince = []int64{1_788_000_000}
		},
		"too many amendments": func(g *types.GenesisState) {
			g.Jewels[0].Amendments = make([]types.Amendment, types.MaxAmendments+1)
		},

		// Cards and claims. A claim lives only on a jewel with a card, signed
		// with that card; ReportStolen closes it.
		"card on no jewel":       func(g *types.GenesisState) { g.Cards[0].JewelId, g.Claims = 9, nil },
		"claim on no jewel":      func(g *types.GenesisState) { g.Claims[0].JewelId = 9 },
		"claim without a card":   func(g *types.GenesisState) { g.Cards = nil },
		"claim with another key": func(g *types.GenesisState) { g.Claims[0].Claim.CardKey = "02" + strings.Repeat("ab", 32) },
		"two cards on one jewel": func(g *types.GenesisState) { g.Cards = append(g.Cards, g.Cards[0]) },
		"two claims on one jewel": func(g *types.GenesisState) {
			g.Claims = append(g.Claims, types.GenesisClaim{JewelId: 1, Claim: types.Claim{Claimer: "c8", CardKey: cardKeyG}})
		},
		"card key in upper case": func(g *types.GenesisState) {
			g.Cards[0].Card.Key, g.Claims = strings.ToUpper(cardKeyG), nil
		},
		"card key off the curve": func(g *types.GenesisState) {
			g.Cards[0].Card.Key, g.Claims = "02"+strings.Repeat("ff", 32), nil
		},
		"card without its issuer": func(g *types.GenesisState) { g.Cards[0].Card.IssuedBy = "" },
		"claim without a claimer": func(g *types.GenesisState) { g.Claims[0].Claim.Claimer = "" },
		"claim on a stolen jewel": func(g *types.GenesisState) { g.Jewels[0].Stolen, g.Offers = true, nil },
	} {
		t.Run(name, func(t *testing.T) {
			gs := validGenesis()
			mutate(&gs)
			require.Error(t, gs.Validate())
		})
	}
}

// A claim key the handler accepted validates: 33 bytes in lower-case hex.
func TestAGenesisClaimKeyOfferValidates(t *testing.T) {
	gs := validGenesis()
	gs.Offers[0].To, gs.Offers[0].ClaimKey = "", "02"+strings.Repeat("ab", 32)
	require.NoError(t, gs.Validate())
}

// A card with no claim validates, and so does a stolen jewel's card: a theft
// report closes the claim and leaves the card.
func TestAGenesisCardWithoutAClaimValidates(t *testing.T) {
	gs := validGenesis()
	gs.Claims = nil
	require.NoError(t, gs.Validate())
	gs.Jewels[0].Stolen, gs.Offers = true, nil
	require.NoError(t, gs.Validate())
}
