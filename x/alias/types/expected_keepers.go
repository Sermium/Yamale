package types

import "context"

// ParticipantKeeper is the slice of x/paymsg this module consults before it
// will let somebody record where an account is.
//
// The jurisdiction record lives here, in the module that owns account identity,
// because every module that has to refuse an out-of-perimeter action needs to
// read it and none of them should have to ask the payments module. But the
// party that *knows* the country is the approved participant that performed the
// KYC, so the write is gated on x/paymsg's own record of who banks whom. Asking
// a hand-written copy of that relationship would only prove this module can
// read a struct it wrote itself.
//
// Read-only, two questions, no types crossing the boundary. x/alias answers
// questions about identity and cannot move money; a wider interface here would
// be a capability grant dressed up as a dependency. The direction matters too:
// x/paymsg knows nothing about x/alias, so the payments module still builds and
// reasons alone.
type ParticipantKeeper interface {
	// ApprovedParticipantExists reports whether an institution is admitted
	// right now. Checked separately from the customer relationship because
	// approval can be withdrawn after customers were registered, and an
	// institution that has been thrown off the rail must not go on stamping
	// perimeters.
	ApprovedParticipantExists(ctx context.Context, participant string) (bool, error)

	// ParticipantOf reports which approved participant acts for an account.
	// Found is false rather than an error for an account that banks nowhere:
	// "nobody acts for this account" is an answer, and the caller turns it into
	// its own refusal.
	ParticipantOf(ctx context.Context, account string) (participant string, found bool, err error)
}
