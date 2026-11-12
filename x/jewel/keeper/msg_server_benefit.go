package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/jewel/types"
)

// AttachBenefit adds a benefit to a jewel. Its own jeweler's staff only.
func (k msgServer) AttachBenefit(ctx context.Context, req *types.MsgAttachBenefit) (*types.MsgAttachBenefitResponse, error) {
	staff, err := k.canonical(req.Staff)
	if err != nil {
		return nil, err
	}
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	jr, err := k.requireStaff(ctx, staff, j.JewelerId)
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(req.Kind)
	now := k.now(ctx)
	if kind == "" || len(kind) > 80 || req.Allowance == 0 || req.Period < 0 ||
		(req.ExpiresAt != 0 && req.ExpiresAt <= now) {
		return nil, types.ErrInvalidBenefit
	}
	id, err := nextID(ctx, k.NextBenefitID)
	if err != nil {
		return nil, err
	}
	b := types.Benefit{Id: id, JewelId: j.Id, JewelerId: jr.Id, Kind: kind,
		Allowance: req.Allowance, Period: req.Period, ExpiresAt: req.ExpiresAt}
	if err := k.Benefit.Set(ctx, id, b); err != nil {
		return nil, err
	}
	if err := k.BenefitsByJewel.Set(ctx, collections.Join(j.Id, id)); err != nil {
		return nil, err
	}
	return &types.MsgAttachBenefitResponse{Id: id}, nil
}

// RedeemBenefit records one use. Staff submit it; the owner's signature on
// the voucher is the consent.
func (k msgServer) RedeemBenefit(ctx context.Context, req *types.MsgRedeemBenefit) (*types.MsgRedeemBenefitResponse, error) {
	staff, err := k.canonical(req.Staff)
	if err != nil {
		return nil, err
	}
	v := req.Voucher
	b, err := k.Benefit.Get(ctx, v.BenefitId)
	if isNotFound(err) {
		return nil, types.ErrNoBenefit.Wrapf("%d", v.BenefitId)
	}
	if err != nil {
		return nil, err
	}
	if _, err := k.requireStaff(ctx, staff, b.JewelerId); err != nil {
		return nil, err
	}
	now := k.now(ctx)
	if b.ExpiresAt != 0 && now >= b.ExpiresAt {
		return nil, types.ErrBenefitExpired
	}
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if v.JewelId != b.JewelId || v.Nonce == "" || len(v.Nonce) > 64 {
		return nil, types.ErrBadVoucher
	}
	if v.ExpiresAt <= now || v.ExpiresAt > now+p.VoucherMaxTtl {
		return nil, types.ErrBadVoucher.Wrap("the code has expired, or was made to last longer than allowed")
	}
	nonceKey := collections.Join(b.Id, v.Nonce)
	if used, err := k.UsedNonce.Has(ctx, nonceKey); err != nil {
		return nil, err
	} else if used {
		return nil, types.ErrVoucherUsed
	}
	j, err := k.getJewel(ctx, b.JewelId)
	if err != nil {
		return nil, err
	}
	if j.Stolen {
		return nil, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	signer, ok := types.VerifySecp256k1(req.OwnerPubKey, types.VoucherSignBytes(k.chainID(ctx), v), req.OwnerSignature)
	if !ok {
		return nil, types.ErrBadVoucher
	}
	signerStr, err := k.addressCodec.BytesToString(signer)
	if err != nil {
		return nil, err
	}
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	if signerStr != owner {
		return nil, types.ErrBadVoucher.Wrap("not signed by the jewel's owner")
	}
	used := 0
	for _, u := range b.Uses {
		if b.Period == 0 || u > now-b.Period {
			used++
		}
	}
	if used >= int(b.Allowance) {
		return nil, types.ErrAllowanceUsed
	}
	b.Uses = append(b.Uses, now)
	if err := k.Benefit.Set(ctx, b.Id, b); err != nil {
		return nil, err
	}
	return &types.MsgRedeemBenefitResponse{}, k.UsedNonce.Set(ctx, nonceKey)
}
