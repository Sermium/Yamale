package types

import "fmt"

// DefaultGenesis returns the default genesis state.
//
// The rotation count starts at one because rotations are numbered from one. A
// rotation id of zero is indistinguishable from an unset field in every client
// that reads one, and a pending rotation nobody could look up would be exactly
// the quiet administrative action the challenge window exists to prevent.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:                  DefaultParams(),
		ValidatorApplicationMap: []ValidatorApplication{}, ApprovedValidatorMap: []ApprovedValidator{},
		RotationCount: 1,
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	validatorApplicationIndexMap := make(map[string]struct{})

	for _, elem := range gs.ValidatorApplicationMap {
		index := fmt.Sprint(elem.Candidate)
		if _, ok := validatorApplicationIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for validatorApplication")
		}
		validatorApplicationIndexMap[index] = struct{}{}
	}
	approvedValidatorIndexMap := make(map[string]struct{})

	for _, elem := range gs.ApprovedValidatorMap {
		index := fmt.Sprint(elem.Candidate)
		if _, ok := approvedValidatorIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for approvedValidator")
		}
		approvedValidatorIndexMap[index] = struct{}{}

		// Every approved validator has to be declared, including the founding
		// set. A validator with no declaration belongs to no entity, no owner
		// and no jurisdiction, so it sits outside all three concentration
		// ceilings — and a genesis is the one place a whole validator set can
		// be admitted that way at once, which is exactly the founding bias the
		// ceilings exist to bound.
		if err := elem.Declaration.Validate(); err != nil {
			return fmt.Errorf("approved validator %s: %w", elem.Candidate, err)
		}
	}

	if err := gs.validateRotations(); err != nil {
		return err
	}
	if err := gs.validateDemotions(approvedValidatorIndexMap); err != nil {
		return err
	}

	return gs.Params.Validate()
}

// validateDemotions checks the demotions carried in genesis.
//
// A demotion names a validator the chain is holding down and says why. Two of
// them against one operator would mean the map they are imported into silently
// keeps one, and the one it dropped would be a restoration that never happens —
// a validator jailed by a rule nobody can point at.
func (gs GenesisState) validateDemotions(approved map[string]struct{}) error {
	seen := make(map[string]struct{}, len(gs.Demotions))

	for _, demotion := range gs.Demotions {
		if demotion.Operator == "" {
			return fmt.Errorf("demotion with no operator")
		}
		if _, dup := seen[demotion.Operator]; dup {
			return fmt.Errorf("duplicate demotion for %s", demotion.Operator)
		}
		seen[demotion.Operator] = struct{}{}

		if _, ok := approved[demotion.Operator]; !ok {
			return fmt.Errorf("%s is demoted but is not an approved validator, so nothing would ever restore it", demotion.Operator)
		}
		if demotion.Cap == CONCENTRATION_CAP_UNSPECIFIED {
			return fmt.Errorf("demotion of %s does not say which ceiling it was for", demotion.Operator)
		}
		if demotion.Group == "" {
			return fmt.Errorf("demotion of %s does not say which group breached", demotion.Operator)
		}
	}

	return nil
}

// validateRotations checks the rotations carried in genesis.
//
// The check that matters is the last one: at most one rotation may be open
// against any operator. The module keeps a single-entry index from operator to
// open rotation and rebuilds it on import, so a genesis carrying two would
// silently lose one of them — and the one it lost would be a pause that never
// lifts or a veto that never lands.
func (gs GenesisState) validateRotations() error {
	seen := make(map[uint64]struct{}, len(gs.OperatorRotations))
	open := make(map[string]uint64)

	for _, rotation := range gs.OperatorRotations {
		if rotation.Id == 0 {
			return fmt.Errorf("operator rotation with id 0; rotations are numbered from 1")
		}
		if _, dup := seen[rotation.Id]; dup {
			return fmt.Errorf("duplicate operator rotation %d", rotation.Id)
		}
		if rotation.Id >= gs.RotationCount {
			return fmt.Errorf("operator rotation %d is at or beyond rotation_count %d, so the next rotation opened would collide with it", rotation.Id, gs.RotationCount)
		}
		seen[rotation.Id] = struct{}{}

		if rotation.CurrentOperator == "" {
			return fmt.Errorf("operator rotation %d has no current operator", rotation.Id)
		}
		if rotation.NewOperator == "" {
			return fmt.Errorf("operator rotation %d has no new operator", rotation.Id)
		}
		if rotation.CurrentOperator == rotation.NewOperator {
			return fmt.Errorf("operator rotation %d rotates %s to itself", rotation.Id, rotation.CurrentOperator)
		}
		if rotation.Kind == ROTATION_KIND_UNSPECIFIED {
			return fmt.Errorf("operator rotation %d has an unspecified kind", rotation.Id)
		}
		if rotation.Status == ROTATION_STATUS_UNSPECIFIED {
			return fmt.Errorf("operator rotation %d has an unspecified status", rotation.Id)
		}
		// A planned rotation is approved by its signature, so the flag is
		// meaningless on one. Carrying it set would make the export of a
		// re-imported chain differ from what was imported.
		if rotation.Kind == ROTATION_KIND_PLANNED && rotation.Approved {
			return fmt.Errorf("operator rotation %d is planned but carries the recovery approval flag", rotation.Id)
		}
		if rotation.Status != ROTATION_STATUS_PENDING {
			continue
		}
		// A pending planned rotation always has a clock running; a pending
		// recovery has one only once it has been approved.
		if rotation.Kind == ROTATION_KIND_PLANNED && rotation.CompletesAtHeight <= 0 {
			return fmt.Errorf("operator rotation %d is a pending planned rotation with no completion height, so nothing would ever complete it", rotation.Id)
		}
		if rotation.Kind == ROTATION_KIND_RECOVERY && rotation.Approved != (rotation.CompletesAtHeight > 0) {
			return fmt.Errorf("operator rotation %d is an approved recovery with no completion height, or an unapproved one with a completion height", rotation.Id)
		}
		if other, dup := open[rotation.CurrentOperator]; dup {
			return fmt.Errorf("operator rotations %d and %d are both open against %s", other, rotation.Id, rotation.CurrentOperator)
		}
		open[rotation.CurrentOperator] = rotation.Id
	}

	return nil
}
