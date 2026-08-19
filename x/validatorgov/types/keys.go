package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name
	ModuleName = "validatorgov"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency with x/gov.
	// It should be synced with the gov module's name if it is ever changed.
	// See: https://github.com/cosmos/cosmos-sdk/blob/v0.52.0-beta.2/x/gov/types/keys.go#L9
	GovModuleName = "gov"
)

// ParamsKey is the prefix to retrieve all Params
var ParamsKey = collections.NewPrefix("p_validatorgov")

var (
	// RotationKey holds every rotation ever opened, keyed by id. Nothing is
	// ever removed: a recovery that was vetoed is the record of somebody having
	// claimed a key was lost when it was not, and deleting it would leave only
	// the rotations that succeeded.
	RotationKey = collections.NewPrefix("rotation/value/")

	// RotationSeqKey numbers rotations.
	RotationSeqKey = collections.NewPrefix("rotation/seq/")

	// PendingRotationKey maps an operator address to the one open rotation
	// against it.
	//
	// It exists for the ante decorator, which asks "is a recovery open against
	// this signer" for every signer of every transaction the chain processes.
	// Answering that by scanning rotations would make every transaction on the
	// chain pay for the module's entire history.
	PendingRotationKey = collections.NewPrefix("rotation/pending/")

	// RotationQueueKey indexes rotations that are counting down by (completion
	// height, rotation id), so the end blocker's cost depends on what falls due
	// now rather than on how many rotations have ever happened.
	RotationQueueKey = collections.NewPrefix("rotation/queue/")

	// DemotionKey holds the validators the epoch check currently holds down,
	// keyed by operator address.
	//
	// Only the ones in force. A demotion that has been restored is deleted
	// rather than kept with a status, because this map is read twice every
	// epoch — once to decide what to restore and once to decide what to demote
	// — and a growing history behind a scan would make the check cost more
	// every year. The record that a demotion happened lives in the events,
	// which is where an auditor looks for a history anyway.
	DemotionKey = collections.NewPrefix("demotion/value/")
)
