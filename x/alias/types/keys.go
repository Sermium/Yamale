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
	// ViewingKeysKey holds (address, version) -> ViewingKey. Every version is
	// kept, never overwritten: an envelope sealed last year names the version
	// that was live last year, and a store that replaced the record on rotation
	// would turn a readable payload into an unopenable one at the moment of an
	// operational act nobody connects to it.
	ViewingKeysKey = collections.NewPrefix(6)
	// RegulatorsKey holds country -> RegulatorAppointment, one per country.
	RegulatorsKey = collections.NewPrefix(7)
	// AuditorGrantsKey holds address -> AuditorGrant, expired ones included.
	AuditorGrantsKey = collections.NewPrefix(8)
	// RoleGrantsKey holds (holder, role, jurisdiction) -> RoleGrant.
	//
	// The jurisdiction is part of the key rather than a field inside one record
	// per (holder, role), because a holder may legitimately hold the same role in
	// several countries and a single record would have to carry a list — which is
	// a set that can be appended to by a message that meant to replace it.
	RoleGrantsKey = collections.NewPrefix(9)
	// GrantsByScopeKey indexes (jurisdiction, role, holder). Derived from
	// RoleGrantsKey and rebuilt by InitGenesis rather than exported, for the same
	// reason Owners and Perimeter are.
	//
	// It exists because the two questions an operator asks are opposite
	// directions of the same fact: "what may this key do" and "who may act on my
	// country". Answering the second by walking every grant on the chain is a
	// query whose cost is the whole chain, which is a query an operator learns
	// not to run — and the chain-wide exceptions are listed off the same index,
	// so the exception list cannot disagree with the grants it summarises.
	GrantsByScopeKey = collections.NewPrefix(10)
)
