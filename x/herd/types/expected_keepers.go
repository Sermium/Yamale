package types

import (
	"context"

	"github.com/cosmos/cosmos-sdk/x/group"
)

// GroupKeeper is the slice of x/group the registry needs.
//
// One question is asked of it: is this address a group policy? That is what
// turns "the office signed" into "several people inside the office signed",
// and it is the difference between a registry where one bribe strikes an
// animal off and one where it does not.
//
// The same reasoning as x/land, and deliberately the same shape. An office
// that enrols herds, certifies parentage and validates transfers is at least
// as attackable as one that registers title, because the animal can be moved
// and the parcel cannot.
type GroupKeeper interface {
	GroupPolicyInfo(
		ctx context.Context, req *group.QueryGroupPolicyInfoRequest,
	) (*group.QueryGroupPolicyInfoResponse, error)
}
