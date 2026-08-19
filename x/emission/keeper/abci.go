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
	}

	provisions, ok := math.NewIntFromString(state.CurrentProvisionsPerBlock)
	if !ok {
		return types.ErrInvalidState
	}
	if provisions.IsPositive() {
		coins := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, provisions))
		if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
			return err
		}
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, authtypes.FeeCollectorName, coins); err != nil {
			return err
		}
	}

	return k.EmissionState.Set(ctx, state)
}
