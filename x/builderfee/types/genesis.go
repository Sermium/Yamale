package types

import "fmt"

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:                DefaultParams(),
		BuilderApplicationMap: []BuilderApplication{}, ApprovedBuilderMap: []ApprovedBuilder{}}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	builderApplicationIndexMap := make(map[string]struct{})

	for _, elem := range gs.BuilderApplicationMap {
		index := fmt.Sprint(elem.MsgTypeUrl)
		if _, ok := builderApplicationIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for builderApplication")
		}
		builderApplicationIndexMap[index] = struct{}{}
	}
	approvedBuilderIndexMap := make(map[string]struct{})

	for _, elem := range gs.ApprovedBuilderMap {
		index := fmt.Sprint(elem.MsgTypeUrl)
		if _, ok := approvedBuilderIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for approvedBuilder")
		}
		approvedBuilderIndexMap[index] = struct{}{}
	}

	return gs.Params.Validate()
}
