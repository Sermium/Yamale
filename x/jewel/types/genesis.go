package types

// DefaultGenesis returns an empty register with the default settings.
func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

// Validate refuses a genesis the module could not have produced.
func (gs GenesisState) Validate() error {
	return gs.Params.Validate()
}
