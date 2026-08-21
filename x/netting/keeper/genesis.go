package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	"yamale/blockchain/x/netting/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
//
// Three pieces of state are rebuilt here rather than read from the file: the
// locked figure behind every reserve, the participant index over obligations,
// and the retry queue of held slices. All three are derived, and derived state
// that can also be imported is state that can be imported wrong.
//
// The locked figure is the one that would hurt. It is what stops a participant
// withdrawing collateral that is already backing an unsettled position, so a
// genesis able to state it independently of the positions it is meant to cover
// would be a genesis able to unlock money that is committed — quietly, at the
// one moment nobody is watching balances.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	// Validated here and not only in `genesis validate`, because the two are
	// not the same gate. `genesis validate` is a command an operator may or may
	// not run against the file they actually distributed; this runs in
	// InitChain, on the bytes the chain is really starting from. A netting
	// state whose positions do not sum to zero, or whose debits exceed the
	// reserves behind them, cannot settle — and refusing to start is a far
	// better way to find that out than an end blocker holding every cycle.
	if err := genState.Validate(); err != nil {
		return fmt.Errorf("netting genesis is invalid, refusing to start: %w", err)
	}

	// Addresses are checked here rather than in Validate() because this is the
	// only place with an address codec: GenesisState.Validate() would have to
	// reach for the global bech32 configuration, which is set by the app and is
	// not set at all in a bare types test.
	//
	// The check is not cosmetic. Every participant string below becomes a store
	// key directly, and nothing on the settlement path decodes it again — so a
	// mistyped address in a hand-edited genesis produces a reserve credited to
	// an identifier no key can sign for, and a cycle that settles value into it
	// forever. There is no message that moves it back out.
	if err := k.assertGenesisAddresses(genState); err != nil {
		return fmt.Errorf("netting genesis is invalid, refusing to start: %w", err)
	}

	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	for _, cycle := range genState.Cycles {
		if err := k.Cycle.Set(ctx, cycle.Id, cycle); err != nil {
			return err
		}
		for _, outcome := range cycle.Outcomes {
			if outcome.Status != types.DENOM_STATUS_HELD {
				continue
			}
			if err := k.HeldSlice.Set(ctx, collections.Join(cycle.Id, outcome.Denom)); err != nil {
				return err
			}
		}
	}

	for _, obligation := range genState.Obligations {
		if err := k.Obligation.Set(ctx, collections.Join(obligation.CycleId, obligation.Id), obligation); err != nil {
			return err
		}
		if err := k.ObligationByParticipant.Set(ctx, collections.Join3(obligation.FromParticipant, obligation.CycleId, obligation.Id)); err != nil {
			return err
		}
		if err := k.ObligationByParticipant.Set(ctx, collections.Join3(obligation.ToParticipant, obligation.CycleId, obligation.Id)); err != nil {
			return err
		}
	}

	for _, reserve := range genState.Reserves {
		if err := k.setReserve(ctx, reserve.Participant, reserve.Denom, reserve.Amount); err != nil {
			return err
		}
	}

	// Positions are written and the locked figures accumulated in one pass, so
	// the two can never disagree about which positions exist.
	cycles := make(map[uint64]types.Cycle, len(genState.Cycles))
	for _, cycle := range genState.Cycles {
		cycles[cycle.Id] = cycle
	}
	locked := make(map[string]math.Int, len(genState.Positions))
	for _, position := range genState.Positions {
		if err := k.setPosition(ctx, position.CycleId, position.Denom, position.Participant, position.Amount); err != nil {
			return err
		}
		if !position.Amount.IsNegative() {
			continue
		}
		if !types.SliceUnsettled(cycles[position.CycleId], position.Denom) {
			continue
		}
		// Keyed by a plain concatenated string, never by a collections.Pair. A
		// Pair holds pointers, so two Pairs naming the same participant and
		// denom are different Go map keys and the grouping silently never
		// matches — which in this loop would mean every participant appearing
		// to owe only its last position.
		key := position.Participant + "\x00" + position.Denom
		running, ok := locked[key]
		if !ok {
			running = math.ZeroInt()
		}
		locked[key] = running.Add(position.Amount.Neg())
	}
	// Iterated as a map, which is safe here for the one reason it is not safe
	// in the settlement path: every entry is written to its own key and no
	// entry's value depends on the order the others were applied in. InitGenesis
	// runs identically on every node regardless of the sequence.
	for key, amount := range locked {
		participant, denom, ok := splitLockedKey(key)
		if !ok {
			return fmt.Errorf("malformed locked key %q", key)
		}
		if err := k.setLocked(ctx, participant, denom, amount); err != nil {
			return err
		}
	}

	if err := k.CurrentCycle.Set(ctx, genState.CurrentCycle); err != nil {
		return err
	}
	if err := k.CycleSeq.Set(ctx, genState.CycleCount); err != nil {
		return err
	}
	return k.ObligationSeq.Set(ctx, genState.ObligationCount)
}

// ExportGenesis returns the module's exported genesis.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	genesis := types.DefaultGenesis()
	genesis.Params = params
	// Cleared rather than appended to: DefaultGenesis carries the window a
	// fresh chain opens with, and exporting a running chain on top of it would
	// emit a cycle 1 that no longer looks like the one in state.
	genesis.Cycles = nil

	if err := k.Cycle.Walk(ctx, nil, func(_ uint64, cycle types.Cycle) (bool, error) {
		genesis.Cycles = append(genesis.Cycles, cycle)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if err := k.Obligation.Walk(ctx, nil, func(_ collections.Pair[uint64, uint64], obligation types.Obligation) (bool, error) {
		genesis.Obligations = append(genesis.Obligations, obligation)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if err := k.Position.Walk(ctx, nil, func(key collections.Triple[uint64, string, string], amount math.Int) (bool, error) {
		genesis.Positions = append(genesis.Positions, types.Position{
			CycleId:     key.K1(),
			Denom:       key.K2(),
			Participant: key.K3(),
			Amount:      amount,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}

	if err := k.Reserve.Walk(ctx, nil, func(key collections.Pair[string, string], amount math.Int) (bool, error) {
		genesis.Reserves = append(genesis.Reserves, types.Reserve{
			Participant: key.K1(),
			Denom:       key.K2(),
			Amount:      amount,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}

	current, err := k.CurrentCycle.Get(ctx)
	if err != nil {
		return nil, err
	}
	genesis.CurrentCycle = current

	// Peeked, not consumed: exporting must not change what the next window or
	// the next obligation would be numbered.
	cycleCount, err := k.CycleSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	genesis.CycleCount = cycleCount

	obligationCount, err := k.ObligationSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	genesis.ObligationCount = obligationCount

	return genesis, nil
}

// assertGenesisAddresses refuses a genesis naming a participant that is not an
// address, on either side of an obligation, in a position or in a reserve.
func (k Keeper) assertGenesisAddresses(genState types.GenesisState) error {
	check := func(what, participant string) error {
		if _, err := k.addressCodec.StringToBytes(participant); err != nil {
			return fmt.Errorf("%s names %q, which is not an address: %w", what, participant, err)
		}
		return nil
	}

	for _, obligation := range genState.Obligations {
		if err := check(fmt.Sprintf("obligation %d", obligation.Id), obligation.FromParticipant); err != nil {
			return err
		}
		if err := check(fmt.Sprintf("obligation %d", obligation.Id), obligation.ToParticipant); err != nil {
			return err
		}
	}
	for _, position := range genState.Positions {
		if err := check(fmt.Sprintf("position in cycle %d", position.CycleId), position.Participant); err != nil {
			return err
		}
	}
	for _, reserve := range genState.Reserves {
		if err := check(fmt.Sprintf("reserve in %s", reserve.Denom), reserve.Participant); err != nil {
			return err
		}
	}
	return nil
}

// splitLockedKey undoes the concatenation above. The separator is a NUL byte,
// which bech32 addresses and denominations both exclude, so the split is
// unambiguous for every value this module can hold.
func splitLockedKey(key string) (participant, denom string, ok bool) {
	for i := range len(key) {
		if key[i] == 0 {
			return key[:i], key[i+1:], true
		}
	}
	return "", "", false
}
