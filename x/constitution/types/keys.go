package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name
	ModuleName = "constitution"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency with x/gov.
	// It should be synced with the gov module's name if it is ever changed.
	GovModuleName = "gov"
)

var (
	// InvariantsKey holds the settlement in force. A single item, not a map:
	// there is one constitution, and a module able to hold two versions of it
	// at once would need a rule for which one x/enforcement is checked against.
	InvariantsKey = collections.NewPrefix("invariants")

	// AmendmentKey holds every amendment ever opened, keyed by id. Nothing is
	// ever removed: an amendment that lapsed is the record of somebody having
	// tried to move a ceiling, and a history filtered down to the ones that
	// succeeded would hide exactly the pattern worth noticing.
	AmendmentKey = collections.NewPrefix("amendment/value/")

	// AmendmentSeqKey numbers amendments.
	AmendmentSeqKey = collections.NewPrefix("amendment/seq/")

	// RatificationKey holds who agreed to what, keyed by (amendment id,
	// validator). Keyed by both so that a second ratification from the same
	// validator is a lookup rather than a scan — the tally is a running total,
	// so a double count would not be caught by recomputing it.
	RatificationKey = collections.NewPrefix("ratification/value/")

	// AmendmentQueueKey indexes pending amendments by (effective height, id),
	// so the end blocker pays for what falls due now rather than for every
	// amendment there has ever been.
	AmendmentQueueKey = collections.NewPrefix("amendment/queue/")
)
