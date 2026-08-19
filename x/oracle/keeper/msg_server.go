package keeper

import (
	"bytes"
	"context"

	errorsmod "cosmossdk.io/errors"

	"yamale/blockchain/x/oracle/types"
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

// UpdateParams updates the module parameters. Only the gov module account may
// call it.
func (k msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := k.assertAuthority(msg.Authority); err != nil {
		return nil, err
	}

	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, k.Params.Set(ctx, msg.Params)
}

// assertAuthority checks a message signed by governance.
func (k Keeper) assertAuthority(signer string) error {
	addr, err := k.addressCodec.StringToBytes(signer)
	if err != nil {
		return errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), addr) {
		expected, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expected, signer)
	}
	return nil
}
