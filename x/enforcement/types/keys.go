package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name
	ModuleName = "enforcement"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency with x/gov.
	// It should be synced with the gov module's name if it is ever changed.
	GovModuleName = "gov"
)

// ParamsKey is the prefix to retrieve all Params
var ParamsKey = collections.NewPrefix("p_enforcement")

var (
	// CaseKey holds every case ever opened, keyed by id. Nothing is ever
	// removed: a case that was withdrawn or rejected is part of the record of
	// how this power has been used, and deleting it would leave only the
	// accusations that succeeded.
	CaseKey = collections.NewPrefix("case/value/")

	// CaseSeqKey numbers cases.
	CaseSeqKey = collections.NewPrefix("case/seq/")

	// VoteKey holds validators' votes, keyed by (case id, validator).
	VoteKey = collections.NewPrefix("vote/value/")

	// FreezeKey holds the frozen addresses, keyed by address.
	//
	// This is the hottest key in the module by a wide margin: it is read on
	// every transfer the chain processes, through the bank send restriction.
	FreezeKey = collections.NewPrefix("freeze/value/")

	// VotingQueueKey indexes the open cases by the height their vote ends,
	// keyed by (end height, case id). The end blocker walks this rather than
	// scanning every case ever opened, so a chain with a long history does not
	// pay for it every block.
	VotingQueueKey = collections.NewPrefix("queue/voting/")

	// FreezeExpiryQueueKey indexes provisional freezes by the height they
	// lapse, keyed by (expiry height, address). Same reasoning.
	FreezeExpiryQueueKey = collections.NewPrefix("queue/freeze/")

	// RecoveredKey holds the running total of everything seized, so the
	// question "how much has this power actually taken" is answerable without
	// replaying every case.
	RecoveredKey = collections.NewPrefix("recovered/total/")

	// CasesPassedKey counts the cases that passed, for the same reason.
	CasesPassedKey = collections.NewPrefix("recovered/passed/")
)
