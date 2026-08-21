package types

// DONTCOVER

import (
	"cosmossdk.io/errors"
)

// x/netting module sentinel errors
var (
	ErrInvalidSigner = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")

	// ErrNotApprovedParticipant is raised for either side of an obligation. An
	// institution that is not on the rail cannot owe on it and cannot be owed
	// on it, because there would be nobody the chain could hold to either half.
	ErrNotApprovedParticipant = errors.Register(ModuleName, 1101, "not an approved participant")

	ErrInvalidAmount    = errors.Register(ModuleName, 1102, "invalid amount")
	ErrInvalidBatchHash = errors.Register(ModuleName, 1103, "batch_hash must be 32 bytes of SHA-256 over the salted batch")
	ErrSelfObligation   = errors.Register(ModuleName, 1104, "a participant cannot owe itself")

	// ErrNetDebitCapExceeded is the module's central refusal, and the reason
	// settlement cannot fail later. An obligation that would take a
	// participant's net debit beyond what it has prefunded is rejected here,
	// synchronously, with a normal transaction error — rather than accepted and
	// discovered to be unfundable in an end blocker, where there is no
	// transaction to fail and no counterparty left to warn.
	ErrNetDebitCapExceeded = errors.Register(ModuleName, 1105, "obligation would take the participant's net debit beyond its posted reserve")

	// ErrReserveCommitted refuses a withdrawal of collateral that is already
	// backing an unsettled position. Without it, a participant could submit its
	// obligations and then withdraw the money behind them in the same block.
	ErrReserveCommitted = errors.Register(ModuleName, 1106, "reserve is committed to positions that have not settled")

	ErrInsufficientReserve = errors.Register(ModuleName, 1107, "insufficient reserve")
	ErrCycleNotFound       = errors.Register(ModuleName, 1108, "no such cycle")

	// ErrPositionsUnbalanced is the invariant that says netting neither created
	// nor destroyed value. Net positions in a currency must sum to zero; if
	// they do not, the module's own books disagree with themselves and the
	// correct response is to settle nothing rather than to settle the part that
	// looks consistent.
	ErrPositionsUnbalanced = errors.Register(ModuleName, 1109, "net positions in a currency do not sum to zero")

	// ErrNettingDisabled is distinct from a missing denom policy: "this chain
	// does no netting" and "this currency has not been enabled for netting"
	// send an operator to two different places.
	ErrNettingDisabled = errors.Register(ModuleName, 1110, "netting is disabled")
)
