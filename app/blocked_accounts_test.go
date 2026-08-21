package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	nettingtypes "yamale/blockchain/x/netting/types"
	paymsgtypes "yamale/blockchain/x/paymsg/types"
	stablecointypes "yamale/blockchain/x/stablecoin/types"
	treasurytypes "yamale/blockchain/x/treasury/types"
)

// Every module here holds coins against bookkeeping it maintains separately.
// A direct bank transfer into one of them credits the account without touching
// that bookkeeping, and the coins are then unreachable forever: no module
// message pays them out, because every payout path consults the module's own
// records, and no bank message moves them, because a module account cannot
// sign. The sender simply loses the funds.
//
// This was reachable before the accounts were blocked — a bank send of 7 YML to
// the treasury module account was accepted, leaving its ledger reporting
// 2,826,667 while the account held 9,826,667.
func TestCustodyModuleAccountsRejectDirectTransfers(t *testing.T) {
	blocked := BlockedAddresses()

	mustBlock := []string{
		treasurytypes.ModuleName,
		stablecointypes.ModuleName,
		paymsgtypes.ModuleName,
		// x/netting custodies every participant's settlement reserve, and
		// discharges a window by moving claims on it. A transfer that arrived
		// without a reserve record behind it would be money the module can
		// never allocate to anybody.
		nettingtypes.ModuleName,
		authtypes.FeeCollectorName,
	}

	mustBlock = append(mustBlock, profileBlockedAccountNames...)

	for _, name := range mustBlock {
		addr := authtypes.NewModuleAddress(name).String()
		require.True(t, blocked[addr],
			"%s holds custody against its own accounting, so a direct transfer to %s would strand the funds permanently",
			name, addr)
	}
}

// Governance is deliberately left open: proposals fund the community pool by
// sending to it, so blocking it would break a legitimate flow.
func TestGovernanceAccountStillAcceptsTransfers(t *testing.T) {
	blocked := BlockedAddresses()
	govAddr := authtypes.NewModuleAddress("gov").String()
	require.False(t, blocked[govAddr], "governance must remain able to receive deposits")
}
