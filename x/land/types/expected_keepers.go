package types

import (
	"context"

	"github.com/cosmos/cosmos-sdk/x/group"

	aliastypes "yamale/blockchain/x/alias/types"
)

// GroupKeeper is the slice of x/group the registry needs.
//
// Only one question is asked of it: *is this address a group policy?* That is
// what turns "the office signed" into "several people inside the office
// signed", and it is the difference between a registry where one bribe moves
// land and one where it does not.
//
// The alternative — trusting governance to only ever admit group addresses —
// puts the entire intra-office protection in a human review step that will
// eventually be rushed. Checking costs one lookup at admission time.
type GroupKeeper interface {
	GroupPolicyInfo(
		ctx context.Context, req *group.QueryGroupPolicyInfoRequest,
	) (*group.QueryGroupPolicyInfoResponse, error)
}

// ScopeKeeper is the jurisdictional perimeter, and one method of it.
//
// A registry office acts on land inside the jurisdiction it administers, and the
// jurisdiction it administers is a field on its own admission record — so this
// module always has the country in hand and never needs the shape of the check
// that looks a country up from an account. One method, because the second would
// be a capability this module has no use for.
//
// Note that the office's *own* declared jurisdiction is what gets checked, not
// the holder's. Land sits where it sits: a Ghanaian parcel held by a Nigerian is
// Ghana's to administer, and scoping the check to the holder's country would
// hand a parcel to whichever authority its owner happened to bank with.
//
// The dependency runs one way. x/alias must never be given this module's keeper:
// x/land is excluded from the settlement profile and x/alias is in every profile,
// so an edge in the other direction would drag the cadastre into a settlement
// binary that scope §3 keeps it out of.
type ScopeKeeper interface {
	// AssertScopeIn returns nil only when the actor holds a grant of the role
	// covering the named jurisdiction. Every other outcome — no grant, a grant
	// naming another country, an unset role, a jurisdiction that is not a country
	// — is an error, and there is no argument that makes it permissive.
	AssertScopeIn(ctx context.Context, actor string, role aliastypes.Role, jurisdiction string) error
}
