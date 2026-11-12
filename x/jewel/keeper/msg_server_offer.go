package keeper

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"yamale/blockchain/x/jewel/types"
)

// OfferJewel opens an offer to an account or to a claim key.
func (k msgServer) OfferJewel(ctx context.Context, req *types.MsgOfferJewel) (*types.MsgOfferJewelResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if j.Stolen {
		return nil, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	owner, err := k.actsForOwner(ctx, req.Owner, j)
	if err != nil {
		return nil, err
	}
	if (req.To == "") == (req.ClaimKey == "") {
		return nil, types.ErrOfferTarget
	}
	claimKey := strings.ToLower(req.ClaimKey)
	if req.To != "" {
		if _, err := k.addressCodec.StringToBytes(req.To); err != nil {
			return nil, types.ErrInvalidAddress.Wrapf("%s", req.To)
		}
		if req.To == owner {
			return nil, types.ErrSelfOffer
		}
	} else if b, err := hex.DecodeString(claimKey); err != nil || len(b) != secp256k1.PubKeySize {
		return nil, types.ErrOfferTarget.Wrap("the claim key must be a compressed secp256k1 public key in hex")
	}

	now := k.now(ctx)
	if existing, err := k.Offer.Get(ctx, j.Id); err == nil && existing.ExpiresAt > now {
		return nil, types.ErrOfferOpen
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	offer := types.Offer{JewelId: j.Id, From: owner, To: req.To, ClaimKey: claimKey, ExpiresAt: now + p.OfferTtl}
	if err := k.Offer.Set(ctx, j.Id, offer); err != nil {
		return nil, err
	}
	return &types.MsgOfferJewelResponse{ExpiresAt: offer.ExpiresAt}, nil
}

// CancelOffer closes the open offer.
func (k msgServer) CancelOffer(ctx context.Context, req *types.MsgCancelOffer) (*types.MsgCancelOfferResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.actsForOwner(ctx, req.Owner, j); err != nil {
		return nil, err
	}
	if has, err := k.Offer.Has(ctx, j.Id); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNoOffer
	}
	return &types.MsgCancelOfferResponse{}, k.Offer.Remove(ctx, j.Id)
}

// openOffer returns the jewel's offer if one exists and has not lapsed.
func (k msgServer) openOffer(ctx context.Context, j types.Jewel) (types.Offer, error) {
	if j.Stolen {
		return types.Offer{}, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	o, err := k.Offer.Get(ctx, j.Id)
	if isNotFound(err) {
		return o, types.ErrNoOffer
	}
	if err != nil {
		return o, err
	}
	if k.now(ctx) >= o.ExpiresAt {
		return o, types.ErrOfferExpired
	}
	return o, nil
}

// AcceptOffer takes a jewel offered to the signer's account.
func (k msgServer) AcceptOffer(ctx context.Context, req *types.MsgAcceptOffer) (*types.MsgAcceptOfferResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	o, err := k.openOffer(ctx, j)
	if err != nil {
		return nil, err
	}
	if o.To == "" || o.To != req.Recipient {
		return nil, types.ErrNotRecipient
	}
	return &types.MsgAcceptOfferResponse{}, k.transfer(ctx, &j, req.Recipient)
}

// ClaimOffer takes a jewel offered to a claim key. The key signs the
// claimer's own address, so a claim copied from the mempool is worthless.
func (k msgServer) ClaimOffer(ctx context.Context, req *types.MsgClaimOffer) (*types.MsgClaimOfferResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	o, err := k.openOffer(ctx, j)
	if err != nil {
		return nil, err
	}
	if o.ClaimKey == "" || hex.EncodeToString(req.ClaimPubKey) != o.ClaimKey {
		return nil, types.ErrBadClaim
	}
	msg := types.ClaimSignBytes(k.chainID(ctx), j.Id, req.Claimer)
	if _, ok := types.VerifySecp256k1(req.ClaimPubKey, msg, req.ClaimSignature); !ok {
		return nil, types.ErrBadClaim
	}
	if req.Claimer == o.From {
		return nil, types.ErrSelfOffer
	}
	return &types.MsgClaimOfferResponse{}, k.transfer(ctx, &j, req.Claimer)
}

// ReportStolen flags the jewel and closes its open offer.
func (k msgServer) ReportStolen(ctx context.Context, req *types.MsgReportStolen) (*types.MsgReportStolenResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.actsForOwner(ctx, req.Owner, j); err != nil {
		return nil, err
	}
	j.Stolen = true
	if err := k.Offer.Remove(ctx, j.Id); err != nil {
		return nil, err
	}
	return &types.MsgReportStolenResponse{}, k.Jewel.Set(ctx, j.Id, j)
}

// ClearStolen lifts the flag.
func (k msgServer) ClearStolen(ctx context.Context, req *types.MsgClearStolen) (*types.MsgClearStolenResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.actsForOwner(ctx, req.Owner, j); err != nil {
		return nil, err
	}
	if !j.Stolen {
		return nil, types.ErrNotStolen
	}
	j.Stolen = false
	return &types.MsgClearStolenResponse{}, k.Jewel.Set(ctx, j.Id, j)
}
