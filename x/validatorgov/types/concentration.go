package types

import (
	"sort"
	"strings"

	constitutiontypes "yamale/blockchain/x/constitution/types"
)

// The cap arithmetic lives here, as pure functions over a slice, and not in the
// keeper.
//
// That is the seam the epoch check is built around. Everything that makes a
// concentration ceiling hard — deterministic ordering, a fixed denominator, the
// floor below which the chain reports instead of acting — is decided in this
// file against values a test can hand it directly, and the keeper's only job is
// to read the validator set, call this, and carry out what it says. A bug in
// the ordering here is a failing table test rather than a chain that halts on
// one node and not another.
//
// It is also what the owner's open question about voting power turns on. These
// functions take a power figure per validator and never ask where it came from:
// under equal seats it is a seat count, under stake weighting it is
// tokens over the power reduction, and the arithmetic is identical either way.
// Nothing below would change if the chain switched between them.

// SeatHolder is one validator as the ceilings see it: an operator, the power it
// carries, and who it declared itself to be.
type SeatHolder struct {
	// Operator is the address in its account form, matching how the approval
	// allowlist and the rotation records key a validator.
	Operator string

	Power int64

	Declaration Declaration
}

// GroupHolding is one entity, owner or jurisdiction and what it holds.
type GroupHolding struct {
	Cap   ConcentrationCap
	Group string
	Power int64

	// Members are the operators counted in it, ordered largest first and then
	// by address. The order is the demotion order, so it is fixed here rather
	// than left to whatever the caller iterates in: a demotion that depended on
	// Go's map iteration would be computed differently on every node.
	Members []string
}

// CapSet is the ceilings the arithmetic is measured against, lifted out of the
// invariants so this package does not have to care where they are stored.
type CapSet struct {
	Entity          uint64
	BeneficialOwner uint64
	Jurisdiction    uint64
	MinActive       uint32
}

// CapsFrom reads the ceilings out of the settlement in force.
func CapsFrom(inv constitutiontypes.Invariants) CapSet {
	return CapSet{
		Entity:          inv.MaxEntityPowerBps,
		BeneficialOwner: inv.MaxBeneficialOwnerPowerBps,
		Jurisdiction:    inv.MaxJurisdictionPowerBps,
		MinActive:       inv.MinActiveValidators,
	}
}

// CapBps returns the ceiling for one kind of group.
func (c CapSet) CapBps(cap ConcentrationCap) uint64 {
	switch cap {
	case CONCENTRATION_CAP_ENTITY:
		return c.Entity
	case CONCENTRATION_CAP_BENEFICIAL_OWNER:
		return c.BeneficialOwner
	case CONCENTRATION_CAP_JURISDICTION:
		return c.Jurisdiction
	default:
		return 0
	}
}

// capOrder is the order the ceilings are applied in, and it is fixed rather
// than iterated over a map for the usual reason: two nodes disagreeing about
// which ceiling demoted a validator first would disagree about which validator
// was demoted.
var capOrder = []ConcentrationCap{
	CONCENTRATION_CAP_ENTITY,
	CONCENTRATION_CAP_BENEFICIAL_OWNER,
	CONCENTRATION_CAP_JURISDICTION,
}

// groupKey is the declared value a holder is counted under for one ceiling.
//
// An empty declaration is not a group. Counting every validator that declared
// nothing as one enormous entity would demote most of a chain the first time
// somebody forgot a field, and counting them as separate groups would let an
// owner escape a ceiling by leaving the box blank — which is why the field is
// required on the application in the first place, and why anything that got
// past that is skipped here rather than guessed at.
func groupKey(cap ConcentrationCap, d Declaration) string {
	switch cap {
	case CONCENTRATION_CAP_ENTITY:
		return strings.TrimSpace(d.LegalEntityId)
	case CONCENTRATION_CAP_BENEFICIAL_OWNER:
		return strings.TrimSpace(d.BeneficialOwnerId)
	case CONCENTRATION_CAP_JURISDICTION:
		return strings.TrimSpace(d.Jurisdiction)
	default:
		return ""
	}
}

// TotalPower sums what the holders carry. It is the denominator every share is
// measured against, computed once at the start of an epoch and not recomputed
// as validators are demoted within it.
//
// Holding it fixed is deliberate. Recomputing it would mean each demotion
// raised everybody else's share, so one breach could cascade through the whole
// set inside a single block — and the set it stopped at would depend on the
// order the groups happened to be processed in. A fixed denominator corrects at
// most one round of breaches per epoch, which is slower, visible between
// epochs, and cannot run away.
func TotalPower(holders []SeatHolder) int64 {
	var total int64
	for _, h := range holders {
		if h.Power > 0 {
			total += h.Power
		}
	}
	return total
}

// Holdings groups the holders under every ceiling, in a fixed order.
func Holdings(holders []SeatHolder) []GroupHolding {
	sorted := append([]SeatHolder(nil), holders...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Power != sorted[j].Power {
			return sorted[i].Power > sorted[j].Power
		}
		return sorted[i].Operator < sorted[j].Operator
	})

	out := make([]GroupHolding, 0, len(sorted))
	for _, cap := range capOrder {
		index := map[string]int{}
		for _, h := range sorted {
			if h.Power <= 0 {
				continue
			}
			key := groupKey(cap, h.Declaration)
			if key == "" {
				continue
			}
			at, seen := index[key]
			if !seen {
				index[key] = len(out)
				out = append(out, GroupHolding{Cap: cap, Group: key})
				at = len(out) - 1
			}
			out[at].Power += h.Power
			out[at].Members = append(out[at].Members, h.Operator)
		}
	}

	// The map above only accumulates; the order the result is returned in comes
	// from this sort, so nothing downstream depends on Go's map iteration.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Cap != out[j].Cap {
			return out[i].Cap < out[j].Cap
		}
		return out[i].Group < out[j].Group
	})
	return out
}

// DemotionPlan is one validator the epoch check will take the seats from.
type DemotionPlan struct {
	Operator      string
	Cap           ConcentrationCap
	Group         string
	GroupPowerBps uint64
	CapBps        uint64
}

// UncorrectedBreach is a group over its ceiling that the check will not act on,
// because acting would take the active set below the floor.
type UncorrectedBreach struct {
	Cap           ConcentrationCap
	Group         string
	GroupPowerBps uint64
	CapBps        uint64
	Active        uint32
}

// Plan works out what has to be demoted for every ceiling to hold.
//
// Largest member first within a breaching group: it removes the breach in the
// fewest demotions, and the largest single holding is the most concentrated
// thing in the group by definition. Ties break on the operator address, so two
// validators of equal power are never separated by anything a node could
// compute differently.
//
// A group that is still over its ceiling when the floor is reached is returned
// as an uncorrected breach rather than corrected anyway. That is the honest
// half of the design: a ceiling can be arithmetically unsatisfiable — three
// validators under three owners cannot all sit below a fifth of the power — and
// a check that kept demoting until the numbers worked would demote the chain
// into a halt. Enforcement must never be the thing that stops block production.
func Plan(holders []SeatHolder, caps CapSet) ([]DemotionPlan, []UncorrectedBreach) {
	total := TotalPower(holders)
	if total <= 0 {
		return nil, nil
	}

	power := make(map[string]int64, len(holders))
	for _, h := range holders {
		power[h.Operator] = h.Power
	}

	demoted := map[string]bool{}
	plans := make([]DemotionPlan, 0)
	uncorrected := make([]UncorrectedBreach, 0)

	active := uint32(0)
	for _, h := range holders {
		if h.Power > 0 {
			active++
		}
	}

	for _, holding := range Holdings(holders) {
		capBps := caps.CapBps(holding.Cap)
		if capBps == 0 {
			// A ceiling of zero would demote every member of every group. The
			// settlement refuses one at genesis; this is the second guard, at
			// the point of use, because that is where a zero from an upgrade
			// would arrive.
			continue
		}
		allowed := constitutiontypes.AllowedPower(total, capBps)

		remaining := holding.Power
		for _, member := range holding.Members {
			if demoted[member] {
				remaining -= power[member]
			}
		}

		for _, member := range holding.Members {
			if remaining <= allowed {
				break
			}
			if demoted[member] {
				continue
			}
			if active <= caps.MinActive {
				break
			}
			demoted[member] = true
			active--
			remaining -= power[member]
			plans = append(plans, DemotionPlan{
				Operator:      member,
				Cap:           holding.Cap,
				Group:         holding.Group,
				GroupPowerBps: constitutiontypes.PowerBps(holding.Power, total),
				CapBps:        capBps,
			})
		}

		if remaining > allowed {
			uncorrected = append(uncorrected, UncorrectedBreach{
				Cap:           holding.Cap,
				Group:         holding.Group,
				GroupPowerBps: constitutiontypes.PowerBps(holding.Power, total),
				CapBps:        capBps,
				Active:        active,
			})
		}
	}

	return plans, uncorrected
}

// WithinCaps reports whether every group a candidate belongs to would still be
// inside its ceiling with the candidate counted in.
//
// This is what decides a restoration, and it is asked as a hypothetical rather
// than by looking at the current state: a demoted validator carries no power,
// so its groups are inside their ceilings precisely because it was demoted, and
// a check that read the state as it stands would restore it every epoch and
// demote it again in the same one.
func WithinCaps(candidate SeatHolder, others []SeatHolder, caps CapSet) bool {
	if candidate.Power <= 0 {
		return true
	}

	combined := append(append([]SeatHolder(nil), others...), candidate)
	total := TotalPower(combined)
	if total <= 0 {
		return true
	}

	for _, holding := range Holdings(combined) {
		if groupKey(holding.Cap, candidate.Declaration) != holding.Group {
			continue
		}
		capBps := caps.CapBps(holding.Cap)
		if capBps == 0 {
			continue
		}
		if holding.Power > constitutiontypes.AllowedPower(total, capBps) {
			return false
		}
	}
	return true
}
