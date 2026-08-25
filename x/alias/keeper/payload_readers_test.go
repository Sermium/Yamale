package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/alias/types"
)

// What ROLE_SUPERVISOR confers, and what it does not.
//
// The role had no consumer anywhere in the tree, which role.proto's own comment
// calls a name in a registry pretending to be a control. What it confers now is
// a reading entitlement over the encrypted payload of payments settling in the
// country it covers, published by this query so a sender can resolve the whole
// recipient set in one call.
//
// The shape is decided by a fact about gRPC rather than by preference: a query
// carries no signer, so the chain cannot gate a read by role. What it can do is
// say authoritatively who is entitled, from the same registry every other
// authority is granted through.

func TestPayloadReadersListsTheRegulatorAndTheCountrysSupervisors(t *testing.T) {
	env, k, ms, qs := setup(t)

	_, regulator := env.Addr(t)
	_, national := env.Addr(t)
	_, wide := env.Addr(t)
	_, elsewhere := env.Addr(t)

	supervise(t, k, env.Ctx, regulator, country)
	supervise(t, k, env.Ctx, national, country)
	supervise(t, k, env.Ctx, wide, types.ChainWide)
	supervise(t, k, env.Ctx, elsewhere, "GH")

	_, err := ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
		Authority: env.AuthorityString(t), Country: country, Address: regulator,
	})
	require.NoError(t, err)

	key := publicKey(t)
	_, err = ms.RegisterViewingKey(env.Ctx, &types.MsgRegisterViewingKey{
		Account: national, PublicKey: key,
	})
	require.NoError(t, err)

	res, err := qs.PayloadReaders(env.Ctx, &types.QueryPayloadReadersRequest{Country: country})
	require.NoError(t, err)

	basis := map[string]types.PayloadReaderBasis{}
	scope := map[string]string{}
	keys := map[string][]byte{}
	for _, r := range res.Readers {
		basis[r.Address] = r.Basis
		scope[r.Address] = r.Scope
		keys[r.Address] = r.Key.PublicKey
		require.Equal(t, r.Address, r.Key.Address)
	}

	require.Len(t, res.Readers, 3)
	require.Equal(t, types.PAYLOAD_READER_REGULATOR, basis[regulator],
		"the appointed regulator is the one entitlement that also carries standing to act")
	require.Equal(t, types.PAYLOAD_READER_SUPERVISOR, basis[national])
	require.Equal(t, types.PAYLOAD_READER_SUPERVISOR, basis[wide])
	require.NotContains(t, basis, elsewhere,
		"a supervisor of another country has no entitlement over this one's payments")

	// The scope is carried so an operator can tell a supervisor this country
	// granted from one that appears in every country's list. Losing that
	// distinction here would undo what the chain-wide-grants endpoint exists for.
	require.Equal(t, country, scope[national])
	require.Equal(t, types.ChainWide, scope[wide])
	require.Empty(t, scope[regulator],
		"the regulator's entitlement comes from the appointment, not from a scope")

	// A reader with a published key comes back with it; one without comes back
	// with an empty public_key rather than being dropped, because "there is
	// nobody" and "there is somebody who cannot be sealed to yet" are different
	// facts and only the second has an operator who can fix it.
	require.Equal(t, key, keys[national])
	require.Empty(t, keys[regulator])
	require.Empty(t, keys[wide])
}

// The regulator holds ROLE_SUPERVISOR by the rule AppointRegulator enforces, so
// one account is entitled twice. It must appear once, as the regulator.
func TestPayloadReadersDeduplicatesTheRegulatorThatIsAlsoASupervisor(t *testing.T) {
	env, k, ms, qs := setup(t)

	_, regulator := env.Addr(t)
	supervise(t, k, env.Ctx, regulator, country)
	_, err := ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
		Authority: env.AuthorityString(t), Country: country, Address: regulator,
	})
	require.NoError(t, err)

	res, err := qs.PayloadReaders(env.Ctx, &types.QueryPayloadReadersRequest{Country: country})
	require.NoError(t, err)
	require.Len(t, res.Readers, 1,
		"an envelope with two recipient blocks for one key is a claim that the reader is two people")
	require.Equal(t, types.PAYLOAD_READER_REGULATOR, res.Readers[0].Basis,
		"the stronger of the two statements about the account is the one that should be shown")
}

// A country nobody watches is an empty answer, not an error. A sender must be
// able to act on it rather than retry it.
func TestPayloadReadersIsEmptyForACountryNobodyWatches(t *testing.T) {
	env, _, _, qs := setup(t)

	res, err := qs.PayloadReaders(env.Ctx, &types.QueryPayloadReadersRequest{Country: "KE"})
	require.NoError(t, err)
	require.Empty(t, res.Readers)
	_ = env
}

// The two values that are legal elsewhere in this module are refused here, for
// the same reason AssertScopeIn refuses them: no payment settles chain-wide, and
// one declaring the absence of a national perimeter would be one no authority is
// accountable for.
func TestPayloadReadersRefusesTheWildcardAndTheReservedCode(t *testing.T) {
	env, k, _, qs := setup(t)

	_, wide := env.Addr(t)
	supervise(t, k, env.Ctx, wide, types.ChainWide)

	for _, bad := range []string{types.ChainWide, types.FoundationCountry, "NX", ""} {
		_, err := qs.PayloadReaders(env.Ctx, &types.QueryPayloadReadersRequest{Country: bad})
		require.Errorf(t, err, "%q was accepted as a settlement jurisdiction", bad)
	}

	// And the country form is accepted however it was typed, like every other
	// country argument in this module.
	res, err := qs.PayloadReaders(env.Ctx, &types.QueryPayloadReadersRequest{Country: "ng"})
	require.NoError(t, err)
	require.Len(t, res.Readers, 1)
	require.Equal(t, wide, res.Readers[0].Address)
}

// A supervisor whose grant is revoked stops being entitled to payloads sealed
// afterwards. Nothing is said about the ones already sealed to it, because
// ciphertext that exists cannot be recalled.
func TestRevokingASupervisorRemovesItFromTheReaderSet(t *testing.T) {
	env, k, ms, qs := setup(t)

	_, watcher := env.Addr(t)
	supervise(t, k, env.Ctx, watcher, country)

	res, err := qs.PayloadReaders(env.Ctx, &types.QueryPayloadReadersRequest{Country: country})
	require.NoError(t, err)
	require.Len(t, res.Readers, 1)

	_, err = ms.RevokeRole(env.Ctx, &types.MsgRevokeRole{
		Authority: env.AuthorityString(t), Holder: watcher,
		Role: types.ROLE_SUPERVISOR, Jurisdiction: country,
	})
	require.NoError(t, err)

	res, err = qs.PayloadReaders(env.Ctx, &types.QueryPayloadReadersRequest{Country: country})
	require.NoError(t, err)
	require.Empty(t, res.Readers)
}
