package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper is the narrowest surface this module needs.
//
// It mints and burns the shareholding it issued, moves income into and out of
// vaults it owns, and reads balances to settle the index. It has no business
// touching anything else, and a wider interface here would be a capability
// grant dressed up as a dependency.
type BankKeeper interface {
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipient sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, sender sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	GetSupply(ctx context.Context, denom string) sdk.Coin
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}

// LandKeeper is the land registry surface this module consults before it will
// open a vehicle over a parcel.
//
// Read-only, and deliberately so. x/tokenisation asks the registry questions
// and never answers any: a vehicle that could write to the registry could
// record a fractionalisation the registry never approved, which is the exact
// bypass this whole bridge exists to close.
//
// The results are plain structs owned by this package rather than x/land's own
// records, so the dependency stays one-way. The registry must be able to run
// with no tokenisation module compiled in at all — land law does not depend on
// anybody having built a vehicle over it, and an import cycle here would mean
// neither module could be reasoned about alone.
type LandKeeper interface {
	// LandParcel reports title and any standing bar on selling the parcel in
	// shares. Exists is false rather than an error for an unknown parcel: "no
	// such parcel" is an answer, and callers turn it into their own error.
	LandParcel(ctx context.Context, parcelID uint64) (LandParcel, error)

	// LandAuthorisation reports the registry's standing permission to
	// fractionalise the parcel.
	LandAuthorisation(ctx context.Context, parcelID uint64) (LandAuthorisation, error)
}

// LandParcel is what this module needs to know about a piece of ground.
type LandParcel struct {
	Exists bool

	// The account the registry currently records as holding title. Title lives
	// in x/land and never moves here, so this is the only thing that says who
	// is entitled to sell rights over the parcel.
	Holder string

	// True when an unlifted restriction bars fractionalisation. It outranks the
	// registry's own authorisation, because a restriction imposed after a
	// permission was granted has to be able to stop issuance — otherwise a
	// standing authorisation is a way to sell around a limit the office set
	// yesterday.
	ForbidsFractionalisation bool
}

// LandAuthorisation is the registry's permission to sell a parcel in shares.
//
// Withdrawal and expiry are reported separately rather than collapsed into one
// "live" flag so the refusal can say which of them happened. An issuer told
// only "not permitted" cannot tell a lapsed permission they should renew from
// one the office deliberately took away.
type LandAuthorisation struct {
	Granted   bool
	Withdrawn bool

	// Unix seconds. The authorisation stops being live at this instant.
	ExpiresAt int64

	// Ceiling on the fraction that may be issued, in basis points. It caps the
	// share the tokens carry, not the share the sponsor retains.
	MaxShareBps uint32
}
