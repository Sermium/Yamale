//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	authmodulev1 "cosmossdk.io/api/cosmos/auth/module/v1"
	"cosmossdk.io/depinject/appconfig"

	marketmodulekeeper "yamale/blockchain/x/market/keeper"
	_ "yamale/blockchain/x/market/module" // registers the module with appconfig
	marketmoduletypes "yamale/blockchain/x/market/types"
)

// The half of the x/market contract that exists when the module is compiled in.
// See profile_market_off.go for the other half.

var (
	// No Minter and no Burner, which is the whole shape of this module: it
	// creates nothing and destroys nothing, it only holds other people's
	// property between an offer and its acceptance. The account is declared
	// with no permissions at all — like x/treasury and x/netting — because it
	// still has to EXIST for SendCoinsFromAccountToModule to reach it, and a
	// module account that is never declared is a panic on the first escrow
	// rather than a missing feature.
	marketModuleAccPerms = []*authmodulev1.ModuleAccountPermission{
		{Account: marketmoduletypes.ModuleName},
	}

	// The escrow account holds every resting order's shares and every resting
	// bid's money. Blocked, like every other custody account on this chain.
	//
	// The consequence of leaving it open is specific here rather than generic.
	// Property arriving outside List, OpenAuction, Bid or PlaceOrder matches no
	// record, so no cancel returns it and no fill spends it — and because this
	// module's cancel paths compute what to return FROM the order rather than
	// from the account balance, the stray funds would sit there permanently
	// while making the account's balance disagree with the sum of what the
	// orders claim. The first person to reconcile the two would find a surplus
	// and no way to tell whose it was.
	marketBlockedAccounts = []string{marketmoduletypes.ModuleName}

	marketInitGenesis = []string{marketmoduletypes.ModuleName}

	marketModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   marketmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&marketmoduletypes.Module{}),
		},
	}
)

type marketKeepers struct {
	MarketKeeper marketmodulekeeper.Keeper
}

func (app *App) marketDepinjectOutputs() []any {
	return []any{&app.MarketKeeper}
}

// registerMarketEscrow tells x/tokenisation that cost basis stands still when
// shares cross the market's account.
//
// Without it the module still works and every number it reports is wrong, which
// is the worst of the available failures.
//
// x/tokenisation's send restriction carries a sender's cost basis along with
// the shares on any transfer, because a plain transfer names no price and the
// sender's own cost is the only figure there is. That is right between two
// people and wrong for an escrow twice over. On the way IN, a seller who listed
// shares and then withdrew the listing would get them back with their cost
// spent — a loss the chain manufactured out of an offer nobody took. On the way
// OUT, the buyer would be handed a proportional slice of a pool mixed from every
// seller who ever used the module, and then ALSO credited the price they
// actually paid, because a fill writes that from the trade. Their reported
// return would fall by roughly the size of a stranger's cost.
//
// So the escrow is registered as basis-neutral and the two events that know a
// real price — ReleaseBasis on the seller, RecordPurchase on the buyer — are the
// only things that move cost through a trade.
//
// It is done here, in wiring, rather than by a message, for the same reason the
// send restrictions are: this must be true on every node before the first block,
// and a fact that a transaction could change is a fact a governance vote could
// use to silently redirect where cost basis flows.
func (app *App) registerMarketEscrow() {
	app.TokenisationKeeper.RegisterEscrow(app.MarketKeeper.ModuleAddress())
}
