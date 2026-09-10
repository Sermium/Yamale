//go:build !settlement

package app

import (
	"context"

	ammtypes "yamale/blockchain/x/amm/types"
)

// restrictedDenoms asks several classifiers, because the AMM only holds one.
//
// # The bug this exists to prevent
//
// `AmmKeeper.SetRestrictedDenomKeeper` assigns through a pointer — it REPLACES
// whatever was there. With one module producing restricted denoms that is
// invisible and harmless. The moment a second one appears, the later call
// silently discards the earlier classifier, and the AMM goes back to happily
// pooling the first module's denoms.
//
// Silently is the operative word. Nothing errors, no test that exercises the
// second module fails, and the failure only surfaces the way K-1 surfaced
// originally: a vehicle is realised, its transfer restriction halts every send
// that is not to its own module account, a pool is a different module account,
// and every liquidity provider's COUNTER-ASSET — the ordinary token they put in
// beside it — is locked in that pool for good.
//
// So the classifiers are composed rather than assigned over each other, and the
// app wires exactly one composite.
//
// # Why an error is a refusal
//
// Same reason as the single classifier it replaces: a denom the AMM could not
// vet is a denom it should not pool. The first error wins and short-circuits;
// asking the remaining classifiers could only turn a refusal into a permission.
type restrictedDenoms []ammtypes.RestrictedDenomKeeper

var _ ammtypes.RestrictedDenomKeeper = restrictedDenoms(nil)

// IsRestrictedDenom reports whether ANY classifier restricts the denom.
func (r restrictedDenoms) IsRestrictedDenom(ctx context.Context, denom string) (bool, error) {
	for _, k := range r {
		if k == nil {
			continue
		}
		restricted, err := k.IsRestrictedDenom(ctx, denom)
		if err != nil {
			return false, err
		}
		if restricted {
			return true, nil
		}
	}
	return false, nil
}

// registerRestrictedDenomClassifiers hands the AMM every classifier at once.
//
// Behind the !settlement tag because the AMM itself is: the settlement profile
// has no pools to keep a denom out of. See restricted_denoms_settlement.go.
//
// Called after the dependency graph is built and after each profile has had a
// chance to contribute. A profile that links neither module contributes nothing
// and the AMM is left with an empty composite, which reports every denom
// poolable — correct, because that chain has no restricted denoms to pool.
func (app *App) registerRestrictedDenomClassifiers() {
	app.AmmKeeper.SetRestrictedDenomKeeper(restrictedDenoms(app.restrictedDenomKeepers()))
}

// restrictedDenomKeepers is every classifier this profile links.
//
// One line per module rather than a registry, because the compiler is then the
// thing that notices a new module was added and this was not updated — a
// registry populated by init() would silently stay short.
func (app *App) restrictedDenomKeepers() []ammtypes.RestrictedDenomKeeper {
	var out []ammtypes.RestrictedDenomKeeper
	out = append(out, app.tokenisationRestrictedDenoms()...)
	out = append(out, app.syndicateRestrictedDenoms()...)
	return out
}
