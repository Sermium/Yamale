package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name.
	ModuleName = "custody"
	// StoreKey defines the primary module store key.
	StoreKey = ModuleName
	// GovModuleName is the governance module's account name.
	GovModuleName = "gov"
)

// Store prefixes. Distinct, stable, never reused — retired data does not
// disappear, so a reused byte reads somebody else's records as its own.
var (
	ParamsKey        = collections.NewPrefix(0)
	AssetsKey        = collections.NewPrefix(1)
	AttestorsKey     = collections.NewPrefix(2)
	DepositsKey      = collections.NewPrefix(3)
	AttestationsKey  = collections.NewPrefix(4)
	RedemptionsKey   = collections.NewPrefix(5)
	ReservesKey      = collections.NewPrefix(6)
	DepositSeqKey    = collections.NewPrefix(7)
	RedemptionSeqKey = collections.NewPrefix(8)
	ExternalRefsKey  = collections.NewPrefix(9)
)
