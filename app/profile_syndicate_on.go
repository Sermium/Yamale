//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	ammtypes "yamale/blockchain/x/amm/types"
	syndicatemodulekeeper "yamale/blockchain/x/syndicate/keeper"
	_ "yamale/blockchain/x/syndicate/module" // registers the module with appconfig
	syndicatemoduletypes "yamale/blockchain/x/syndicate/types"
)

// The half of the x/syndicate contract that exists when the module is compiled
// in. See profile_syndicate_off.go for the other half.

var (
	syndicateModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		// Mints a deal token when a syndication closes. No Burner: a deal token
		// is never destroyed.
		//
		// That is a deliberate difference from x/tokenisation, which burns on
		// redemption. A syndicate position is not redeemed — the investment is
		// exited, the proceeds are distributed pro-rata, and the token stays in
		// the holder's wallet as the record of what they held and what they
		// were paid. Burning it would erase the only durable evidence an
		// investor has of a position they can no longer point at anything to
		// prove, which is precisely the complaint about paper syndicates.
		{Account: syndicatemoduletypes.ModuleName, Permissions: []string{authtypes.Minter}},
	}

	// The module account holds committed money between commitment and close,
	// and every deal's distribution pot. Money arriving here outside MsgCommit
	// or MsgDistribute belongs to nobody the module can identify — it raises no
	// index and matches no commitment — so it could never be refunded and never
	// be distributed. Blocked, like every other custody account on this chain.
	syndicateBlockedAccounts = []string{syndicatemoduletypes.ModuleName}

	syndicateInitGenesis = []string{syndicatemoduletypes.ModuleName}

	syndicateModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   syndicatemoduletypes.ModuleName,
			Config: appconfig.WrapAny(&syndicatemoduletypes.Module{}),
		},
	}
)

type syndicateKeepers struct {
	SyndicateKeeper syndicatemodulekeeper.Keeper
}

func (app *App) syndicateDepinjectOutputs() []any {
	return []any{&app.SyndicateKeeper}
}

// registerSyndicateSendRestriction is where the KYC gate is actually enforced,
// and where the distribution index is kept correct.
//
// Both of those are the same reason x/tokenisation's restriction exists, and
// this one is registered at the same time as the module rather than later,
// because the tokenisation restriction was written, commented, and referenced
// by nothing in the repository — so no transfer ever settled, and every
// holder's entitlement read zero however much the vault held. That was found
// against a live vehicle on 2026-08-27. The lesson is that a restriction is not
// registered by being written.
//
// The KYC point is worth stating separately. A deal token is an ordinary bank
// denomination: it moves by MsgSend, by authz, through an AMM, out of a
// treasury. A gate that lived in the platform's own handlers or its user
// interface would bind on none of those, and the platform would be telling a
// regulator its tokens are held only by onboarded investors while the chain
// said otherwise. This is the one place all transfers pass through.
func (app *App) registerSyndicateSendRestriction() {
	app.BankKeeper.AppendSendRestriction(app.SyndicateKeeper.SendRestrictionFn)
}

// syndicateRestrictedDenoms is this profile's contribution to the AMM's denom
// classifier.
//
// The same restriction that settles a transfer halts every transfer of a
// REALISED deal's token, so a pooled deal token would lock every liquidity
// provider's counter-asset the moment the investment is exited — the same trap
// as a realised vehicle's fraction denom.
//
// It is worth noting what this does NOT rely on. It does not merely stop a pool
// over a realised deal; it keeps a deal token out of a pool at all. It has to:
// an AMM pool is a holder, and a pool cannot complete KYC onboarding. A deal
// token sitting in a pool would be held by an address the send restriction
// refuses to credit, so the pool would fail on its first payout with the
// liquidity already inside. Refusing at creation is the only outcome that
// leaves nobody stranded.
//
// Contributed to a composite rather than assigned, because the setter it feeds
// replaces rather than appends — and x/tokenisation was already using it.
func (app *App) syndicateRestrictedDenoms() []ammtypes.RestrictedDenomKeeper {
	return []ammtypes.RestrictedDenomKeeper{app.SyndicateKeeper}
}
