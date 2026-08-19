//go:build ibc

package app

import (
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	icacontrollerkeeper "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/controller/keeper"
	icahostkeeper "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/host/keeper"
	icatypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/types"
	ibctransferkeeper "github.com/cosmos/ibc-go/v10/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibcexported "github.com/cosmos/ibc-go/v10/modules/core/exported"
	ibckeeper "github.com/cosmos/ibc-go/v10/modules/core/keeper"
)

// The half of the IBC contract that exists when IBC is compiled in. See
// profile_ibc_off.go for why this is the opt-in side rather than the default.

var (
	ibcModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		{Account: ibctransfertypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner}},
		{Account: icatypes.ModuleName},
	}

	ibcBeginBlockers = []string{ibcexported.ModuleName}

	ibcInitGenesis = []string{
		ibcexported.ModuleName,
		ibctransfertypes.ModuleName,
		icatypes.ModuleName,
	}
)

// ibcKeepers is embedded in App so that App.IBCKeeper and friends exist only in
// a build that has IBC. See emissionKeepers for why the fields are embedded
// rather than left in place and zeroed.
type ibcKeepers struct {
	IBCKeeper           *ibckeeper.Keeper
	ICAControllerKeeper icacontrollerkeeper.Keeper
	ICAHostKeeper       icahostkeeper.Keeper
	TransferKeeper      ibctransferkeeper.Keeper
}

// ibcDepinjectSupplies hands depinject the getter the IBC modules need.
//
// The keeper does not exist yet at the point the container is built, so the
// getter is supplied instead of the keeper. Supplying it in the no-IBC build
// would put an ibc-go type into the container's type graph and defeat the
// exclusion, which is why this is a tagged function rather than a nil check.
func (app *App) ibcDepinjectSupplies() []any {
	return []any{app.GetIBCKeeper}
}
