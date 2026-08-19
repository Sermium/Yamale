package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/paymsg/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	for _, elem := range genState.ParticipantApplicationMap {
		if err := k.ParticipantApplication.Set(ctx, elem.Creator, elem); err != nil {
			return err
		}
	}
	for _, elem := range genState.ApprovedParticipantMap {
		if err := k.ApprovedParticipant.Set(ctx, elem.Participant, elem); err != nil {
			return err
		}
	}
	for _, elem := range genState.PaymentRecordMap {
		if err := k.PaymentRecord.Set(ctx, collections.Join(elem.InstructingParticipant, elem.EndToEndId), elem); err != nil {
			return err
		}
	}
	for _, elem := range genState.CustomerMap {
		if err := k.Customer.Set(ctx, elem.Customer, elem); err != nil {
			return err
		}
	}

	return k.Params.Set(ctx, genState.Params)
}

// ExportGenesis returns the module's exported genesis.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	var err error

	genesis := types.DefaultGenesis()
	genesis.Params, err = k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := k.ParticipantApplication.Walk(ctx, nil, func(_ string, val types.ParticipantApplication) (stop bool, err error) {
		genesis.ParticipantApplicationMap = append(genesis.ParticipantApplicationMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.ApprovedParticipant.Walk(ctx, nil, func(_ string, val types.ApprovedParticipant) (stop bool, err error) {
		genesis.ApprovedParticipantMap = append(genesis.ApprovedParticipantMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.PaymentRecord.Walk(ctx, nil, func(_ collections.Pair[string, string], val types.PaymentRecord) (stop bool, err error) {
		genesis.PaymentRecordMap = append(genesis.PaymentRecordMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Customer.Walk(ctx, nil, func(_ string, val types.Customer) (stop bool, err error) {
		genesis.CustomerMap = append(genesis.CustomerMap, val)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return genesis, nil
}
