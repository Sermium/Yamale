package types

import "fmt"

// DefaultGenesis returns an empty register with the default settings.
func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

// Validate refuses a genesis the module could not have produced. The counter
// checks matter most: a counter at or below an existing id would issue it
// again, and the second record would silently overwrite the first.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	jewelers := map[uint64]bool{}
	businesses := map[string]uint64{}
	staff := map[string]uint64{}
	for _, j := range gs.Jewelers {
		if j.Id == 0 || j.Id >= gs.NextJewelerId {
			return fmt.Errorf("jeweler %d is zero or not below next_jeweler_id %d", j.Id, gs.NextJewelerId)
		}
		if prev, ok := businesses[j.Business]; ok {
			return fmt.Errorf("jewelers %d and %d share the business account %s", prev, j.Id, j.Business)
		}
		businesses[j.Business] = j.Id
		for _, s := range j.Staff {
			if prev, ok := staff[s]; ok {
				return fmt.Errorf("%s is staff of jewelers %d and %d", s, prev, j.Id)
			}
			staff[s] = j.Id
		}
		jewelers[j.Id] = true
	}

	jewels := map[uint64]bool{}
	jewelerOf := map[uint64]uint64{}
	inscriptions := map[string]uint64{}
	for _, j := range gs.Jewels {
		if j.Id == 0 || j.Id >= gs.NextJewelId {
			return fmt.Errorf("jewel %d is zero or not below next_jewel_id %d", j.Id, gs.NextJewelId)
		}
		if !jewelers[j.JewelerId] {
			return fmt.Errorf("jewel %d names jeweler %d, which is not in this genesis", j.Id, j.JewelerId)
		}
		code, err := NormaliseInscription(j.Inscription)
		if err != nil || code != j.Inscription {
			return fmt.Errorf("jewel %d's inscription %q is not in normalised form", j.Id, j.Inscription)
		}
		key := fmt.Sprintf("%d/%s", j.JewelerId, code)
		if prev, ok := inscriptions[key]; ok {
			return fmt.Errorf("jewels %d and %d share inscription %s at jeweler %d", prev, j.Id, code, j.JewelerId)
		}
		inscriptions[key] = j.Id
		jewels[j.Id] = true
		jewelerOf[j.Id] = j.JewelerId
	}

	for _, o := range gs.Offers {
		if !jewels[o.JewelId] {
			return fmt.Errorf("an offer names jewel %d, which is not in this genesis", o.JewelId)
		}
		if (o.To == "") == (o.ClaimKey == "") {
			return fmt.Errorf("the offer on jewel %d must name exactly one of an account and a claim key", o.JewelId)
		}
	}

	benefits := map[uint64]bool{}
	for _, b := range gs.Benefits {
		if b.Id == 0 || b.Id >= gs.NextBenefitId {
			return fmt.Errorf("benefit %d is zero or not below next_benefit_id %d", b.Id, gs.NextBenefitId)
		}
		if !jewels[b.JewelId] || !jewelers[b.JewelerId] {
			return fmt.Errorf("benefit %d names a jewel or jeweler not in this genesis", b.Id)
		}
		// The handler derives a benefit's jeweler from its jewel; a benefit
		// naming another jeweler would let that jeweler's staff honour it.
		if jewelerOf[b.JewelId] != b.JewelerId {
			return fmt.Errorf("benefit %d names jeweler %d, but its jewel %d belongs to jeweler %d",
				b.Id, b.JewelerId, b.JewelId, jewelerOf[b.JewelId])
		}
		benefits[b.Id] = true
	}
	for _, n := range gs.UsedNonces {
		if !benefits[n.BenefitId] {
			return fmt.Errorf("a used voucher names benefit %d, which is not in this genesis", n.BenefitId)
		}
	}
	for _, r := range gs.Reassignments {
		if !jewels[r.JewelId] {
			return fmt.Errorf("a reassignment names jewel %d, which is not in this genesis", r.JewelId)
		}
	}
	return nil
}
