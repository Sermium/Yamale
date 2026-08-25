package keeper_test

import (
	"testing"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/alias/keeper"
	module "yamale/blockchain/x/alias/module"
	"yamale/blockchain/x/alias/types"
)

// The v2-to-v3 migration, tested against state written the way v2 wrote it.
//
// The whole difficulty of this migration is that the field it carries across no
// longer exists in the Go type, so a test that built the old parameters with a
// struct literal could not exist — there is no struct with that field any more.
// The bytes are therefore encoded by hand, which is not a shortcut: it is the
// only way to produce the state a chain that has actually run is holding, and it
// is what makes this a test of the migration rather than of a fixture.

// v2Params encodes an x/alias Params as v2 wrote it: payload_length in field 1,
// and one length-delimited string in field 2 per foundation administrator.
//
// Hand-encoded, and deliberately at the wire level rather than through any
// generated type. The generated type is exactly what cannot express this any
// more.
func v2Params(payloadLength byte, administrators ...string) []byte {
	raw := []byte{0x08, payloadLength} // field 1, varint
	for _, a := range administrators {
		raw = append(raw, 0x12, byte(len(a))) // field 2, length-delimited
		raw = append(raw, a...)
	}
	return raw
}

// writeV2Params puts those bytes under the parameters key, which is what a chain
// running the old binary has in its store.
func writeV2Params(t *testing.T, env *integration.Env, raw []byte) {
	t.Helper()
	store := env.StoreService.OpenKVStore(env.Ctx)
	require.NoError(t, store.Set(types.ParamsKey.Bytes(), raw))
}

// The administrator on the live chain survives the upgrade as a grant, and can
// still correct a country afterwards.
//
// This is the assertion the migration exists for. The chain has exactly one
// administrator; it is the only account that may correct the country recorded
// against another account; and after the upgrade that path reads a grant instead
// of a parameter. An upgrade that ran the code change without the migration
// would leave the appointment reading as done in every query and conferring
// nothing.
func TestMigrate2to3CarriesTheExistingAdministrator(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, nil, nil)
	ms := keeper.NewMsgServerImpl(k)

	_, administrator := env.Addr(t)
	_, outsider := env.Addr(t)
	writeV2Params(t, env, v2Params(types.PayloadLength, administrator))

	// Before: the parameters name somebody the handlers no longer look at, so
	// nothing works. This is the state the migration is being asked to repair,
	// asserted rather than assumed — without it the test below would pass on a
	// chain where the administrator had been carried across by accident.
	_, err := k.CountryOf(env.Ctx, administrator)
	require.ErrorIs(t, err, types.ErrNoJurisdiction)

	require.NoError(t, keeper.NewMigrator(k).Migrate2to3(env.Ctx))

	// After: the grant is there, chain-wide, attributed to governance because
	// governance is what set the parameter.
	grants, err := k.GrantsOf(env.Ctx, administrator)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, types.ROLE_FOUNDATION_ADMINISTRATOR, grants[0].Role)
	require.Equal(t, types.ChainWide, grants[0].Jurisdiction)
	require.Equal(t, env.AuthorityString(t), grants[0].GrantedBy)
	require.Nil(t, grants[0].RequiredShape,
		"the administrators on a running chain are bare addresses; recording a shape they do not meet would delete the authority rather than carry it")

	// The exemption works: the account is placed nowhere and gets the reserved
	// code rather than a refusal.
	country, err := k.CountryOf(env.Ctx, administrator)
	require.NoError(t, err)
	require.Equal(t, types.FoundationCountry, country)

	// And the power that matters still works. A country recorded by a
	// participant, then corrected by the administrator.
	_, placedAcct := env.Addr(t)
	_, err = ms.SetJurisdiction(env.Ctx, &types.MsgSetJurisdiction{
		Recorder: env.AuthorityString(t), Account: placedAcct, Country: "NG",
	})
	require.NoError(t, err)

	_, err = ms.SetJurisdiction(env.Ctx, &types.MsgSetJurisdiction{
		Recorder: administrator, Account: placedAcct, Country: "GH",
	})
	require.NoError(t, err)
	corrected, err := k.CountryOf(env.Ctx, placedAcct)
	require.NoError(t, err)
	require.Equal(t, "GH", corrected)

	// A non-administrator is refused, which is the other half of the same claim:
	// the migration carried one authority across, not the absence of a check.
	_, err = ms.SetJurisdiction(env.Ctx, &types.MsgSetJurisdiction{
		Recorder: outsider, Account: placedAcct, Country: "KE",
	})
	require.ErrorIs(t, err, types.ErrJurisdictionSet)
	still, err := k.CountryOf(env.Ctx, placedAcct)
	require.NoError(t, err)
	require.Equal(t, "GH", still, "a refused correction must change nothing")

	// The retired field is gone from the store, so the raw bytes and an export
	// cannot disagree about what the parameters are.
	store := env.StoreService.OpenKVStore(env.Ctx)
	raw, err := store.Get(types.ParamsKey.Bytes())
	require.NoError(t, err)
	require.NotContains(t, string(raw), administrator,
		"the migration rewrote the parameters, so the address must not still be in the stored bytes")
}

// Several administrators, and the chain's own reading of the parameters is
// unaffected by the field that was scanned out of them.
func TestMigrate2to3CarriesEveryAdministratorAndKeepsPayloadLength(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, nil, nil)

	addresses := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		_, a := env.Addr(t)
		addresses = append(addresses, a)
	}
	// A payload_length that is not the default, so a migration that rewrote the
	// parameters from DefaultParams instead of from what was stored would be
	// caught. Twelve is inside the 8..16 the module allows.
	writeV2Params(t, env, v2Params(12, addresses...))

	require.NoError(t, keeper.NewMigrator(k).Migrate2to3(env.Ctx))

	for _, a := range addresses {
		country, err := k.CountryOf(env.Ctx, a)
		require.NoErrorf(t, err, "%s did not survive the migration", a)
		require.Equal(t, types.FoundationCountry, country)
	}

	params, err := k.Params.Get(env.Ctx)
	require.NoError(t, err)
	require.Equal(t, uint32(12), params.PayloadLength,
		"the migration must rewrite the parameters it read, not the defaults")
}

// A chain that never named an administrator gets no grants, and the migration is
// not an error on it.
//
// The empty case is the one worth being careful about, for the same reason the
// parameter's own empty case was: an unset list meant nobody, and a migration
// that invented a holder from an empty field would grant the widest exemption on
// the chain to whatever the zero value decoded to.
func TestMigrate2to3OnAChainWithNoAdministratorsGrantsNothing(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, nil, nil)
	require.NoError(t, k.InitGenesis(env.Ctx, *types.DefaultGenesis()))

	require.NoError(t, keeper.NewMigrator(k).Migrate2to3(env.Ctx))

	wide, err := k.GrantsOf(env.Ctx, "")
	require.NoError(t, err)
	require.Empty(t, wide, "an empty parameter list must not become a grant to the empty address")

	exported, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.Empty(t, exported.RoleGrants)
}

// Running it twice writes the same state, which is what an operator re-running a
// halted upgrade needs.
func TestMigrate2to3IsIdempotent(t *testing.T) {
	env := integration.New(t, types.ModuleName, module.AppModule{})
	k := keeper.NewKeeper(env.Codec, env.AddressCodec, env.StoreService,
		log.NewNopLogger(), env.AuthorityString(t), nil, nil, nil)

	_, administrator := env.Addr(t)
	writeV2Params(t, env, v2Params(types.PayloadLength, administrator))

	require.NoError(t, keeper.NewMigrator(k).Migrate2to3(env.Ctx))
	first, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)

	// The second run reads parameters that no longer carry the field, finds
	// nothing to carry, and leaves the grant the first run wrote alone.
	require.NoError(t, keeper.NewMigrator(k).Migrate2to3(env.Ctx))
	second, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Len(t, second.RoleGrants, 1)
}
