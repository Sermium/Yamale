//go:build settlement

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The settlement profile gates which denominations may pay a fee. Without it a
// chain that has no native token accepts a fee in any denom a sender cares to
// invent, and block space is free.
func TestTheFeeDenomGateIsWiredIntoThisProfile(t *testing.T) {
	app := &App{}
	require.Len(t, app.profileAnteFeeDecorators(), 1)
}

// And x/builderfee contributes nothing, because it is not in this build. The
// two mechanisms cannot both run: the builder's share is taken from the fee
// collector before the sweep, which would leave the operating treasury short by
// an amount nothing in the treasury records.
func TestTheBuilderFeeSplitIsAbsentFromThisProfile(t *testing.T) {
	app := &App{}
	require.Empty(t, app.builderfeePostDecorators())

	handler, err := newPostHandler(app)
	require.NoError(t, err)
	require.Nil(t, handler, "an empty decorator chain should be no post handler at all")
}
