package keeper

import (
	"context"
	"strings"

	"yamale/blockchain/x/jewel/types"
)

// ProposeReassignment opens the public window. A proposal left stale by a
// change of owner is replaced rather than blocking.
func (k msgServer) ProposeReassignment(ctx context.Context, req *types.MsgProposeReassignment) (*types.MsgProposeReassignmentResponse, error) {
	if err := k.requireOffice(ctx, req.Office); err != nil {
		return nil, err
	}
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	// A reported jewel is one its owner does not consent to moving.
	if j.Stolen {
		return nil, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	to, err := k.canonical(req.To)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, types.ErrNoReason
	}
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	if to == owner {
		return nil, types.ErrSelfOffer
	}
	if existing, err := k.Reassignment.Get(ctx, j.Id); err == nil && existing.From == owner {
		return nil, types.ErrReassignmentOpen
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	r := types.Reassignment{JewelId: j.Id, From: owner, To: to, Reason: req.Reason,
		WindowEnds: k.now(ctx) + p.ReassignmentWindow}
	if err := k.Reassignment.Set(ctx, j.Id, r); err != nil {
		return nil, err
	}
	if err := k.emit(ctx, &types.EventReassignmentProposed{JewelId: j.Id, From: owner, To: to,
		WindowEnds: r.WindowEnds}); err != nil {
		return nil, err
	}
	return &types.MsgProposeReassignmentResponse{WindowEnds: r.WindowEnds}, nil
}

// VetoReassignment closes it. The current owner's key only, and for stock
// that means the business account itself: staff act for the business in
// selling, not in refusing to let the office move a jewel. A veto stays
// possible after the window ends, until someone completes the reassignment;
// the window is the least time the owner has, not the most.
func (k msgServer) VetoReassignment(ctx context.Context, req *types.MsgVetoReassignment) (*types.MsgVetoReassignmentResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if has, err := k.Reassignment.Has(ctx, j.Id); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNoReassignment
	}
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	signer, err := k.canonical(req.Owner)
	if err != nil {
		return nil, err
	}
	if signer != owner {
		return nil, types.ErrNotOwner.Wrapf("%s", signer)
	}
	if err := k.Reassignment.Remove(ctx, j.Id); err != nil {
		return nil, err
	}
	return &types.MsgVetoReassignmentResponse{}, k.emit(ctx, &types.EventReassignmentVetoed{JewelId: j.Id, Owner: owner})
}

// CompleteReassignment moves the jewel once the window has passed. Anyone may
// call it, so the office cannot hold a reassignment hostage.
func (k msgServer) CompleteReassignment(ctx context.Context, req *types.MsgCompleteReassignment) (*types.MsgCompleteReassignmentResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if j.Stolen {
		return nil, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	r, err := k.Reassignment.Get(ctx, j.Id)
	if isNotFound(err) {
		return nil, types.ErrNoReassignment
	}
	if err != nil {
		return nil, err
	}
	if k.now(ctx) < r.WindowEnds {
		return nil, types.ErrWindowOpen
	}
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	if owner != r.From {
		return nil, types.ErrReassignmentStale
	}
	if err := k.Reassignment.Remove(ctx, j.Id); err != nil {
		return nil, err
	}
	return &types.MsgCompleteReassignmentResponse{}, k.transfer(ctx, &j, r.To, types.TransferReassign)
}
