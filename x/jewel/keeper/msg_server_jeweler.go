package keeper

import (
	"context"
	"slices"
	"strings"

	"cosmossdk.io/x/nft"

	"yamale/blockchain/x/jewel/types"
)

// AdmitJeweler admits a business and opens its x/nft collection.
func (k msgServer) AdmitJeweler(ctx context.Context, req *types.MsgAdmitJeweler) (*types.MsgAdmitJewelerResponse, error) {
	if err := k.requireOffice(ctx, req.Office); err != nil {
		return nil, err
	}
	if err := k.requireBareSendClosed(ctx); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, types.ErrNoName
	}
	business, err := k.canonical(req.Business)
	if err != nil {
		return nil, err
	}
	if has, err := k.ByBusiness.Has(ctx, business); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrBusinessTaken.Wrapf("%s", business)
	}

	id, err := nextID(ctx, k.NextJewelerID)
	if err != nil {
		return nil, err
	}
	if err := k.nft.SaveClass(ctx, nft.Class{Id: types.ClassID(id), Name: name, Symbol: "JEWEL"}); err != nil {
		return nil, err
	}
	j := types.Jeweler{Id: id, Name: name, Business: business, Status: types.JEWELER_ACTIVE}
	if err := k.Jeweler.Set(ctx, id, j); err != nil {
		return nil, err
	}
	if err := k.ByBusiness.Set(ctx, business, id); err != nil {
		return nil, err
	}
	return &types.MsgAdmitJewelerResponse{Id: id}, nil
}

// SuspendJeweler stops a jeweler acting. Its jewels and histories stay.
func (k msgServer) SuspendJeweler(ctx context.Context, req *types.MsgSuspendJeweler) (*types.MsgSuspendJewelerResponse, error) {
	if err := k.requireOffice(ctx, req.Office); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, types.ErrNoReason
	}
	j, err := k.getJeweler(ctx, req.JewelerId)
	if err != nil {
		return nil, err
	}
	j.Status = types.JEWELER_SUSPENDED
	return &types.MsgSuspendJewelerResponse{}, k.Jeweler.Set(ctx, j.Id, j)
}

// ReinstateJeweler lets a suspended jeweler act again.
func (k msgServer) ReinstateJeweler(ctx context.Context, req *types.MsgReinstateJeweler) (*types.MsgReinstateJewelerResponse, error) {
	if err := k.requireOffice(ctx, req.Office); err != nil {
		return nil, err
	}
	j, err := k.getJeweler(ctx, req.JewelerId)
	if err != nil {
		return nil, err
	}
	j.Status = types.JEWELER_ACTIVE
	return &types.MsgReinstateJewelerResponse{}, k.Jeweler.Set(ctx, j.Id, j)
}

// SetStaff adds and removes the people who act for a jeweler. Allowed while
// suspended, because removing someone matters most then.
func (k msgServer) SetStaff(ctx context.Context, req *types.MsgSetStaff) (*types.MsgSetStaffResponse, error) {
	j, err := k.getJeweler(ctx, req.JewelerId)
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
	for _, raw := range req.Remove {
		s, err := k.canonical(raw)
		if err != nil {
			return nil, err
		}
		if got, err := k.ByStaff.Get(ctx, s); err == nil && got == j.Id {
			if err := k.ByStaff.Remove(ctx, s); err != nil {
				return nil, err
			}
		}
		j.Staff = slices.DeleteFunc(j.Staff, func(x string) bool { return x == s })
	}
	for _, raw := range req.Add {
		s, err := k.canonical(raw)
		if err != nil {
			return nil, types.ErrInvalidAddress.Wrapf("staff %s", raw)
		}
		got, err := k.ByStaff.Get(ctx, s)
		switch {
		case err == nil && got == j.Id:
			continue
		case err == nil:
			return nil, types.ErrStaffTaken.Wrapf("%s", s)
		case !isNotFound(err):
			return nil, err
		}
		if err := k.ByStaff.Set(ctx, s, j.Id); err != nil {
			return nil, err
		}
		j.Staff = append(j.Staff, s)
	}
	return &types.MsgSetStaffResponse{}, k.Jeweler.Set(ctx, j.Id, j)
}
