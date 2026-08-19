package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name
	ModuleName = "oracle"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency with x/gov.
	// It should be synced with the gov module's name if it is ever changed.
	GovModuleName = "gov"
)

// ParamsKey is the prefix to retrieve all Params
var ParamsKey = collections.NewPrefix("p_oracle")

var (
	// ExchangeRateKey holds the agreed rate per denom.
	ExchangeRateKey = collections.NewPrefix("rate/value/")

	// VoteKey holds the current round's votes, keyed by (validator, denom).
	// Cleared at the end of every round: a vote describes a moment, and
	// carrying it into the next round would agree a price on evidence nobody
	// still stands behind.
	VoteKey = collections.NewPrefix("vote/value/")

	// FeederKey maps a validator to the hot key allowed to vote for it.
	FeederKey = collections.NewPrefix("feeder/value/")

	// MissCounterKey records reporting reliability per validator.
	MissCounterKey = collections.NewPrefix("miss/value/")

	// AppraiserKey holds valuers and applicants, keyed by address.
	AppraiserKey = collections.NewPrefix("appraiser/value/")

	// AppraisalKey holds the current valuation, keyed by (class id, nft id).
	AppraisalKey = collections.NewPrefix("appraisal/value/")

	// AppraisalHistoryKey retains superseded valuations, keyed by
	// (class id, nft id, sequence). Old valuations are kept rather than
	// overwritten because the record of what an asset was said to be worth,
	// and by whom, is what an auditor needs after a dispute.
	AppraisalHistoryKey = collections.NewPrefix("appraisal/history/")

	// AppraisalSeqKey numbers history entries per asset.
	AppraisalSeqKey = collections.NewPrefix("appraisal/seq/")
)
