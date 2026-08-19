package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/paymsg/types"
)

// SetPayloadStore records where a participant serves encrypted payment
// payloads.
//
// Signed by the participant itself, and by nobody else. Whoever can rewrite
// this field decides which host the payee's client asks for the detail of a
// payment — so a third party able to set it could point every retrieval at a
// server it controls and collect the requests, learning which payments are
// being read and by whom even though it can decrypt none of them. Governance
// approves participants; participants operate their own infrastructure.
//
// Setting it to the empty string withdraws the store. That is a supported act
// rather than an error: a participant winding down its service should be able
// to say so, and a client that then reports the detail as unavailable is
// telling the truth, where one still calling a dead host reports a network
// fault and invites a retry that will never work.
func (k msgServer) SetPayloadStore(ctx context.Context, msg *types.MsgSetPayloadStore) (*types.MsgSetPayloadStoreResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Participant); err != nil {
		return nil, errorsmod.Wrap(err, "invalid participant address")
	}
	if err := types.ValidatePayloadStoreURL(msg.Url); err != nil {
		return nil, err
	}

	// Read the record rather than only checking membership, so the other fields
	// survive. Writing a fresh ApprovedParticipant here would blank the code and
	// the name — which are what a statement export prints — to set a URL.
	participant, err := k.ApprovedParticipant.Get(ctx, msg.Participant)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedParticipant,
			"%s is not an approved participant, so it has no payments whose payloads it could serve", msg.Participant)
	}

	participant.PayloadStoreUrl = msg.Url
	if err := k.ApprovedParticipant.Set(ctx, msg.Participant, participant); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
		"payload_store_set",
		sdk.NewAttribute("participant", msg.Participant),
		sdk.NewAttribute("url", msg.Url),
	))
	return &types.MsgSetPayloadStoreResponse{}, nil
}
