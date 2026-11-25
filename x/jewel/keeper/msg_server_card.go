package keeper

import (
	"context"
	"encoding/hex"

	"yamale/blockchain/x/jewel/types"
)

// The ownership card: the shop prints a secret on a card for a stock jewel,
// the buyer files a claim signed with it, and the shop's staff approve the
// claim once they know this person paid. Approval moves the jewel itself, so
// the buyer never has to accept anything. A card photographed in the shop is
// harmless, because nobody approves that claim.

// IssueCard sets a stock jewel's ownership card, replacing any earlier one,
// and drops a pending claim that was signed with the earlier card.
func (k msgServer) IssueCard(ctx context.Context, req *types.MsgIssueCard) (*types.MsgIssueCardResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if j.Stolen {
		return nil, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	if _, err := k.actsForOwner(ctx, req.Staff, j); err != nil {
		return nil, err
	}
	if _, err := k.requireStock(ctx, j); err != nil {
		return nil, err
	}
	// A suspended jeweler cannot hand over stock; its business account still
	// passes actsForOwner, as the owner, so this is checked on its own.
	if _, err := k.activeJeweler(ctx, j.JewelerId); err != nil {
		return nil, err
	}
	key, err := types.NormaliseCardKey(req.CardKey)
	if err != nil {
		return nil, err
	}
	by, err := k.canonical(req.Staff)
	if err != nil {
		return nil, err
	}
	if c, err := k.Claim.Get(ctx, j.Id); err == nil && c.CardKey != key {
		if err := k.dropClaim(ctx, j); err != nil {
			return nil, err
		}
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}
	if err := k.Card.Set(ctx, j.Id, types.Card{Key: key, IssuedBy: by, IssuedAt: k.now(ctx)}); err != nil {
		return nil, err
	}
	return &types.MsgIssueCardResponse{}, k.emit(ctx, &types.EventCardIssued{JewelId: j.Id, By: by})
}

// FileClaim records a buyer's claim on a stock jewel. The card's secret signs
// the claimer's own address and the card's key, so a claim copied off the
// network names its claimer and cannot be refiled for anyone else, and a
// signature made with a cancelled card fails.
func (k msgServer) FileClaim(ctx context.Context, req *types.MsgFileClaim) (*types.MsgFileClaimResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if j.Stolen {
		return nil, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	card, err := k.Card.Get(ctx, j.Id)
	if isNotFound(err) {
		return nil, types.ErrNoCard
	}
	if err != nil {
		return nil, err
	}
	if hex.EncodeToString(req.CardPubKey) != card.Key {
		return nil, types.ErrCardNotLive
	}
	claimer, err := k.canonical(req.Claimer)
	if err != nil {
		return nil, err
	}
	msg := types.CardClaimSignBytes(k.chainID(ctx), j.Id, card.Key, claimer)
	if _, ok := types.VerifySecp256k1(req.CardPubKey, msg, req.CardSignature); !ok {
		return nil, types.ErrBadCardClaim
	}
	jr, err := k.requireStock(ctx, j)
	if err != nil {
		return nil, err
	}
	if jr.Status != types.JEWELER_ACTIVE {
		return nil, types.ErrJewelerSuspended.Wrapf("%s", jr.Name)
	}
	if claimer == jr.Business {
		return nil, types.ErrShopSelfClaim
	}
	if has, err := k.Claim.Has(ctx, j.Id); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrClaimPending
	}
	if err := k.setClaim(ctx, j, types.Claim{Claimer: claimer, CardKey: card.Key, FiledAt: k.now(ctx)}); err != nil {
		return nil, err
	}
	return &types.MsgFileClaimResponse{}, k.emit(ctx, &types.EventClaimFiled{JewelId: j.Id, Claimer: claimer})
}

// ApproveClaim moves the jewel to the claimer. transfer spends the card and
// the claim with it.
func (k msgServer) ApproveClaim(ctx context.Context, req *types.MsgApproveClaim) (*types.MsgApproveClaimResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	c, err := k.Claim.Get(ctx, j.Id)
	if isNotFound(err) {
		return nil, types.ErrNoClaim
	}
	if err != nil {
		return nil, err
	}
	// Unreachable while ReportStolen drops claims; kept so that no future path
	// to a claim on a stolen jewel can move it.
	if j.Stolen {
		return nil, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	if _, err := k.actsForOwner(ctx, req.Staff, j); err != nil {
		return nil, err
	}
	if _, err := k.requireStock(ctx, j); err != nil {
		return nil, err
	}
	if _, err := k.activeJeweler(ctx, j.JewelerId); err != nil {
		return nil, err
	}
	by, err := k.canonical(req.Staff)
	if err != nil {
		return nil, err
	}
	if err := k.transfer(ctx, &j, c.Claimer, types.TransferCard); err != nil {
		return nil, err
	}
	return &types.MsgApproveClaimResponse{}, k.emit(ctx,
		&types.EventClaimApproved{JewelId: j.Id, Claimer: c.Claimer, By: by})
}

// RejectClaim removes the pending claim. The card stays valid, so the real
// buyer can file with it afterwards: a stranger who files first only delays
// them.
func (k msgServer) RejectClaim(ctx context.Context, req *types.MsgRejectClaim) (*types.MsgRejectClaimResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.actsForOwner(ctx, req.Staff, j); err != nil {
		return nil, err
	}
	by, err := k.canonical(req.Staff)
	if err != nil {
		return nil, err
	}
	if has, err := k.Claim.Has(ctx, j.Id); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNoClaim
	}
	if err := k.dropClaim(ctx, j); err != nil {
		return nil, err
	}
	return &types.MsgRejectClaimResponse{}, k.emit(ctx, &types.EventClaimRejected{JewelId: j.Id, By: by})
}

// WithdrawClaim removes the signer's own pending claim.
func (k msgServer) WithdrawClaim(ctx context.Context, req *types.MsgWithdrawClaim) (*types.MsgWithdrawClaimResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	c, err := k.Claim.Get(ctx, j.Id)
	if isNotFound(err) {
		return nil, types.ErrNoClaim
	}
	if err != nil {
		return nil, err
	}
	claimer, err := k.canonical(req.Claimer)
	if err != nil {
		return nil, err
	}
	if claimer != c.Claimer {
		return nil, types.ErrNotClaimer
	}
	if err := k.dropClaim(ctx, j); err != nil {
		return nil, err
	}
	return &types.MsgWithdrawClaimResponse{}, k.emit(ctx, &types.EventClaimWithdrawn{JewelId: j.Id, Claimer: claimer})
}
