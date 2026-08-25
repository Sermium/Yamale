package keeper_test

import (
	"testing"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/alias/keeper"
	module "yamale/blockchain/x/alias/module"
	"yamale/blockchain/x/alias/types"
	paymsgkeeper "yamale/blockchain/x/paymsg/keeper"
	paymsgmodule "yamale/blockchain/x/paymsg/module"
	paymsgtypes "yamale/blockchain/x/paymsg/types"
)

// x/paymsg is real here rather than stubbed.
//
// The rule under test is "only the institution that onboarded this account may
// say where it is", and that relationship is x/paymsg's record. Asserting it
// against a hand-written stub would only prove this module can read a struct
// the test wrote itself — it would not prove that what the payments module
// actually stores is what this module actually reads.

type jFixture struct {
	env *integration.Env
	k   keeper.Keeper
	ms  types.MsgServer
	qs  types.QueryServer

	paymsgK  paymsgkeeper.Keeper
	paymsgMS paymsgtypes.MsgServer

	// bank is an approved participant; rival is an approved participant that
	// banks somebody else.
	bank  string
	rival string
	// admin holds the chain-wide exemption; outsider holds nothing.
	admin    string
	outsider string
}

func jSetup(t *testing.T) *jFixture {
	t.Helper()

	env := integration.NewWith(t,
		[]string{types.ModuleName, paymsgtypes.ModuleName},
		module.AppModule{}, paymsgmodule.AppModule{})

	paymsgK := paymsgkeeper.NewKeeper(
		env.Store(paymsgtypes.ModuleName), env.Codec, env.AddressCodec,
		env.Authority, env.BankKeeper)
	require.NoError(t, paymsgK.Params.Set(env.Ctx, paymsgtypes.DefaultParams()))

	k := keeper.NewKeeper(
		env.Codec, env.AddressCodec, env.Store(types.ModuleName),
		log.NewNopLogger(), env.AuthorityString(t), paymsgK, nil, nil)

	f := &jFixture{
		env:      env,
		k:        k,
		ms:       keeper.NewMsgServerImpl(k),
		qs:       keeper.NewQueryServerImpl(k),
		paymsgK:  paymsgK,
		paymsgMS: paymsgkeeper.NewMsgServerImpl(paymsgK),
	}

	_, f.admin = env.Addr(t)
	_, f.outsider = env.Addr(t)

	gs := types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{administratorGrant(f.admin)}
	require.NoError(t, k.InitGenesis(env.Ctx, *gs))

	f.bank = f.approvedParticipant(t, "BANK1")
	f.rival = f.approvedParticipant(t, "BANK2")
	return f
}

// approvedParticipant walks an institution through the real admission path:
// application, then a governance approval. Writing the approved record directly
// would skip the state the module actually reads.
func (f *jFixture) approvedParticipant(t *testing.T, code string) string {
	t.Helper()
	_, addr := f.env.Addr(t)

	_, err := f.paymsgMS.ApplyParticipant(f.env.Ctx, &paymsgtypes.MsgApplyParticipant{
		Creator: addr, Code: code, Name: code,
	})
	require.NoError(t, err)
	_, err = f.paymsgMS.ApproveParticipant(f.env.Ctx, &paymsgtypes.MsgApproveParticipant{
		Authority: f.env.AuthorityString(t), Participant: addr, Approve: true,
	})
	require.NoError(t, err)
	return addr
}

// customerOf registers a fresh account as the participant's customer.
func (f *jFixture) customerOf(t *testing.T, participant string) string {
	t.Helper()
	_, addr := f.env.Addr(t)
	_, err := f.paymsgMS.RegisterCustomer(f.env.Ctx, &paymsgtypes.MsgRegisterCustomer{
		Participant: participant, Customer: addr, Registered: true,
	})
	require.NoError(t, err)
	return addr
}

func TestParticipantRecordsTheCountryAndTheAliasCarriesIt(t *testing.T) {
	f := jSetup(t)
	customer := f.customerOf(t, f.bank)

	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.bank, Account: customer, Country: "NG",
	})
	require.NoError(t, err)

	res, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: customer})
	require.NoError(t, err)
	require.Equal(t, "NG", types.Country(res.Id))
	require.True(t, types.Valid(res.Id))

	// The record is attributed. "Who says this account is Nigerian" is the
	// question asked when the answer turns out to be wrong.
	got, err := f.qs.Jurisdiction(f.env.Ctx, &types.QueryJurisdictionRequest{Address: customer})
	require.NoError(t, err)
	require.Equal(t, "NG", got.Jurisdiction.Country)
	require.Equal(t, f.bank, got.Jurisdiction.RecordedBy)
}

// The refusal the whole perimeter rests on. Not a permissive default, not a
// placeholder country — no identifier at all.
func TestAnAccountWithNoJurisdictionGetsNoIdentifier(t *testing.T) {
	f := jSetup(t)
	_, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: f.outsider})
	require.ErrorIs(t, err, types.ErrNoJurisdiction)

	// And nothing was half-written on the way out.
	_, err = f.qs.AliasOf(f.env.Ctx, &types.QueryAliasOfRequest{Address: f.outsider})
	require.Error(t, err)
}

// The one exception, and the proof it is only one.
func TestOnlyAFoundationAdministratorMayHoldNoCountry(t *testing.T) {
	f := jSetup(t)

	res, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: f.admin})
	require.NoError(t, err)
	require.Equal(t, types.FoundationCountry, types.Country(res.Id))

	// Everyone else with no country is refused: a customer of an approved
	// participant, the participant itself, and a stranger. The exemption is an
	// exact-address list and nothing about being on the rail earns it.
	for _, who := range []string{f.customerOf(t, f.bank), f.bank, f.outsider} {
		_, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: who})
		require.ErrorIsf(t, err, types.ErrNoJurisdiction,
			"%s was issued an identifier with no country recorded", who)
	}
}

// The exemption covers the absence of a jurisdiction and nothing else. An
// administrator that has been placed in a country uses that country, which is
// what keeps the exception as narrow as it can be made.
func TestAFoundationAdministratorWithACountryUsesIt(t *testing.T) {
	f := jSetup(t)

	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.env.AuthorityString(t), Account: f.admin, Country: "GH",
	})
	require.NoError(t, err)

	res, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: f.admin})
	require.NoError(t, err)
	require.Equal(t, "GH", types.Country(res.Id))
}

// The reserved code marks the absence of a perimeter. Recorded as one it would
// be a perimeter no authority holds, worn by an ordinary account.
func TestTheFoundationCodeCannotBeRecordedAsAJurisdiction(t *testing.T) {
	f := jSetup(t)
	customer := f.customerOf(t, f.bank)

	for _, code := range []string{types.FoundationCountry, "zz", "NX", "QK", "", "NGA"} {
		_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
			Recorder: f.env.AuthorityString(t), Account: customer, Country: code,
		})
		require.ErrorIsf(t, err, types.ErrInvalidCountry, "%q was accepted as a country", code)
	}
}

func TestOnlyTheOnboardingParticipantMayRecordACountry(t *testing.T) {
	f := jSetup(t)
	customer := f.customerOf(t, f.bank)

	// A participant that banks somebody else.
	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.rival, Account: customer, Country: "GH",
	})
	require.ErrorIs(t, err, types.ErrNotTheRecorder)

	// The account itself. A perimeter somebody may choose is the perimeter with
	// no authority watching it.
	_, err = f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: customer, Account: customer, Country: "GH",
	})
	require.ErrorIs(t, err, types.ErrNotTheRecorder)

	// A stranger.
	_, err = f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.outsider, Account: customer, Country: "GH",
	})
	require.ErrorIs(t, err, types.ErrNotTheRecorder)
}

// Approval is withdrawable, and an institution thrown off the rail must not go
// on stamping perimeters with the customers it accumulated while it was on it.
func TestAWithdrawnParticipantMayNoLongerRecordACountry(t *testing.T) {
	f := jSetup(t)
	customer := f.customerOf(t, f.bank)
	// x/paymsg has no withdrawal message yet, so the state is written directly.
	// What is under test is on this side of the boundary anyway: whether
	// x/alias re-asks whether the institution is admitted, or infers it from a
	// customer record registered when the answer was different.
	require.NoError(t, f.paymsgK.ApprovedParticipant.Remove(f.env.Ctx, f.bank))

	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.bank, Account: customer, Country: "NG",
	})
	require.ErrorIs(t, err, types.ErrNotTheRecorder)
}

// A participant records once. Rewriting a country already recorded would let it
// move a customer out from under the authority investigating them.
func TestAParticipantCannotCorrectACountryItAlreadyRecorded(t *testing.T) {
	f := jSetup(t)
	customer := f.customerOf(t, f.bank)

	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.bank, Account: customer, Country: "NG",
	})
	require.NoError(t, err)

	_, err = f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.bank, Account: customer, Country: "GH",
	})
	require.ErrorIs(t, err, types.ErrJurisdictionSet)

	got, err := f.qs.Jurisdiction(f.env.Ctx, &types.QueryJurisdictionRequest{Address: customer})
	require.NoError(t, err)
	require.Equal(t, "NG", got.Jurisdiction.Country)
}

// The prefix cannot go stale, because a correction retires the identifier that
// carried the old country in the same message that changes it.
func TestCorrectingTheCountryRetiresTheIdentifierCarryingTheOldOne(t *testing.T) {
	f := jSetup(t)
	customer := f.customerOf(t, f.bank)

	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.bank, Account: customer, Country: "NG",
	})
	require.NoError(t, err)
	first, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: customer})
	require.NoError(t, err)
	require.Equal(t, "NG", types.Country(first.Id))

	moved, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.admin, Account: customer, Country: "GH",
	})
	require.NoError(t, err)
	require.Equal(t, first.Id, moved.Retired)
	require.Equal(t, "GH", types.Country(moved.Id))

	// The old one resolves to nothing and is tombstoned, exactly as a rotation
	// leaves it — so it is never issued again and never repointed.
	_, err = f.qs.Alias(f.env.Ctx, &types.QueryAliasRequest{Id: first.Id})
	require.Error(t, err)
	dead, err := f.qs.Retired(f.env.Ctx, &types.QueryRetiredRequest{Id: first.Id})
	require.NoError(t, err)
	require.True(t, dead.Retired)

	// One identifier per account still holds.
	held, err := f.k.Owners.Get(f.env.Ctx, customer)
	require.NoError(t, err)
	require.Equal(t, moved.Id, held)
}

// Resubmitting the same country is the ordinary consequence of a timeout, and
// it must not cost the account its live handle.
func TestRecordingTheSameCountryTwiceRetiresNothing(t *testing.T) {
	f := jSetup(t)
	customer := f.customerOf(t, f.bank)

	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.bank, Account: customer, Country: "NG",
	})
	require.NoError(t, err)
	first, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: customer})
	require.NoError(t, err)

	again, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.admin, Account: customer, Country: "ng",
	})
	require.NoError(t, err)
	require.Empty(t, again.Retired)

	held, err := f.k.Owners.Get(f.env.Ctx, customer)
	require.NoError(t, err)
	require.Equal(t, first.Id, held)
}

// An authority's whole reason for querying is to know what it may act on, so a
// perimeter that still listed an account that has left it would be worse than
// none.
func TestPerimeterListsOneCountryAndFollowsCorrections(t *testing.T) {
	f := jSetup(t)

	inNG := []string{f.customerOf(t, f.bank), f.customerOf(t, f.bank)}
	inGH := []string{f.customerOf(t, f.rival)}
	for _, a := range inNG {
		_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
			Recorder: f.bank, Account: a, Country: "NG",
		})
		require.NoError(t, err)
	}
	for _, a := range inGH {
		_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
			Recorder: f.rival, Account: a, Country: "GH",
		})
		require.NoError(t, err)
	}

	require.ElementsMatch(t, inNG, f.perimeter(t, "NG"))
	require.ElementsMatch(t, inGH, f.perimeter(t, "GH"))

	// Move one across the border.
	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.admin, Account: inNG[0], Country: "GH",
	})
	require.NoError(t, err)

	require.ElementsMatch(t, inNG[1:], f.perimeter(t, "NG"),
		"an account that left the perimeter is still listed inside it")
	require.ElementsMatch(t, append(inGH, inNG[0]), f.perimeter(t, "GH"))
}

func (f *jFixture) perimeter(t *testing.T, cc string) []string {
	t.Helper()
	res, err := f.qs.Perimeter(f.env.Ctx, &types.QueryPerimeterRequest{Country: cc})
	require.NoError(t, err)
	addrs := make([]string, 0, len(res.Jurisdictions))
	for _, j := range res.Jurisdictions {
		require.Equal(t, cc, j.Country, "the index returned a record from another country")
		addrs = append(addrs, j.Address)
	}
	return addrs
}

// Retired-never-reissued has to survive everything that now retires an
// identifier — rotation and a jurisdiction correction both.
func TestARetiredIdentifierIsNeverReissuedAcrossCorrections(t *testing.T) {
	f := jSetup(t)
	customer := f.customerOf(t, f.bank)

	_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: f.bank, Account: customer, Country: "NG",
	})
	require.NoError(t, err)
	first, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: customer})
	require.NoError(t, err)

	seen := map[string]bool{first.Id: true}
	for i, cc := range []string{"GH", "NG", "GH", "NG", "GH", "KE", "NG", "KE"} {
		moved, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
			Recorder: f.admin, Account: customer, Country: cc,
		})
		require.NoError(t, err)
		require.Falsef(t, seen[moved.Id], "round %d reissued %q", i, moved.Id)
		seen[moved.Id] = true

		rot, err := f.ms.RotateAlias(f.env.Ctx, &types.MsgRotateAlias{Account: customer})
		require.NoError(t, err)
		require.Falsef(t, seen[rot.Id], "round %d reissued %q on rotation", i, rot.Id)
		require.Equalf(t, cc, types.Country(rot.Id),
			"rotation carried a country the chain does not record")
		seen[rot.Id] = true
	}

	for id := range seen {
		if held, _ := f.k.Owners.Get(f.env.Ctx, customer); held == id {
			continue
		}
		dead, err := f.k.Retired.Has(f.env.Ctx, id)
		require.NoError(t, err)
		require.Truef(t, dead, "identifier %q lost its tombstone", id)
	}
}

func TestGenesisRoundTripsWithJurisdictions(t *testing.T) {
	f := jSetup(t)

	for i, cc := range []string{"NG", "GH", "CI", "SL", "ZA"} {
		customer := f.customerOf(t, f.bank)
		_, err := f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
			Recorder: f.bank, Account: customer, Country: cc,
		})
		require.NoError(t, err)
		_, err = f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: customer})
		require.NoError(t, err)
		if i%2 == 0 {
			_, err = f.ms.RotateAlias(f.env.Ctx, &types.MsgRotateAlias{Account: customer})
			require.NoError(t, err)
		}
	}
	// The administrator's identifier, the only one with no jurisdiction behind
	// it, has to survive the round trip too.
	_, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: f.admin})
	require.NoError(t, err)

	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate())

	// A second environment, so the import lands in an empty store.
	//
	// Reusing the first one would leave both derived indexes already populated
	// by the messages above, and the assertions below would pass whether
	// InitGenesis rebuilt them or not — which is exactly what they exist to
	// check.
	other := integration.NewWith(t,
		[]string{types.ModuleName, paymsgtypes.ModuleName},
		module.AppModule{}, paymsgmodule.AppModule{})
	fresh := keeper.NewKeeper(other.Codec, other.AddressCodec,
		other.Store(types.ModuleName), log.NewNopLogger(), other.AuthorityString(t), nil, nil, nil)
	require.NoError(t, fresh.InitGenesis(other.Ctx, *exported))

	again, err := fresh.ExportGenesis(other.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, again)

	// Both derived indexes are rebuilt rather than carried, so both have to
	// agree with the records they index.
	freshQS := keeper.NewQueryServerImpl(fresh)
	for _, a := range again.Aliases {
		held, err := fresh.Owners.Get(other.Ctx, a.Address)
		require.NoError(t, err)
		require.Equal(t, a.Id, held)
	}
	for _, j := range again.Jurisdictions {
		res, err := freshQS.Perimeter(other.Ctx, &types.QueryPerimeterRequest{Country: j.Country})
		require.NoError(t, err)
		listed := false
		for _, r := range res.Jurisdictions {
			listed = listed || r.Address == j.Address
		}
		require.Truef(t, listed, "%s is recorded in %s but the rebuilt index does not list it",
			j.Address, j.Country)
	}
}

// Genesis is the one place an identifier is written without passing through the
// handler that checks its prefix, so the check has to exist here too.
func TestGenesisRefusesALyingPrefix(t *testing.T) {
	_, admin := integration.New(t, types.ModuleName, module.AppModule{}).Addr(t)
	const acct = "yml1chmca667fk4wtsf47ghnrzvnfgw7kds4u97a8p"

	// An identifier claiming a country the account is not recorded in.
	gs := types.DefaultGenesis()
	gs.Jurisdictions = []types.Jurisdiction{{Address: acct, Country: "GH"}}
	gs.Aliases = []types.Alias{{Id: types.Derive("NG", acct, 0, types.PayloadLength), Address: acct}}
	require.ErrorContains(t, gs.Validate(), "claims NG")

	// An identifier for an account nobody placed.
	gs = types.DefaultGenesis()
	gs.Aliases = []types.Alias{{Id: types.Derive("NG", acct, 0, types.PayloadLength), Address: acct}}
	require.ErrorContains(t, gs.Validate(), "no recorded jurisdiction")

	// The foundation prefix worn by an account holding no administrator grant.
	gs = types.DefaultGenesis()
	gs.Aliases = []types.Alias{{
		Id: types.Derive(types.FoundationCountry, acct, 0, types.PayloadLength), Address: acct,
	}}
	require.ErrorContains(t, gs.Validate(), "holds no chain-wide grant")

	// A grant of the role naming a COUNTRY does not carry the exemption, and the
	// file is refused outright rather than loading with an identifier nothing
	// backs. The rule is checked in the file for the same reason it is checked in
	// the handler: nothing re-examines a grant seeded at height zero.
	gs = types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{{
		Holder: acct, Role: types.ROLE_FOUNDATION_ADMINISTRATOR, Jurisdiction: "NG",
	}}
	gs.Aliases = []types.Alias{{
		Id: types.Derive(types.FoundationCountry, acct, 0, types.PayloadLength), Address: acct,
	}}
	require.ErrorContains(t, gs.Validate(), `is "*" or nothing`)

	// And the same file with the chain-wide grant loads.
	gs = types.DefaultGenesis()
	gs.RoleGrants = []types.RoleGrant{administratorGrant(acct)}
	gs.Aliases = []types.Alias{{
		Id: types.Derive(types.FoundationCountry, acct, 0, types.PayloadLength), Address: acct,
	}}
	require.NoError(t, gs.Validate())
	_ = admin
}

// The exemption is the one rule the perimeter rests on, so every way it could
// widen without anyone noticing is closed.
//
// The three closures used to be properties of a parameter list — governance
// only, no duplicates, capped at eight — and two of them stopped being checks at
// all when the administrators became grants, because a grant registry cannot
// express them. That is the point rather than a loss, so this test now says
// which mechanism carries each one.
func TestTheExemptionCannotWidenQuietly(t *testing.T) {
	f := jSetup(t)

	// Governance only, and it still is. A chain-wide grant may be made by
	// governance and by nobody else, so an administrator cannot appoint another
	// one — including the foundation, which may admit a country and may not
	// manufacture authority over every country.
	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority:    f.admin,
		Holder:       f.outsider,
		Role:         types.ROLE_FOUNDATION_ADMINISTRATOR,
		Jurisdiction: types.ChainWide,
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	// A country scope is refused. This is what keeps the appointment as
	// governance-only as the parameter list was: without it the foundation could
	// sign the message above by naming a country.
	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority:    f.env.AuthorityString(t),
		Holder:       f.outsider,
		Role:         types.ROLE_FOUNDATION_ADMINISTRATOR,
		Jurisdiction: "NG",
	})
	require.ErrorIs(t, err, types.ErrInvalidScope)

	// A duplicate is no longer refused; it is impossible. A grant is keyed by
	// (holder, role, jurisdiction), so granting the same triple twice writes one
	// record and the second call is the idempotence every other role has.
	grant := &types.MsgGrantRole{
		Authority:    f.env.AuthorityString(t),
		Holder:       f.outsider,
		Role:         types.ROLE_FOUNDATION_ADMINISTRATOR,
		Jurisdiction: types.ChainWide,
	}
	_, err = f.ms.GrantRole(f.env.Ctx, grant)
	require.NoError(t, err)
	_, err = f.ms.GrantRole(f.env.Ctx, grant)
	require.NoError(t, err)
	held, err := f.k.GrantsOf(f.env.Ctx, f.outsider)
	require.NoError(t, err)
	require.Len(t, held, 1, "granting the same triple twice must write one record")

	// And the cap holds, so a proposal cannot widen the exemption because nobody
	// scrolled. Two are already held — the fixture's administrator and the one
	// just granted — so the cap is reached six grants later.
	for i := 0; i < types.MaxFoundationAdministrators-2; i++ {
		_, a := f.env.Addr(t)
		_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
			Authority:    f.env.AuthorityString(t),
			Holder:       a,
			Role:         types.ROLE_FOUNDATION_ADMINISTRATOR,
			Jurisdiction: types.ChainWide,
		})
		require.NoErrorf(t, err, "administrator %d of %d", i+3, types.MaxFoundationAdministrators)
	}
	_, ninth := f.env.Addr(t)
	_, err = f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority:    f.env.AuthorityString(t),
		Holder:       ninth,
		Role:         types.ROLE_FOUNDATION_ADMINISTRATOR,
		Jurisdiction: types.ChainWide,
	})
	require.ErrorIs(t, err, types.ErrInvalidRole)
	require.ErrorContains(t, err, "at most 8 accounts may hold")

	// Re-granting one that already holds it is still accepted at the cap, or the
	// eighth administrator's grant could never be amended.
	_, err = f.ms.GrantRole(f.env.Ctx, grant)
	require.NoError(t, err)
}

// The exemption covers absence of a jurisdiction and nothing else, and it covers
// exactly the accounts holding the grant.
func TestTheExemptionCoversOnlyItsHolders(t *testing.T) {
	f := jSetup(t)

	_, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: f.outsider})
	require.ErrorIs(t, err, types.ErrNoJurisdiction)

	res, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: f.admin})
	require.NoError(t, err)
	require.Equal(t, types.FoundationCountry, types.Country(res.Id))
}

// The devnet's prefixless identifiers, handled deliberately.
func TestMigrationRetiresEveryPrefixlessIdentifier(t *testing.T) {
	f := jSetup(t)

	// v1 state, written the way v1 wrote it: no jurisdiction anywhere, and
	// identifiers with no country in them.
	legacy := map[string]string{}
	for i := 0; i < 3; i++ {
		_, addr := f.env.Addr(t)
		id := legacyID(t, addr)
		require.NoError(t, f.k.Aliases.Set(f.env.Ctx, id, types.Alias{Id: id, Address: addr}))
		require.NoError(t, f.k.Owners.Set(f.env.Ctx, addr, id))
		legacy[addr] = id
	}

	require.NoError(t, keeper.NewMigrator(f.k).Migrate1to2(f.env.Ctx))

	for addr, id := range legacy {
		// Nothing resolves, in either direction.
		_, err := f.qs.Alias(f.env.Ctx, &types.QueryAliasRequest{Id: id})
		require.Errorf(t, err, "prefixless identifier %q still resolves", id)
		_, err = f.qs.AliasOf(f.env.Ctx, &types.QueryAliasOfRequest{Address: addr})
		require.Error(t, err)

		// But somebody holding the old handle written down is told it was given
		// up, not that it never existed.
		dead, err := f.k.Retired.Has(f.env.Ctx, id)
		require.NoError(t, err)
		require.Truef(t, dead, "%q was deleted rather than tombstoned", id)
	}

	// The migration invents no perimeters: an account is placed by its
	// participant afterwards, and only then gets an identifier again.
	for addr := range legacy {
		_, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: addr})
		require.ErrorIs(t, err, types.ErrNoJurisdiction)
	}

	// The tombstones survive a genesis round trip, so the guarantee outlives
	// the upgrade that created them.
	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, exported.Validate(), "the migration's tombstones fail genesis validation")
	require.Len(t, exported.Retired, len(legacy))
}

// legacyID builds an identifier the way the module did before countries, so the
// migration runs against the shape it will actually meet.
//
// The check character is found by trying each symbol against ValidLegacy rather
// than by reaching into the package, so the fixture is built only from what a
// v1 node would have accepted.
func legacyID(t *testing.T, addr string) string {
	t.Helper()
	id := types.Derive("ZZ", addr, 0, types.PayloadLength)
	payload := id[types.CountryLength : len(id)-1]
	for i := 0; i < len(types.Alphabet); i++ {
		if candidate := payload + string(types.Alphabet[i]); types.ValidLegacy(candidate) {
			return candidate
		}
	}
	t.Fatalf("no check character makes %q a valid pre-jurisdiction identifier", payload)
	return ""
}
