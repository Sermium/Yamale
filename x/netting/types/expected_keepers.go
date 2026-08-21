package types

import (
	"context"

	"cosmossdk.io/core/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// AuthKeeper is how the module finds its own account address, which is where
// every participant's reserve is custodied.
type AuthKeeper interface {
	AddressCodec() address.Codec
	GetModuleAddress(name string) sdk.AccAddress
	GetAccount(context.Context, sdk.AccAddress) sdk.AccountI // only used for simulation
}

// BankKeeper is what moves the money.
//
// Note what is not here: nothing that mints or burns. This module never
// changes the supply of anything. It takes custody of what participants
// prefund, rearranges the claims on that custody when a window closes, and
// hands it back on request — so a bug in the netting arithmetic can misallocate
// value between institutions but can never create it, and total supply stays
// an arithmetic fact anybody can check.
type BankKeeper interface {
	SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SpendableCoins(ctx context.Context, addr sdk.AccAddress) sdk.Coins
}

// ParticipantKeeper is the slice of x/paymsg this module consults before it
// will record a debt between two institutions.
//
// One question, read-only, and the dependency runs one way: x/paymsg knows
// nothing about this module and still builds and reasons alone. The reason to
// ask it rather than to keep a list here is that admission is withdrawable —
// approval is a state, not an event — so the question has to be asked at the
// moment of the act. An institution thrown off the rail must stop being able
// to add to what it owes, and must stop being nameable as a creditor by
// somebody trying to strand value on it.
//
// Obligations already accepted are unaffected by a later withdrawal, and that
// is deliberate: they were funded when they were accepted, and cancelling them
// because their counterparty's licence lapsed is exactly the retroactive
// rewriting this module refuses to do anywhere else.
type ParticipantKeeper interface {
	// ApprovedParticipantExists reports whether an institution is admitted
	// right now.
	ApprovedParticipantExists(ctx context.Context, participant string) (bool, error)
}
