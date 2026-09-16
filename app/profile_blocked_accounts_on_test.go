//go:build !settlement

package app

import (
	ammtypes "yamale/blockchain/x/amm/types"
	builderfeetypes "yamale/blockchain/x/builderfee/types"
	custodytypes "yamale/blockchain/x/custody/types"
	emissiontypes "yamale/blockchain/x/emission/types"
	markettypes "yamale/blockchain/x/market/types"
	syndicatetypes "yamale/blockchain/x/syndicate/types"
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
	// x/syndicate escrows committed money between commitment and close, and
	// holds every deal’s distribution pot. It was blocked in app_config.go
	// from the start and never named here, so the test that exists to catch
	// exactly this omission was not checking it.
	syndicatetypes.ModuleName,
	// x/market holds every resting order’s property: the shares behind an ask
	// or a listing, and the money behind a bid or a standing auction bid. Its
	// cancel paths compute what to return FROM the order rather than from the
	// account balance, so a stray direct transfer is not merely unattributable
	// — it makes the account balance disagree with the sum of what the orders
	// claim, and the first person to reconcile the two finds a surplus with no
	// owner.
	markettypes.ModuleName,
}
