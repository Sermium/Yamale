//go:build !settlement

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A profile with a native token must not carry the fee-denomination gate. The
// native token has no approved issuer — it is minted by x/emission, not by
// anyone governance appointed — so the gate applied here would refuse every
// transaction on the chain.
func TestTheFeeDenomGateIsAbsentFromThisProfile(t *testing.T) {
	app := &App{}
	require.Empty(t, app.profileAnteFeeDecorators())
}

// And the post handler still carries the builder fee split, which is the only
// thing in it.
func TestTheBuilderFeeSplitIsInThisProfilesPostHandler(t *testing.T) {
	app := &App{}
	require.Len(t, app.builderfeePostDecorators(), 1)
}
