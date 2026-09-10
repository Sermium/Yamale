package keeper

import (
	"context"
	"strings"
	"time"

	"yamale/blockchain/x/herd/types"
)

// The pedigree checks, which are the difference between a certificate and a
// claim.
//
// A Ladoum birth certificate issued today carries a line reading
// "Classico / Atomique / Palmarin". Those are names, in free text. Nobody can
// walk them, and — this is the part that matters — nobody can CONTRADICT them.
// A breeder can declare a sire that was dead, a dam that gave birth a fortnight
// earlier, or an ancestor that is the animal itself, and the document will look
// exactly as good as a true one.
//
// Here an ancestor is a record, so each of those becomes a question with an
// answer. What follows is that list of questions. None of them proves a
// pedigree true — only the laboratory does that — but each one refuses a
// pedigree that cannot be true, and a registry that refuses the impossible is
// worth more than one that records everything.

// checkParentage refuses a declared line that cannot be true of living animals.
func (k Keeper) checkParentage(ctx context.Context, child types.Animal, sireID, damID uint64) error {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}

	if sireID != 0 {
		sire, err := k.mustAnimal(ctx, sireID, "sire")
		if err != nil {
			return err
		}
		// A sire that is not male is not a mistake about this animal, it is a
		// mistake about that one — and it would propagate into every line that
		// passes through it.
		if sire.Sex != types.SEX_MALE {
			return types.ErrSireNotMale.Wrapf("animal %d is %s", sireID, sire.Sex)
		}
		if err := k.assertBornBefore(sire, child, "sire"); err != nil {
			return err
		}
	}

	if damID != 0 {
		dam, err := k.mustAnimal(ctx, damID, "dam")
		if err != nil {
			return err
		}
		if dam.Sex != types.SEX_FEMALE {
			return types.ErrDamNotFemale.Wrapf("animal %d is %s", damID, dam.Sex)
		}
		if err := k.assertBornBefore(dam, child, "dam"); err != nil {
			return err
		}
		if err := k.assertBirthInterval(ctx, dam, child, params.MinBirthIntervalDays); err != nil {
			return err
		}
	}

	// Walk upward from each declared parent. An animal that is its own ancestor
	// is not merely wrong, it is a cycle that would make every later pedigree
	// walk run until the depth bound saves it.
	return k.assertNoCycle(ctx, child.Id, sireID, damID)
}

// mustAnimal fetches a declared parent, saying which side it was named on so
// that the error tells a breeder what to correct.
func (k Keeper) mustAnimal(ctx context.Context, id uint64, side string) (types.Animal, error) {
	a, err := k.Animal.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return types.Animal{}, types.ErrParentMissing.Wrapf("%s %d", side, id)
		}
		return types.Animal{}, err
	}
	return a, nil
}

// assertBornBefore refuses a parent younger than its child.
//
// Dates here are days, not timestamps, and either may be absent: a pastoral
// birth is known to the day at best and often not at all. An absent date is not
// a failure — it is the ordinary state of an animal enrolled from a paper
// record — so the check passes when it cannot be made, and says nothing it
// cannot support.
func (k Keeper) assertBornBefore(parent, child types.Animal, side string) error {
	p, okP := parseDay(parent.BornOn)
	c, okC := parseDay(child.BornOn)
	if !okP || !okC {
		return nil
	}
	if !p.Before(c) {
		return types.ErrParentYounger.Wrapf(
			"%s %d was born %s and the animal %s", side, parent.Id, parent.BornOn, child.BornOn)
	}
	return nil
}

// assertBirthInterval refuses a second birth too soon after the first.
//
// This is the check that catches a fabricated line, because inventing ancestry
// usually means attaching several animals to one prestigious dam without
// thinking about the calendar. A ewe cannot produce two unrelated lambs a
// fortnight apart.
//
// Twins are the exception, and they are common enough that a check without one
// would refuse real records: two animals with the same dam, the same birth date
// and a birth type that is not "single" are one birth, not two.
func (k Keeper) assertBirthInterval(
	ctx context.Context, dam, child types.Animal, minDays uint32,
) error {
	born, ok := parseDay(child.BornOn)
	if !ok {
		return nil
	}

	var conflict error
	err := k.Animal.Walk(ctx, nil, func(_ uint64, sibling types.Animal) (bool, error) {
		if sibling.Id == child.Id || sibling.DamId != dam.Id {
			return false, nil
		}
		other, ok := parseDay(sibling.BornOn)
		if !ok {
			return false, nil
		}
		if sameLitter(child, sibling, born, other) {
			return false, nil
		}
		gap := born.Sub(other)
		if gap < 0 {
			gap = -gap
		}
		if gap < time.Duration(minDays)*24*time.Hour {
			conflict = types.ErrBirthTooClose.Wrapf(
				"dam %d has animal %d born %s, and this one %s, %d days apart with a minimum of %d",
				dam.Id, sibling.Id, sibling.BornOn, child.BornOn,
				int(gap.Hours()/24), minDays)
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	return conflict
}

// sameLitter reports whether two animals are one birth rather than two.
func sameLitter(a, b types.Animal, dayA, dayB time.Time) bool {
	if !dayA.Equal(dayB) {
		return false
	}
	return isMultiple(a.BirthType) && isMultiple(b.BirthType)
}

// isMultiple reports whether a birth type describes more than one animal.
//
// Matched loosely on purpose. Breeders write what they write — "twin", "Twins",
// "jumeau" — and a registry that refused an unrecognised word would be refusing
// the record rather than the error. Anything that is not explicitly "single"
// and not empty is treated as a multiple birth, which errs toward accepting a
// real litter over rejecting one.
func isMultiple(birthType string) bool {
	t := strings.ToLower(strings.TrimSpace(birthType))
	return t != "" && t != types.BirthTypeSingle
}

// assertNoCycle refuses a declaration that would make an animal its own
// ancestor.
//
// Bounded by MaxPedigreeDepth like every other walk here. The bound is not
// merely a safeguard against a malicious graph: a cycle that already exists
// would make an unbounded walk run forever, and this check is precisely what
// runs before one can exist.
//
// `seen` deliberately does NOT start with the child in it, and that is the whole
// correctness of this function. Seeding it looks right — the child is where we
// started, so it is visited — but the same map prunes the frontier, so the one
// node the walk exists to find would be the one node it refused to enqueue, and
// this returned nil on every cycle. It was written that way, and the test that
// should have caught it did not: the animals it used had birth dates, so
// assertBornBefore refused the declaration first and this never ran. A cycle is
// only reachable through undated animals in the first place, which here are the
// records enrolled from paper — the common case, not the exotic one.
func (k Keeper) assertNoCycle(ctx context.Context, childID, sireID, damID uint64) error {
	seen := map[uint64]bool{}
	frontier := []uint64{}
	for _, id := range []uint64{sireID, damID} {
		if id != 0 {
			frontier = append(frontier, id)
		}
	}

	for depth := 0; depth < types.MaxPedigreeDepth && len(frontier) > 0; depth++ {
		next := make([]uint64, 0, len(frontier)*2)
		for _, id := range frontier {
			if id == childID {
				return types.ErrPedigreeCycle.Wrapf("animal %d appears in its own line", childID)
			}
			if seen[id] {
				continue
			}
			seen[id] = true

			ancestor, err := k.Animal.Get(ctx, id)
			if err != nil {
				if isNotFound(err) {
					continue
				}
				return err
			}
			for _, parent := range []uint64{ancestor.SireId, ancestor.DamId} {
				if parent != 0 && !seen[parent] {
					next = append(next, parent)
				}
			}
		}
		frontier = next
	}
	return nil
}

// parseDay reads a birth date.
//
// Accepts the two forms the field actually arrives in: an ISO date, and the
// day/month/year a Ladoum certificate prints. Anything else is treated as
// absent rather than as an error, because a registry that refused an
// unparseable date would refuse the enrolment of every animal whose paper
// record says "about 2019".
func parseDay(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
