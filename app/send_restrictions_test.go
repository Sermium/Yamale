package app

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

// Embedded rather than read at run time. A test that opens a file by relative
// path passes under `go test` and fails anywhere the binary is executed from a
// different directory — which is how this suite is actually run on the arm64
// node, since Windows app-control will not execute a locally built Go binary.
//
//go:embed app.go
var appSource string

//go:embed ante.go
var anteSource string

// The freeze and the shareholding settlement must both be BANK SEND
// RESTRICTIONS, not ante decorators.
//
// This distinction was raised by an independent review on 2026-08-31, which
// could not tell which layer enforced the freeze and correctly identified what
// turns on it. An ante decorator only ever sees messages that arrived as
// transactions. A module calling SendCoins directly — x/netting settling gross,
// a treasury spend, a swap, an interchain account, an authz grant — walks
// straight past it. The project already documents that bypass for interchain
// accounts; it would apply to every one of those paths.
//
// A send restriction sits on the bank keeper itself, which is the one place all
// of them pass through.
//
// Asserted against the source rather than by exercising a frozen account,
// because what matters is WHERE the check lives. A behavioural test would pass
// just as happily with the check in the ante chain, and would keep passing
// while every module-internal transfer bypassed it.
func TestValueGuardsAreSendRestrictionsAndNotAnteDecorators(t *testing.T) {
	text := appSource

	for _, guard := range []struct{ what, call string }{
		{
			what: "the enforcement freeze",
			call: "app.BankKeeper.AppendSendRestriction(app.EnforcementKeeper.SendRestriction)",
		},
		{
			what: "the x/tokenisation shareholding settlement",
			call: "app.registerTokenisationSendRestriction()",
		},
		{
			// Same mechanism, and additionally the KYC gate: a deal token is an
			// ordinary bank denomination, so a gate anywhere else binds on none
			// of MsgSend, authz, a swap or a treasury spend.
			what: "the x/syndicate settlement and onboarding gate",
			call: "app.registerSyndicateSendRestriction()",
		},
	} {
		require.Contains(t, text, guard.call,
			"%s is not registered as a bank send restriction; if it moved to the ante chain, "+
				"every module that calls SendCoins directly now bypasses it", guard.what)
	}

	// And the ante chain must not be where a freeze is decided. Named
	// explicitly so that adding one there is a deliberate act with a failing
	// test in front of it, rather than a plausible-looking commit.
	require.NotContains(t, anteSource, "FreezeOf",
		"a freeze is being decided in the ante chain, which authz, interchain accounts, "+
			"treasury spends and swaps all reach the bank without traversing")
}

//go:embed profile_tokenisation_on.go
var tokenisationProfileSource string

//go:embed profile_syndicate_on.go
var syndicateProfileSource string

//go:embed restricted_denoms.go
var restrictedDenomsSource string

// The AMM must be told which denoms are fraction denoms, or K-1 comes back.
//
// A pool pays both reserve legs in one send, and a realised vehicle's fraction
// token cannot be sent to anyone but the tokenisation module account — so a
// pooled fraction denom locks every LP's counter-asset the moment the vehicle
// is sold. CreatePool refuses a fraction denom, but only once it has been told
// what one is, and that telling is a single line wired after the graph is built.
//
// Asserted against the source for the same reason the send restriction above is:
// the danger is not that the guard is wrong but that it is absent. The send
// restriction it sits beside was written, committed, and registered nowhere for
// weeks, and every entitlement read zero the whole time. This is the line that
// stops the same shape happening to the pool guard.
func TestTheAmmIsToldWhichDenomsAreFractionDenoms(t *testing.T) {
	// This used to pin the literal call
	// `app.AmmKeeper.SetRestrictedDenomKeeper(app.TokenisationKeeper)` in the
	// tokenisation profile, and it caught the change that made this test
	// necessary in its present form.
	//
	// SetRestrictedDenomKeeper ASSIGNS through a pointer. With one module
	// producing restricted denoms that is invisible. When x/syndicate arrived
	// with deal tokens carrying the same halt, a second direct call would have
	// silently replaced the tokenisation classifier — and K-1 would have been
	// back for fraction denoms with nothing failing anywhere, which is the
	// precise failure this test was written to prevent.
	//
	// So the classifiers are now composed, and what is pinned is the whole
	// chain: each profile contributes, the app composes, and the composite is
	// what reaches the AMM. Any of those three going missing puts a restricted
	// denom back in a pool.
	require.Contains(t, tokenisationProfileSource,
		"func (app *App) tokenisationRestrictedDenoms() []ammtypes.RestrictedDenomKeeper",
		"the tokenisation profile no longer contributes a fraction-denom classifier; "+
			"CreatePool will pool a fraction denom and K-1 (a realised vehicle locking an "+
			"LP's counter-asset) is back")
	require.Contains(t, syndicateProfileSource,
		"func (app *App) syndicateRestrictedDenoms() []ammtypes.RestrictedDenomKeeper",
		"the syndicate profile no longer contributes a deal-token classifier; a pool over "+
			"a deal token fails on its first payout, with the liquidity already inside")

	require.Contains(t, restrictedDenomsSource,
		"out = append(out, app.tokenisationRestrictedDenoms()...)",
		"the composite classifier has dropped x/tokenisation")
	require.Contains(t, restrictedDenomsSource,
		"out = append(out, app.syndicateRestrictedDenoms()...)",
		"the composite classifier has dropped x/syndicate")
	require.Contains(t, restrictedDenomsSource,
		"app.AmmKeeper.SetRestrictedDenomKeeper(restrictedDenoms(app.restrictedDenomKeepers()))",
		"the composite is built but never handed to the AMM, which is the same as having none")
	require.Contains(t, appSource, "app.registerRestrictedDenomClassifiers()",
		"nothing calls the classifier wiring; a guard that is written and not registered "+
			"is the exact shape that left every entitlement reading zero for weeks")

	// And nobody may go back to assigning directly, in any profile: the second
	// such call silently wins and the first module's denoms become poolable.
	for what, source := range map[string]string{
		"the tokenisation profile": tokenisationProfileSource,
		"the syndicate profile":    syndicateProfileSource,
	} {
		require.NotContains(t, source, "app.AmmKeeper.SetRestrictedDenomKeeper(",
			"%s assigns the AMM classifier directly; the setter REPLACES, so whichever "+
				"module is wired last would be the only one classified", what)
	}
}
