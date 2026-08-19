package types

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	params := DefaultParams()
	return &GenesisState{
		Params: params,
		EmissionState: &EmissionState{
			CurrentProvisionsPerBlock: params.GenesisProvisionsPerBlock,
			LastReductionPeriod:       0,
		},
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	return gs.Params.Validate()
}
