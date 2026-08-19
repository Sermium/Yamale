package types

import "fmt"

// DefaultGenesis returns the default genesis state
//
// PoolCount starts at one for the same reason treasury ids do: in proto3 an id
// of zero cannot be told apart from a field that was never set, so pool 0 is an
// id a client reaches by leaving the field out rather than by choosing it.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:    DefaultParams(),
		PoolList:  []Pool{},
		PoolCount: 1,
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	poolIdMap := make(map[uint64]bool)
	poolCount := gs.GetPoolCount()
	for _, elem := range gs.PoolList {
		if _, ok := poolIdMap[elem.Id]; ok {
			return fmt.Errorf("duplicated id for pool")
		}
		if elem.Id >= poolCount {
			return fmt.Errorf("pool id should be lower or equal than the last id")
		}
		poolIdMap[elem.Id] = true
	}

	return gs.Params.Validate()
}
