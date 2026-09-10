//go:build settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"

	ammtypes "yamale/blockchain/x/amm/types"
)

// x/syndicate is not linked into the settlement profile.
//
// Not because a syndication platform is sovereign — it is the opposite, and
// deliberately so: diaspora investors in one set of jurisdictions, companies in
// another, and a platform entity likely in a third. The reason is narrower.
//
// The settlement profile is a monetary-union binary: what it carries is what
// several states have to agree to validate for each other. A syndicated private
// placement is a commercial product one platform operates, and every state on
// such a ledger would be validating blocks containing the cap table of a
// private company in somebody else's country. That is a large ask for no
// benefit to them, and the regulatory treatment of tokenised private ownership
// is precisely the question the platform's own proposal lists as unsettled.
//
// It sits in the registry profile with x/tokenisation, whose fractional claim
// it is a sibling of.

var (
	syndicateModuleAccPerms  []*authmodulev1.ModuleAccountPermission
	syndicateBlockedAccounts []string
	syndicateInitGenesis     []string
	syndicateModuleConfigs   []*appv1alpha1.ModuleConfig
)

type syndicateKeepers struct{}

func (app *App) syndicateDepinjectOutputs() []any { return nil }

// No module, no deal tokens, nothing to settle or to gate.
func (app *App) registerSyndicateSendRestriction() {}

// And no deal tokens for the AMM to refuse.
func (app *App) syndicateRestrictedDenoms() []ammtypes.RestrictedDenomKeeper { return nil }
