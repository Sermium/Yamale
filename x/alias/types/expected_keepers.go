package types

import (
	"context"

	"github.com/cosmos/cosmos-sdk/x/group"

	constitutiontypes "yamale/blockchain/x/constitution/types"
)

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

// GroupKeeper is the slice of x/group this module needs to grant a role and to
// keep holding an office to the shape it was granted under.
//
// It used to be one question, asked once, at the moment a role was granted: *is
// this address a group policy?* That is what turns "the central bank signed" into
// "several people inside the central bank signed", and it is the difference
// between a perimeter where one bribe moves an authority and one where it does
// not.
//
// It was not enough, and the reason is worth stating rather than discovering. The
// question has a yes for a one-of-one group, so multisig was guaranteed in form
// and not in substance; and it was asked once, at grant time, while an office is
// a group that administers itself and can therefore vote to change its own
// threshold afterwards. A country could hold a proper ceremony, stand up a
// three-of-five enforcement authority, and that office could later reduce itself
// to one-of-one while keeping the power to freeze accounts, with nothing notified
// and nothing refused.
//
// So there are three questions now, and the second and third are asked on every
// action a grant with a recorded shape permits:
//
//   - is this address a group policy, and what is its decision policy? That is
//     where the threshold lives.
//   - who are the group's members, and what weight does each hold? That is where
//     the count lives, and it is also the only way to tell a three-of-five from a
//     one-of-five whose first member weighs three.
//
// Read-only, and still narrow: no message this module sends can change a group,
// and nothing here lets it. The same interface x/land keeps for admitting a
// registry office, widened by the two queries the shape check needs.
type GroupKeeper interface {
	GroupPolicyInfo(
		ctx context.Context, req *group.QueryGroupPolicyInfoRequest,
	) (*group.QueryGroupPolicyInfoResponse, error)

	// GroupMembers lists a group's members and their weights.
	//
	// Paged, and this module always passes a limit — see types.MaxOfficeMembers
	// for why a page boundary has to be an explicit refusal rather than a count
	// that happens to be short.
	GroupMembers(
		ctx context.Context, req *group.QueryGroupMembersRequest,
	) (*group.QueryGroupMembersResponse, error)
}

// ConstitutionKeeper is the slice of x/constitution this module needs in order
// to know who the foundation is.
//
// One question, and it is deliberately not "is this account privileged". The
// module asks for the settlement in force and reads one field of it: the address
// every seized asset on the chain is sent to. That address is the foundation,
// and identifying it this way rather than by a list of its own is the whole
// point. x/constitution does not let governance edit an invariant at all —
// changing one is a constitutional amendment, weeks in public with a four-fifths
// ratification by the validator set. A parameter list naming "the foundation"
// would be a list one ordinary proposal could append to, and appending to it
// would be appending to the set of accounts that may admit a country.
//
// So the account that may admit a country is the same account the constitution
// protects, and it cannot be widened without amending the constitution.
//
// The dependency runs one way and moves no money. x/constitution imports nothing
// from this repository — its ModuleInputs takes x/staking and nothing else — so
// there is no cycle to wire around, and its InitGenesis runs before this
// module's, so the invariants are in the store before any message reaches here.
type ConstitutionKeeper interface {
	// GetInvariants returns the settlement in force, or an error when nothing has
	// been written.
	//
	// The error is load-bearing. A chain with no constitution must not be one
	// where the foundation's address reads as the empty string, because an unset
	// signer would then compare equal to it — which is the proto3 zero-value trap
	// this repository has been caught by four times.
	GetInvariants(ctx context.Context) (constitutiontypes.Invariants, error)
}
