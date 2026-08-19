package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name.
	ModuleName = "alias"
	// StoreKey defines the primary module store key.
	StoreKey = ModuleName
	// GovModuleName is the governance module's account name.
	GovModuleName = "gov"
)

// Store prefixes. Distinct, stable, and never reused — old data does not
// disappear when a prefix is retired, so a reused byte reads somebody else's
// records as its own.
var (
	ParamsKey        = collections.NewPrefix(0)
	AliasesKey       = collections.NewPrefix(1)
	OwnersKey        = collections.NewPrefix(2)
	RetiredKey       = collections.NewPrefix(3)
	JurisdictionsKey = collections.NewPrefix(4)
	// PerimeterKey indexes (country, address). Derived from JurisdictionsKey and
	// rebuilt by InitGenesis rather than exported, for the same reason Owners is.
	PerimeterKey = collections.NewPrefix(5)
)
