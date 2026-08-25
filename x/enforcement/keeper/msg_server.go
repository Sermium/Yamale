package keeper

import (
	"bytes"
	"context"
	"strings"

	errorsmod "cosmossdk.io/errors"

	aliastypes "yamale/blockchain/x/alias/types"
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

	// The half of the ombudsman's exclusion that used to be a comparison inside
	// Params.Validate.
	//
	// It refused parameters in which the ombudsman and the emergency authority
	// were the same address, because the emergency authority can open a case and
	// an ombudsman holding that key would hold both halves of an office whose
	// whole safety is the asymmetry. The emergency authority is a grant now, in
	// another module, so a struct method cannot see it — and a check that
	// disappeared with the field would have taken a real property with it.
	//
	// This catches one of the two orders: appointing an ombudsman that already
	// holds the role. The other order — granting the role to a sitting ombudsman
	// — is caught by the handlers, which refuse the ombudsman outright wherever a
	// case is opened or advanced, and that was always the check doing the work.
	// The parameters could never have seen a grant made after they were written,
	// which is exactly the class of hole the module's own comments warn about.
	//
	// Fails closed on a missing registry. A chain that cannot check this must not
	// be able to appoint an ombudsman past it.
	if strings.TrimSpace(msg.Params.Ombudsman) != "" {
		authority, err := k.holdsEnforcementRole(ctx, msg.Params.Ombudsman)
		if err != nil {
			return nil, errorsmod.Wrap(err,
				"cannot check whether the proposed ombudsman is also an enforcement authority")
		}
		if authority {
			return nil, errorsmod.Wrapf(types.ErrOmbudsmanCannotInitiate,
				"the proposed ombudsman %s holds %s; that office can open a case, and an ombudsman holding it would hold both halves. "+
					"Revoke the grant, or appoint a different ombudsman",
				msg.Params.Ombudsman, aliastypes.RoleName(aliastypes.ROLE_ENFORCEMENT_AUTHORITY))
		}
	}

	return &types.MsgUpdateParamsResponse{}, k.Params.Set(ctx, msg.Params)
}
