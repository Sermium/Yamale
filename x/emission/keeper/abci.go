package keeper

import (
	"context"

	"yamale/blockchain/x/emission/types"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// BeginBlocker mints this block's native token emission and sends it to the
// fee collector, exactly where x/mint used to send newly-minted coins, so
// distribution allocates it to validators/community pool as usual. Unlike
// x/mint's bonded-ratio feedback loop, emission follows a fixed decaying
// schedule: provisions_per_block is cut by reduction_factor every
// reduction_period_in_blocks, so total supply approaches a fixed asymptote
// over time rather than growing indefinitely.
func (k Keeper) BeginBlocker(ctx context.Context) error {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}
	state, err := k.EmissionState.Get(ctx)
	if err != nil {
		return err
	}

	// The same guard the oracle needs, for the same reason: Params.Validate()
	// rejects a zero reduction period, but a genesis file edited after
	// `validate-genesis` reaches this line unvalidated, and an integer division
	// by zero is a panic rather than an error. The chain would not produce its
	// first block. Treating zero as "never reduce" keeps emission running on its
	// genesis provisions until governance fixes the parameter.
	currentPeriod := uint64(0)
	height := sdk.UnwrapSDKContext(ctx).BlockHeight()
	if params.ReductionPeriodInBlocks > 0 {
		currentPeriod = uint64(height) / params.ReductionPeriodInBlocks
	}

	// Whether this block changed anything, so an unchanged state is not
	// rewritten. See the note above the Set at the end.
	changed := false

	if currentPeriod > state.LastReductionPeriod {
		provisions, ok := math.NewIntFromString(state.CurrentProvisionsPerBlock)
		if !ok {
			return types.ErrInvalidState
		}
		factor, err := math.LegacyNewDecFromStr(params.ReductionFactor)
		if err != nil {
			return err
		}

		// Applied one period at a time so the result matches what the chain
		// would have computed had it passed through each period in turn.
		//
		// The loop runs once per elapsed period, and governance shortening the
		// reduction period can make that jump by millions at once. Measured, five
		// million iterations cost 0.37s — a slow block, not a stalled chain — so
		// the early exit is an efficiency guard rather than a fix for a denial of
		// service. It is exact, not an approximation: once provisions reach zero
		// every further multiplication returns zero.
		elapsed := currentPeriod - state.LastReductionPeriod
		reduced := math.LegacyNewDecFromInt(provisions)
		for i := uint64(0); i < elapsed; i++ {
			if reduced.IsZero() {
				break
			}
			reduced = reduced.Mul(factor)
		}

		state.CurrentProvisionsPerBlock = reduced.TruncateInt().String()
		state.LastReductionPeriod = currentPeriod
		changed = true
	}

	provisions, ok := math.NewIntFromString(state.CurrentProvisionsPerBlock)
	if !ok {
		return types.ErrInvalidState
	}
	if provisions.IsPositive() {
		// Asked of x/staking, not read from sdk.DefaultBondDenom.
		//
		// That global is a package-level variable rewritten by app/config.go's
		// init(), so minting against it was correct only for as long as the app
		// package happened to be linked in — and it would go on being minted
		// after a governance change to bond_denom, quietly filling the fee
		// collector with a denomination nobody stakes.
		bondDenom, err := k.stakingKeeper.BondDenom(ctx)
		if err != nil {
			return err
		}
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, provisions))
		if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
			return err
		}
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, authtypes.FeeCollectorName, coins); err != nil {
			return err
		}
	}

	// Written only when this block changed it.
	//
	// This used to be unconditional, and on a chain whose provisions have
	// reached zero it rewrote byte-identical state on every block for the rest
	// of the chain's life. Measured on devnet-2: provisions have been 0 since
	// reduction period 4,522, and this line still ran 450,000 times.
	//
	// It is not free. A write per block is a write per block — but the reason it
	// matters more than its size is that it is the kind of thing that makes a
	// chain unable to go quiet. An app that touches its store every block
	// changes its AppHash every block, and CometBFT then has to produce a block
	// to commit the new hash even when nobody sent a transaction, which defeats
	// `create_empty_blocks = false` entirely.
	//
	// Fixing this alone does NOT make the chain idle: x/slashing writes a
	// liveness index for every validator on every block, which is SDK
	// behaviour and dominates. Measured with this module's own state stable, the
	// AppHash still changed on every empty block. So this is one unnecessary
	// write removed rather than the door to an idle chain — but leaving a
	// pointless write in a BeginBlocker because something else also writes is
	// how a chain ends up with several.
	if !changed {
		return nil
	}
	return k.EmissionState.Set(ctx, state)
}
