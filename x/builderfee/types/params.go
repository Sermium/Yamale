package types

import "fmt"

// DefaultBuilderFeeShareBps is the default share of gas fees (30%) paid to
// registered dApp builders, out of 10000 basis points.
const DefaultBuilderFeeShareBps = 3000

// NewParams creates a new Params instance.
func NewParams(builderFeeShareBps uint64) Params {
	return Params{BuilderFeeShareBps: builderFeeShareBps}
}

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	return NewParams(DefaultBuilderFeeShareBps)
}

// Validate validates the set of params.
func (p Params) Validate() error {
	if p.BuilderFeeShareBps > 10000 {
		return fmt.Errorf("builder fee share bps cannot exceed 10000 (100%%), got %d", p.BuilderFeeShareBps)
	}
	return nil
}
