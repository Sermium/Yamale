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
	require.Contains(t, tokenisationProfileSource,
		"app.AmmKeeper.SetRestrictedDenomKeeper(app.TokenisationKeeper)",
		"the AMM has not been given the fraction-denom classifier; CreatePool will pool a "+
			"fraction denom and K-1 (a realised vehicle locking an LP's counter-asset) is back")
}
