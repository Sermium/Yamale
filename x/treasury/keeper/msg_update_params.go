package keeper

import (
	"bytes"
	"context"

	errorsmod "cosmossdk.io/errors"

	"yamale/blockchain/x/treasury/types"
)

// UpdateParams updates the module parameters. Only the gov module account may
// call it.
func (k msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authority) {
		expected, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expected, msg.Authority)
	}

	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, k.Params.Set(ctx, msg.Params)
}
