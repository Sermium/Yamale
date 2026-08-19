package types

import (
	"context"

	"github.com/cosmos/cosmos-sdk/x/group"
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
