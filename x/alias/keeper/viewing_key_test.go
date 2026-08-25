package keeper_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"testing"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/alias/keeper"
	module "yamale/blockchain/x/alias/module"
	"yamale/blockchain/x/alias/types"
)

// publicKey returns a well-formed X25519 public half.
//
// Generated rather than a fixed byte pattern, because the registry's own check
// refuses degenerate values and a test that published the same 32 bytes
// everywhere would not notice if that check disappeared.
func publicKey(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv.PublicKey().Bytes()
}

func TestRegisterViewingKeyIssuesVersionOneAndItIsQueryable(t *testing.T) {
	env, _, ms, qs := setup(t)
	_, addr := env.Addr(t)
	key := publicKey(t)

	res, err := ms.RegisterViewingKey(env.Ctx, &types.MsgRegisterViewingKey{
		Account: addr, PublicKey: key,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), res.Version)

	got, err := qs.ViewingKeys(env.Ctx, &types.QueryViewingKeysRequest{Address: addr})
	require.NoError(t, err)
	require.Len(t, got.Keys, 1)
	require.Equal(t, key, got.Keys[0].PublicKey)
	require.Equal(t, addr, got.Keys[0].Address)
	require.True(t, got.Keys[0].Live())
}

// Rotation is forward-only. The old version stays exactly where it is, because
// every envelope already sealed to it names that version — and nothing rewraps
// history behind an operator's back.
func TestRotationKeepsEveryPreviousVersion(t *testing.T) {
	env, _, ms, qs := setup(t)
	_, addr := env.Addr(t)

	first, second, third := publicKey(t), publicKey(t), publicKey(t)
	for i, key := range [][]byte{first, second, third} {
		res, err := ms.RegisterViewingKey(env.Ctx, &types.MsgRegisterViewingKey{
			Account: addr, PublicKey: key,
		})
		require.NoError(t, err)
		require.Equal(t, uint64(i+1), res.Version, "versions must climb by one and never be reused")
	}

	got, err := qs.ViewingKeys(env.Ctx, &types.QueryViewingKeysRequest{Address: addr})
	require.NoError(t, err)
	require.Len(t, got.Keys, 3, "a rotation destroyed a previous version, so every payload sealed to it is now unreadable")

	// Newest first, which is what a sender reads to find the current key.
	require.Equal(t, uint64(3), got.Keys[0].Version)
	require.Equal(t, third, got.Keys[0].PublicKey)
	require.Equal(t, uint64(1), got.Keys[2].Version)
	require.Equal(t, first, got.Keys[2].PublicKey)
}

// Revocation flags exposure and stops future sealing. It does not delete the
// key and does not claim the payloads sealed to it became unreadable, because
// ciphertext that has been distributed cannot be recalled.
func TestRevocationFlagsAKeyWithoutRemovingIt(t *testing.T) {
	env, k, ms, qs := setup(t)
	_, addr := env.Addr(t)

	_, err := ms.RegisterViewingKey(env.Ctx, &types.MsgRegisterViewingKey{
		Account: addr, PublicKey: publicKey(t),
	})
	require.NoError(t, err)

	// Revoked at a height that is not zero, so the recorded height can be
	// asserted. The height-zero case is the one the boolean exists for and is
	// covered separately below.
	at := env.Ctx.WithBlockHeight(77)
	_, err = ms.RevokeViewingKey(at, &types.MsgRevokeViewingKey{Account: addr, Version: 1})
	require.NoError(t, err)

	got, err := qs.ViewingKeys(at, &types.QueryViewingKeysRequest{Address: addr})
	require.NoError(t, err)
	require.Len(t, got.Keys, 1, "revocation removed the key, so the payloads sealed to it can no longer even be identified")
	require.False(t, got.Keys[0].Live())
	require.Equal(t, int64(77), got.Keys[0].RevokedAtHeight,
		"the height the exposure began is what decides which stored payloads are affected")

	// Latest still reports it, because it is still the newest thing this
	// account has published. Whether it may be sealed to is Live(), which is a
	// separate question from which version is current.
	latest, found, err := k.LatestViewingKey(at, addr)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, latest.Live())

	// Resubmitting is not an error. A revocation sent twice after a timeout is
	// the ordinary way this message arrives, and failing the second would leave
	// an operator handling a compromised key unsure whether the first landed.
	_, err = ms.RevokeViewingKey(env.Ctx, &types.MsgRevokeViewingKey{Account: addr, Version: 1})
	require.NoError(t, err)

	_, err = ms.RevokeViewingKey(env.Ctx, &types.MsgRevokeViewingKey{Account: addr, Version: 9})
	require.ErrorIs(t, err, types.ErrViewingKeyNotFound)
}

// A revocation at height zero must read back as revoked.
//
// This is the failure the boolean replaced. While liveness was derived from
// revoked_at_height != 0, a key revoked at genesis — or on any chain that had
// not yet produced a block — came back live, and senders went on sealing
// payment detail to a key its holder had declared compromised.
func TestARevocationAtHeightZeroIsStillARevocation(t *testing.T) {
	env, _, ms, qs := setup(t)
	_, addr := env.Addr(t)

	_, err := ms.RegisterViewingKey(env.Ctx, &types.MsgRegisterViewingKey{
		Account: addr, PublicKey: publicKey(t),
	})
	require.NoError(t, err)

	genesis := env.Ctx.WithBlockHeight(0)
	_, err = ms.RevokeViewingKey(genesis, &types.MsgRevokeViewingKey{Account: addr, Version: 1})
	require.NoError(t, err)

	got, err := qs.ViewingKeys(genesis, &types.QueryViewingKeysRequest{Address: addr})
	require.NoError(t, err)
	require.Len(t, got.Keys, 1)
	require.Zero(t, got.Keys[0].RevokedAtHeight)
	require.False(t, got.Keys[0].Live(),
		"a key revoked at height zero read back as live, so the height is being used as the flag again")
}

// A key that is not an X25519 public key is refused where it is published,
// because nothing downstream can refuse it: an envelope sealed to one is
// well-formed, stores cleanly and opens for nobody.
func TestMalformedViewingKeysAreRefused(t *testing.T) {
	env, _, ms, _ := setup(t)
	_, addr := env.Addr(t)

	for name, key := range map[string][]byte{
		"empty":      {},
		"too short":  make([]byte, 31),
		"too long":   make([]byte, 33),
		"all zeroes": make([]byte, 32),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ms.RegisterViewingKey(env.Ctx, &types.MsgRegisterViewingKey{
				Account: addr, PublicKey: key,
			})
			require.ErrorIs(t, err, types.ErrInvalidViewingKey)
		})
	}
}

// An account that has published no key is an answer, not a failure. A sender
// needs to learn it cannot seal to this account as a fact it can act on rather
// than as a request it might retry.
func TestAnAccountWithNoViewingKeyReturnsAnEmptyList(t *testing.T) {
	env, _, _, qs := setup(t)
	_, addr := env.Addr(t)

	got, err := qs.ViewingKeys(env.Ctx, &types.QueryViewingKeysRequest{Address: addr})
	require.NoError(t, err)
	require.Empty(t, got.Keys)
}

func TestAppointRegulatorIsAuthorityGated(t *testing.T) {
	env, k, ms, qs := setup(t)
	_, regulator := env.Addr(t)
	_, outsider := env.Addr(t)

	// The account that would most like to appoint itself is refused.
	_, err := ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
		Authority: outsider, Country: country, Address: outsider,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	// Governance signing is not enough on its own: the appointee has to hold
	// ROLE_SUPERVISOR over the country. This is the check that stops the module
	// holding two answers to "who is watching this country".
	_, err = ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
		Authority: env.AuthorityString(t), Country: country, Address: regulator,
	})
	require.ErrorIs(t, err, types.ErrOutOfScope)
	require.ErrorContains(t, err, "cannot be appointed regulator of "+country)

	// A supervisor of somewhere else does not qualify either.
	supervise(t, k, env.Ctx, regulator, "GH")
	_, err = ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
		Authority: env.AuthorityString(t), Country: country, Address: regulator,
	})
	require.ErrorIs(t, err, types.ErrOutOfScope)

	supervise(t, k, env.Ctx, regulator, country)
	_, err = ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
		Authority: env.AuthorityString(t), Country: country, Address: regulator,
	})
	require.NoError(t, err)

	// A country the chain does not assign appoints a regulator of nowhere, and
	// every payment declaring a real country goes on being sealed without one
	// while the appointment sits in state looking done.
	_, err = ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
		Authority: env.AuthorityString(t), Country: "NX", Address: regulator,
	})
	require.ErrorIs(t, err, types.ErrInvalidCountry)

	key := publicKey(t)
	_, err = ms.RegisterViewingKey(env.Ctx, &types.MsgRegisterViewingKey{
		Account: regulator, PublicKey: key,
	})
	require.NoError(t, err)

	got, err := qs.Regulator(env.Ctx, &types.QueryRegulatorRequest{Country: country})
	require.NoError(t, err)
	require.Equal(t, regulator, got.Appointment.Address)
	require.Equal(t, key, got.Key.PublicKey)
}

// A regulator appointed but without a published key must come back with an
// empty public_key rather than thirty-two zero bytes, or a sender would seal an
// envelope that looks addressed to the regulator and opens for nobody.
func TestARegulatorWithNoKeyIsVisibleAsSuch(t *testing.T) {
	env, k, ms, qs := setup(t)
	_, regulator := env.Addr(t)
	supervise(t, k, env.Ctx, regulator, country)

	_, err := ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
		Authority: env.AuthorityString(t), Country: country, Address: regulator,
	})
	require.NoError(t, err)

	got, err := qs.Regulator(env.Ctx, &types.QueryRegulatorRequest{Country: country})
	require.NoError(t, err)
	require.Equal(t, regulator, got.Appointment.Address)
	require.Empty(t, got.Key.PublicKey)
	require.Equal(t, regulator, got.Key.Address)

	// And a country nobody regulates is not-found rather than an empty
	// appointment that reads as somebody being named.
	_, err = qs.Regulator(env.Ctx, &types.QueryRegulatorRequest{Country: "GH"})
	require.Error(t, err)
}

// One regulator per country. Two would reintroduce exactly the contest over
// standing that the single settlement declaration exists to end.
func TestAppointingAgainReplacesRatherThanAdds(t *testing.T) {
	env, k, ms, qs := setup(t)
	_, first := env.Addr(t)
	_, second := env.Addr(t)

	for _, addr := range []string{first, second} {
		supervise(t, k, env.Ctx, addr, country)
		_, err := ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
			Authority: env.AuthorityString(t), Country: country, Address: addr,
		})
		require.NoError(t, err)
	}

	got, err := qs.Regulator(env.Ctx, &types.QueryRegulatorRequest{Country: country})
	require.NoError(t, err)
	require.Equal(t, second, got.Appointment.Address)
	require.Equal(t, env.AuthorityString(t), got.Appointment.AppointedBy)
}

// The auditor grant expires by itself, is capped, and records who made it. All
// three because this role reads payment detail belonging to people who have no
// relationship with the holder.
func TestAuditorGrantsAreTimeBoxedAndCapped(t *testing.T) {
	env, _, ms, qs := setup(t)

	height := env.Ctx.BlockHeight()

	// No zero-means-forever, and nothing in the past.
	_, past := env.Addr(t)
	_, err := ms.GrantAuditor(env.Ctx, &types.MsgGrantAuditor{
		Authority: env.AuthorityString(t), Address: past, ExpiresAtHeight: 0,
	})
	require.ErrorIs(t, err, types.ErrInvalidAuditorGrant)
	_, err = ms.GrantAuditor(env.Ctx, &types.MsgGrantAuditor{
		Authority: env.AuthorityString(t), Address: past, ExpiresAtHeight: height,
	})
	require.ErrorIs(t, err, types.ErrInvalidAuditorGrant)

	granted := make([]string, 0, types.MaxLiveAuditorGrants)
	for i := 0; i < types.MaxLiveAuditorGrants; i++ {
		_, addr := env.Addr(t)
		granted = append(granted, addr)
		_, err := ms.GrantAuditor(env.Ctx, &types.MsgGrantAuditor{
			Authority: env.AuthorityString(t), Address: addr, ExpiresAtHeight: height + 1000,
		})
		require.NoError(t, err)
	}

	// One past the cap is refused. An unbounded list is both an unbounded
	// audience and an unbounded per-payment cost paid by every sender.
	_, extra := env.Addr(t)
	_, err = ms.GrantAuditor(env.Ctx, &types.MsgGrantAuditor{
		Authority: env.AuthorityString(t), Address: extra, ExpiresAtHeight: height + 1000,
	})
	require.ErrorIs(t, err, types.ErrInvalidAuditorGrant)

	// Renewing one that already holds a grant is not refused by the cap it
	// already occupies, or an auditor could not be extended once the cap was
	// full and the only route would be to let supervision lapse.
	_, err = ms.GrantAuditor(env.Ctx, &types.MsgGrantAuditor{
		Authority: env.AuthorityString(t), Address: granted[0], ExpiresAtHeight: height + 2000,
	})
	require.NoError(t, err)

	live, err := qs.Auditors(env.Ctx, &types.QueryAuditorsRequest{})
	require.NoError(t, err)
	require.Len(t, live.Auditors, types.MaxLiveAuditorGrants)
	for _, a := range live.Auditors {
		require.Equal(t, env.AuthorityString(t), a.Grant.GrantedBy)
	}
}

// A grant is gone at the height it names, not one block later.
func TestAnExpiredAuditorGrantStopsBeingListed(t *testing.T) {
	env, _, ms, qs := setup(t)
	_, auditor := env.Addr(t)

	expiry := env.Ctx.BlockHeight() + 5
	_, err := ms.GrantAuditor(env.Ctx, &types.MsgGrantAuditor{
		Authority: env.AuthorityString(t), Address: auditor, ExpiresAtHeight: expiry,
	})
	require.NoError(t, err)

	live, err := qs.Auditors(env.Ctx, &types.QueryAuditorsRequest{})
	require.NoError(t, err)
	require.Len(t, live.Auditors, 1)

	// One block before the expiry it still holds.
	before := env.Ctx.WithBlockHeight(expiry - 1)
	live, err = qs.Auditors(before, &types.QueryAuditorsRequest{})
	require.NoError(t, err)
	require.Len(t, live.Auditors, 1)

	// At the named height it is gone.
	at := env.Ctx.WithBlockHeight(expiry)
	live, err = qs.Auditors(at, &types.QueryAuditorsRequest{})
	require.NoError(t, err)
	require.Empty(t, live.Auditors)
}

// Every version, every appointment and every grant survives an export and
// import unchanged, including the revoked and the expired. Dropping any of them
// would destroy the record of who could read what, at an upgrade, where nobody
// is looking at payment detail.
func TestConfidentialityRegistriesRoundTripThroughGenesis(t *testing.T) {
	env, k, ms, _ := setup(t)

	_, holder := env.Addr(t)
	for i := 0; i < 3; i++ {
		_, err := ms.RegisterViewingKey(env.Ctx, &types.MsgRegisterViewingKey{
			Account: holder, PublicKey: publicKey(t),
		})
		require.NoError(t, err)
	}
	_, err := ms.RevokeViewingKey(env.Ctx, &types.MsgRevokeViewingKey{Account: holder, Version: 2})
	require.NoError(t, err)

	_, regulator := env.Addr(t)
	supervise(t, k, env.Ctx, regulator, country)
	_, err = ms.AppointRegulator(env.Ctx, &types.MsgAppointRegulator{
		Authority: env.AuthorityString(t), Country: country, Address: regulator,
	})
	require.NoError(t, err)

	_, auditor := env.Addr(t)
	_, err = ms.GrantAuditor(env.Ctx, &types.MsgGrantAuditor{
		Authority: env.AuthorityString(t), Address: auditor,
		ExpiresAtHeight: env.Ctx.BlockHeight() + 1,
	})
	require.NoError(t, err)

	exported, err := k.ExportGenesis(env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())
	require.Len(t, exported.ViewingKeys, 3, "a superseded or revoked version was dropped from the export")
	require.Len(t, exported.Regulators, 1)
	require.Len(t, exported.AuditorGrants, 1)

	// A second environment, so the import lands in an empty store rather than
	// on top of the state the messages above already wrote.
	other := integration.New(t, types.ModuleName, module.AppModule{})
	fresh := keeper.NewKeeper(other.Codec, other.AddressCodec, other.StoreService,
		log.NewNopLogger(), other.AuthorityString(t), nil, nil, nil)
	require.NoError(t, fresh.InitGenesis(other.Ctx, *exported))

	again, err := fresh.ExportGenesis(other.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, again)

	// The revoked version came back revoked. A round trip that quietly relit a
	// revoked key would have senders sealing to a key its holder has declared
	// compromised.
	var revoked bool
	for _, v := range again.ViewingKeys {
		if v.Version == 2 {
			revoked = !v.Live()
		}
	}
	require.True(t, revoked, "the revocation did not survive the round trip")
}

// The genesis rules the keeper depends on, checked where nothing later can
// check them: a record seeded at height zero is never re-examined.
func TestGenesisRefusesMalformedConfidentialityState(t *testing.T) {
	base := func() *types.GenesisState { return types.DefaultGenesis() }
	good := publicKey(t)

	t.Run("version zero", func(t *testing.T) {
		gs := base()
		gs.ViewingKeys = []types.ViewingKey{{Address: "yml1a", Version: 0, PublicKey: good}}
		require.ErrorContains(t, gs.Validate(), "versions start at 1")
	})
	t.Run("duplicate version", func(t *testing.T) {
		gs := base()
		gs.ViewingKeys = []types.ViewingKey{
			{Address: "yml1a", Version: 1, PublicKey: good},
			{Address: "yml1a", Version: 1, PublicKey: publicKey(t)},
		}
		require.ErrorContains(t, gs.Validate(), "two viewing keys at version 1")
	})
	t.Run("malformed key", func(t *testing.T) {
		gs := base()
		gs.ViewingKeys = []types.ViewingKey{{Address: "yml1a", Version: 1, PublicKey: make([]byte, 32)}}
		require.ErrorContains(t, gs.Validate(), "all zero")
	})
	t.Run("revoked before registered", func(t *testing.T) {
		gs := base()
		gs.ViewingKeys = []types.ViewingKey{{
			Address: "yml1a", Version: 1, PublicKey: good,
			RegisteredAtHeight: 100, Revoked: true, RevokedAtHeight: 50,
		}}
		require.ErrorContains(t, gs.Validate(), "before it was registered")
	})
	t.Run("a revocation height with no flag", func(t *testing.T) {
		// The pair is what proto3 cannot express as one field. A height alone
		// reads as live to the keeper and as revoked to a human, so the file is
		// refused rather than interpreted either way.
		gs := base()
		gs.ViewingKeys = []types.ViewingKey{{
			Address: "yml1a", Version: 1, PublicKey: good, RevokedAtHeight: 50,
		}}
		require.ErrorContains(t, gs.Validate(), "not marked revoked")
	})
	t.Run("a revocation at height zero survives", func(t *testing.T) {
		// The case the boolean exists for: genesis is height zero, so a key
		// revoked there has nothing but the flag to say so.
		gs := base()
		gs.ViewingKeys = []types.ViewingKey{{
			Address: "yml1a", Version: 1, PublicKey: good, Revoked: true,
		}}
		require.NoError(t, gs.Validate())
		require.False(t, gs.ViewingKeys[0].Live())
	})
	t.Run("two regulators for one country", func(t *testing.T) {
		gs := base()
		gs.Regulators = []types.RegulatorAppointment{
			{Country: country, Address: "yml1a"},
			{Country: country, Address: "yml1b"},
		}
		require.ErrorContains(t, gs.Validate(), "two appointed regulators")
	})
	t.Run("unassigned country", func(t *testing.T) {
		gs := base()
		gs.Regulators = []types.RegulatorAppointment{{Country: "NX", Address: "yml1a"}}
		require.ErrorContains(t, gs.Validate(), "not an assigned country code")
	})
	t.Run("auditor grant with no expiry", func(t *testing.T) {
		gs := base()
		gs.AuditorGrants = []types.AuditorGrant{{Address: "yml1a", ExpiresAtHeight: 0}}
		require.ErrorContains(t, gs.Validate(), "no expiry height")
	})
	t.Run("an expired grant is kept, not refused", func(t *testing.T) {
		// The record of who could read what is the thing that answers the
		// question years later, so a file carrying lapsed grants must load.
		gs := base()
		gs.AuditorGrants = []types.AuditorGrant{{Address: "yml1a", ExpiresAtHeight: 1}}
		require.NoError(t, gs.Validate())
	})
}
