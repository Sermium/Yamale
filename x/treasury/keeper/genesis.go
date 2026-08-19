package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/treasury/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
//
// The indexes are rebuilt from the locks rather than carried in genesis: they
// are derived data, and regenerating them means an exported genesis can never
// disagree with itself.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	for _, t := range genState.TreasuryList {
		if err := k.Treasury.Set(ctx, t.Id, t); err != nil {
			return err
		}
	}
	// Seeded at one when a genesis predates the convention, so the first
	// treasury on any chain is id 1 rather than the id a client produces by
	// leaving the field out.
	if err := k.TreasurySeq.Set(ctx, atLeastOne(genState.TreasuryCount)); err != nil {
		return err
	}

	// The active-lock counter is derived, so it is rebuilt here rather than
	// carried in genesis: a count that disagreed with the locks would either
	// block a treasury below its cap or let it exceed it.
	activeLocks := map[uint64]uint64{}
	for _, l := range genState.LockList {
		if err := k.Lock.Set(ctx, l.Id, l); err != nil {
			return err
		}
		if err := k.LockByTreasury.Set(ctx, collections.Join(l.TreasuryId, l.Id)); err != nil {
			return err
		}
		if err := k.LockByBeneficiary.Set(ctx, collections.Join(l.Beneficiary, l.Id)); err != nil {
			return err
		}
		if l.Active {
			activeLocks[l.TreasuryId]++
		}
	}
	for treasuryID, count := range activeLocks {
		if err := k.ActiveLockCount.Set(ctx, treasuryID, count); err != nil {
			return err
		}
	}
	if err := k.LockSeq.Set(ctx, atLeastOne(genState.LockCount)); err != nil {
		return err
	}

	for _, b := range genState.BalanceList {
		if err := k.Balance.Set(ctx, collections.Join(b.TreasuryId, b.Denom), b); err != nil {
			return err
		}
	}
	for _, r := range genState.RoleList {
		if err := k.Role.Set(ctx, collections.Join(r.TreasuryId, r.Address), r); err != nil {
			return err
		}
	}
	for _, p := range genState.SpendPolicyList {
		if err := k.SpendPolicy.Set(ctx, collections.Join(p.TreasuryId, p.Denom), p); err != nil {
			return err
		}
	}
	for _, w := range genState.SpendWindowList {
		if err := k.SpendWindow.Set(ctx, collections.Join(w.TreasuryId, w.Denom), w); err != nil {
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

	if err := k.Treasury.Walk(ctx, nil, func(_ uint64, v types.Treasury) (bool, error) {
		genesis.TreasuryList = append(genesis.TreasuryList, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	genesis.TreasuryCount, err = k.peekSequence(ctx, k.TreasurySeq)
	if err != nil {
		return nil, err
	}

	if err := k.Lock.Walk(ctx, nil, func(_ uint64, v types.Lock) (bool, error) {
		genesis.LockList = append(genesis.LockList, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	genesis.LockCount, err = k.peekSequence(ctx, k.LockSeq)
	if err != nil {
		return nil, err
	}

	if err := k.Balance.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.TreasuryBalance) (bool, error) {
		genesis.BalanceList = append(genesis.BalanceList, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Role.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.RoleAssignment) (bool, error) {
		genesis.RoleList = append(genesis.RoleList, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.SpendPolicy.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.SpendPolicy) (bool, error) {
		genesis.SpendPolicyList = append(genesis.SpendPolicyList, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.SpendWindow.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.SpendWindow) (bool, error) {
		genesis.SpendWindowList = append(genesis.SpendWindowList, v)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return genesis, nil
}

// peekSequence reads a sequence's next value without consuming it.
func (k Keeper) peekSequence(ctx context.Context, seq collections.Sequence) (uint64, error) {
	v, err := seq.Peek(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

// atLeastOne keeps a sequence from ever handing out zero.
//
// Ids are numbered from one on this chain because proto3 cannot tell an id of
// zero from a field nobody set. A genesis written before that convention —
// or by hand — carries a count of zero, and importing it verbatim would hand
// the next treasury the id every buggy client can reach by omission.
func atLeastOne(count uint64) uint64 {
	if count == 0 {
		return 1
	}
	return count
}
