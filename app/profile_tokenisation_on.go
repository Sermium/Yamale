//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	tokenisationmodulekeeper "yamale/blockchain/x/tokenisation/keeper"
	_ "yamale/blockchain/x/tokenisation/module" // registers the module with appconfig
	tokenisationmoduletypes "yamale/blockchain/x/tokenisation/types"
)

// The half of the x/tokenisation contract that exists when the module is
// compiled in. See profile_tokenisation_off.go for the other half.

var (
	tokenisationModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		// Mints a shareholding when an asset is fractionalised, burns it as
		// holders redeem.
		{Account: tokenisationmoduletypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner}},
	}

	// The vaults hold what shareholders are owed. Money arriving here outside
	// MsgFundVault raises no index and is owed to nobody.
	tokenisationBlockedAccounts = []string{tokenisationmoduletypes.ModuleName}

	tokenisationInitGenesis = []string{tokenisationmoduletypes.ModuleName}

	tokenisationModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   tokenisationmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&tokenisationmoduletypes.Module{}),
		},
	}
)

type tokenisationKeepers struct {
	TokenisationKeeper tokenisationmodulekeeper.Keeper
}

func (app *App) tokenisationDepinjectOutputs() []any {
	return []any{&app.TokenisationKeeper}
}

// registerTokenisationSendRestriction settles both sides of a share transfer
// before the balances move.
//
// Income is paid by a cumulative-per-token index: a holder earns the movement
// in that index across the period they held, so a position must be settled at
// the moment a balance changes or the arithmetic attributes their income to
// whoever holds the shares next. Doing it inside the module's own handlers
// would catch only the transfers it issues, and a fraction token is an ordinary
// bank denomination - it moves by MsgSend, by authz, through the AMM, out of a
// treasury. This is the one place all of them pass through, which is the same
// reason the freeze lives here.
//
// It went unregistered from the beginning. SendRestrictionFn was written,
// commented, and referenced by nothing in the repository, so no transfer ever
// settled and every holder's entitlement read zero however much the vault held.
// Found on 2026-08-27 against a live vehicle holding 72 YML against 1,000,000
// shares, which the chain's own query said was owed to nobody.
func (app *App) registerTokenisationSendRestriction() {
	app.BankKeeper.AppendSendRestriction(app.TokenisationKeeper.SendRestrictionFn)
}
