package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name
	ModuleName = "treasury"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency with x/gov.
	// It should be synced with the gov module's name if it is ever changed.
	// See: https://github.com/cosmos/cosmos-sdk/blob/v0.52.0-beta.2/x/gov/types/keys.go#L9
	GovModuleName = "gov"
)

// ParamsKey is the prefix to retrieve all Params
var ParamsKey = collections.NewPrefix("p_treasury")

var (
	TreasuryKey      = collections.NewPrefix("treasury/value/")
	TreasuryCountKey = collections.NewPrefix("treasury/count/")

	LockKey      = collections.NewPrefix("lock/value/")
	LockCountKey = collections.NewPrefix("lock/count/")

	// BalanceKey holds the module's ledger, keyed by (treasury id, denom).
	BalanceKey = collections.NewPrefix("balance/value/")

	// RoleKey holds role assignments, keyed by (treasury id, address).
	RoleKey = collections.NewPrefix("role/value/")

	// SpendPolicyKey holds spend policies, keyed by (treasury id, denom).
	SpendPolicyKey = collections.NewPrefix("policy/value/")

	// SpendWindowKey holds period consumption, keyed by (treasury id, denom).
	SpendWindowKey = collections.NewPrefix("window/value/")

	// LockByTreasuryKey indexes (treasury id, lock id) so a treasury's locks can
	// be listed without scanning every lock on the chain.
	LockByTreasuryKey = collections.NewPrefix("lock/by_treasury/")

	// LockByBeneficiaryKey indexes (beneficiary, lock id) so a beneficiary can
	// find what is owed to them without knowing any treasury id.
	LockByBeneficiaryKey = collections.NewPrefix("lock/by_beneficiary/")

	// ActiveLockCountKey tracks how many of a treasury's locks are still live.
	// Counting them by walking the index instead would grow more expensive with
	// every lock ever created, since retired locks stay indexed for the audit
	// trail.
	ActiveLockCountKey = collections.NewPrefix("lock/active_count/")
)
