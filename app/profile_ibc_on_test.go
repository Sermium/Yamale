//go:build ibc

package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	icatypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibcexported "github.com/cosmos/ibc-go/v10/modules/core/exported"
)

// The counterpart to TestIBCIsAbsentFromThisProfile. Together they prove the
// tag moves the wiring rather than only the build.
func TestIBCIsWiredIntoThisProfile(t *testing.T) {
	require.Contains(t, GetMaccPerms(), ibctransfertypes.ModuleName,
		"transfer escrows and mints vouchers, so it needs Minter and Burner")
	require.Contains(t, GetMaccPerms(), icatypes.ModuleName)

	require.Equal(t, []string{ibcexported.ModuleName}, ibcBeginBlockers)
	require.Equal(t, []string{
		ibcexported.ModuleName,
		ibctransfertypes.ModuleName,
		icatypes.ModuleName,
	}, ibcInitGenesis)
}
