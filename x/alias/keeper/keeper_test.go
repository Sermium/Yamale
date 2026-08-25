package keeper_test

import (
	"testing"

	"cosmossdk.io/log"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/alias/keeper"
	module "yamale/blockchain/x/alias/module"
	"yamale/blockchain/x/alias/types"
)

// No bank, no staking, no auth beyond address decoding. The module resolves
// identifiers and cannot move money, so a fixture that wired a bank keeper into
// it would be testing a capability the module deliberately does not have.
//
// The participant keeper is nil here, and that is the assertion: nothing in
// this file records a jurisdiction as a participant, so any call into x/paymsg
// from these paths would panic rather than quietly pass. The participant route
// is exercised in jurisdiction_test.go, against a real x/paymsg keeper.

// country is where the accounts in this file live. Any assigned code would do;
// what matters is that it is one the chain accepts and that the identifiers
// issued below carry it.
const country = "NG"

// administratorGrant seeds a foundation administrator the way one exists now: a
// chain-wide grant of ROLE_FOUNDATION_ADMINISTRATOR, rather than an entry in the
// parameter list this module used to carry.
//
// No required_shape, which is what the v2-to-v3 migration writes and therefore
// what the accounts on a chain that has been upgraded actually hold. A helper
// that recorded one would test a state no existing administrator is in.
func administratorGrant(holder string) types.RoleGrant {
	return types.RoleGrant{
		Holder:       holder,
		Role:         types.ROLE_FOUNDATION_ADMINISTRATOR,
		Jurisdiction: types.ChainWide,
	}
}

// supervise makes an account a supervisor of a country, which is now the
// precondition for being appointed its regulator.
//
// Written straight into the registry rather than through MsgGrantRole, because
// the fixtures that call it wire no group keeper and GrantRole would refuse a
// holder that is not a group policy. The rule being exercised in those tests is
// the appointment rule, not the office rule, and role_test.go covers the second
// against a group keeper that answers.
func supervise(t *testing.T, k keeper.Keeper, ctx sdk.Context, holder, cc string) {
	t.Helper()
	require.NoError(t, k.GrantForUpgrade(ctx, types.RoleGrant{
		Holder:       holder,
		Role:         types.ROLE_SUPERVISOR,
		Jurisdiction: cc,
	}))
}

func setup(t *testing.T) (*integration.Env, keeper.Keeper, types.MsgServer, types.QueryServer) {
	t.Helper()
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(
		env.Codec,
		env.AddressCodec,
		env.StoreService,
		log.NewNopLogger(),
		env.AuthorityString(t),
		nil,
		nil,
		nil,
	)
	// InitGenesis, not a bare Params.Set: the keeper reads params on every
	// issue, and a fixture that skipped genesis would exercise a state the
	// chain can never actually be in.
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))
	return env, k, keeper.NewMsgServerImpl(k), keeper.NewQueryServerImpl(k)
}

// placed returns a fresh account with a country recorded against it.
//
// Recorded through the message server rather than written into the store, so
// every test in this file starts from a state the chain could actually reach.
func placed(t *testing.T, env *integration.Env, ms types.MsgServer) string {
	t.Helper()
	_, addr := env.Addr(t)
	_, err := ms.SetJurisdiction(env.Ctx, &types.MsgSetJurisdiction{
		Recorder: env.AuthorityString(t), Account: addr, Country: country,
	})
	require.NoError(t, err)
	return addr
}

func TestRegisterAssignsAResolvableIdentifier(t *testing.T) {
	env, _, ms, qs := setup(t)
	addr := placed(t, env, ms)

	res, err := ms.RegisterAlias(env.Ctx, &types.MsgRegisterAlias{Account: addr})
	require.NoError(t, err)
	require.True(t, types.Valid(res.Id), "assigned identifier %q fails its own check", res.Id)
	require.Equal(t, country, types.Country(res.Id),
		"identifier %q does not carry the country recorded for the account", res.Id)

	// Forward and reverse must agree, and both are what a client actually calls.
	fwd, err := qs.Alias(env.Ctx, &types.QueryAliasRequest{Id: res.Id})
	require.NoError(t, err)
	require.Equal(t, addr, fwd.Alias.Address)

	rev, err := qs.AliasOf(env.Ctx, &types.QueryAliasOfRequest{Address: addr})
	require.NoError(t, err)
	require.Equal(t, res.Id, rev.Alias.Id)
}

func TestIdentifierResolvesHoweverItWasTyped(t *testing.T) {
	env, _, ms, qs := setup(t)
	addr := placed(t, env, ms)
	res, err := ms.RegisterAlias(env.Ctx, &types.MsgRegisterAlias{Account: addr})
	require.NoError(t, err)

	// The forms somebody actually produces reading one off paper.
	for _, form := range []string{
		res.Id,
		types.Format(res.Id),
		lower(types.Format(res.Id)),
	} {
		got, err := qs.Alias(env.Ctx, &types.QueryAliasRequest{Id: form})
		require.NoErrorf(t, err, "form %q was refused", form)
		require.Equal(t, addr, got.Alias.Address)
	}
}

func TestOneIdentifierPerAccount(t *testing.T) {
	env, _, ms, _ := setup(t)
	addr := placed(t, env, ms)

	_, err := ms.RegisterAlias(env.Ctx, &types.MsgRegisterAlias{Account: addr})
	require.NoError(t, err)

	// "Which of your two handles did you mean" is a question a payment
	// interface cannot answer, so the second registration is refused.
	_, err = ms.RegisterAlias(env.Ctx, &types.MsgRegisterAlias{Account: addr})
	require.ErrorIs(t, err, types.ErrAlreadyRegistered)
}

func TestRotateRetiresTheOldIdentifierForever(t *testing.T) {
	env, k, ms, qs := setup(t)
	addr := placed(t, env, ms)

	first, err := ms.RegisterAlias(env.Ctx, &types.MsgRegisterAlias{Account: addr})
	require.NoError(t, err)

	rot, err := ms.RotateAlias(env.Ctx, &types.MsgRotateAlias{Account: addr})
	require.NoError(t, err)
	require.Equal(t, first.Id, rot.Retired)
	require.NotEqual(t, rot.Retired, rot.Id, "rotation returned the identifier it just gave up")

	// The old one resolves to nothing. This is the property that makes an
	// immutable binding survivable: a payment to the handle a thief now holds
	// the key for arrives nowhere rather than with the thief.
	_, err = qs.Alias(env.Ctx, &types.QueryAliasRequest{Id: rot.Retired})
	require.Error(t, err)

	// And it is distinguishable from one that never existed.
	dead, err := qs.Retired(env.Ctx, &types.QueryRetiredRequest{Id: rot.Retired})
	require.NoError(t, err)
	require.True(t, dead.Retired)

	// The new one works, and the reverse index followed.
	live, err := qs.Alias(env.Ctx, &types.QueryAliasRequest{Id: rot.Id})
	require.NoError(t, err)
	require.Equal(t, addr, live.Alias.Address)

	held, err := k.Owners.Get(env.Ctx, addr)
	require.NoError(t, err)
	require.Equal(t, rot.Id, held)
}

func TestRetiredIdentifiersAreNeverReissued(t *testing.T) {
	env, k, ms, _ := setup(t)
	addr := placed(t, env, ms)

	_, err := ms.RegisterAlias(env.Ctx, &types.MsgRegisterAlias{Account: addr})
	require.NoError(t, err)

	seen := map[string]bool{}
	for i := 0; i < 25; i++ {
		rot, err := ms.RotateAlias(env.Ctx, &types.MsgRotateAlias{Account: addr})
		require.NoError(t, err)
		require.Falsef(t, seen[rot.Id], "identifier %q was issued twice", rot.Id)
		seen[rot.Id] = true

		// Every one given up stays tombstoned, not just the most recent.
		for id := range seen {
			if id == rot.Id {
				continue
			}
			dead, err := k.Retired.Has(env.Ctx, id)
			require.NoError(t, err)
			require.Truef(t, dead, "identifier %q lost its tombstone", id)
		}
	}
}

func TestRotateNeedsAnIdentifierToRotate(t *testing.T) {
	env, _, ms, _ := setup(t)
	addr := placed(t, env, ms)
	_, err := ms.RotateAlias(env.Ctx, &types.MsgRotateAlias{Account: addr})
	require.ErrorIs(t, err, types.ErrNotRegistered)
}

func TestGenesisRoundTrips(t *testing.T) {
	env, k, ms, _ := setup(t)

	for i := 0; i < 5; i++ {
		addr := placed(t, env, ms)
		_, err := ms.RegisterAlias(env.Ctx, &types.MsgRegisterAlias{Account: addr})
		require.NoError(t, err)
		if i%2 == 0 {
			_, err = ms.RotateAlias(env.Ctx, &types.MsgRotateAlias{Account: addr})
			require.NoError(t, err)
		}
	}

	exported, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())

	// Import into a fresh keeper and export again. Equality here is what
	// upgrades and state migrations depend on; the reverse index is rebuilt
	// rather than carried, so it cannot drift from the bindings.
	//
	// A second environment, so the import lands in an empty store. A keeper
	// built over `env.StoreService` is only a fresh *struct* — it shares every
	// byte of state with the one above, so Owners is already populated by the
	// messages that built the state, and the assertion below passes whether
	// InitGenesis rebuilds the index or not. Deleting the rebuild from
	// InitGenesis used to leave this test green, which made it a test of
	// nothing at all.
	other := integration.New(t, types.ModuleName, module.AppModule{})
	fresh := keeper.NewKeeper(other.Codec, other.AddressCodec, other.StoreService,
		log.NewNopLogger(), other.AuthorityString(t), nil, nil, nil)
	require.NoError(t, fresh.InitGenesis(other.Ctx, *exported))

	again, err := fresh.ExportGenesis(other.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, again)

	// The rebuilt reverse index must agree with every binding.
	require.NotEmpty(t, again.Aliases, "nothing to check the rebuilt index against")
	for _, a := range again.Aliases {
		held, err := fresh.Owners.Get(other.Ctx, a.Address)
		require.NoError(t, err)
		require.Equal(t, a.Id, held)
	}
}

func TestUpdateParamsIsGovernanceOnly(t *testing.T) {
	env, _, ms, _ := setup(t)
	_, notGov := env.Addr(t)

	_, err := ms.UpdateParams(env.Ctx, &types.MsgUpdateParams{
		Authority: notGov,
		Params:    types.NewParams(10),
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	// And a length outside the bounds is refused rather than clamped: a
	// proposal that silently became something other than what was voted on is
	// worse than one that fails.
	_, err = ms.UpdateParams(env.Ctx, &types.MsgUpdateParams{
		Authority: env.AuthorityString(t),
		Params:    types.NewParams(4),
	})
	require.ErrorIs(t, err, types.ErrInvalidParams)

	_, err = ms.UpdateParams(env.Ctx, &types.MsgUpdateParams{
		Authority: env.AuthorityString(t),
		Params:    types.NewParams(10),
	})
	require.NoError(t, err)
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}
