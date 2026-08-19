package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// Validator application lifecycle states.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

// RotationGrantedMsgs are the messages a completed rotation authorises the
// incoming operator to send on the outgoing one's behalf.
//
// The list is short on purpose, and what is missing from it matters more than
// what is on it. MsgUndelegate, MsgBeginRedelegate and MsgSetWithdrawAddress
// are all deliberately absent: each would let the incoming operator move the
// outgoing one's money, and rotation moves who signs for the validator, not
// who owns what is staked behind it. An operator whose key is genuinely lost
// loses their self-delegation with it — rotation is not a way to recover
// somebody's funds, and a list that let it be one would make a false recovery
// claim worth filing.
var RotationGrantedMsgs = []string{
	sdk.MsgTypeURL(&stakingtypes.MsgEditValidator{}),
	sdk.MsgTypeURL(&slashingtypes.MsgUnjail{}),
	sdk.MsgTypeURL(&distrtypes.MsgWithdrawValidatorCommission{}),
}
