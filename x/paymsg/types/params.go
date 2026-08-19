package types

// NewParams creates a new Params instance.
func NewParams() Params {
	return Params{}
}

// DefaultParams returns a default set of parameters.
//
// require_settlement_jurisdiction defaults off, and the default is what a new
// chain starts with as much as it is what an existing one keeps. A deployment
// already holds payments that named no jurisdiction; starting the requirement
// on would refuse them on replay, and a node that cannot replay its own history
// cannot sync.
func DefaultParams() Params {
	return NewParams()
}

// Validate validates the set of params.
func (p Params) Validate() error {

	return nil
}
