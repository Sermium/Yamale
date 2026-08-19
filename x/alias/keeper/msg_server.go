package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/alias/types"
)

type msgServer struct{ Keeper }

// NewMsgServerImpl returns the Msg service implementation.
func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{Keeper: k} }

// RegisterAlias issues an identifier to the sending account.
//
// The message carries no identifier, so there is nothing to validate about the
// caller's choice and nothing for anyone to squat. One per account: a second
// call is an error rather than a second identifier, because "which of your two
// handles did you mean" is a question a payment interface cannot answer.
//
// It also carries no country. The prefix comes from the jurisdiction already
// recorded against the account, and that is what makes the prefix incapable of
// lying: there is no path through this handler that issues an identifier naming
// a country the chain does not record the account as being in. An account with
// none recorded is refused outright — see Keeper.CountryOf for the rule and for
// its one exception.
func (k msgServer) RegisterAlias(ctx context.Context, msg *types.MsgRegisterAlias) (*types.MsgRegisterAliasResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Account); err != nil {
		return nil, errorsmod.Wrap(err, "invalid account address")
	}

	held, err := k.Owners.Has(ctx, msg.Account)
	if err != nil {
		return nil, err
	}
	if held {
		return nil, types.ErrAlreadyRegistered
	}

	country, err := k.CountryOf(ctx, msg.Account)
	if err != nil {
		return nil, err
	}

	id, err := k.issue(ctx, country, msg.Account)
	if err != nil {
		return nil, err
	}
	if err := k.bind(ctx, id, msg.Account); err != nil {
		return nil, err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"alias_registered",
		sdk.NewAttribute("id", id),
		sdk.NewAttribute("address", msg.Account),
		sdk.NewAttribute("country", country),
	))
	return &types.MsgRegisterAliasResponse{Id: id}, nil
}

// RotateAlias retires the sender's identifier and issues a replacement.
//
// One message rather than release-then-register, because that ordering has to
// be atomic: an account between the two would hold no identifier, and anything
// resolving it in that gap would get a not-found for an account that exists.
//
// The old identifier is tombstoned and never issued again. That is what makes
// "an identifier is never repointed" survivable — somebody whose key was stolen
// can leave, and a payment to the handle they used to answer to arrives nowhere
// rather than in the thief's account.
func (k msgServer) RotateAlias(ctx context.Context, msg *types.MsgRotateAlias) (*types.MsgRotateAliasResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Account); err != nil {
		return nil, errorsmod.Wrap(err, "invalid account address")
	}

	old, err := k.Owners.Get(ctx, msg.Account)
	if err != nil {
		return nil, types.ErrNotRegistered
	}

	// Re-read rather than reuse the prefix of the identifier being given up. A
	// jurisdiction corrected between registration and rotation would otherwise
	// be carried forward, and the replacement would inherit a country the chain
	// no longer records.
	country, err := k.CountryOf(ctx, msg.Account)
	if err != nil {
		return nil, err
	}

	// Tombstone before deriving, so the replacement cannot come back as the one
	// just given up.
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

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"alias_rotated",
		sdk.NewAttribute("country", country),
		sdk.NewAttribute("retired", old),
		sdk.NewAttribute("id", id),
		sdk.NewAttribute("address", msg.Account),
	))
	return &types.MsgRotateAliasResponse{Retired: old, Id: id}, nil
}

// UpdateParams sets the module parameters. Governance only.
func (k msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != k.GetAuthority() {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner,
			"expected %s, got %s", k.GetAuthority(), msg.Authority)
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, errorsmod.Wrap(types.ErrInvalidParams, err.Error())
	}
	if err := k.Params.Set(ctx, msg.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
