package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/land/keeper"
	"yamale/blockchain/x/land/types"
)

// fixture builds a registry with one office in charge and three independent
// offices able to attest — the smallest set that can actually exercise the
// quorum rule rather than assert it.
type fixture struct {
	env    *integration.Env
	k      keeper.Keeper
	srv    types.MsgServer
	office string
	others []string
	holder string
	buyer  string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := integration.New(t, types.ModuleName)

	// nil group keeper: these tests exercise the transfer rules, not admission.
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, nil)
	require.NoError(t, k.Params.Set(env.Ctx, types.DefaultParams()))

	f := &fixture{env: env, k: k, srv: keeper.NewMsgServerImpl(k)}

	admit := func() string {
		_, addr := env.Addr(t)
		require.NoError(t, k.Authority.Set(env.Ctx, addr, types.Authority{
			Address: addr, Name: "office", Jurisdiction: "test", Active: true,
		}))
		return addr
	}
	f.office = admit()
	f.others = []string{admit(), admit(), admit()}

	_, f.holder = env.Addr(t)
	_, f.buyer = env.Addr(t)
	return f
}

// Parcel 0 must never exist: x/tokenisation says "this vehicle is over land" by
// carrying a parcel id, and a zero id is what an unset proto field looks like.
func TestParcelIdsNeverStartAtZero(t *testing.T) {
	f := setup(t)
	require.Equal(t, uint64(1), f.register(t, "survey-A", "REF-001"))
	require.Equal(t, uint64(2), f.register(t, "survey-B", "REF-002"))
}

func (f *fixture) register(t *testing.T, geometry, ref string) uint64 {
	t.Helper()
	res, err := f.srv.RegisterParcel(f.env.Ctx, &types.MsgRegisterParcel{
		Creator: f.office, GeometryHash: geometry, CadastralRef: ref, Holder: f.holder,
	})
	require.NoError(t, err)
	return res.Id
}

// The guarantee the whole module exists for: the same ground cannot be titled
// twice. Tested by trying, not by inspecting the index.
func TestSameGroundCannotBeTitledTwice(t *testing.T) {
	f := setup(t)
	f.register(t, "survey-hash-A", "REF-001")

	_, err := f.srv.RegisterParcel(f.env.Ctx, &types.MsgRegisterParcel{
		Creator: f.office, GeometryHash: "survey-hash-A", CadastralRef: "REF-002",
		Holder: f.buyer,
	})
	require.ErrorIs(t, err, types.ErrGeometryTitled)
}

func TestCadastralReferenceCannotBeReused(t *testing.T) {
	f := setup(t)
	f.register(t, "survey-hash-A", "REF-001")

	_, err := f.srv.RegisterParcel(f.env.Ctx, &types.MsgRegisterParcel{
		Creator: f.office, GeometryHash: "survey-hash-B", CadastralRef: "REF-001",
		Holder: f.buyer,
	})
	require.ErrorIs(t, err, types.ErrRefTaken)
}

func TestOnlyAnOfficeMayRegister(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterParcel(f.env.Ctx, &types.MsgRegisterParcel{
		Creator: f.holder, GeometryHash: "g", CadastralRef: "r", Holder: f.holder,
	})
	require.ErrorIs(t, err, types.ErrNotAuthority)
}

// An authority must not be able to start the sale of somebody's land.
func TestOnlyTheHolderMayProposeATransfer(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")

	_, err := f.srv.ProposeTransfer(f.env.Ctx, &types.MsgProposeTransfer{
		Creator: f.office, ParcelId: id, To: f.buyer,
	})
	require.ErrorIs(t, err, types.ErrNotHolder)
}

// The anti-bribery rule, stated as a test: the office holding the parcel cannot
// also supply an attestation toward the quorum that checks it.
func TestProposingOfficeCannotAttest(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")
	tid := f.propose(t, id)

	_, err := f.srv.AttestTransfer(f.env.Ctx, &types.MsgAttestTransfer{
		Creator: f.office, TransferId: tid,
	})
	require.ErrorIs(t, err, types.ErrNotIndependent)
}

func TestAnOfficeCannotAttestTwice(t *testing.T) {
	f := setup(t)
	tid := f.propose(t, f.register(t, "g", "r"))

	_, err := f.srv.AttestTransfer(f.env.Ctx, &types.MsgAttestTransfer{
		Creator: f.others[0], TransferId: tid,
	})
	require.NoError(t, err)

	_, err = f.srv.AttestTransfer(f.env.Ctx, &types.MsgAttestTransfer{
		Creator: f.others[0], TransferId: tid,
	})
	require.ErrorIs(t, err, types.ErrAlreadyAttested)
}

func TestTransferNeedsValidationQuorumAndTime(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")
	tid := f.propose(t, id)

	// Nothing yet: no validation.
	_, err := f.srv.CompleteTransfer(f.env.Ctx, &types.MsgCompleteTransfer{
		Creator: f.buyer, TransferId: tid,
	})
	require.ErrorIs(t, err, types.ErrNotValidated)

	_, err = f.srv.ValidateTransfer(f.env.Ctx, &types.MsgValidateTransfer{
		Creator: f.office, TransferId: tid,
	})
	require.NoError(t, err)

	// Validated but unattested.
	_, err = f.srv.CompleteTransfer(f.env.Ctx, &types.MsgCompleteTransfer{
		Creator: f.buyer, TransferId: tid,
	})
	require.ErrorIs(t, err, types.ErrNoQuorum)

	f.attestAll(t, tid)

	// Quorum reached, but the challenge window has not run. This is the step
	// most likely to be quietly dropped for convenience, so it is asserted.
	_, err = f.srv.CompleteTransfer(f.env.Ctx, &types.MsgCompleteTransfer{
		Creator: f.buyer, TransferId: tid,
	})
	require.ErrorIs(t, err, types.ErrChallengeWindowOpen)

	// Past the window, anyone may finalise — including the buyer, so that no
	// official can withhold the last step as leverage.
	params, err := f.k.Params.Get(f.env.Ctx)
	require.NoError(t, err)
	f.env.Ctx = f.env.Ctx.WithBlockTime(
		f.env.Ctx.BlockTime().Add(time.Duration(params.ChallengeWindow+1) * time.Second))

	_, err = f.srv.CompleteTransfer(f.env.Ctx, &types.MsgCompleteTransfer{
		Creator: f.buyer, TransferId: tid,
	})
	require.NoError(t, err)

	parcel, err := f.k.Parcel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, f.buyer, parcel.Holder)
	require.Equal(t, types.STATUS_REGISTERED, parcel.Status)
}

// One objection stops everything, and it needs no standing — the person being
// robbed usually has none.
func TestAnyoneMayObjectAndItStopsTheTransfer(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")
	tid := f.propose(t, id)
	f.attestAll(t, tid)

	_, stranger := f.env.Addr(t)
	_, err := f.srv.Object(f.env.Ctx, &types.MsgObject{
		Creator: stranger, TransferId: tid, Reason: "this is my family's land",
	})
	require.NoError(t, err)

	parcel, err := f.k.Parcel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.STATUS_DISPUTED, parcel.Status)

	// And it cannot be completed afterwards, however much time passes.
	params, _ := f.k.Params.Get(f.env.Ctx)
	f.env.Ctx = f.env.Ctx.WithBlockTime(
		f.env.Ctx.BlockTime().Add(time.Duration(params.ChallengeWindow+1) * time.Second))
	_, err = f.srv.CompleteTransfer(f.env.Ctx, &types.MsgCompleteTransfer{
		Creator: f.buyer, TransferId: tid,
	})
	require.ErrorIs(t, err, types.ErrTransferDisputed)
}

// A parcel already being transferred cannot start a second transfer — the
// on-chain form of selling the same land twice.
func TestNoSecondTransferWhilePending(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")
	f.propose(t, id)

	_, err := f.srv.ProposeTransfer(f.env.Ctx, &types.MsgProposeTransfer{
		Creator: f.holder, ParcelId: id, To: f.buyer,
	})
	require.ErrorIs(t, err, types.ErrParcelNotTransferable)
}

// A restriction outranks the office's own permission, or restrictions would be
// decorative.
func TestRestrictionBlocksFractionalisation(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")

	_, err := f.srv.SetRestriction(f.env.Ctx, &types.MsgSetRestriction{
		Creator: f.office, ParcelId: id,
		Kind: types.RestrictionNoFractionalisation, Detail: "customary tenure",
	})
	require.NoError(t, err)

	_, err = f.srv.AuthoriseFractionalisation(f.env.Ctx,
		&types.MsgAuthoriseFractionalisation{
			Creator: f.office, ParcelId: id, Right: "exploitation", MaxShareBps: 4000,
			ExpiresAt: farFuture,
		})
	require.ErrorIs(t, err, types.ErrFractionalisationForbidden)

	// Nothing was recorded. A refused grant that still wrote the permission
	// would leave x/tokenisation reading an authorisation the office never
	// gave, which is the whole failure this refusal exists to prevent.
	_, err = f.k.FractionalisationAuthority.Get(f.env.Ctx, id)
	require.Error(t, err)
}

// farFuture is an expiry comfortably past any block time these tests run at.
const farFuture = int64(4_102_444_800) // 2100-01-01

// The authorisation is a record, not a message that happened. x/tokenisation
// checks it at every issuance, so a grant that left nothing behind would leave
// the ceiling and the expiry unenforceable.
func TestAuthorisationIsRecordedAndWithdrawable(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")

	_, err := f.srv.AuthoriseFractionalisation(f.env.Ctx,
		&types.MsgAuthoriseFractionalisation{
			Creator: f.office, ParcelId: id, Right: "exploitation",
			MaxShareBps: 4000, ExpiresAt: farFuture,
		})
	require.NoError(t, err)

	auth, err := f.k.FractionalisationAuthority.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, uint32(4000), auth.MaxShareBps)
	require.Equal(t, farFuture, auth.ExpiresAt)
	require.Equal(t, f.office, auth.GrantedBy)
	require.True(t, auth.Live(0))

	_, err = f.srv.AuthoriseFractionalisation(f.env.Ctx,
		&types.MsgAuthoriseFractionalisation{
			Creator: f.office, ParcelId: id, Withdraw: true,
		})
	require.NoError(t, err)

	// Marked, not deleted: the evidence that the registry once permitted this,
	// and who permitted it, has to survive the withdrawal.
	auth, err = f.k.FractionalisationAuthority.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.True(t, auth.Withdrawn)
	require.Equal(t, f.office, auth.GrantedBy)
	require.False(t, auth.Live(0))
}

// Zero is what an unset field looks like. Read as "no expiry" it would be a
// permission that never lapses, which is the one thing the expiry exists to
// stop.
func TestAuthorisationRefusesAnUnsetExpiry(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")

	_, err := f.srv.AuthoriseFractionalisation(f.env.Ctx,
		&types.MsgAuthoriseFractionalisation{
			Creator: f.office, ParcelId: id, Right: "exploitation", MaxShareBps: 4000,
		})
	require.ErrorIs(t, err, types.ErrBadExpiry)
}

func TestParamsRejectADisabledQuorum(t *testing.T) {
	p := types.DefaultParams()
	p.AttestationQuorum = 0
	require.Error(t, p.Validate())

	p = types.DefaultParams()
	p.ChallengeWindow = 0
	require.Error(t, p.Validate())
}

func (f *fixture) propose(t *testing.T, parcelID uint64) uint64 {
	t.Helper()
	res, err := f.srv.ProposeTransfer(f.env.Ctx, &types.MsgProposeTransfer{
		Creator: f.holder, ParcelId: parcelID, To: f.buyer, Price: "1000",
	})
	require.NoError(t, err)
	return res.TransferId
}

func (f *fixture) attestAll(t *testing.T, transferID uint64) {
	t.Helper()
	_, err := f.srv.ValidateTransfer(f.env.Ctx, &types.MsgValidateTransfer{
		Creator: f.office, TransferId: transferID,
	})
	if err != nil {
		require.ErrorIs(t, err, types.ErrAlreadyValidated)
	}
	for _, office := range f.others {
		_, err := f.srv.AttestTransfer(f.env.Ctx, &types.MsgAttestTransfer{
			Creator: office, TransferId: transferID,
		})
		require.NoError(t, err)
	}
}

// The freeze was the one act on this module that required a justification and
// then threw it away. A holder could read that their land was stopped and
// nothing about why, which leaves a fraud inquiry and an extortion looking
// identical from the register — and neither can be argued with.
func TestFreezeRecordsItsGrounds(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")

	const order = "court order 2026/114, succession disputed"
	_, err := f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: id, Reason: order,
	})
	require.NoError(t, err)

	parcel, err := f.k.Parcel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.STATUS_FROZEN, parcel.Status)
	require.Len(t, parcel.Freezes, 1)
	require.Equal(t, order, parcel.Freezes[0].Reason)
	require.Equal(t, f.office, parcel.Freezes[0].ImposedBy)
	require.False(t, parcel.Freezes[0].Lifted)
	require.Equal(t, 0, parcel.LiveFreeze())
}

// A freeze without grounds is still refused, and the refusal must leave no
// entry behind — a record of a freeze that did not happen is worse than none.
func TestFreezeWithoutGroundsIsRefused(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")

	_, err := f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: id,
	})
	require.ErrorIs(t, err, types.ErrNoReason)

	parcel, err := f.k.Parcel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Empty(t, parcel.Freezes)
	require.Equal(t, types.STATUS_REGISTERED, parcel.Status)
}

// Lifting marks the entry rather than removing it, like a released encumbrance:
// a freeze that vanishes takes with it the evidence that an office ever stopped
// this land. The release is attributed for the same reason the freeze is.
func TestLiftingAFreezeMarksItRatherThanErasingIt(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")

	_, err := f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: id, Reason: "fraud inquiry",
	})
	require.NoError(t, err)
	_, err = f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: id, Unfreeze: true,
		Reason: "inquiry closed with no finding",
	})
	require.NoError(t, err)

	parcel, err := f.k.Parcel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.STATUS_REGISTERED, parcel.Status)
	require.Len(t, parcel.Freezes, 1)
	require.Equal(t, "fraud inquiry", parcel.Freezes[0].Reason)
	require.True(t, parcel.Freezes[0].Lifted)
	require.Equal(t, f.office, parcel.Freezes[0].LiftedBy)
	require.Equal(t, "inquiry closed with no finding", parcel.Freezes[0].LiftReason)
	require.Equal(t, -1, parcel.LiveFreeze())

	// Refreezing appends rather than reopening the closed entry. A parcel
	// stopped twice by two different orders has two orders in its history, and
	// somebody contesting the second one needs to be able to show the first.
	_, err = f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: id, Reason: "appeal lodged",
	})
	require.NoError(t, err)

	parcel, err = f.k.Parcel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Len(t, parcel.Freezes, 2)
	require.True(t, parcel.Freezes[0].Lifted)
	require.Equal(t, "appeal lodged", parcel.Freezes[1].Reason)
	require.Equal(t, 1, parcel.LiveFreeze())
}

// A registry imported from a genesis written before freezes were recorded has
// parcels that are FROZEN with no entry to lift. Lifting one must work and must
// not manufacture a record: an entry invented at lift time would assert grounds
// no office ever gave, and an entry invented at import would put derived data
// in an export that InitGenesis never received.
func TestLiftingAFreezeWithNoRecordedOrder(t *testing.T) {
	f := setup(t)
	id := f.register(t, "g", "r")

	parcel, err := f.k.Parcel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	parcel.Status = types.STATUS_FROZEN
	require.NoError(t, f.k.Parcel.Set(f.env.Ctx, id, parcel))

	_, err = f.srv.FreezeParcel(f.env.Ctx, &types.MsgFreezeParcel{
		Creator: f.office, ParcelId: id, Unfreeze: true,
	})
	require.NoError(t, err)

	parcel, err = f.k.Parcel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, types.STATUS_REGISTERED, parcel.Status)
	require.Empty(t, parcel.Freezes)
}
