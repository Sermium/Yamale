package types_test

import (
	"math"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

// The default has to be the behaviour the chain already has. Turning a live
// payment system from immediate settlement to deferred settlement at an upgrade
// is a change to the credit risk every participant is running, and it is not
// something a binary should decide for them.
func TestDefaultParamsLeaveNettingOff(t *testing.T) {
	params := types.DefaultParams()
	require.NoError(t, params.Validate())
	require.False(t, params.NettingEnabled())
	require.Empty(t, params.DenomPolicies)
}

// cycle_blocks is a divisor. NettingEnabled is the guard in front of the
// modulo, so it has to refuse both ends: zero, and any value that cannot be
// compared against an int64 block height.
func TestNettingEnabledGuardsTheDivisor(t *testing.T) {
	require.False(t, types.Params{CycleBlocks: 0}.NettingEnabled(),
		"zero would panic the end blocker and stop the chain")
	require.True(t, types.Params{CycleBlocks: 1}.NettingEnabled())
	require.True(t, types.Params{CycleBlocks: types.MaxCycleBlocks}.NettingEnabled())
	require.False(t, types.Params{CycleBlocks: types.MaxCycleBlocks + 1}.NettingEnabled(),
		"a window beyond the bound is one the chain would never close")
	require.False(t, types.Params{CycleBlocks: math.MaxUint64}.NettingEnabled(),
		"a value that converts to a negative int64 is a window that never closes")
}

func TestParamsValidateRejectsAnOverlongWindow(t *testing.T) {
	require.ErrorContains(t,
		types.Params{CycleBlocks: types.MaxCycleBlocks + 1}.Validate(),
		"exceeds the maximum")
}

// Two policies for one currency is not a merge problem, it is an ambiguity:
// whichever the lookup found first would decide what nets.
func TestParamsValidateRejectsDuplicatePolicies(t *testing.T) {
	params := types.Params{
		CycleBlocks: 10,
		DenomPolicies: []types.DenomPolicy{
			{Denom: "ueur", GrossThreshold: sdkmath.NewInt(1)},
			{Denom: "ueur", GrossThreshold: sdkmath.NewInt(2)},
		},
	}
	require.ErrorContains(t, params.Validate(), "two denom policies")
}

func TestParamsValidateRejectsANegativeThreshold(t *testing.T) {
	params := types.Params{
		CycleBlocks:   10,
		DenomPolicies: []types.DenomPolicy{{Denom: "ueur", GrossThreshold: sdkmath.NewInt(-1)}},
	}
	require.ErrorContains(t, params.Validate(), "negative gross_threshold")
}

// "Governance set the threshold to zero" and "governance has never considered
// this currency" both settle gross today, but only one of them is a decision,
// and a caller must be able to tell them apart.
func TestGrossThresholdForDistinguishesUnsetFromZero(t *testing.T) {
	params := types.Params{
		CycleBlocks:   10,
		DenomPolicies: []types.DenomPolicy{{Denom: "ueur", GrossThreshold: sdkmath.ZeroInt()}},
	}

	threshold, configured := params.GrossThresholdFor("ueur")
	require.True(t, configured)
	require.True(t, threshold.IsZero())

	_, configured = params.GrossThresholdFor("ungn")
	require.False(t, configured)
}

// The link from an interbank figure back to the items behind it is the
// auditability a netted system gives up if it is optional.
func TestValidateObligationFields(t *testing.T) {
	good := make([]byte, types.BatchHashLength)

	require.NoError(t, types.ValidateObligationFields(bankA, bankB, "ueur", sdkmath.NewInt(1), good))

	require.ErrorIs(t,
		types.ValidateObligationFields(bankA, bankA, "ueur", sdkmath.NewInt(1), good),
		types.ErrSelfObligation)
	require.ErrorIs(t,
		types.ValidateObligationFields(bankA, bankB, "ueur", sdkmath.ZeroInt(), good),
		types.ErrInvalidAmount)
	require.ErrorIs(t,
		types.ValidateObligationFields(bankA, bankB, "ueur", sdkmath.NewInt(-1), good),
		types.ErrInvalidAmount)
	require.ErrorIs(t,
		types.ValidateObligationFields(bankA, bankB, "ueur", sdkmath.NewInt(1), make([]byte, 31)),
		types.ErrInvalidBatchHash)
	require.ErrorIs(t,
		types.ValidateObligationFields(bankA, bankB, "NOT A DENOM", sdkmath.NewInt(1), good),
		types.ErrInvalidAmount)
}
