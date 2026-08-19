//go:build settlement

package app

// profileBlockedAccountNames are the module accounts this profile adds to the
// set TestCustodyModuleAccountsRejectDirectTransfers checks.
//
// The settlement profile adds none: every module that holds coins against its
// own bookkeeping — emission, amm, builderfee, custody, tokenisation — is
// compiled out of it, and the accounts that remain are already in the
// unconditional list.
var profileBlockedAccountNames []string
