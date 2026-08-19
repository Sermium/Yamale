package types

import (
	"fmt"

	"cosmossdk.io/math"
)

// Default emission schedule: a period is 100 blocks (short and fast so the
// reduction curve is actually observable on a testnet, rather than only
// over real-world years like a mainnet schedule would be). Provisions per
// block are cut to 2/3 of the previous period's value at each boundary.
// Since a geometric series with ratio r converges to genesis/(1-r), these
// defaults converge to an asymptotic total of ~1,000,000,000 YML
// (1e15 uyml) minted on top of whatever was allocated at genesis.
const (
	DefaultReductionPeriodInBlocks   = 100
	DefaultReductionFactor           = "0.666666666666666667"
	DefaultGenesisProvisionsPerBlock = "3333333333333"
)

// NewParams creates a new Params instance.
func NewParams(reductionPeriodInBlocks uint64, reductionFactor, genesisProvisionsPerBlock string) Params {
	return Params{
		ReductionPeriodInBlocks:   reductionPeriodInBlocks,
		ReductionFactor:           reductionFactor,
		GenesisProvisionsPerBlock: genesisProvisionsPerBlock,
	}
}

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	return NewParams(DefaultReductionPeriodInBlocks, DefaultReductionFactor, DefaultGenesisProvisionsPerBlock)
}

// Validate validates the set of params.
func (p Params) Validate() error {
	if p.ReductionPeriodInBlocks == 0 {
		return fmt.Errorf("reduction_period_in_blocks must be positive")
	}
	factor, err := math.LegacyNewDecFromStr(p.ReductionFactor)
	if err != nil {
		return fmt.Errorf("invalid reduction_factor %q: %w", p.ReductionFactor, err)
	}
	if factor.IsNegative() || factor.GT(math.LegacyOneDec()) {
		return fmt.Errorf("reduction_factor must be between 0 and 1, got %s", p.ReductionFactor)
	}
	if _, ok := math.NewIntFromString(p.GenesisProvisionsPerBlock); !ok {
		return fmt.Errorf("invalid genesis_provisions_per_block %q", p.GenesisProvisionsPerBlock)
	}
	return nil
}
