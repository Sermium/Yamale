//go:build !settlement

package app

import (
	ammtypes "yamale/blockchain/x/amm/types"
	builderfeetypes "yamale/blockchain/x/builderfee/types"
	custodytypes "yamale/blockchain/x/custody/types"
	emissiontypes "yamale/blockchain/x/emission/types"
	tokenisationtypes "yamale/blockchain/x/tokenisation/types"
)

// profileBlockedAccountNames are the module accounts this profile adds to the
// set TestCustodyModuleAccountsRejectDirectTransfers checks.
//
// Each holds coins against bookkeeping only its own module maintains: emission
// mints into its account before forwarding to the fee collector, the AMM holds
// pool reserves, builderfee holds an undistributed share, custody holds claims
// mid-redemption, tokenisation holds vault income. A direct bank send to any of
// them credits the account and none of the records, and nothing can ever pay it
// out. Which is why the list has to move with the profile: an account that
// stops being blocked because its module was compiled out is fine, but one that
// stays unblocked because the list was not updated is a permanent loss waiting
// for someone to paste an address.
var profileBlockedAccountNames = []string{
	emissiontypes.ModuleName,
	ammtypes.ModuleName,
	builderfeetypes.ModuleName,
	custodytypes.ModuleName,
	tokenisationtypes.ModuleName,
}
