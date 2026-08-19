package keeper

import (
	"bytes"
	"context"

	"yamale/blockchain/x/stablecoin/types"

	errorsmod "cosmossdk.io/errors"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// ApproveIssuer is executed exclusively by the governance module account
// once a proposal to approve or reject a pending currency registration has
// passed. Approval registers the issuer as the sole address permitted to
// mint/burn the denom and publishes its bank denom metadata.
func (k msgServer) ApproveIssuer(ctx context.Context, msg *types.MsgApproveIssuer) (*types.MsgApproveIssuerResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authority) {
		expectedAuthorityStr, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expectedAuthorityStr, msg.Authority)
	}

	application, err := k.IssuerApplication.Get(ctx, msg.Denom)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotFound, "no application found for denom %s", msg.Denom)
	}
	if application.Status != types.StatusPending {
		return nil, errorsmod.Wrapf(types.ErrApplicationNotPending, "application for %s has status %s", msg.Denom, application.Status)
	}

	if msg.Approve {
		application.Status = types.StatusApproved
		if err := k.ApprovedIssuer.Set(ctx, msg.Denom, types.ApprovedIssuer{
			Denom:  msg.Denom,
			Issuer: application.Creator,
		}); err != nil {
			return nil, err
		}

		k.bankKeeper.SetDenomMetaData(ctx, banktypes.Metadata{
			Description: application.Description,
			Base:        application.Denom,
			Display:     application.DisplayDenom,
			Name:        application.Name,
			Symbol:      application.Symbol,
			DenomUnits: []*banktypes.DenomUnit{
				{Denom: application.Denom, Exponent: 0},
				{Denom: application.DisplayDenom, Exponent: uint32(application.Exponent)},
			},
		})
	} else {
		application.Status = types.StatusRejected
	}

	if err := k.IssuerApplication.Set(ctx, msg.Denom, application); err != nil {
		return nil, err
	}

	return &types.MsgApproveIssuerResponse{}, nil
}
