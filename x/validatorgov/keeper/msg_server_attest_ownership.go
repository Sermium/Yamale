package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/validatorgov/types"
)

// AttestOwnership is an approved operator re-signing for who is behind it.
//
// Signed by the operator itself and by nobody else. There is no approval step
// and no vote: the chain cannot check a declaration against anything, so the
// only thing it can offer is a signature with a date on it, and requiring a
// vote to update one would mean the honest response to an ownership change —
// declaring it — is the slow one.
//
// The whole declaration is restated rather than a timestamp bumped. An operator
// whose owner changed and who re-attested the old values has put a false
// statement on the record under its own key, which is a fact a supervisor can
// act on; a bare heartbeat would have let the same operator keep a stale
// declaration fresh without ever repeating it.
func (k msgServer) AttestOwnership(ctx context.Context, msg *types.MsgAttestOwnership) (*types.MsgAttestOwnershipResponse, error) {
	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid creator address")
	}

	approved, err := k.ApprovedValidator.Get(ctx, msg.Creator)
	if err != nil {
		if isNotFound(err) {
			return nil, errorsmod.Wrapf(types.ErrNotApprovedValidator, "%s", msg.Creator)
		}
		return nil, err
	}

	declaration := types.NormaliseDeclaration(msg.LegalEntityId, msg.BeneficialOwnerId, msg.Jurisdiction)
	if err := declaration.Validate(); err != nil {
		return nil, errorsmod.Wrap(types.ErrInvalidDeclaration, err.Error())
	}
	declaration.AttestedAtHeight = sdk.UnwrapSDKContext(ctx).BlockHeight()

	approved.Declaration = declaration
	if err := k.ApprovedValidator.Set(ctx, msg.Creator, approved); err != nil {
		return nil, err
	}

	// The application keeps the declaration it was admitted on. The two are
	// deliberately allowed to diverge: the application is what the set voted
	// for and the approval is what is claimed now, and an admission record
	// silently rewritten by the applicant afterwards would destroy the only
	// evidence that anything changed.
	return &types.MsgAttestOwnershipResponse{}, nil
}
