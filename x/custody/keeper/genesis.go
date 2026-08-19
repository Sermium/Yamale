package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/custody/types"
)

// InitGenesis writes the genesis state, rebuilding the derived indexes rather
// than reading them from the file.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, a := range gs.Assets {
		if err := k.Assets.Set(ctx, a.Denom, a); err != nil {
			return err
		}
	}
	for _, a := range gs.Attestors {
		if err := k.Attestors.Set(ctx, a); err != nil {
			return err
		}
	}
	for _, d := range gs.Deposits {
		if err := k.Deposits.Set(ctx, d.Id, d); err != nil {
			return err
		}
		// The replay guard is derived, not carried. A credited deposit implies
		// its reference is spent; storing that separately in genesis would be a
		// second copy of the same fact, able to disagree with the first.
		if d.Status == types.DepositStatus_DEPOSIT_STATUS_CREDITED {
			if err := k.ExternalRefs.Set(ctx, collections.Join(d.Denom, d.ExternalRef)); err != nil {
				return err
			}
		}
	}
	for _, r := range gs.Redemptions {
		if err := k.Redemptions.Set(ctx, r.Id, r); err != nil {
			return err
		}
	}
	for _, r := range gs.Reserves {
		if err := k.Reserves.Set(ctx, r.Denom, r); err != nil {
			return err
		}
	}
	if err := k.DepositSeq.Set(ctx, gs.NextDeposit); err != nil {
		return err
	}
	return k.RedeemSeq.Set(ctx, gs.NextRedemption)
}

// ExportGenesis reads the genesis state back out.
//
// Emits neither the attestation records nor the external-reference set: the
// first is per-deposit detail that a settled chain no longer needs, and the
// second is derived above. Export and import must round-trip byte-for-byte, and
// the way to guarantee that is to emit each fact exactly once.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	gs := types.GenesisState{
		Params:      params,
		Assets:      []types.Asset{},
		Attestors:   []string{},
		Deposits:    []types.Deposit{},
		Redemptions: []types.Redemption{},
		Reserves:    []types.Reserve{},
	}

	if err := k.Assets.Walk(ctx, nil, func(_ string, a types.Asset) (bool, error) {
		gs.Assets = append(gs.Assets, a)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Attestors.Walk(ctx, nil, func(a string) (bool, error) {
		gs.Attestors = append(gs.Attestors, a)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Deposits.Walk(ctx, nil, func(_ string, d types.Deposit) (bool, error) {
		gs.Deposits = append(gs.Deposits, d)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Redemptions.Walk(ctx, nil, func(_ string, r types.Redemption) (bool, error) {
		gs.Redemptions = append(gs.Redemptions, r)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Reserves.Walk(ctx, nil, func(_ string, r types.Reserve) (bool, error) {
		gs.Reserves = append(gs.Reserves, r)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if gs.NextDeposit, err = k.DepositSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.NextRedemption, err = k.RedeemSeq.Peek(ctx); err != nil {
		return nil, err
	}
	return &gs, nil
}
