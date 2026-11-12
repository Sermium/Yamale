package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

func TestARegisteredJewelIsTheJewelersStock(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "md 2026-0001")

	j := f.jewel(t, id)
	require.Equal(t, jid, j.JewelerId)
	require.Equal(t, "MD20260001", j.Inscription)
	require.Zero(t, j.OwnerCount, "stock is not an owner")
	require.Equal(t, business, f.owner(t, id))
}

func TestAnInscriptionIsRegisteredOncePerJeweler(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	first := f.register(t, staff, "MD-2026-0001")
	_, err := f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: staff, Inscription: "md 2026 0001", RecordHash: recordHash, RecordUri: "u",
	})
	require.ErrorIs(t, err, types.ErrInscriptionTaken)
	require.ErrorContains(t, err, "jewel 1", "the error names the existing jewel")
	_ = first
}

// Two houses may use the same code; the search shows both, with their jeweler.
func TestTwoJewelersMayShareACode(t *testing.T) {
	f := setup(t)
	_, _, a := f.admit(t, "Maison Diop")
	_, _, b := f.admit(t, "Atelier Ba")
	f.register(t, a, "RING-0001")
	f.register(t, b, "RING-0001")
}

func TestOnlyActiveStaffRegister(t *testing.T) {
	f := setup(t)
	id, business, staff := f.admit(t, "Maison Diop")

	_, err := f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: business, Inscription: "MD-0001", RecordHash: recordHash, RecordUri: "u",
	})
	require.ErrorIs(t, err, types.ErrNotStaff, "the business account is not staff")

	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: id, Reason: "audit"})
	require.NoError(t, err)
	_, err = f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: staff, Inscription: "MD-0001", RecordHash: recordHash, RecordUri: "u",
	})
	require.ErrorIs(t, err, types.ErrJewelerSuspended)
}

func TestNothingIsMintedWhileTheBareSendIsOpen(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	f.circuit.open = true
	_, err := f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: staff, Inscription: "MD-0001", RecordHash: recordHash, RecordUri: "u",
	})
	require.ErrorIs(t, err, types.ErrBareSendOpen)
}

func TestAnAmendmentAppendsAndNeverOverwrites(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	corrected := "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"

	_, err := f.srv.AmendJewel(f.env.Ctx, &types.MsgAmendJewel{Staff: staff, JewelId: id,
		RecordHash: corrected, RecordUri: "u2"})
	require.ErrorIs(t, err, types.ErrNoReason)

	_, err = f.srv.AmendJewel(f.env.Ctx, &types.MsgAmendJewel{Staff: staff, JewelId: id,
		RecordHash: corrected, RecordUri: "u2", Note: "carat weight 1.02, not 1.20"})
	require.NoError(t, err)

	j := f.jewel(t, id)
	require.Equal(t, recordHash, j.RecordHash, "the original stays")
	require.Len(t, j.Amendments, 1)
	require.Equal(t, corrected, j.Amendments[0].RecordHash)
	token, _ := f.nft.GetNFT(f.env.Ctx, types.ClassID(j.JewelerId), types.TokenID(id))
	require.Equal(t, corrected, token.UriHash, "x/nft points at the latest record")
}

func TestAnotherJewelersStaffCannotAmend(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	_, _, other := f.admit(t, "Atelier Ba")
	id := f.register(t, staff, "MD-0001")
	_, err := f.srv.AmendJewel(f.env.Ctx, &types.MsgAmendJewel{Staff: other, JewelId: id,
		RecordHash: recordHash, RecordUri: "u", Note: "x"})
	require.ErrorIs(t, err, types.ErrNotStaff)
}
