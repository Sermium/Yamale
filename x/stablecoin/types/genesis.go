package types

import "fmt"

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:               DefaultParams(),
		IssuerApplicationMap: []IssuerApplication{}, ApprovedIssuerMap: []ApprovedIssuer{}}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	issuerApplicationIndexMap := make(map[string]struct{})

	for _, elem := range gs.IssuerApplicationMap {
		index := fmt.Sprint(elem.Denom)
		if _, ok := issuerApplicationIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for issuerApplication")
		}
		issuerApplicationIndexMap[index] = struct{}{}
	}
	approvedIssuerIndexMap := make(map[string]struct{})

	for _, elem := range gs.ApprovedIssuerMap {
		index := fmt.Sprint(elem.Denom)
		if _, ok := approvedIssuerIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for approvedIssuer")
		}
		approvedIssuerIndexMap[index] = struct{}{}
	}

	return gs.Params.Validate()
}
