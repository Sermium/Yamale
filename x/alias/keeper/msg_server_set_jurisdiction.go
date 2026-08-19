package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/alias/types"
)

// SetJurisdiction records or corrects the country an account belongs to.
//
// Who may write it is the whole of the design. The first recording belongs to
// the approved participant that onboarded the account, because that is the only
// party that performed the KYC and therefore the only one that knows the
// answer. A correction belongs to a foundation administrator, because a
// participant able to rewrite a country it had already recorded could move a
// customer out from under the authority investigating them. And nobody may
// declare their own: an account free to name its own perimeter would name the
// one with no authority watching it.
//
// Correcting the country of an account that already holds an identifier retires
// that identifier and issues a replacement, here, in the same message. It has
// to. Left alone, the prefix would name a country the chain no longer records —
// and a prefix that can go stale is a prefix that can lie, which is the one
// property this whole feature exists to guarantee. Every other property
// survives: the old identifier is tombstoned rather than repointed, and is
// never issued again.
func (k msgServer) SetJurisdiction(ctx context.Context, msg *types.MsgSetJurisdiction) (*types.MsgSetJurisdictionResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Recorder); err != nil {
		return nil, errorsmod.Wrap(err, "invalid recorder address")
	}
	if _, err := k.addressCodec.StringToBytes(msg.Account); err != nil {
		return nil, errorsmod.Wrap(err, "invalid account address")
	}

	country := types.NormaliseCountry(msg.Country)
	// AssignedCountry, not IssuableCountry: the foundation's reserved code is
	// refused here. It marks the absence of a national perimeter, and recorded
	// as a jurisdiction it would let an ordinary account be issued an
	// identifier that reads as chain-wide authority.
	if !types.AssignedCountry(country) {
		return nil, errorsmod.Wrapf(types.ErrInvalidCountry, "%q", msg.Country)
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	administrator := params.IsFoundationAdministrator(msg.Recorder)
	governance := msg.Recorder == k.GetAuthority()

	existing, err := k.Jurisdictions.Get(ctx, msg.Account)
	switch {
	case err == nil:
		// Already recorded, so this is a correction, and a correction is an
		// authority's act.
		if !administrator && !governance {
			return nil, errorsmod.Wrapf(types.ErrJurisdictionSet,
				"%s is recorded in %s", msg.Account, existing.Country)
		}
		if existing.Country == country {
			// Nothing changed, so nothing is retired. Rotating an identifier
			// for a no-op would destroy a live handle for free, and a message
			// resubmitted after a timeout is the ordinary way that happens.
			return &types.MsgSetJurisdictionResponse{}, nil
		}
	case errors.Is(err, collections.ErrNotFound):
		if !administrator && !governance {
			if err := k.assertOnboardingParticipant(ctx, msg.Recorder, msg.Account); err != nil {
				return nil, err
			}
		}
	default:
		return nil, err
	}

	if err := k.record(ctx, msg.Account, country, msg.Recorder); err != nil {
		return nil, err
	}

	res := &types.MsgSetJurisdictionResponse{}
	if old, err := k.Owners.Get(ctx, msg.Account); err == nil {
		if err := k.retire(ctx, old, msg.Account); err != nil {
			return nil, err
		}
		id, err := k.issue(ctx, country, msg.Account)
		if err != nil {
			return nil, err
		}
		if err := k.bind(ctx, id, msg.Account); err != nil {
			return nil, err
		}
		res.Retired, res.Id = old, id
	} else if !errors.Is(err, collections.ErrNotFound) {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"jurisdiction_recorded",
		sdk.NewAttribute("address", msg.Account),
		sdk.NewAttribute("country", country),
		sdk.NewAttribute("recorded_by", msg.Recorder),
		sdk.NewAttribute("retired", res.Retired),
		sdk.NewAttribute("id", res.Id),
	))
	return res, nil
}

// assertOnboardingParticipant refuses anyone but the institution that actually
// banks the account.
//
// Approval is re-checked rather than inferred from the customer record, because
// approval can be withdrawn after customers were registered — and an
// institution thrown off the rail must not go on stamping perimeters with the
// relationships it accumulated while it was on it.
//
// It also refuses self-declaration by construction: an account cannot be its
// own approved participant and its own customer, because RegisterCustomer is
// signed by the participant and names somebody else.
func (k Keeper) assertOnboardingParticipant(ctx context.Context, recorder, account string) error {
	participant, found, err := k.participants.ParticipantOf(ctx, account)
	if err != nil {
		return err
	}
	if !found || participant != recorder {
		return errorsmod.Wrapf(types.ErrNotTheRecorder,
			"%s does not act for %s", recorder, account)
	}
	approved, err := k.participants.ApprovedParticipantExists(ctx, recorder)
	if err != nil {
		return err
	}
	if !approved {
		return errorsmod.Wrapf(types.ErrNotTheRecorder,
			"%s is no longer an approved participant", recorder)
	}
	return nil
}
