package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/enforcement/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
//
// Freezes are restored exactly as exported, along with the queues that expire
// them. A chain that rebuilt freezes from cases would be guessing, and a chain
// that dropped them would hand every frozen account back at the next upgrade —
// quietly, at the one moment nobody is watching balances.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	// Validated here and not only in `genesis validate`, because the two are
	// not the same gate. `genesis validate` is a command an operator may or may
	// not run against the file they actually distributed; this runs in
	// InitChain, on the bytes the chain is really starting from. Returning an
	// error here stops the chain from starting at all, which for a module that
	// can take somebody's assets is the correct response to a genesis that
	// does not say where those assets would go.
	if err := genState.Validate(); err != nil {
		return fmt.Errorf("enforcement genesis is invalid, refusing to start: %w", err)
	}

	// And checked against the constitution, on the same reasoning one step
	// further. A genesis whose seizure threshold disagrees with the one the
	// chain says it fixed is a chain with two answers to the only question that
	// matters about this module, and it is better for it not to start than for
	// the answer to depend on which module somebody happened to read.
	inv, err := k.constitutionKeeper.GetInvariants(ctx)
	if err != nil {
		return fmt.Errorf("enforcement genesis cannot be checked against this chain's constitution, refusing to start: %w", err)
	}
	if err := genState.Params.AssertConstitutional(inv); err != nil {
		return fmt.Errorf("enforcement genesis disagrees with this chain's constitution, refusing to start: %w", err)
	}

	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	for _, enforcementCase := range genState.Cases {
		if err := k.Case.Set(ctx, enforcementCase.Id, enforcementCase); err != nil {
			return err
		}
		// Only a case still being voted on belongs in the queue. Putting a
		// resolved one back would resolve it a second time at the height it
		// originally ended.
		if enforcementCase.Status == types.CASE_STATUS_VOTING {
			if err := k.VotingQueue.Set(ctx, collections.Join(enforcementCase.VotingEndsAtHeight, enforcementCase.Id)); err != nil {
				return err
			}
		}
		if enforcementCase.Action == types.CASE_ACTION_SEIZE && !enforcementCase.Recovered.IsZero() {
			if err := k.addRecovered(ctx, enforcementCase.Recovered); err != nil {
				return err
			}
		}
		if enforcementCase.Status == types.CASE_STATUS_PASSED {
			passed, err := k.CasesPassed.Get(ctx)
			if err != nil && !isNotFound(err) {
				return err
			}
			if err := k.CasesPassed.Set(ctx, passed+1); err != nil {
				return err
			}
		}
	}

	for _, vote := range genState.Votes {
		if err := k.Vote.Set(ctx, collections.Join(vote.CaseId, vote.Validator), vote); err != nil {
			return err
		}
	}

	for _, freeze := range genState.Freezes {
		if err := k.Freeze.Set(ctx, freeze.Address, freeze); err != nil {
			return err
		}
		if freeze.ExpiresAtHeight > 0 {
			if err := k.FreezeExpiryQueue.Set(ctx, collections.Join(freeze.ExpiresAtHeight, freeze.Address)); err != nil {
				return err
			}
		}
	}

	count := genState.CaseCount
	if count == 0 {
		// A genesis written before ids started at one, or by hand. The first
		// case must still not be case zero.
		count = 1
	}
	return k.CaseSeq.Set(ctx, count)
}

// ExportGenesis returns the module's exported genesis.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	genesis := types.DefaultGenesis()
	genesis.Params = params

	if err := k.Case.Walk(ctx, nil, func(_ uint64, enforcementCase types.Case) (bool, error) {
		genesis.Cases = append(genesis.Cases, enforcementCase)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if err := k.Vote.Walk(ctx, nil, func(_ collections.Pair[uint64, string], vote types.Vote) (bool, error) {
		genesis.Votes = append(genesis.Votes, vote)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if err := k.Freeze.Walk(ctx, nil, func(_ string, freeze types.Freeze) (bool, error) {
		genesis.Freezes = append(genesis.Freezes, freeze)
		return false, nil
	}); err != nil {
		return nil, err
	}

	// Peeked, not consumed: exporting must not change what the next case would
	// be numbered.
	count, err := k.CaseSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	genesis.CaseCount = count

	return genesis, nil
}

// TotalRecovered sums what the module has taken, across every case.
func (k Keeper) TotalRecovered(ctx context.Context) (sdk.Coins, error) {
	total := sdk.NewCoins()
	err := k.Recovered.Walk(ctx, nil, func(denom string, amount math.Int) (bool, error) {
		if amount.IsPositive() {
			total = total.Add(sdk.NewCoin(denom, amount))
		}
		return false, nil
	})
	return total, err
}

func isNotFound(err error) bool {
	return errors.Is(err, collections.ErrNotFound)
}
