package types_test

import (
	"os"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/constitution/types"
)

// The account prefix, because Validate() decodes the recovery destination and
// the SDK default is "cosmos". Set here rather than by importing app for its
// init() side effect: this package's tests should not pull in the whole
// application to check arithmetic.
func TestMain(m *testing.M) {
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount("yml", "ymlpub")
	os.Exit(m.Run())
}

// liveSettlement is what devnet-2 is running, read off the chain on 2026-09-07.
// The amendment under discussion changes four of these and restates the rest,
// because MsgProposeAmendment takes the complete set rather than a delta.
func liveSettlement() types.Invariants {
	return types.Invariants{
		MaxEntityPowerBps:                  10_000,
		MaxBeneficialOwnerPowerBps:         10_000,
		MaxJurisdictionPowerBps:            10_000,
		ConcentrationEpochBlocks:           120,
		MinActiveValidators:                1,
		EnforcementThresholdBps:            6_667,
		EnforcementRecoveryDestination:     "yml1afk9zr2hn2jsac63h4hm60vl9z3e5u69gndzf7c99cqge3vzwjzs3xm8uj",
		EnforcementVotingPeriodBlocks:      360,
		EnforcementProvisionalFreezeBlocks: 720,
		AmendmentDelayBlocks:               120_960,
		AmendmentThresholdBps:              8_000,
		FoundationCustodianCount:           5,
		FoundationSignatureThreshold:       3,
	}
}

// A ceiling below one seat's worth of power is a contradiction, and proposal 12
// was exactly that: it asked for 3300 bps alongside a floor of two active
// validators, where one of two holds 5000 by arithmetic. It passed its vote and
// then failed at execution, which is the expensive way to find out — the vote
// runs for thirty minutes and the failure is only visible afterwards.
//
// This pins the relationship in both directions so the next proposal is checked
// here rather than on the chain.
func TestCeilingsMustBeReachableAtTheValidatorFloor(t *testing.T) {
	for _, tc := range []struct {
		name      string
		minActive uint32
		ceiling   uint64
		ok        bool
	}{
		{"proposal 12, as submitted and rejected", 2, 3_300, false},
		{"the tightest a floor of two can hold", 2, 5_000, true},
		{"one basis point under it", 2, 4_999, false},
		{"a third needs a floor of three", 3, 3_334, true},
		{"and 3333 does not reach it", 3, 3_333, false},
		{"a quarter needs a floor of four", 4, 2_500, true},
		{"a floor of one admits no ceiling at all", 1, 9_999, false},
		{"which is why the chain launched at 100%", 1, 10_000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := liveSettlement()
			inv.MinActiveValidators = tc.minActive
			inv.MaxEntityPowerBps = tc.ceiling
			inv.MaxBeneficialOwnerPowerBps = tc.ceiling
			inv.MaxJurisdictionPowerBps = tc.ceiling

			err := inv.Validate()
			if tc.ok {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), "could ever satisfy it")
		})
	}
}

// The settlement proposal 13 asks for, validated here before it is submitted.
func TestProposedSettlementIsValid(t *testing.T) {
	inv := liveSettlement()
	inv.MaxEntityPowerBps = 5_000
	inv.MaxBeneficialOwnerPowerBps = 5_000
	inv.MaxJurisdictionPowerBps = 5_000
	inv.MinActiveValidators = 2

	require.NoError(t, inv.Validate())
}

// And the settlement it replaces is valid too, or the chain would not be
// running it — which is the point of restating the nine unchanged fields rather
// than trusting that an omitted field means "leave alone". It does not: it
// means zero, and several of these are divisors.
func TestLiveSettlementIsValid(t *testing.T) {
	require.NoError(t, liveSettlement().Validate())
}
