//go:build settlement

package app

// The settlement profile links no AMM, so there are no pools to keep a
// restricted denom out of — and no classifier to hand anybody.
//
// A no-op rather than an absent method, so that app.go can call it
// unconditionally. The alternative is a second build-tagged branch at the call
// site, and a wiring step that exists in one profile and not the other is
// exactly the shape that leaves a guard unregistered.
func (app *App) registerRestrictedDenomClassifiers() {}
