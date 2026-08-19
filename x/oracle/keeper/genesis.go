package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/oracle/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
//
// Votes are deliberately absent from genesis and are not restored: a voting
// round that was open when the chain stopped describes a moment that has
// passed, and resuming it would agree a price on reports nobody currently
// stands behind.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	for _, rate := range genState.ExchangeRates {
		if err := k.ExchangeRate.Set(ctx, rate.Denom, rate); err != nil {
			return err
		}
	}
	for _, appraiser := range genState.Appraisers {
		if err := k.Appraiser.Set(ctx, appraiser.Address, appraiser); err != nil {
			return err
		}
	}
	for _, appraisal := range genState.Appraisals {
		if err := k.Appraisal.Set(ctx, collections.Join(appraisal.ClassId, appraisal.NftId), appraisal); err != nil {
			return err
		}
	}
	for _, delegation := range genState.FeederDelegations {
		if err := k.Feeder.Set(ctx, delegation.Validator, delegation.Feeder); err != nil {
			return err
		}
	}
	for _, counter := range genState.MissCounters {
		if err := k.MissCounter.Set(ctx, counter.Validator, counter); err != nil {
			return err
		}
	}

	// History sequences are rebuilt rather than carried. They are contiguous per
	// asset, so counting reproduces exactly the keys that were exported — and a
	// counter carried in genesis that disagreed with the entries would either
	// overwrite a superseded valuation or leave a gap in the record.
	//
	// Counted against a plain struct rather than a collections.Pair: Pair holds
	// pointers, so two Joins of the same asset are different Go map keys and
	// every entry would be numbered 1, silently overwriting the asset's history
	// down to its last revaluation.
	// The counter is rewritten as each entry is placed rather than in a second
	// pass over the map, so the writes happen in the order the genesis file
	// lists them on every node.
	type asset struct{ classID, nftID string }
	seq := map[asset]uint64{}
	for _, appraisal := range genState.AppraisalHistory {
		key := asset{appraisal.ClassId, appraisal.NftId}
		seq[key]++

		if err := k.AppraisalHistory.Set(ctx, collections.Join3(appraisal.ClassId, appraisal.NftId, seq[key]), appraisal); err != nil {
			return err
		}
		if err := k.AppraisalSeq.Set(ctx, collections.Join(appraisal.ClassId, appraisal.NftId), seq[key]); err != nil {
			return err
		}
	}

	return nil
}

// ExportGenesis returns the module's exported genesis.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	genesis := types.DefaultGenesis()

	var err error
	genesis.Params, err = k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	if err := k.ExchangeRate.Walk(ctx, nil, func(_ string, v types.ExchangeRate) (bool, error) {
		genesis.ExchangeRates = append(genesis.ExchangeRates, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Appraiser.Walk(ctx, nil, func(_ string, v types.Appraiser) (bool, error) {
		genesis.Appraisers = append(genesis.Appraisers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Appraisal.Walk(ctx, nil, func(_ collections.Pair[string, string], v types.Appraisal) (bool, error) {
		genesis.Appraisals = append(genesis.Appraisals, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Feeder.Walk(ctx, nil, func(validator string, feeder string) (bool, error) {
		genesis.FeederDelegations = append(genesis.FeederDelegations, types.FeederDelegation{
			Validator: validator,
			Feeder:    feeder,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.MissCounter.Walk(ctx, nil, func(_ string, v types.MissCounter) (bool, error) {
		genesis.MissCounters = append(genesis.MissCounters, v)
		return false, nil
	}); err != nil {
		return nil, err
	}

	// Walked in key order, so the sequence numbers rebuilt on import land on the
	// same entries they came from.
	if err := k.AppraisalHistory.Walk(ctx, nil, func(_ collections.Triple[string, string, uint64], v types.Appraisal) (bool, error) {
		genesis.AppraisalHistory = append(genesis.AppraisalHistory, v)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return genesis, nil
}
