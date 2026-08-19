package keeper

import (
	"errors"

	"yamale/blockchain/x/land/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns the module's Msg service implementation.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

// errorsIs is wrapped so the keeper files read the same whether the error came
// from collections or from a wrapped module error.
func errorsIs(err, target error) bool {
	return errors.Is(err, target)
}
