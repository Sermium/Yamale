package keeper

import (
	"bytes"
	"context"

	errorsmod "cosmossdk.io/errors"

	"yamale/blockchain/x/netting/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// UpdateParams replaces the module's parameters. Governance only.
//
// Changing a currency's policy does not disturb windows already open. An
// obligation is classified by the policy in force when it was submitted, and a
// position already taken settles whatever governance decides afterwards —
// otherwise a parameter change would retroactively alter what institutions
// owe each other, which is the one thing this module never does.
func (k msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	authorityBytes, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authorityBytes) {
		expectedAuthority, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expectedAuthority, msg.Authority)
	}

	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, k.Params.Set(ctx, msg.Params)
}
