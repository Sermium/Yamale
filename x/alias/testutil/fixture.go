// Package testutil builds a real x/alias keeper for the modules that consult the
// jurisdictional perimeter.
//
// A real keeper and not a stub, and this is the case where it matters most. The
// rule under test in the consuming modules is "refuse an authority acting outside
// its perimeter", and a stub that answered that question would be the test
// writing down the answer it wanted. The refusals below come from the same
// AssertScope the chain runs, over the same store, seeded through the same
// messages an operator would send.
package testutil

import (
	"testing"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/alias/keeper"
	"yamale/blockchain/x/alias/types"
)

// Perimeter is a keeper plus the two things a consuming test needs to do with
// it: place an account in a country, and grant somebody a role.
type Perimeter struct {
	Keeper keeper.Keeper

	env *integration.Env
	ms  types.MsgServer
}

// Init builds the keeper over the store the environment mounted for x/alias and
// runs it through genesis.
//
// Name types.ModuleName in integration.NewWith for the store to exist, and pass
// module.AppModule{} so the codec knows this module's types.
//
// All three consulted keepers are nil, deliberately.
//
// Nothing here records a jurisdiction as a participant — governance does it, the
// route a deployment uses for the accounts that are not somebody's payment
// customer — so a call into x/paymsg from these paths would panic rather than
// quietly pass. The nil group keeper means Grant does not require its holders to
// be group accounts, which is the one rule these fixtures cannot exercise
// without standing up x/group; it is covered in x/alias's own tests.
//
// The nil constitution keeper means there is no foundation, so governance is the
// only account that may grant a role here. That is the pre-widening rule, which
// is the right default for a fixture whose consumers all act as governance: a
// test that wanted to prove the foundation route works must construct the
// constitution it is relying on rather than inherit one, and x/alias's own tests
// do exactly that.
func Init(t *testing.T, env *integration.Env) *Perimeter {
	t.Helper()

	k := keeper.NewKeeper(
		env.Codec,
		env.AddressCodec,
		env.Store(types.ModuleName),
		log.NewNopLogger(),
		env.AuthorityString(t),
		nil,
		nil,
		nil,
	)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))

	return &Perimeter{Keeper: k, env: env, ms: keeper.NewMsgServerImpl(k)}
}

// Place records a country against an account, as governance.
//
// Through the message server rather than by writing the store, so a consuming
// test starts from a state the chain could actually reach.
func (p *Perimeter) Place(t *testing.T, account, country string) {
	t.Helper()
	_, err := p.ms.SetJurisdiction(p.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: p.env.AuthorityString(t),
		Account:  account,
		Country:  country,
	})
	require.NoError(t, err)
}

// NewPlacedAddr returns a fresh account already recorded in a country.
func (p *Perimeter) NewPlacedAddr(t *testing.T, country string) string {
	t.Helper()
	_, addr := p.env.Addr(t)
	p.Place(t, addr, country)
	return addr
}

// Grant grants a role in a jurisdiction, as governance.
//
// Again through the message server: granting by writing the store would skip the
// validation that refuses an unset role and a jurisdiction naming nowhere, which
// is exactly the state a consuming test must not be able to construct by
// accident.
func (p *Perimeter) Grant(t *testing.T, holder string, role types.Role, jurisdiction string) {
	t.Helper()
	_, err := p.ms.GrantRole(p.env.Ctx, &types.MsgGrantRole{
		Authority:    p.env.AuthorityString(t),
		Holder:       holder,
		Role:         role,
		Jurisdiction: jurisdiction,
	})
	require.NoError(t, err)
}

// Revoke removes a grant, as governance.
func (p *Perimeter) Revoke(t *testing.T, holder string, role types.Role, jurisdiction string) {
	t.Helper()
	_, err := p.ms.RevokeRole(p.env.Ctx, &types.MsgRevokeRole{
		Authority:    p.env.AuthorityString(t),
		Holder:       holder,
		Role:         role,
		Jurisdiction: jurisdiction,
	})
	require.NoError(t, err)
}
