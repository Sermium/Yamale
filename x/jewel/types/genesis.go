package types

import (
	"encoding/hex"
	"fmt"
	"strings"
)

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
		if jewelers[j.Id] {
			return fmt.Errorf("jeweler %d appears twice", j.Id)
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
	stolen := map[uint64]bool{}
	jewelerOf := map[uint64]uint64{}
	inscriptions := map[string]uint64{}
	for _, j := range gs.Jewels {
		if j.Id == 0 || j.Id >= gs.NextJewelId {
			return fmt.Errorf("jewel %d is zero or not below next_jewel_id %d", j.Id, gs.NextJewelId)
		}
		if jewels[j.Id] {
			return fmt.Errorf("jewel %d appears twice", j.Id)
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
		if len(j.Amendments) > MaxAmendments {
			return fmt.Errorf("jewel %d carries %d amendments, more than %d", j.Id, len(j.Amendments), MaxAmendments)
		}
		for i, a := range j.Amendments {
			if len(a.Note) > MaxAmendmentNote {
				return fmt.Errorf("jewel %d amendment %d has a note of %d bytes, more than %d", j.Id, i, len(a.Note), MaxAmendmentNote)
			}
		}
		jewels[j.Id] = true
		stolen[j.Id] = j.Stolen
		jewelerOf[j.Id] = j.JewelerId
	}

	offered := map[uint64]bool{}
	for _, o := range gs.Offers {
		if !jewels[o.JewelId] {
			return fmt.Errorf("an offer names jewel %d, which is not in this genesis", o.JewelId)
		}
		if offered[o.JewelId] {
			return fmt.Errorf("jewel %d has two offers; a jewel has one open offer at a time", o.JewelId)
		}
		offered[o.JewelId] = true
		// Reporting a theft closes the offer, and no offer opens while the
		// flag is up.
		if stolen[o.JewelId] {
			return fmt.Errorf("jewel %d is reported stolen and cannot carry an offer", o.JewelId)
		}
		if (o.To == "") == (o.ClaimKey == "") {
			return fmt.Errorf("the offer on jewel %d must name exactly one of an account and a claim key", o.JewelId)
		}
		if o.ClaimKey != "" && !validClaimKey(o.ClaimKey) {
			return fmt.Errorf("the offer on jewel %d has claim key %q; it must be 33 bytes in lower-case hex", o.JewelId, o.ClaimKey)
		}
		if o.Maker == "" {
			return fmt.Errorf("the offer on jewel %d does not name who made it", o.JewelId)
		}
	}

	benefits := map[uint64]bool{}
	for _, b := range gs.Benefits {
		if b.Id == 0 || b.Id >= gs.NextBenefitId {
			return fmt.Errorf("benefit %d is zero or not below next_benefit_id %d", b.Id, gs.NextBenefitId)
		}
		if benefits[b.Id] {
			return fmt.Errorf("benefit %d appears twice", b.Id)
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
	nonces := map[string]bool{}
	for _, n := range gs.UsedNonces {
		if !benefits[n.BenefitId] {
			return fmt.Errorf("a used voucher names benefit %d, which is not in this genesis", n.BenefitId)
		}
		key := fmt.Sprintf("%d/%s", n.BenefitId, n.Nonce)
		if nonces[key] {
			return fmt.Errorf("voucher %q of benefit %d appears twice", n.Nonce, n.BenefitId)
		}
		nonces[key] = true
	}
	reassigned := map[uint64]bool{}
	for _, r := range gs.Reassignments {
		if !jewels[r.JewelId] {
			return fmt.Errorf("a reassignment names jewel %d, which is not in this genesis", r.JewelId)
		}
		if reassigned[r.JewelId] {
			return fmt.Errorf("jewel %d has two pending reassignments", r.JewelId)
		}
		reassigned[r.JewelId] = true
		if r.To == "" {
			return fmt.Errorf("the reassignment of jewel %d names no one to receive it", r.JewelId)
		}
	}
	return nil
}

// validClaimKey is the form OfferJewel stores: a compressed secp256k1 public
// key, 33 bytes, in lower-case hex.
func validClaimKey(k string) bool {
	if len(k) != 66 || k != strings.ToLower(k) {
		return false
	}
	_, err := hex.DecodeString(k)
	return err == nil
}
