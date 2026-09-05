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

	// A claim, not yet a relationship.
	//
	// Only the participant signs this, so on its own it is one institution
	// asserting something about somebody else's account. Left as the whole of
	// the record it let an approved institution attach itself to any address on
	// the chain, be named as the instructing participant on that account's
	// payments, and — because one participant per customer is enforced above —
	// lock the account out of banking anywhere else, with only the claimant
	// able to release it.
	//
	// The account's own signature is what makes it a relationship. Re-claiming
	// does not clear an existing confirmation: a participant should not be able
	// to un-confirm its way out of a customer's decision.
	confirmed := false
	if err == nil {
		confirmed = existing.Confirmed
	}
	return &types.MsgRegisterCustomerResponse{}, k.Customer.Set(ctx, msg.Customer, types.Customer{
		Customer:    msg.Customer,
		Participant: msg.Participant,
		Confirmed:   confirmed,
	})
}

// ConfirmParticipant is the account's own word on who banks it.
//
// Signed by the customer, and it is the only message in this module that is.
// Confirming turns a participant's claim into a relationship a payment may rely
// on; refusing removes the record entirely, which is how an account that was
// claimed without being asked gets out.
func (k msgServer) ConfirmParticipant(ctx context.Context, msg *types.MsgConfirmParticipant) (*types.MsgConfirmParticipantResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Customer); err != nil {
		return nil, errorsmod.Wrap(err, "invalid customer address")
	}

	existing, err := k.Customer.Get(ctx, msg.Customer)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			// Leaving a relationship that does not exist is not an error; the
			// account wanted not to bank there, and it does not.
			if !msg.Confirm {
				return &types.MsgConfirmParticipantResponse{}, nil
			}
			return nil, errorsmod.Wrapf(types.ErrNotACustomer,
				"no participant has claimed %s, so there is nothing to confirm", msg.Customer)
		}
		return nil, err
	}

	// The participant is named on the message so that a confirmation cannot be
	// replayed against a claim the account never read — a claim withdrawn and
	// replaced by a different institution's must be confirmed again.
	if existing.Participant != msg.Participant {
		return nil, errorsmod.Wrapf(types.ErrNotACustomer,
			"%s is claimed by %s, not by %s", msg.Customer, existing.Participant, msg.Participant)
	}

	if !msg.Confirm {
		return &types.MsgConfirmParticipantResponse{}, k.Customer.Remove(ctx, msg.Customer)
	}

	existing.Confirmed = true
	return &types.MsgConfirmParticipantResponse{}, k.Customer.Set(ctx, msg.Customer, existing)
}
