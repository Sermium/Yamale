package types

import "fmt"

// Default bounds. Each exists because some path walks or stores the thing it
// bounds, and leaving any of them open would let one treasury make work for
// everybody else.
const (
	DefaultMaxLocksPerTreasury           = 500
	DefaultMaxRoleAssignmentsPerTreasury = 100
	DefaultMinLockSeconds                = 60
	DefaultMaxSpendPolicyAddresses       = 200
)

// NewParams creates a new Params instance.
func NewParams(maxLocks, maxRoles, minLockSeconds, maxPolicyAddresses uint64) Params {
	return Params{
		MaxLocksPerTreasury:           maxLocks,
		MaxRoleAssignmentsPerTreasury: maxRoles,
		MinLockSeconds:                minLockSeconds,
		MaxSpendPolicyAddresses:       maxPolicyAddresses,
	}
}

// DefaultParams returns a default set of parameters.
//
// Fee routing is off. There is no address or treasury id that is the right
// default on somebody else's network, and a chain that routed its fees
// somewhere by default would be routing them somewhere nobody chose.
func DefaultParams() Params {
	return NewParams(
		DefaultMaxLocksPerTreasury,
		DefaultMaxRoleAssignmentsPerTreasury,
		DefaultMinLockSeconds,
		DefaultMaxSpendPolicyAddresses,
	)
}

// Validate validates the set of params.
func (p Params) Validate() error {
	if p.MaxLocksPerTreasury == 0 {
		return fmt.Errorf("max_locks_per_treasury must be positive")
	}
	if p.MaxRoleAssignmentsPerTreasury == 0 {
		return fmt.Errorf("max_role_assignments_per_treasury must be positive")
	}
	if p.MinLockSeconds == 0 {
		return fmt.Errorf("min_lock_seconds must be positive")
	}
	if p.MaxSpendPolicyAddresses == 0 {
		return fmt.Errorf("max_spend_policy_addresses must be positive")
	}
	return nil
}
