//go:build !settlement

package app

import sdk "github.com/cosmos/cosmos-sdk/types"

// Every profile other than settlement has a native token, so fees are paid in
// it and distributed to validators the way x/distribution has always done. See
// profile_settlementfee_on.go for what replaces that when there is no native
// token to pay in.
//
// The fee-denomination gate is deliberately absent here rather than present and
// permissive: applied to this profile it would refuse every transaction on the
// chain, since the native token has no approved issuer — it is minted by
// x/emission, not by anyone governance appointed.

func (app *App) profileAnteFeeDecorators() []sdk.AnteDecorator { return nil }

func (app *App) installProfileFeeRouting() {}
