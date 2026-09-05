package keeper

import (
	"bytes"
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/stablecoin/types"
)

// RevokeIssuer takes a currency's issuing licence away.
//
// Until this existed there was no way to remove an approved issuer at all. The
// message set held ApproveIssuer and nothing else, and ApproveIssuer refuses an
// application whose status is no longer Pending — so on a chain where one key
// was the approved issuer for all 43 currencies, a compromise of that key could
// not be answered by governance at all. The remedy was a chain upgrade.
//
// What this does not do is unwind anything. The currency stays registered and
// its outstanding supply stays where it is: revocation stops new issuance, and
// that is the whole of it. Burning what is already held belongs to whoever
// holds it, and a message that could reach into balances would be a far larger
// power than the one being taken away here.
func (k msgServer) RevokeIssuer(ctx context.Context, msg *types.MsgRevokeIssuer) (*types.MsgRevokeIssuerResponse, error) {
	authority, err := k.addressCodec.StringToBytes(msg.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}
	governance := bytes.Equal(k.GetAuthority(), authority)

	approved, getErr := k.ApprovedIssuer.Get(ctx, msg.Denom)
	if getErr != nil {
		return nil, errorsmod.Wrapf(types.ErrNotApprovedIssuer, "%s has no approved issuer to revoke", msg.Denom)
	}

	// The same test ApproveIssuer applies, and read from the same place: the
	// stored record, never the message. A signer who could name the perimeter
	// they are acting inside would be choosing their own authority.
	if !governance {
		if err := k.assertScope(ctx, msg.Authority, approved.Issuer); err != nil {
			return nil, err
		}
	}

	if err := k.ApprovedIssuer.Remove(ctx, msg.Denom); err != nil {
		return nil, err
	}
	// The application goes too, so the currency can be applied for again by
	// somebody else. Leaving it would make the denomination unclaimable for
	// exactly the reason a rejected application used to be — see ApproveIssuer.
	if err := k.IssuerApplication.Remove(ctx, msg.Denom); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(
		"issuer_revoked",
		sdk.NewAttribute("denom", msg.Denom),
		sdk.NewAttribute("issuer", approved.Issuer),
		sdk.NewAttribute("authority", msg.Authority),
		sdk.NewAttribute("reason", msg.Reason),
	))

	return &types.MsgRevokeIssuerResponse{}, nil
}
