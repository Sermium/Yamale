package keeper

import (
	"context"

	"yamale/blockchain/x/selfcustody/types"
)

// InitGenesis writes the records in.
//
// Nothing here is derived. Both collections are primary facts — a disclosure is
// the date somebody's custody ended, a request is time a person has already
// served — and neither can be recomputed from anything else in the file.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}

	for _, d := range gs.Disclosures {
		if err := k.Disclosure.Set(ctx, d.Account, d); err != nil {
			return err
		}
	}
	for _, r := range gs.Requests {
		if err := k.Request.Set(ctx, r.Account, r); err != nil {
			return err
		}
	}
	return nil
}

// ExportGenesis reads them out.
//
// Requests are exported along with the disclosures. Dropping them at an upgrade
// would silently restart every waiting period in flight, which to the holder is
// indistinguishable from the operator stalling them — and the whole claim this
// module makes is that the operator cannot do that.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	gs := types.GenesisState{Params: params}

	if err := k.Disclosure.Walk(ctx, nil, func(_ string, d types.Disclosure) (bool, error) {
		gs.Disclosures = append(gs.Disclosures, d)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Request.Walk(ctx, nil, func(_ string, r types.Request) (bool, error) {
		gs.Requests = append(gs.Requests, r)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &gs, nil
}
