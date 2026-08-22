package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/land/keeper"
	"yamale/blockchain/x/land/types"
)

// The jurisdictional perimeter in the registry.
//
// This module uses the shape of the check where the jurisdiction is named rather
// than looked up, because a registry office administers a country and that
// country is a field on its own admission record. Land sits where it sits: a
// Ghanaian parcel held by a Nigerian is Ghana's to administer, so scoping to the
// holder's country would hand a parcel to whichever authority its owner happened
// to bank with.
//
// The check is inside activeAuthority, which every authority action in the module
// goes through, so the tests below walk several of those actions rather than one:
// what is being asserted is that there is no authority path that misses it.

// admitElsewhere admits an office for a country and grants it that country,
// which is the ordinary well-formed case for a second jurisdiction.
func (f *fixture) admitElsewhere(t *testing.T, cc string) string {
	t.Helper()
	_, addr := f.env.Addr(t)
	require.NoError(t, f.k.Authority.Set(f.env.Ctx, addr, types.Authority{
		Address: addr, Name: "office", Jurisdiction: cc, Active: true,
	}))
	f.perimeter.Grant(t, addr, aliastypes.ROLE_REGISTRY_AUTHORITY, cc)
	return addr
}

// An office admitted for one country and granted another may not act at all.
//
// This is the state a governance mistake produces — the admission and the grant
// disagreeing — and it has to be a refusal rather than a resolution in favour of
// either one.
func TestAnOfficeGrantedAnotherCountryCannotAct(t *testing.T) {
	f := setup(t)

	_, mismatched := f.env.Addr(t)
	require.NoError(t, f.k.Authority.Set(f.env.Ctx, mismatched, types.Authority{
		Address: mismatched, Name: "office", Jurisdiction: "KE", Active: true,
	}))
	f.perimeter.Grant(t, mismatched, aliastypes.ROLE_REGISTRY_AUTHORITY, jurisdiction)

	_, err := f.srv.RegisterParcel(f.env.Ctx, &types.MsgRegisterParcel{
		Creator: mismatched, GeometryHash: "survey-X", CadastralRef: "REF-X", Holder: f.holder,
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)
	require.ErrorContains(t, err, "KE")

	// Granted the country it was admitted for, the very same message lands. That
	// is what makes the refusal a perimeter rather than a defect.
	f.perimeter.Grant(t, mismatched, aliastypes.ROLE_REGISTRY_AUTHORITY, "KE")
	res, err := f.srv.RegisterParcel(f.env.Ctx, &types.MsgRegisterParcel{
		Creator: mismatched, GeometryHash: "survey-X", CadastralRef: "REF-X", Holder: f.holder,
	})
	require.NoError(t, err)
	require.NotZero(t, res.Id)
}

// An office admitted with no grant at all is refused. Admission says which office
// holds a file; the grant says governance agreed that office may act.
func TestAnOfficeWithNoGrantIsRefusedEveryAuthorityAction(t *testing.T) {
	f := setup(t)
	id := f.register(t, "survey-A", "REF-001")

	_, ungranted := f.env.Addr(t)
	require.NoError(t, f.k.Authority.Set(f.env.Ctx, ungranted, types.Authority{
		Address: ungranted, Name: "office", Jurisdiction: jurisdiction, Active: true,
	}))

	_, err := f.srv.RegisterParcel(f.env.Ctx, &types.MsgRegisterParcel{
		Creator: ungranted, GeometryHash: "survey-B", CadastralRef: "REF-002", Holder: f.holder,
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope, "registering")

	_, err = f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: ungranted, ParcelId: id, Reason: "no grant",
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope, "freezing")

	_, err = f.srv.SetRestriction(f.env.Ctx, &types.MsgSetRestriction{
		Creator: ungranted, ParcelId: id, Kind: "caveat",
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope, "restricting")

	_, err = f.srv.AttachDeed(f.env.Ctx, &types.MsgAttachDeed{
		Creator: ungranted, ParcelId: id, DocumentHash: "abc",
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope, "attaching a deed")

	_, err = f.srv.AuthoriseFractionalisation(f.env.Ctx, &types.MsgAuthoriseFractionalisation{
		Creator: ungranted, ParcelId: id, Right: "lease", MaxShareBps: 1000, ExpiresAt: farFuture,
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope, "authorising fractionalisation")

	// And the parcel is untouched by every one of them.
	parcel, err := f.k.Parcel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.STATUS_REGISTERED, parcel.Status)
	require.Empty(t, parcel.Restrictions)
	require.Empty(t, parcel.Deeds)
}

// Validating and attesting a transfer go through the same check, so a revoked
// grant stops an office mid-transfer.
func TestARevokedGrantStopsAnOfficeMidTransfer(t *testing.T) {
	f := setup(t)
	id := f.register(t, "survey-A", "REF-001")

	res, err := f.srv.ProposeTransfer(f.env.Ctx, &types.MsgProposeTransfer{
		Creator: f.holder, ParcelId: id, To: f.buyer, Price: "1000",
	})
	require.NoError(t, err)

	f.perimeter.Revoke(t, f.office, aliastypes.ROLE_REGISTRY_AUTHORITY, jurisdiction)

	_, err = f.srv.ValidateTransfer(f.env.Ctx, &types.MsgValidateTransfer{
		Creator: f.office, TransferId: res.TransferId,
	})
	require.ErrorIs(t, err, aliastypes.ErrOutOfScope)

	// An attestor whose grant is intact is unaffected: the perimeter is per
	// office, not per transfer.
	_, err = f.srv.AttestTransfer(f.env.Ctx, &types.MsgAttestTransfer{
		Creator: f.others[0], TransferId: res.TransferId,
	})
	require.NoError(t, err)

	// And an attestor of another country, properly granted there, is refused
	// here — its grant covers its own jurisdiction and this parcel's office
	// belongs to a different one.
	foreign := f.admitElsewhere(t, "KE")
	_, err = f.srv.AttestTransfer(f.env.Ctx, &types.MsgAttestTransfer{
		Creator: foreign, TransferId: res.TransferId,
	})
	require.NoError(t, err,
		"an office granted its own country may attest; the parcel's perimeter is its own office's")
}

// An office admitted for somewhere that is not a country is refused at admission,
// rather than admitted into a perimeter no grant can ever cover.
func TestAnOfficeCannotBeAdmittedForSomewhereThatIsNotACountry(t *testing.T) {
	f := setup(t)
	group := f.office // any address; the group check is not what is under test

	for _, cc := range []string{"test", "", "NX", "GHA", aliastypes.ChainWide, aliastypes.FoundationCountry} {
		_, err := f.srv.RegisterAuthority(f.env.Ctx, &types.MsgRegisterAuthority{
			Authority: f.env.AuthorityString(t), Office: group,
			Name: "office", Jurisdiction: cc, Active: true,
		})
		require.ErrorIsf(t, err, types.ErrInvalidJurisdiction,
			"%q was accepted as an office's jurisdiction", cc)
	}

	// And a real code is accepted, normalised.
	_, err := f.srv.RegisterAuthority(f.env.Ctx, &types.MsgRegisterAuthority{
		Authority: f.env.AuthorityString(t), Office: group,
		Name: "office", Jurisdiction: "gh", Active: true,
	})
	require.NoError(t, err)
	office, err := f.k.Authority.Get(f.env.Ctx, group)
	require.NoError(t, err)
	require.Equal(t, "GH", office.Jurisdiction)
}

// A registry assembled with no perimeter refuses rather than permits.
//
// This is the wiring mistake that matters: a nil dependency must never be an
// authorisation. The keeper is built here exactly as the genesis tests build it,
// which is the only place in this repository that keeper exists without one.
func TestAnUnwiredPerimeterRefusesEveryAuthorityAction(t *testing.T) {
	f := setup(t)

	unwired := keeper.NewKeeper(f.env.StoreService, f.env.Codec, f.env.AddressCodec,
		f.env.Authority, nil, nil)
	srv := keeper.NewMsgServerImpl(unwired)

	_, err := srv.RegisterParcel(f.env.Ctx, &types.MsgRegisterParcel{
		Creator: f.office, GeometryHash: "survey-Z", CadastralRef: "REF-Z", Holder: f.holder,
	})
	require.ErrorIs(t, err, aliastypes.ErrNoScopeKeeper)
}
