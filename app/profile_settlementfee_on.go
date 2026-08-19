//go:build settlement

package app

import (
	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	stablecoinante "yamale/blockchain/x/stablecoin/ante"
)

// The settlement profile's fee model, which is the half of "no native token"
// that removing x/emission does not cover.
//
// Scope §3: fees are denominated in the issued currency and routed to a
// treasury-governed operating account, with validators compensated by service
// contract. Two mechanisms implement that, and they are separate because they
// answer different questions:
//
//   - the ante decorator answers "may this fee be offered", refusing any
//     denomination governance has not approved an issuer for;
//   - the sweep answers "where does it end up", emptying the fee collector into
//     the treasury before x/distribution's BeginBlocker allocates it.
//
// This replaces x/builderfee for this profile rather than extending it. See
// profile_builderfee_off.go for why the two cannot both run.

// profileAnteFeeDecorators inserts the fee-denomination gate ahead of fee
// deduction.
func (app *App) profileAnteFeeDecorators() []sdk.AnteDecorator {
	return []sdk.AnteDecorator{
		stablecoinante.NewApprovedFeeDenomDecorator(app.StablecoinKeeper),
	}
}

// installProfileFeeRouting wraps the PreBlocker so the fee collector is emptied
// before any BeginBlocker runs.
//
// PreBlock is the only hook available for this. The sweep has to happen before
// x/distribution's BeginBlocker, which takes the entire fee collector balance
// and allocates it to validators; but runtime.App.Load overwrites whatever
// BeginBlocker is set, and by the time Load returns the app is sealed. Load
// leaves an already-set PreBlocker alone, so this is both the correct position
// and the only reachable one. It must therefore be called before app.Load.
//
// A failure here is logged and not returned. Routing is configured by a
// governance parameter, and a chain that halts on a bad parameter cannot be
// governed back to a good one — the fees stay in the fee collector and are
// distributed as they were before, which is wrong but recoverable, and the log
// line names it every block until somebody fixes the parameter.
func (app *App) installProfileFeeRouting() {
	app.SetPreBlocker(func(ctx sdk.Context, req *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
		// The module PreBlockers run first: x/upgrade applies migrations here,
		// and a sweep that ran before them would run against the old state.
		res, err := app.App.PreBlocker(ctx, req)
		if err != nil {
			return res, err
		}

		swept, err := app.TreasuryKeeper.SweepIntoOperatingTreasury(ctx, authtypes.FeeCollectorName)
		if err != nil {
			ctx.Logger().Error(
				"fee routing failed; fees remain in the fee collector and will be distributed as staking rewards",
				"err", err,
			)
			return res, nil
		}
		if !swept.IsZero() {
			ctx.Logger().Debug("fees routed to the operating treasury", "amount", swept.String())
		}

		return res, nil
	})
}
