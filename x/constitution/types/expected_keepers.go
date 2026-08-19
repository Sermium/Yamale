package types

import (
	"context"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// StakingKeeper is how the module learns who may ratify an amendment and how
// much their agreement weighs.
//
// Ratification is weighed by consensus power, which under equal seats is a seat
// count: the same arithmetic reads correctly whether power comes from stake or
// from seats, because a seat is implemented as a fixed quantity of bonded
// tokens rather than as a parallel notion of power.
type StakingKeeper interface {
	GetLastTotalPower(ctx context.Context) (math.Int, error)
	Validator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.ValidatorI, error)
}

// AuthKeeper defines the expected interface for the Auth module.
type AuthKeeper interface {
	AddressCodec() address.Codec
}
