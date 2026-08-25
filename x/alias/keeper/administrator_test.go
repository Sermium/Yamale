package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/alias/types"
)

// The foundation administrator asks two different questions of one grant, and
// this file is about the difference between them.
//
// Placement is a fact: "where is this account", which the ZZ exemption answers
// and which AssertScope asks about a TARGET. Authority is an act: correcting a
// country, appointing a regulator, granting the auditor role. An office's
// recorded M-of-N bears on the second and not on the first, and getting that
// backwards fails closed in the wrong direction — it would remove an account
// from every perimeter rather than refuse it a power.

// An administrator office that has fallen below the shape its grant records
// cannot correct a country, and is told which of the two problems it has.
func TestAnAdministratorBelowItsShapeCannotCorrectACountry(t *testing.T) {
	f := roleSetup(t)

	// A three-of-five office, granted the role under a requirement it meets.
	office := f.officeShaped(t, 3, 5)
	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority:     f.env.AuthorityString(t),
		Holder:        office,
		Role:          types.ROLE_FOUNDATION_ADMINISTRATOR,
		Jurisdiction:  types.ChainWide,
		RequiredShape: shape(3, 5),
	})
	require.NoError(t, err)

	account := f.placed(t, "NG")

	// While it keeps its shape it may correct.
	_, err = f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: office, Account: account, Country: "GH",
	})
	require.NoError(t, err)

	// An office administers itself, so it can vote itself down to one key. That
	// is the whole reason the requirement is on the grant and re-read rather than
	// checked once when the grant was made.
	f.groups.add(office, 1, 1)

	_, err = f.ms.SetJurisdiction(f.env.Ctx, &types.MsgSetJurisdiction{
		Recorder: office, Account: account, Country: "KE",
	})
	require.ErrorIs(t, err, types.ErrOfficeShape)
	// The message matters as much as the code. "Your office is a one-of-one and
	// this grant requires three-of-five" is a proposal the office passes by
	// itself; "you are not an administrator" sends somebody to governance for a
	// grant they already hold.
	require.ErrorContains(t, err, "1-of-1")

	country, err := f.k.CountryOf(f.env.Ctx, account)
	require.NoError(t, err)
	require.Equal(t, "GH", country, "a refused correction must change nothing")
}

// The same office keeps the exemption itself, because being placed nowhere is a
// fact about it rather than a power it is exercising.
func TestAnAdministratorBelowItsShapeStillHasNoCountry(t *testing.T) {
	f := roleSetup(t)

	office := f.officeShaped(t, 3, 5)
	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority:     f.env.AuthorityString(t),
		Holder:        office,
		Role:          types.ROLE_FOUNDATION_ADMINISTRATOR,
		Jurisdiction:  types.ChainWide,
		RequiredShape: shape(3, 5),
	})
	require.NoError(t, err)

	f.groups.add(office, 1, 1)

	// If CountryOf consulted the shape, this would be ErrNoJurisdiction — and
	// the office would have stopped being an account any authority could act on,
	// which is a sentence about the wrong thing entirely.
	country, err := f.k.CountryOf(f.env.Ctx, office)
	require.NoError(t, err)
	require.Equal(t, types.FoundationCountry, country)

	// And its own identifier is still issuable, which is the visible half of the
	// same property.
	res, err := f.ms.RegisterAlias(f.env.Ctx, &types.MsgRegisterAlias{Account: office})
	require.NoError(t, err)
	require.Equal(t, types.FoundationCountry, types.Country(res.Id))
}

// A store the module cannot read is not a permission it denied.
//
// Every path here returns the error rather than a false, so a lookup that could
// not be made refuses instead of quietly reporting "not an administrator" — the
// same rule the rest of the module follows, and the one an unwired keeper would
// otherwise walk around.
func TestAGenesisFileCannotGrantMoreAdministratorsThanTheCap(t *testing.T) {
	env, _, _, _ := setup(t)

	gs := types.DefaultGenesis()
	for i := 0; i <= types.MaxFoundationAdministrators; i++ {
		_, addr := env.Addr(t)
		gs.RoleGrants = append(gs.RoleGrants, administratorGrant(addr))
	}
	require.Len(t, gs.RoleGrants, types.MaxFoundationAdministrators+1)

	err := gs.Validate()
	require.Error(t, err, "a file may not seed a ninth administrator; nothing re-examines a grant written at height zero")
	require.ErrorContains(t, err, "at most 8 accounts may hold")

	// Exactly the cap loads, so the check bounds the set rather than forbidding
	// a full one.
	gs.RoleGrants = gs.RoleGrants[:types.MaxFoundationAdministrators]
	require.NoError(t, gs.Validate())
}

// The confidentiality grants are the administrator's other authority, and they
// are held to the office's shape exactly as correcting a country is.
//
// A mutation pass found this one. Deleting the error branch in
// assertChainAuthority changed no test result, because every existing test of
// that function used an administrator with no recorded shape. What the deleted
// branch actually does is turn "your office fell below its M-of-N" into "you are
// not an administrator" — and, worse, turn a store failure into the same
// sentence, which is the permissive reading of a question that could not be
// asked.
func TestAnAdministratorBelowItsShapeCannotAppointARegulatorOrAnAuditor(t *testing.T) {
	f := roleSetup(t)

	office := f.officeShaped(t, 3, 5)
	_, err := f.ms.GrantRole(f.env.Ctx, &types.MsgGrantRole{
		Authority:     f.env.AuthorityString(t),
		Holder:        office,
		Role:          types.ROLE_FOUNDATION_ADMINISTRATOR,
		Jurisdiction:  types.ChainWide,
		RequiredShape: shape(3, 5),
	})
	require.NoError(t, err)

	// A supervisor for it to appoint in both countries, so the appointment below
	// fails on the signer rather than on the appointee.
	watcher := f.office(t)
	f.grant(t, watcher, types.ROLE_SUPERVISOR, "NG")
	f.grant(t, watcher, types.ROLE_SUPERVISOR, "GH")

	// While it keeps its shape, the appointment is permitted.
	_, err = f.ms.AppointRegulator(f.env.Ctx, &types.MsgAppointRegulator{
		Authority: office, Country: "NG", Address: watcher,
	})
	require.NoError(t, err)

	f.groups.add(office, 1, 1)

	_, err = f.ms.AppointRegulator(f.env.Ctx, &types.MsgAppointRegulator{
		Authority: office, Country: "GH", Address: watcher,
	})
	require.ErrorIs(t, err, types.ErrOfficeShape)
	require.ErrorContains(t, err, "1-of-1")

	_, auditor := f.env.Addr(t)
	_, err = f.ms.GrantAuditor(f.env.Ctx, &types.MsgGrantAuditor{
		Authority: office, Address: auditor,
		ExpiresAtHeight: f.env.Ctx.BlockHeight() + 100,
	})
	require.ErrorIs(t, err, types.ErrOfficeShape)

	// The fallen office is told about its shape rather than that it is not an
	// administrator, which is the distinction the returned error carries and the
	// whole reason it is returned rather than folded into a false.
	require.NotErrorIs(t, err, types.ErrInvalidSigner)
}
