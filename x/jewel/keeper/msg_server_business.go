package keeper

import (
	"context"

	"yamale/blockchain/x/jewel/types"
)

// Moving a shop's business account. The business account holds the stock: a
// jewel of this jeweler that it owns is for sale, not owned. So a person whose
// own account is a shop's business account can never own one of that shop's
// pieces. The way out is to hand the shop to an account of its own; the person
// stays staff and becomes an ordinary customer.
//
// Two steps, two signers, two messages: the old account proposes, the new one
// accepts. Neither can take a shop alone, and each signs on its own device.

// ProposeBusinessMove names the account the business account is to move to,
// or withdraws the proposal.
func (k msgServer) ProposeBusinessMove(ctx context.Context, req *types.MsgProposeBusinessMove) (*types.MsgProposeBusinessMoveResponse, error) {
	j, err := k.activeJeweler(ctx, req.JewelerId)
	if err != nil {
		return nil, err
	}
	business, err := k.canonical(req.Business)
	if err != nil {
		return nil, err
	}
	if business != j.Business {
		return nil, types.ErrNotBusiness.Wrapf("%s", req.Business)
	}
	until := int64(0)
	next := ""
	if req.NewBusiness != "" {
		if next, err = k.canonical(req.NewBusiness); err != nil {
			return nil, err
		}
		if next == j.Business {
			return nil, types.ErrMoveToSelf
		}
		if err := k.requireFreeForBusiness(ctx, next, j.Id); err != nil {
			return nil, err
		}
		p, err := k.Params.Get(ctx)
		if err != nil {
			return nil, err
		}
		until = k.now(ctx) + p.OfferTtl
	}
	j.PendingBusiness, j.PendingBusinessUntil = next, until
	if err := k.Jeweler.Set(ctx, j.Id, j); err != nil {
		return nil, err
	}
	return &types.MsgProposeBusinessMoveResponse{},
		k.emit(ctx, &types.EventBusinessMoveProposed{JewelerId: j.Id, NewBusiness: next, Until: until})
}

// AcceptBusinessMove makes the named account the business account and moves
// the stock to it.
func (k msgServer) AcceptBusinessMove(ctx context.Context, req *types.MsgAcceptBusinessMove) (*types.MsgAcceptBusinessMoveResponse, error) {
	j, err := k.activeJeweler(ctx, req.JewelerId)
	if err != nil {
		return nil, err
	}
	next, err := k.canonical(req.NewBusiness)
	if err != nil {
		return nil, err
	}
	if j.PendingBusiness == "" || j.PendingBusiness != next {
		return nil, types.ErrNoBusinessMove.Wrapf("jeweler %d", j.Id)
	}
	if k.now(ctx) > j.PendingBusinessUntil {
		return nil, types.ErrBusinessMoveLapsed
	}
	// Checked again: the account may have joined another jeweler since.
	if err := k.requireFreeForBusiness(ctx, next, j.Id); err != nil {
		return nil, err
	}
	class := types.ClassID(j.Id)
	nextAddr, err := k.addressCodec.StringToBytes(next)
	if err != nil {
		return nil, types.ErrInvalidAddress.Wrapf("%s", next)
	}
	// Pieces it bought from this shop would turn into stock the moment it
	// became the business account, and be sold out from under it.
	if len(k.nft.GetNFTsOfClassByOwner(ctx, class, nextAddr)) > 0 {
		return nil, types.ErrBusinessOwnsPieces
	}
	old := j.Business
	oldAddr, err := k.addressCodec.StringToBytes(old)
	if err != nil {
		return nil, err
	}
	stock := k.nft.GetNFTsOfClassByOwner(ctx, class, oldAddr)
	for _, t := range stock {
		id, err := types.JewelIDOf(t.Id)
		if err != nil {
			return nil, err
		}
		// Not transfer(): the piece has not changed hands, it is still the
		// shop's, so its owner count, its card and any claim stay. An offer or
		// a reassignment was made against the old account, and goes.
		if err := k.nft.Transfer(ctx, class, t.Id, nextAddr); err != nil {
			return nil, err
		}
		if err := k.Offer.Remove(ctx, id); err != nil {
			return nil, err
		}
		if err := k.Reassignment.Remove(ctx, id); err != nil {
			return nil, err
		}
		if err := k.Listing.Remove(ctx, id); err != nil {
			return nil, err
		}
	}
	if err := k.ByBusiness.Remove(ctx, old); err != nil {
		return nil, err
	}
	if err := k.ByBusiness.Set(ctx, next, j.Id); err != nil {
		return nil, err
	}
	j.Business, j.PendingBusiness, j.PendingBusinessUntil = next, "", 0
	if err := k.Jeweler.Set(ctx, j.Id, j); err != nil {
		return nil, err
	}
	n := uint64(len(stock))
	return &types.MsgAcceptBusinessMoveResponse{Jewels: n},
		k.emit(ctx, &types.EventBusinessMoved{JewelerId: j.Id, From: old, To: next, Jewels: n})
}

// requireFreeForBusiness refuses an account that is another jeweler's business
// account or staff. This jeweler's own staff may take it over.
func (k Keeper) requireFreeForBusiness(ctx context.Context, addr string, jewelerID uint64) error {
	if has, err := k.ByBusiness.Has(ctx, addr); err != nil {
		return err
	} else if has {
		return types.ErrAccountTied.Wrapf("%s", addr)
	}
	got, err := k.ByStaff.Get(ctx, addr)
	switch {
	case isNotFound(err):
		return nil
	case err != nil:
		return err
	case got != jewelerID:
		return types.ErrAccountTied.Wrapf("%s", addr)
	}
	return nil
}
