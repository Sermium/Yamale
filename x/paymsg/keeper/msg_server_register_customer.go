package keeper

import (
	"context"
	"errors"

	"yamale/blockchain/x/paymsg/types"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
)

// RegisterCustomer records or removes the relationship between an approved
// participant and an account it acts for.
//
// Only the participant may claim an account, and only an approved one — an
// institution that has not been admitted cannot accumulate customers ahead of a
// governance decision that may never come.
func (k msgServer) RegisterCustomer(ctx context.Context, msg *types.MsgRegisterCustomer) (*types.MsgRegisterCustomerResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Participant); err != nil {
		return nil, errorsmod.Wrap(err, "invalid participant address")
	}
	if _, err := k.addressCodec.StringToBytes(msg.Customer); err != nil {
		return nil, errorsmod.Wrap(err, "invalid customer address")
	}

	if has, err := k.ApprovedParticipant.Has(ctx, msg.Participant); err != nil {
		return nil, err
	} else if !has {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedParticipant, "%s is not an approved participant", msg.Participant)
	}

	existing, err := k.Customer.Get(ctx, msg.Customer)
	switch {
	case err == nil:
		// One participant per customer, so a second institution cannot claim an
		// account already banking elsewhere. Otherwise a participant could
		// attach itself to somebody else's customer and name itself on their
		// payments, which is the impersonation this record exists to prevent.
		if existing.Participant != msg.Participant {
			return nil, errorsmod.Wrapf(types.ErrNotACustomer,
				"%s already banks with %s", msg.Customer, existing.Participant)
		}
	case !errors.Is(err, collections.ErrNotFound):
		return nil, err
	}

	if !msg.Registered {
		// Removing a relationship that does not exist is not an error; the
		// caller wanted the account not to be their customer, and it is not.
		return &types.MsgRegisterCustomerResponse{}, k.Customer.Remove(ctx, msg.Customer)
	}

	return &types.MsgRegisterCustomerResponse{}, k.Customer.Set(ctx, msg.Customer, types.Customer{
		Customer:    msg.Customer,
		Participant: msg.Participant,
	})
}
