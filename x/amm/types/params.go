package types

// NewParams creates a new Params instance.
func NewParams() Params {
	return Params{}
}

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	return NewParams()
}

// Validate validates the set of params.
func (p Params) Validate() error {

	return nil
}

// MaxSwapFeeBps caps a pool's trading fee at ten per cent.
//
// The fee is set once, by whoever opens the pool, and there is no mechanism to
// change it afterwards — so an unbounded value is permanent. Above 10,000 basis
// points the swap arithmetic goes negative outright; the cap here is far below
// that, because a pool taking more than a tenth of every trade is not a market
// anybody should be routed into by accident.
const MaxSwapFeeBps = 1000
