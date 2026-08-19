package types

import "fmt"

// DefaultGenesis returns the default genesis state
//
// The counters start at one because ids are handed out from one. In proto3 a
// uint64 of zero is indistinguishable from a field that was never set, so
// treasury 0 is an id that every client can produce by accident: a request that
// simply omits treasury_id deposits into it. Found on a live chain, where a
// TypeScript client passing null encoded it as 0 and the deposit and the spend
// both succeeded against somebody else's treasury.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:        DefaultParams(),
		TreasuryCount: 1,
		LockCount:     1,
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
//
// The ledger checks matter more than they look. A genesis whose balance entries
// disagree with its locks would start the chain with commitments it cannot
// honour, and nothing later would detect it — the shortfall would only surface
// when a beneficiary tried to claim.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	seenTreasury := make(map[uint64]bool, len(gs.TreasuryList))
	for _, t := range gs.TreasuryList {
		if seenTreasury[t.Id] {
			return fmt.Errorf("duplicate treasury id %d", t.Id)
		}
		if t.Id >= gs.TreasuryCount {
			return fmt.Errorf("treasury id %d is at or beyond treasury_count %d", t.Id, gs.TreasuryCount)
		}
		if t.Admin == "" {
			return fmt.Errorf("treasury %d has no admin", t.Id)
		}
		seenTreasury[t.Id] = true
	}

	// locked[treasuryID][denom] rebuilt from the locks themselves.
	locked := make(map[uint64]map[string]int64)

	seenLock := make(map[uint64]bool, len(gs.LockList))
	for _, l := range gs.LockList {
		if seenLock[l.Id] {
			return fmt.Errorf("duplicate lock id %d", l.Id)
		}
		if l.Id >= gs.LockCount {
			return fmt.Errorf("lock id %d is at or beyond lock_count %d", l.Id, gs.LockCount)
		}
		if !seenTreasury[l.TreasuryId] {
			return fmt.Errorf("lock %d belongs to unknown treasury %d", l.Id, l.TreasuryId)
		}
		seenLock[l.Id] = true

		if !l.Active {
			continue
		}
		remaining := RemainingAmount(l)
		if !remaining.IsInt64() {
			return fmt.Errorf("lock %d has an unrepresentable remaining amount", l.Id)
		}
		if locked[l.TreasuryId] == nil {
			locked[l.TreasuryId] = make(map[string]int64)
		}
		locked[l.TreasuryId][l.Denom] += remaining.Int64()
	}

	for _, b := range gs.BalanceList {
		if !seenTreasury[b.TreasuryId] {
			return fmt.Errorf("balance entry for unknown treasury %d", b.TreasuryId)
		}
		total, ok := parseNonNegative(b.Total)
		if !ok {
			return fmt.Errorf("treasury %d has an invalid total for %s: %q", b.TreasuryId, b.Denom, b.Total)
		}
		lockedAmt, ok := parseNonNegative(b.Locked)
		if !ok {
			return fmt.Errorf("treasury %d has an invalid locked amount for %s: %q", b.TreasuryId, b.Denom, b.Locked)
		}
		if lockedAmt > total {
			return fmt.Errorf("treasury %d has %d %s locked but holds only %d", b.TreasuryId, lockedAmt, b.Denom, total)
		}
		if want := locked[b.TreasuryId][b.Denom]; lockedAmt != want {
			return fmt.Errorf("treasury %d locked balance for %s is %d but its active locks commit %d",
				b.TreasuryId, b.Denom, lockedAmt, want)
		}
	}

	for _, r := range gs.RoleList {
		if !seenTreasury[r.TreasuryId] {
			return fmt.Errorf("role assignment for unknown treasury %d", r.TreasuryId)
		}
		if r.Role == Role_ROLE_UNSPECIFIED {
			return fmt.Errorf("treasury %d assigns %s an unspecified role", r.TreasuryId, r.Address)
		}
	}

	for _, p := range gs.SpendPolicyList {
		if !seenTreasury[p.TreasuryId] {
			return fmt.Errorf("spend policy for unknown treasury %d", p.TreasuryId)
		}
		if p.PeriodLimit != "" && p.PeriodSeconds == 0 {
			return fmt.Errorf("treasury %d sets a period limit for %s with no period length", p.TreasuryId, p.Denom)
		}
	}

	return nil
}

// parseNonNegative reads a decimal string as a non-negative int64.
func parseNonNegative(s string) (int64, bool) {
	if s == "" {
		return 0, true
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
		if n < 0 { // overflowed
			return 0, false
		}
	}
	return n, true
}
