package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"
	"cosmossdk.io/x/nft"

	"yamale/blockchain/x/jewel/types"
)

// RegisterJewel mints a jewel into the signer's jeweler's stock.
func (k msgServer) RegisterJewel(ctx context.Context, req *types.MsgRegisterJewel) (*types.MsgRegisterJewelResponse, error) {
	staff, err := k.canonical(req.Staff)
	if err != nil {
		return nil, err
	}
	jewelerID, err := k.ByStaff.Get(ctx, staff)
	if isNotFound(err) {
		return nil, types.ErrNotStaff.Wrapf("%s", staff)
	}
	if err != nil {
		return nil, err
	}
	jr, err := k.activeJeweler(ctx, jewelerID)
	if err != nil {
		return nil, err
	}
	if err := k.requireBareSendClosed(ctx); err != nil {
		return nil, err
	}
	code, err := types.NormaliseInscription(req.Inscription)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateRecord(req.RecordHash, req.RecordUri); err != nil {
		return nil, err
	}
	key := collections.Join(jr.Id, code)
	if existing, err := k.ByInscription.Get(ctx, key); err == nil {
		return nil, types.ErrInscriptionTaken.Wrapf("%s is jewel %d", code, existing)
	} else if !isNotFound(err) {
		return nil, err
	}

	id, err := nextID(ctx, k.NextJewelID)
	if err != nil {
		return nil, err
	}
	business, err := k.addressCodec.StringToBytes(jr.Business)
	if err != nil {
		return nil, err
	}
	if err := k.nft.Mint(ctx, nft.NFT{
		ClassId: types.ClassID(jr.Id), Id: types.TokenID(id),
		Uri: req.RecordUri, UriHash: req.RecordHash,
	}, business); err != nil {
		return nil, err
	}
	j := types.Jewel{
		Id: id, JewelerId: jr.Id, Inscription: code,
		RecordHash: req.RecordHash, RecordUri: req.RecordUri, RegisteredAt: k.now(ctx),
	}
	if err := k.Jewel.Set(ctx, id, j); err != nil {
		return nil, err
	}
	if err := k.ByInscription.Set(ctx, key, id); err != nil {
		return nil, err
	}
	if err := k.Search.Set(ctx, collections.Join(code, id)); err != nil {
		return nil, err
	}
	return &types.MsgRegisterJewelResponse{Id: id}, nil
}

// AmendJewel appends a correction and points the token at the new record.
func (k msgServer) AmendJewel(ctx context.Context, req *types.MsgAmendJewel) (*types.MsgAmendJewelResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	staff, err := k.canonical(req.Staff)
	if err != nil {
		return nil, err
	}
	if _, err := k.requireStaff(ctx, staff, j.JewelerId); err != nil {
		return nil, err
	}
	if err := types.ValidateRecord(req.RecordHash, req.RecordUri); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Note) == "" {
		return nil, types.ErrNoReason
	}
	if len(req.Note) > types.MaxAmendmentNote {
		return nil, types.ErrNoteTooLong.Wrapf("%d bytes", len(req.Note))
	}
	if len(j.Amendments) >= types.MaxAmendments {
		return nil, types.ErrTooManyAmendments
	}
	j.Amendments = append(j.Amendments, types.Amendment{
		RecordHash: req.RecordHash, RecordUri: req.RecordUri, Note: req.Note,
		Staff: staff, Time: k.now(ctx),
	})
	if err := k.nft.Update(ctx, nft.NFT{
		ClassId: types.ClassID(j.JewelerId), Id: types.TokenID(j.Id),
		Uri: req.RecordUri, UriHash: req.RecordHash,
	}); err != nil {
		return nil, err
	}
	return &types.MsgAmendJewelResponse{}, k.Jewel.Set(ctx, j.Id, j)
}
