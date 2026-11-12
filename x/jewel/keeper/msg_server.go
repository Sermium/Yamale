package keeper

import (
	"bytes"
	"context"

	"github.com/cosmos/cosmos-sdk/x/group"

	"yamale/blockchain/x/jewel/types"
)

type msgServer struct {
	Keeper
	// Embedded until every handler exists; removed in Task 9.
	types.UnimplementedMsgServer
}

// NewMsgServerImpl returns the register's Msg service.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = (*msgServer)(nil)

// UpdateParams sets the settings and appoints the office. Governance only.
func (k msgServer) UpdateParams(ctx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	authority, err := k.addressCodec.StringToBytes(req.Authority)
	if err != nil || !bytes.Equal(k.GetAuthority(), authority) {
		return nil, types.ErrInvalidSigner.Wrapf("%s", req.Authority)
	}
	if err := req.Params.Validate(); err != nil {
		return nil, err
	}
	params := req.Params
	if params.Office != "" {
		office, err := k.canonical(params.Office)
		if err != nil {
			return nil, types.ErrInvalidAddress.Wrapf("office %s", params.Office)
		}
		params.Office = office
		if k.groupKeeper != nil {
			if _, err := k.groupKeeper.GroupPolicyInfo(ctx,
				&group.QueryGroupPolicyInfoRequest{Address: office}); err != nil {
				return nil, types.ErrOfficeNotGroup.Wrapf("%s", office)
			}
		}
	}
	return &types.MsgUpdateParamsResponse{}, k.Params.Set(ctx, params)
}
