package keeper

import (
	"bytes"
	"context"

	errorsmod "cosmossdk.io/errors"

	constitutiontypes "yamale/blockchain/x/constitution/types"
	"yamale/blockchain/x/enforcement/types"
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

// UpdateParams replaces the module's parameters. Governance only.
func (k msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	authorityBytes, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), authorityBytes) {
		expectedAuthority, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expectedAuthority, msg.Authority)
	}

	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}

	// Checked after Validate and before the write. A proposal that passed is
	// not a licence to move the seizure threshold or the address seized assets
	// go to: those are held in x/constitution, and changing them takes an
	// amendment — a second proposal, weeks of public delay, and a supermajority
	// of the validator set ratifying it separately.
	//
	// Failing closed when there is no constitution is deliberate. The
	// alternative is a chain that lost its settlement in an upgrade and
	// silently went back to being an ordinary parameter set, which is exactly
	// the condition this module was in when recovery_destination was found
	// empty on the running devnet.
	inv, err := k.constitutionKeeper.GetInvariants(ctx)
	if err != nil {
		return nil, errorsmod.Wrap(err, "cannot check the proposed parameters against this chain's constitution")
	}
	if err := msg.Params.AssertConstitutional(inv); err != nil {
		return nil, errorsmod.Wrap(constitutiontypes.ErrInvariantViolation, err.Error())
	}

	return &types.MsgUpdateParamsResponse{}, k.Params.Set(ctx, msg.Params)
}
