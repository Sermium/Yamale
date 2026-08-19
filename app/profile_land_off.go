//go:build settlement

package app

import appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"

// x/land is not linked into the settlement profile.
//
// Scope 2 is explicit that land and payments cannot share a validator set: land
// is the most sovereign asset class there is, and no state will place its
// cadastre on a ledger other states validate. The two are separate deployments
// sharing a stack, and a settlement binary that still carries the cadastre
// invites exactly the deployment that decision rules out.

var (
	landInitGenesis   []string
	landModuleConfigs []*appv1alpha1.ModuleConfig
)
