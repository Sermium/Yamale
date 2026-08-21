package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name.
	//
	// It is also the name of the module account that custodies every
	// participant's settlement reserve, which is why the account is granted no
	// permissions at all and is on the blocked list in app_config.go: it holds
	// other institutions' money against this module's own books, so it must
	// never be able to mint, burn, or receive a bank transfer that its
	// accounting does not know about.
	ModuleName = "netting"

	// StoreKey defines the primary module store key.
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency with x/gov.
	// It should be synced with the gov module's name if it is ever changed.
	GovModuleName = "gov"
)

// ParamsKey is the prefix to retrieve all Params.
var ParamsKey = collections.NewPrefix("p_netting")

var (
	// CycleKey holds every netting window ever opened, keyed by id. Nothing is
	// removed: a window is the record of what a set of institutions settled
	// against each other, and it is what they reconcile their own books to
	// years later.
	CycleKey = collections.NewPrefix("cycle/value/")

	// CycleSeqKey numbers windows, and CurrentCycleKey names the open one.
	//
	// CurrentCycleKey is written by InitGenesis in every case, including on a
	// fresh chain, rather than being created lazily on the first obligation. An
	// end blocker that has to cope with the singleton not existing yet is an
	// end blocker with a branch nobody tests, and the block it fails in is not
	// a failed transaction — it is a chain that stops.
	CycleSeqKey     = collections.NewPrefix("cycle/seq/")
	CurrentCycleKey = collections.NewPrefix("cycle/current/")

	// ObligationKey holds every obligation, keyed by (cycle id, obligation id),
	// and ObligationSeqKey numbers them.
	//
	// Deliberately not keyed by denom. The end blocker walks positions, not
	// obligations, so nothing in the settlement path needs obligations grouped
	// by currency — and a key component that no read uses is a key component
	// that has to be carried by every write and every index for nothing.
	ObligationKey    = collections.NewPrefix("obligation/value/")
	ObligationSeqKey = collections.NewPrefix("obligation/seq/")

	// ObligationByParticipantKey indexes obligations by (participant, cycle id,
	// obligation id), with one entry for each side, so a participant can page
	// through what it is a party to without scanning everybody else's.
	//
	// Derived from the obligations themselves and therefore absent from
	// genesis: an index is a second copy of a fact, and a second copy that can
	// be imported independently is a second copy that can disagree.
	ObligationByParticipantKey = collections.NewPrefix("obligation/by_participant/")

	// PositionKey holds the running net positions, keyed by (cycle id, denom,
	// participant).
	//
	// That key order is what the settlement pass depends on. The end blocker
	// walks this prefix in store order, which is byte order, which is identical
	// on every validator — so a currency's positions arrive contiguously and
	// can be grouped in one pass. Accumulating into a Go map instead would
	// scatter a currency across the walk, and a currency split in two is two
	// slices that each fail the zero-sum check: a day's netting held for a
	// reason that is nowhere in the data. Nothing in this module's netting path
	// may iterate a map. See collectPositions for why the resulting *order* is
	// not itself the hazard.
	PositionKey = collections.NewPrefix("position/value/")

	// ReserveKey holds what each participant has prefunded, keyed by
	// (participant, denom).
	ReserveKey = collections.NewPrefix("reserve/value/")

	// LockedKey holds the part of a reserve that is already committed to
	// positions in windows that have not settled, keyed the same way.
	//
	// Kept as state rather than recomputed on demand because it is read and
	// written on every obligation, and because the alternative — walking every
	// unsettled cycle to answer "may this participant owe more" — gets slower
	// exactly as the situation it guards against gets worse.
	//
	// Derived from the positions, and therefore rebuilt by InitGenesis rather
	// than imported. See GenesisState.
	LockedKey = collections.NewPrefix("reserve/locked/")

	// HeldSliceKey indexes the currency slices that failed to settle, keyed by
	// (cycle id, denom), so the end blocker can retry them without walking
	// every cycle ever closed. On a healthy chain it is empty.
	//
	// Derived from the cycles' own recorded outcomes, so it too is rebuilt at
	// import rather than carried in genesis.
	HeldSliceKey = collections.NewPrefix("held/value/")
)
