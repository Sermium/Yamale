package types

import (
	"context"

	"cosmossdk.io/x/nft"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/group"
)

// GroupKeeper answers one question, at appointment: is the office a group
// policy? An office that is one key is one bribe away from admitting a
// forger under a real jeweler's name.
type GroupKeeper interface {
	GroupPolicyInfo(
		ctx context.Context, req *group.QueryGroupPolicyInfoRequest,
	) (*group.QueryGroupPolicyInfoResponse, error)
}

// NFTKeeper is the slice of x/nft the register drives. x/nft holds the
// tokens and is the only record of who owns them.
type NFTKeeper interface {
	SaveClass(ctx context.Context, class nft.Class) error
	Mint(ctx context.Context, token nft.NFT, receiver sdk.AccAddress) error
	Update(ctx context.Context, token nft.NFT) error
	Transfer(ctx context.Context, classID, nftID string, receiver sdk.AccAddress) error
	GetOwner(ctx context.Context, classID, nftID string) sdk.AccAddress
	// GetNFTsOfClassByOwner lists one owner's tokens of a class, from x/nft's
	// owner index: a jeweler's stock is its business account's tokens of its class.
	GetNFTsOfClassByOwner(ctx context.Context, classID string, owner sdk.AccAddress) []nft.NFT
}

// CircuitKeeper says whether a message type may still run.
type CircuitKeeper interface {
	IsAllowed(ctx context.Context, typeURL string) (bool, error)
}
