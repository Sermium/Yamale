package types

import "fmt"

// DefaultGenesis returns the default genesis state.
//
// The case count starts at one because cases are numbered from one. A case id
// of zero is indistinguishable from an unset field in every client that reads
// one, and "frozen by case 0" would be an accusation nobody could look up.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:    DefaultParams(),
		CaseCount: 1,
	}
}

// Validate performs basic genesis state validation.
//
// The check that matters is the last one: every freeze must point at a case
// that justifies it. A freeze imported without its case would stop an account
// sending with no way for anyone — including the chain — to say why, which is
// the exact failure this module is written to avoid.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	cases := make(map[uint64]Case, len(gs.Cases))
	for _, c := range gs.Cases {
		if c.Id == 0 {
			return fmt.Errorf("case with id 0; cases are numbered from 1")
		}
		if _, seen := cases[c.Id]; seen {
			return fmt.Errorf("duplicate case %d", c.Id)
		}
		if c.Id >= gs.CaseCount {
			return fmt.Errorf("case %d is at or beyond case_count %d, so the next case opened would collide with it", c.Id, gs.CaseCount)
		}
		if c.Target == "" {
			return fmt.Errorf("case %d has no target", c.Id)
		}
		if c.Status == CASE_STATUS_UNSPECIFIED {
			return fmt.Errorf("case %d has an unspecified status", c.Id)
		}
		if c.Action == CASE_ACTION_UNSPECIFIED {
			return fmt.Errorf("case %d has an unspecified action", c.Id)
		}
		if !c.Recovered.IsValid() {
			return fmt.Errorf("case %d has an invalid recovered amount: %s", c.Id, c.Recovered)
		}
		// Only a seizure can have taken anything. A freeze case carrying a
		// recovered amount would make the module's own total wrong.
		if c.Action == CASE_ACTION_FREEZE && !c.Recovered.IsZero() {
			return fmt.Errorf("case %d ordered a freeze but recorded a recovery of %s", c.Id, c.Recovered)
		}
		cases[c.Id] = c
	}

	seenVote := make(map[string]bool, len(gs.Votes))
	for _, v := range gs.Votes {
		if _, ok := cases[v.CaseId]; !ok {
			return fmt.Errorf("vote by %s on case %d, which does not exist", v.Validator, v.CaseId)
		}
		if v.Option == VOTE_OPTION_UNSPECIFIED {
			return fmt.Errorf("vote by %s on case %d has no option", v.Validator, v.CaseId)
		}
		key := fmt.Sprintf("%d/%s", v.CaseId, v.Validator)
		if seenVote[key] {
			return fmt.Errorf("validator %s voted twice on case %d", v.Validator, v.CaseId)
		}
		seenVote[key] = true
	}

	seenFreeze := make(map[string]bool, len(gs.Freezes))
	for _, f := range gs.Freezes {
		if f.Address == "" {
			return fmt.Errorf("freeze with no address")
		}
		if seenFreeze[f.Address] {
			return fmt.Errorf("duplicate freeze for %s", f.Address)
		}
		seenFreeze[f.Address] = true

		enforcementCase, ok := cases[f.CaseId]
		if !ok {
			return fmt.Errorf("%s is frozen by case %d, which does not exist", f.Address, f.CaseId)
		}
		if enforcementCase.Target != f.Address {
			return fmt.Errorf("%s is frozen by case %d, which is against %s", f.Address, f.CaseId, enforcementCase.Target)
		}
		switch enforcementCase.Status {
		// HELD is a frozen account by construction: the set has decided, the
		// freeze no longer lapses, and the seizure is waiting. Omitting it here
		// would make every export taken while a seizure was waiting fail to
		// import — which is to say, fail at exactly the moment the module was
		// in the middle of doing the thing it exists for.
		case CASE_STATUS_VOTING, CASE_STATUS_HELD, CASE_STATUS_PASSED:
		default:
			return fmt.Errorf("%s is frozen by case %d, which is %s", f.Address, f.CaseId, enforcementCase.Status)
		}
	}

	seenSeizure := make(map[string]bool, len(gs.Seizures))
	for _, s := range gs.Seizures {
		enforcementCase, ok := cases[s.CaseId]
		if !ok {
			return fmt.Errorf("the rolling window records a seizure by case %d, which does not exist", s.CaseId)
		}
		if enforcementCase.Action != CASE_ACTION_SEIZE {
			return fmt.Errorf("the rolling window records a seizure by case %d, which ordered a freeze", s.CaseId)
		}
		if s.Height < 0 {
			return fmt.Errorf("seizure record for case %d has a negative height", s.CaseId)
		}
		if !s.Amount.IsValid() {
			return fmt.Errorf("seizure record for case %d has an invalid amount: %s", s.CaseId, s.Amount)
		}
		// The ledger is keyed by (height, case id), so two records sharing both
		// would silently collapse into one on import and let a window carry
		// less than it should — which is a cap that has quietly been raised.
		key := fmt.Sprintf("%d/%d", s.Height, s.CaseId)
		if seenSeizure[key] {
			return fmt.Errorf("two seizure records for case %d at height %d", s.CaseId, s.Height)
		}
		seenSeizure[key] = true
	}

	return nil
}
