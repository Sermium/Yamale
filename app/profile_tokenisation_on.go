//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	ammtypes "yamale/blockchain/x/amm/types"
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

// tokenisationRestrictedDenoms is this profile's contribution to the AMM's
// denom classifier.
//
// The same restriction that settles a share transfer halts every transfer of a
// realised vehicle's fraction token — and an AMM pool pays both its reserve
// legs in one send, so a pooled fraction denom becomes a trap that locks every
// LP's counter-asset the moment the vehicle is sold. The AMM refuses to pool a
// fraction denom, and this is what tells it which denoms those are.
//
// It used to call SetRestrictedDenomKeeper directly. That was correct while
// exactly one module produced restricted denoms and became wrong the instant a
// second did: the setter ASSIGNS, so the later caller silently discarded this
// classifier and the AMM went back to pooling fraction denoms with nothing
// failing anywhere. See app/restricted_denoms.go.
func (app *App) tokenisationRestrictedDenoms() []ammtypes.RestrictedDenomKeeper {
	return []ammtypes.RestrictedDenomKeeper{app.TokenisationKeeper}
}
