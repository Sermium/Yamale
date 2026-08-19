package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/emission/types"
)

// emissionDenom is whatever the chain's bond denom is at runtime; app/config.go
// sets it to uyml before the node starts, and the BeginBlocker mints that same
// denom so distribution treats emission exactly like x/mint's output.
var emissionDenom = sdk.DefaultBondDenom

// setSchedule replaces the emission params and resets state to the start of
// the schedule, using round numbers so the decay curve is easy to read.
func setSchedule(t *testing.T, f *fixture, periodBlocks uint64, factor, genesisProvisions string) {
	t.Helper()

	params := types.NewParams(periodBlocks, factor, genesisProvisions)
	require.NoError(t, params.Validate())
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	require.NoError(t, f.keeper.EmissionState.Set(f.ctx, types.EmissionState{
		CurrentProvisionsPerBlock: genesisProvisions,
		LastReductionPeriod:       0,
	}))
}

// beginBlock runs the emission BeginBlocker at the given height.
func beginBlock(t *testing.T, f *fixture, height int64) {
	t.Helper()
	f.env.Ctx = f.env.Ctx.WithBlockHeight(height)
	f.ctx = f.env.Ctx
	require.NoError(t, f.keeper.BeginBlocker(f.env.Ctx))
}

func TestBeginBlockerMintsToFeeCollector(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 10, "0.5", "1000")

	beginBlock(t, f, 1)

	// The whole emission lands in the fee collector, where distribution picks
	// it up, rather than sitting in the emission module account.
	require.Equal(t, math.NewInt(1_000), f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom))
	require.True(t, f.env.ModuleBalance(types.ModuleName, emissionDenom).IsZero())
	require.Equal(t, math.NewInt(1_000), f.env.Supply(emissionDenom))
}

func TestBeginBlockerEmitsEveryBlockWithinAPeriod(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 10, "0.5", "1000")

	for h := int64(1); h <= 9; h++ {
		beginBlock(t, f, h)
	}

	require.Equal(t, math.NewInt(9_000), f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom))

	state, err := f.keeper.EmissionState.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "1000", state.CurrentProvisionsPerBlock, "no reduction before the period boundary")
	require.Equal(t, uint64(0), state.LastReductionPeriod)
}

func TestBeginBlockerReducesAtPeriodBoundary(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 10, "0.5", "1000")

	beginBlock(t, f, 9)
	require.Equal(t, math.NewInt(1_000), f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom))

	// Height 10 opens period 1: provisions halve before this block's mint.
	beginBlock(t, f, 10)
	require.Equal(t, math.NewInt(1_500), f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom))

	state, err := f.keeper.EmissionState.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "500", state.CurrentProvisionsPerBlock)
	require.Equal(t, uint64(1), state.LastReductionPeriod)

	// Height 20 opens period 2.
	beginBlock(t, f, 20)
	require.Equal(t, math.NewInt(1_750), f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom))

	state, err = f.keeper.EmissionState.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "250", state.CurrentProvisionsPerBlock)
	require.Equal(t, uint64(2), state.LastReductionPeriod)
}

// A node that skips blocks (or a chain resuming after a halt) must land on the
// same provisions as one that ran every block, not on the previous period's.
func TestBeginBlockerCatchesUpAcrossSkippedPeriods(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 10, "0.5", "1000")

	// Jump straight from period 0 to period 3.
	beginBlock(t, f, 30)

	state, err := f.keeper.EmissionState.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "125", state.CurrentProvisionsPerBlock, "1000 * 0.5^3")
	require.Equal(t, uint64(3), state.LastReductionPeriod)
	require.Equal(t, math.NewInt(125), f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom))
}

func TestBeginBlockerReductionIsMonotonic(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 10, "0.9", "1000000")

	var previous math.Int
	for period := int64(0); period < 8; period++ {
		before := f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom)
		beginBlock(t, f, period*10)
		minted := f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom).Sub(before)

		if period > 0 {
			require.True(t, minted.LT(previous),
				"period %d minted %s, not less than the previous %s", period, minted, previous)
		}
		previous = minted
	}
}

// Once provisions truncate to zero the schedule is exhausted and the chain
// stops inflating: total supply must stop moving.
func TestBeginBlockerStopsMintingWhenProvisionsReachZero(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 1, "0.5", "4")

	// One block per period: 4 -> 2 -> 1 -> 0.
	for h := int64(1); h <= 6; h++ {
		beginBlock(t, f, h)
	}

	state, err := f.keeper.EmissionState.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "0", state.CurrentProvisionsPerBlock)

	supplyAtExhaustion := f.env.Supply(emissionDenom)
	for h := int64(7); h <= 12; h++ {
		beginBlock(t, f, h)
	}
	require.Equal(t, supplyAtExhaustion, f.env.Supply(emissionDenom),
		"supply must be capped once the emission schedule is exhausted")
}

// A factor of 1.0 is a valid, if unusual, configuration: constant emission
// forever. It must not reduce.
func TestBeginBlockerConstantEmission(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 10, "1.0", "1000")

	beginBlock(t, f, 10)
	beginBlock(t, f, 20)

	state, err := f.keeper.EmissionState.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "1000", state.CurrentProvisionsPerBlock)
	require.Equal(t, math.NewInt(2_000), f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom))
}

// A factor of 0 halts emission at the first boundary.
func TestBeginBlockerZeroFactorHaltsEmission(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 10, "0", "1000")

	beginBlock(t, f, 1)
	require.Equal(t, math.NewInt(1_000), f.env.Supply(emissionDenom))

	beginBlock(t, f, 10)
	require.Equal(t, math.NewInt(1_000), f.env.Supply(emissionDenom), "no further emission after the factor zeroes out")

	state, err := f.keeper.EmissionState.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "0", state.CurrentProvisionsPerBlock)
}

func TestBeginBlockerRejectsCorruptState(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 10, "0.5", "1000")

	require.NoError(t, f.keeper.EmissionState.Set(f.ctx, types.EmissionState{
		CurrentProvisionsPerBlock: "not-a-number",
		LastReductionPeriod:       0,
	}))

	f.env.Ctx = f.env.Ctx.WithBlockHeight(1)
	err := f.keeper.BeginBlocker(f.env.Ctx)
	require.ErrorIs(t, err, types.ErrInvalidState)
	require.True(t, f.env.Supply(emissionDenom).IsZero())
}

// The default mainnet-style schedule converges on the documented asymptote:
// genesis provisions per block, summed over a geometric series with ratio r,
// approaches provisions * period / (1 - r).
func TestDefaultScheduleConvergesToDocumentedTotal(t *testing.T) {
	params := types.DefaultParams()
	require.NoError(t, params.Validate())

	provisions, ok := math.NewIntFromString(params.GenesisProvisionsPerBlock)
	require.True(t, ok)
	factor := math.LegacyMustNewDecFromStr(params.ReductionFactor)

	// Per-period emission is provisions * blocks-per-period; the total is that
	// first term divided by (1 - r).
	firstPeriod := math.LegacyNewDecFromInt(provisions).MulInt64(int64(params.ReductionPeriodInBlocks))
	total := firstPeriod.Quo(math.LegacyOneDec().Sub(factor)).TruncateInt()

	// ~1,000,000,000 YML, i.e. 1e15 in the 6-decimal base unit.
	target := math.NewInt(1_000_000_000_000_000)
	diff := total.Sub(target).Abs()
	require.True(t, diff.LT(math.NewInt(1_000_000_000)),
		"asymptotic total %s is not within tolerance of the documented %s", total, target)
}

// A genesis file edited after `validate-genesis` — which is what every launch
// ceremony does — can carry a zero reduction period. The BeginBlocker divides
// the height by it on the first block, and an integer division by zero is a
// panic rather than an error: the chain never produces a block, and there is no
// chain left to send a corrective transaction to.
//
// Params.Validate() rejects zero, which is precisely why the guard belongs
// here: the one path that reaches this line is the one with no validation in
// front of it.
func TestAZeroReductionPeriodMustNotHaltTheChain(t *testing.T) {
	f := initFixture(t)

	require.NoError(t, f.keeper.Params.Set(f.ctx, types.Params{
		ReductionPeriodInBlocks:   0,
		ReductionFactor:           "0.5",
		GenesisProvisionsPerBlock: "1000",
	}))
	require.NoError(t, f.keeper.EmissionState.Set(f.ctx, types.EmissionState{
		CurrentProvisionsPerBlock: "1000",
		LastReductionPeriod:       0,
	}))

	f.env.Ctx = f.env.Ctx.WithBlockHeight(1)
	f.ctx = f.env.Ctx

	require.NotPanics(t, func() {
		_ = f.keeper.BeginBlocker(f.env.Ctx)
	}, "a zero reduction period must stop the decay, not stop the chain")

	// Emission carries on at its genesis rate until governance fixes the
	// parameter, which is the conservative reading: minting nothing would
	// silently halve validator revenue.
	require.Equal(t, math.NewInt(1_000), f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom))
}

// Governance shortening the reduction period makes the number of periods to
// apply jump by however many of the new, shorter periods have already passed —
// which can be millions. The decay must still land on exactly the value the
// chain would have reached by passing through each period in turn, and the
// early exit in the loop must not change it.
//
// Measured at five million periods the loop costs 0.37s without the early exit,
// so this is a slow block rather than a halt; the assertion here is exactness,
// not a timing bound.
func TestALargePeriodJumpStaysExact(t *testing.T) {
	f := initFixture(t)
	setSchedule(t, f, 1, "0.5", "1000")

	// Height 5,000,000 with a one-block period is five million reductions of a
	// value that reaches zero after ten.
	beginBlock(t, f, 5_000_000)

	state, err := f.keeper.EmissionState.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "0", state.CurrentProvisionsPerBlock)
	require.Equal(t, uint64(5_000_000), state.LastReductionPeriod)
	require.True(t, f.env.ModuleBalance(authtypes.FeeCollectorName, emissionDenom).IsZero())
}
