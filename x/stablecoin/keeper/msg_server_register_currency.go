package keeper

import (
	"context"

	"yamale/blockchain/x/stablecoin/types"

	errorsmod "cosmossdk.io/errors"
)

// RegisterCurrency lets anyone propose a new mock fiat-pegged currency. It
// records a pending application; the applicant only becomes the currency's
// issuer once a governance proposal approves it via MsgApproveIssuer.
func (k msgServer) RegisterCurrency(ctx context.Context, msg *types.MsgRegisterCurrency) (*types.MsgRegisterCurrencyResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	// Checked before anything is read or written: the denom becomes a store key
	// and every other field is permanent state chosen by whoever applied.
	if err := types.ValidateCurrency(
		msg.Denom, msg.DisplayDenom, msg.Name, msg.Symbol, msg.Description, msg.Exponent,
	); err != nil {
		return nil, err
	}

	if has, err := k.IssuerApplication.Has(ctx, msg.Denom); err != nil {
		return nil, err
	} else if has {
		return nil, errorsmod.Wrapf(types.ErrCurrencyExists, "an application for %s already exists", msg.Denom)
	}
	if has, err := k.ApprovedIssuer.Has(ctx, msg.Denom); err != nil {
		return nil, err
	} else if has {
		return nil, errorsmod.Wrapf(types.ErrCurrencyExists, "%s is already an approved currency", msg.Denom)
	}

	if err := k.IssuerApplication.Set(ctx, msg.Denom, types.IssuerApplication{
		Denom:        msg.Denom,
		Status:       types.StatusPending,
		Creator:      msg.Creator,
		DisplayDenom: msg.DisplayDenom,
		Exponent:     msg.Exponent,
		Name:         msg.Name,
		Symbol:       msg.Symbol,
		Description:  msg.Description,
	}); err != nil {
		return nil, err
	}

	return &types.MsgRegisterCurrencyResponse{}, nil
}
