package types

import "cosmossdk.io/collections"

const (
	// ModuleName is the module name.
	ModuleName = "selfcustody"
	// StoreKey is the module's store key.
	StoreKey = ModuleName
	// GovModuleName is the authority that sets the delay and the warning.
	GovModuleName = "gov"
)

var (
	// ParamsKey is the module parameters.
	ParamsKey = collections.NewPrefix("params")

	// DisclosureKey holds the permanent records, keyed by account. Never
	// pruned: a disclosure is the date the platform's obligations ended, which
	// is the operator's defence and the holder's receipt.
	DisclosureKey = collections.NewPrefix("disclosure")

	// RequestKey holds requests in flight. Removed on confirm or cancel, so
	// this collection is only ever the live ones.
	//
	// "idx_" is not needed here — neither prefix is a byte-prefix of the other
	// — but the rule that caught x/herd is worth restating: collections refuses
	// prefixes where one contains the other, and it refuses at construction,
	// which is the right place to find out.
	RequestKey = collections.NewPrefix("request")
)
