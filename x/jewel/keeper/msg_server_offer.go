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
	// actsForOwner accepted the signer, so it has a canonical spelling.
	maker, err := k.canonical(req.Owner)
	if err != nil {
		return nil, err
	}
	if err := k.requireSellable(ctx, owner, j); err != nil {
		return nil, err
	}
	if (req.To == "") == (req.ClaimKey == "") {
		return nil, types.ErrOfferTarget
	}
	claimKey := strings.ToLower(req.ClaimKey)
	to := ""
	if req.To != "" {
		if to, err = k.canonical(req.To); err != nil {
			return nil, err
		}
		if to == owner {
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
	offer := types.Offer{JewelId: j.Id, From: owner, Maker: maker, To: to, ClaimKey: claimKey, ExpiresAt: now + p.OfferTtl}
	if err := k.Offer.Set(ctx, j.Id, offer); err != nil {
		return nil, err
	}
	if err := k.emit(ctx, &types.EventOfferMade{JewelId: j.Id, From: owner, Maker: maker, To: to,
		ExpiresAt: offer.ExpiresAt}); err != nil {
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
	by, err := k.canonical(req.Owner)
	if err != nil {
		return nil, err
	}
	if has, err := k.Offer.Has(ctx, j.Id); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNoOffer
	}
	if err := k.Offer.Remove(ctx, j.Id); err != nil {
		return nil, err
	}
	return &types.MsgCancelOfferResponse{}, k.emit(ctx, &types.EventOfferCancelled{JewelId: j.Id, By: by})
}

// openOffer returns the jewel's offer if it may still be taken: it exists,
// has not lapsed, still speaks for the jewel's current owner, and, when a
// staff member made it from stock, that person still works for the jeweler
// and the jeweler is active. An offer from stock also dies while its jeweler
// is suspended, whoever made it. An offer must not outlive the authority of
// whoever made it.
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
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return o, err
	}
	if o.From != owner {
		return o, types.ErrOfferStale
	}
	if err := k.requireSellable(ctx, o.From, j); err != nil {
		return o, err
	}
	if o.Maker != o.From {
		if _, err := k.requireStaff(ctx, o.Maker, j.JewelerId); err != nil {
			return o, err
		}
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
	recipient, err := k.canonical(req.Recipient)
	if err != nil {
		return nil, err
	}
	if o.To == "" || o.To != recipient {
		return nil, types.ErrNotRecipient
	}
	return &types.MsgAcceptOfferResponse{}, k.transfer(ctx, &j, recipient, types.TransferAccept)
}

// ClaimOffer takes a jewel offered to a claim key. The key signs the claimer's
// own address and the offer's expiry, so a claim copied from the mempool is
// worthless, and so is a signature made for an earlier offer to the same key.
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
	claimer, err := k.canonical(req.Claimer)
	if err != nil {
		return nil, err
	}
	msg := types.ClaimSignBytes(k.chainID(ctx), j.Id, o.ExpiresAt, claimer)
	if _, ok := types.VerifySecp256k1(req.ClaimPubKey, msg, req.ClaimSignature); !ok {
		return nil, types.ErrBadClaim
	}
	if claimer == o.From {
		return nil, types.ErrSelfOffer
	}
	return &types.MsgClaimOfferResponse{}, k.transfer(ctx, &j, claimer, types.TransferClaim)
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
	by, err := k.canonical(req.Owner)
	if err != nil {
		return nil, err
	}
	j.Stolen = true
	if err := k.Offer.Remove(ctx, j.Id); err != nil {
		return nil, err
	}
	if err := k.Jewel.Set(ctx, j.Id, j); err != nil {
		return nil, err
	}
	return &types.MsgReportStolenResponse{}, k.emit(ctx, &types.EventStolenReported{JewelId: j.Id, By: by})
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
	by, err := k.canonical(req.Owner)
	if err != nil {
		return nil, err
	}
	if !j.Stolen {
		return nil, types.ErrNotStolen
	}
	j.Stolen = false
	if err := k.Jewel.Set(ctx, j.Id, j); err != nil {
		return nil, err
	}
	return &types.MsgClearStolenResponse{}, k.emit(ctx, &types.EventStolenCleared{JewelId: j.Id, By: by})
}
