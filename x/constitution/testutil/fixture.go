// Package testutil builds a real x/constitution keeper for the modules that
// consult one.
//
// A real keeper and not a stub, for the same reason the shared integration
// environment wires real auth and bank keepers: the rule being enforced is this
// module's — that a value is fixed and cannot be moved — and a test against a
// hand-written stub of the settlement would only prove that x/enforcement can
// read a struct the test wrote itself.
package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/constitution/keeper"
	"yamale/blockchain/x/constitution/types"
)

// Invariants is a complete settlement, with the one value that has no default
// filled in. Everything else is the shipped default, so a test that cares about
// one ceiling states that ceiling and inherits the rest.
func Invariants(recoveryDestination string) types.Invariants {
	inv := types.DefaultInvariants()
	inv.EnforcementRecoveryDestination = recoveryDestination
	return inv
}

// Init builds the keeper over the store the environment mounted for
// x/constitution and runs it through genesis, which is the only way the
// invariants are ever written.
//
// Name the module in integration.NewWith for the store to exist.
func Init(t *testing.T, env *integration.Env, staking types.StakingKeeper, inv types.Invariants) keeper.Keeper {
	t.Helper()

	k := keeper.NewKeeper(
		env.Store(types.ModuleName),
		env.Codec,
		env.AddressCodec,
		env.Authority,
		staking,
	)

	genesis := types.DefaultGenesis()
	genesis.Invariants = inv
	require.NoError(t, k.InitGenesis(env.Ctx, *genesis))

	return k
}
