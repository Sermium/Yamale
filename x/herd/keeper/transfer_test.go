package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/herd/keeper"
	"yamale/blockchain/x/herd/types"
)

// The supervised transfer.
//
// It is not here to trace a theft afterwards. It is here to make the lawful
// resale of a stolen animal impossible, which is what destroys its value: a
// thief can still take the beast, he simply cannot sell it with a title, and an
// animal that cannot be sold on is worth the meat rather than the pedigree.
//
// Four parties, and each of these tests removes one to show what reopens.

// transferFixture is the pedigree fixture plus what a sale needs: an animal
// held by somebody, a buyer, and two attestors accredited by an office other
// than the one holding the record — because an attestor from the recording
// office is not independent, and the module refuses it.
//
// The attestors are AGENTS, not offices, and that distinction was bought the
// hard way. Attestation required the attestor to be an Authority until a sale
// was run end to end against a real chain — and an Authority must be an x/group
// account admitted by a governance proposal, so every field veterinarian would
// have been a multi-signature institution with a parliamentary vote behind
// them. The quorum is two, so one lawful sale needed two of those to exist
// first. In a pastoral zone that is not a high bar; it is an impossible one.
type transferFixture struct {
	*fixture
	animalID  uint64
	seller    string
	buyer     string
	attestorA string
	attestorB string
	other     string // a second registry office, which accredits the attestors
}

func setupTransfer(t *testing.T) *transferFixture {
	t.Helper()
	f := setup(t)

	_, other := f.env.Addr(t)
	require.NoError(t, f.k.Authority.Set(f.env.Ctx, other, types.Authority{
		Address: other, Name: "Ordre des Vétérinaires", Jurisdiction: jurisdiction, Active: true,
	}))

	_, buyer := f.env.Addr(t)
	_, attestorA := f.env.Addr(t)
	_, attestorB := f.env.Addr(t)
	for _, a := range []string{attestorA, attestorB} {
		require.NoError(t, f.k.Agent.Set(f.env.Ctx, a, types.Agent{
			Address: a, Name: "Vétérinaire accrédité", Authority: other,
			Jurisdiction: jurisdiction, Active: true,
		}))
	}

	id := f.animal(t, "221/682/100001/L", "982000000100001", "2024-04-02", types.SEX_MALE, "single")
	animal, err := f.k.Animal.Get(f.env.Ctx, id)
	require.NoError(t, err)

	return &transferFixture{
		fixture: f, animalID: id, seller: animal.Holder, buyer: buyer,
		attestorA: attestorA, attestorB: attestorB, other: other,
	}
}

func (tf *transferFixture) propose(t *testing.T) uint64 {
	t.Helper()
	res, err := tf.srv.ProposeTransfer(tf.env.Ctx, &types.MsgProposeTransfer{
		Creator: tf.seller, AnimalId: tf.animalID, To: tf.buyer, Price: "150000",
	})
	require.NoError(t, err)
	return res.Id
}

func (tf *transferFixture) accept(t *testing.T, id uint64) {
	t.Helper()
	_, err := tf.srv.AcceptTransfer(tf.env.Ctx, &types.MsgAcceptTransfer{
		Buyer: tf.buyer, TransferId: id,
	})
	require.NoError(t, err)
}

func (tf *transferFixture) attest(t *testing.T, id uint64, who string) error {
	t.Helper()
	_, err := tf.srv.AttestTransfer(tf.env.Ctx, &types.MsgAttestTransfer{
		Attestor: who, TransferId: id, EvidenceHash: "sha256:tag-read-back",
	})
	return err
}

func (tf *transferFixture) status(t *testing.T) types.Status {
	t.Helper()
	animal, err := tf.k.Animal.Get(tf.env.Ctx, tf.animalID)
	require.NoError(t, err)
	return animal.Status
}

func (tf *transferFixture) state(t *testing.T, id uint64) types.Transfer {
	t.Helper()
	transfer, err := tf.k.Transfer.Get(tf.env.Ctx, id)
	require.NoError(t, err)
	return transfer
}

// advance moves the block clock, which is what the challenge window is measured
// against. Days rather than blocks because the window is a period a person has
// to notice something in, and a person experiences days.
func (tf *transferFixture) advance(days int) {
	tf.env.Ctx = tf.env.Ctx.WithBlockTime(
		tf.env.Ctx.BlockTime().Add(time.Duration(days) * 24 * time.Hour))
}

// The whole path, which has to work before any refusal means anything.
func TestASupervisedTransferCompletes(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	require.Equal(t, types.STATUS_TRANSFER_PENDING, tf.status(t))

	tf.accept(t, id)
	require.NoError(t, tf.attest(t, id, tf.attestorA))
	require.NoError(t, tf.attest(t, id, tf.attestorB))
	require.Equal(t, types.TRANSFER_STATE_ATTESTED, tf.state(t, id).State)

	tf.advance(8)
	_, err := tf.srv.CompleteTransfer(tf.env.Ctx, &types.MsgCompleteTransfer{
		Creator: tf.buyer, TransferId: id,
	})
	require.NoError(t, err)

	animal, err := tf.k.Animal.Get(tf.env.Ctx, tf.animalID)
	require.NoError(t, err)
	require.Equal(t, tf.buyer, animal.Holder)
	require.Equal(t, types.STATUS_ACTIVE, animal.Status)
	require.Equal(t, types.TRANSFER_STATE_COMPLETED, tf.state(t, id).State)
}

// The double sale. A keeper opening two transfers against one animal and
// completing whichever pays first is the oldest fraud in livestock, and it is
// the reason the animal is frozen the moment a transfer opens.
func TestAnAnimalCannotBeSoldTwiceAtOnce(t *testing.T) {
	tf := setupTransfer(t)
	tf.propose(t)

	_, other := tf.env.Addr(t)
	_, err := tf.srv.ProposeTransfer(tf.env.Ctx, &types.MsgProposeTransfer{
		Creator: tf.seller, AnimalId: tf.animalID, To: other, Price: "150000",
	})
	require.ErrorIs(t, err, types.ErrAnimalNotActive)
}

// Only the keeper may open a sale, or an animal could be assigned away from
// the person holding it.
func TestOnlyTheKeeperMayProposeATransfer(t *testing.T) {
	tf := setupTransfer(t)
	_, stranger := tf.env.Addr(t)

	_, err := tf.srv.ProposeTransfer(tf.env.Ctx, &types.MsgProposeTransfer{
		Creator: stranger, AnimalId: tf.animalID, To: tf.buyer, Price: "150000",
	})
	require.ErrorIs(t, err, types.ErrNotKeeper)
}

// The buyer has to agree, or a keeper could dump an animal on somebody who
// never asked for it and walk away from the sanitary obligation that comes
// with holding it.
func TestOnlyTheNamedBuyerMayAccept(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	_, stranger := tf.env.Addr(t)

	_, err := tf.srv.AcceptTransfer(tf.env.Ctx, &types.MsgAcceptTransfer{
		Buyer: stranger, TransferId: id,
	})
	require.ErrorIs(t, err, types.ErrNotBuyer)
}

// An attestor accredited by the office that holds the animal's record is not
// independent. Allowing it collapses the four-party shape back to one office
// deciding, which is the arrangement the quorum exists to replace.
func TestAnAgentOfTheRecordingOfficeCannotAttestItsOwnRecord(t *testing.T) {
	tf := setupTransfer(t)
	_, insider := tf.env.Addr(t)
	require.NoError(t, tf.k.Agent.Set(tf.env.Ctx, insider, types.Agent{
		Address: insider, Name: "Vétérinaire de l'office", Authority: tf.office,
		Jurisdiction: jurisdiction, Active: true,
	}))

	id := tf.propose(t)
	tf.accept(t, id)

	require.ErrorIs(t, tf.attest(t, id, insider), types.ErrNotIndependent)
}

// An office is not a person, and cannot attest by being one.
//
// The office is a group account: its members already decide together, so an
// attestation in its name is not one person saying they stood in front of the
// animal — which is the only thing an attestation is for. The chain cannot
// check that somebody looked at a beast; what it can do is require that a named
// individual answers for the claim.
func TestAnOfficeCannotAttest(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)

	require.ErrorIs(t, tf.attest(t, id, tf.other), types.ErrNotAgent)
}

// A struck-off agent stops being able to act, but keeps their record — every
// attestation they ever made names them, and "was this person accredited when
// they signed" has to stay answerable.
func TestAStruckOffAgentCannotAttest(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)
	require.NoError(t, tf.attest(t, id, tf.attestorA))

	agent, err := tf.k.Agent.Get(tf.env.Ctx, tf.attestorB)
	require.NoError(t, err)
	agent.Active = false
	require.NoError(t, tf.k.Agent.Set(tf.env.Ctx, tf.attestorB, agent))

	require.ErrorIs(t, tf.attest(t, id, tf.attestorB), types.ErrAgentInactive)

	// And the record survives the striking off.
	still, err := tf.k.Agent.Get(tf.env.Ctx, tf.attestorB)
	require.NoError(t, err)
	require.Equal(t, tf.other, still.Authority)
}

// Accreditation is the office's own act, not governance's. This is the message
// that made a lawful sale reachable at all.
func TestAnOfficeAccreditsItsOwnAgents(t *testing.T) {
	tf := setupTransfer(t)
	_, vet := tf.env.Addr(t)

	_, err := tf.srv.AccreditAgent(tf.env.Ctx, &types.MsgAccreditAgent{
		Authority: tf.other, Agent: vet, Name: "Dr Sow", Active: true,
	})
	require.NoError(t, err)

	got, err := tf.k.Agent.Get(tf.env.Ctx, vet)
	require.NoError(t, err)
	require.Equal(t, tf.other, got.Authority)
	// The perimeter is copied from the office rather than accepted from the
	// message: an agent cannot be accredited into a jurisdiction their office
	// does not hold.
	require.Equal(t, jurisdiction, got.Jurisdiction)
	require.True(t, got.Active)
}

func TestOnlyAnAdmittedOfficeMayAccredit(t *testing.T) {
	tf := setupTransfer(t)
	_, stranger := tf.env.Addr(t)
	_, vet := tf.env.Addr(t)

	_, err := tf.srv.AccreditAgent(tf.env.Ctx, &types.MsgAccreditAgent{
		Authority: stranger, Agent: vet, Name: "Dr Personne", Active: true,
	})
	require.ErrorIs(t, err, types.ErrNotAuthority)
}

// An office holding a plain-key agent record would have a second identity that
// attests alone — the independence rule defeated from inside.
func TestAnOfficeCannotBeAccreditedAsAnAgent(t *testing.T) {
	tf := setupTransfer(t)

	_, err := tf.srv.AccreditAgent(tf.env.Ctx, &types.MsgAccreditAgent{
		Authority: tf.other, Agent: tf.office, Name: "not a person", Active: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidAgent)
}

// Moving an agent between offices silently would rewrite what "independent"
// meant for every attestation they have already made.
func TestAnotherOfficeCannotPoachAnAccreditedAgent(t *testing.T) {
	tf := setupTransfer(t)

	_, err := tf.srv.AccreditAgent(tf.env.Ctx, &types.MsgAccreditAgent{
		Authority: tf.office, Agent: tf.attestorA, Name: "Dr Sow", Active: true,
	})
	require.ErrorIs(t, err, types.ErrInvalidAgent)

	// Unchanged, which is the point: the other office must strike them off.
	got, err := tf.k.Agent.Get(tf.env.Ctx, tf.attestorA)
	require.NoError(t, err)
	require.Equal(t, tf.other, got.Authority)
}

// One accredited agent signing twice is one agent, not a quorum.
func TestOneAttestorCannotMakeAQuorumAlone(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)

	require.NoError(t, tf.attest(t, id, tf.attestorA))
	require.ErrorIs(t, tf.attest(t, id, tf.attestorA), types.ErrAlreadyAttested)
	require.Equal(t, types.TRANSFER_STATE_ACCEPTED, tf.state(t, id).State)

	tf.advance(8)
	_, err := tf.srv.CompleteTransfer(tf.env.Ctx, &types.MsgCompleteTransfer{
		Creator: tf.buyer, TransferId: id,
	})
	require.ErrorIs(t, err, types.ErrQuorumNotMet)
}

// The window runs from the last attestation, not from the proposal. Otherwise
// a transfer could be opened quietly, left to age, and completed the instant it
// was attested — a window in name only, and the exact shape a thief would use.
func TestTheWindowRunsFromAttestationNotFromProposal(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)

	// A fortnight passes with the transfer merely open.
	tf.advance(14)
	require.NoError(t, tf.attest(t, id, tf.attestorA))
	require.NoError(t, tf.attest(t, id, tf.attestorB))

	_, err := tf.srv.CompleteTransfer(tf.env.Ctx, &types.MsgCompleteTransfer{
		Creator: tf.buyer, TransferId: id,
	})
	require.ErrorIs(t, err, types.ErrWindowOpen,
		"ageing the proposal must not consume the challenge window")
}

// Completion is permissionless on purpose: every condition has already been
// checked by the chain, and requiring a particular signer would let that signer
// sit on a completed bargain — a veto nobody voted to give them.
func TestAnybodyMayPayTheGasToCompleteATransfer(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)
	require.NoError(t, tf.attest(t, id, tf.attestorA))
	require.NoError(t, tf.attest(t, id, tf.attestorB))
	tf.advance(8)

	_, stranger := tf.env.Addr(t)
	_, err := tf.srv.CompleteTransfer(tf.env.Ctx, &types.MsgCompleteTransfer{
		Creator: stranger, TransferId: id,
	})
	require.NoError(t, err)
	require.Equal(t, tf.buyer, tf.state(t, id).To)
}

// The objection is open to anybody, and this is the test that says why. The
// person best placed to recognise a stolen animal is the neighbour who has seen
// it every day — not a party to the sale, not accredited, not known to the
// registry at all.
func TestAStrangerMayStopASale(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)
	require.NoError(t, tf.attest(t, id, tf.attestorA))
	require.NoError(t, tf.attest(t, id, tf.attestorB))

	_, neighbour := tf.env.Addr(t)
	_, err := tf.srv.ObjectTransfer(tf.env.Ctx, &types.MsgObjectTransfer{
		Objector: neighbour, TransferId: id, Reason: "this is my ram, taken in July",
	})
	require.NoError(t, err)

	require.Equal(t, types.STATUS_DISPUTED, tf.status(t))
	transfer := tf.state(t, id)
	require.Equal(t, types.TRANSFER_STATE_OBJECTED, transfer.State)
	require.Equal(t, neighbour, transfer.ObjectedBy)

	tf.advance(30)
	_, err = tf.srv.CompleteTransfer(tf.env.Ctx, &types.MsgCompleteTransfer{
		Creator: tf.buyer, TransferId: id,
	})
	require.ErrorIs(t, err, types.ErrTransferObjected,
		"waiting out the window must not defeat an objection")
}

// Objecting twice would overwrite the first objector's name with the second's,
// so the cheapest way to erase an inconvenient objection from the record would
// be to object to the same transfer yourself.
func TestASecondObjectionCannotOverwriteTheFirst(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)

	_, neighbour := tf.env.Addr(t)
	_, err := tf.srv.ObjectTransfer(tf.env.Ctx, &types.MsgObjectTransfer{
		Objector: neighbour, TransferId: id, Reason: "this is my ram",
	})
	require.NoError(t, err)

	_, thief := tf.env.Addr(t)
	_, err = tf.srv.ObjectTransfer(tf.env.Ctx, &types.MsgObjectTransfer{
		Objector: thief, TransferId: id, Reason: "nothing to see",
	})
	require.ErrorIs(t, err, types.ErrTransferClosed)
	require.Equal(t, neighbour, tf.state(t, id).ObjectedBy)
}

// The hole this whole file was written around.
//
// An objection costs one transaction and can be made by anybody. Before there
// was a way to answer one, it left the animal DISPUTED with no message able to
// return it to ACTIVE — so a single adversary iterating over open transfers
// could have taken the national herd out of lawful circulation for a few
// thousand francs, permanently, and no honest party could have cleared it.
func TestAGroundlessObjectionCanBeDismissed(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)
	require.NoError(t, tf.attest(t, id, tf.attestorA))
	require.NoError(t, tf.attest(t, id, tf.attestorB))

	_, mischief := tf.env.Addr(t)
	_, err := tf.srv.ObjectTransfer(tf.env.Ctx, &types.MsgObjectTransfer{
		Objector: mischief, TransferId: id, Reason: "",
	})
	require.NoError(t, err)
	require.Equal(t, types.STATUS_DISPUTED, tf.status(t))

	_, err = tf.srv.ResolveObjection(tf.env.Ctx, &types.MsgResolveObjection{
		Authority: tf.office, TransferId: id, Uphold: false,
		Reason: "objector produced no evidence and is not known to the herd",
	})
	require.NoError(t, err)

	transfer := tf.state(t, id)
	require.Equal(t, types.TRANSFER_STATE_ATTESTED, transfer.State,
		"a dismissed objection returns the transfer to where it had got to")
	require.Equal(t, tf.office, transfer.ResolvedBy)
	require.False(t, transfer.ObjectionUpheld)
	require.Equal(t, mischief, transfer.ObjectedBy,
		"the objection stays on the record even once dismissed")
	require.Equal(t, types.STATUS_TRANSFER_PENDING, tf.status(t))

	tf.advance(8)
	_, err = tf.srv.CompleteTransfer(tf.env.Ctx, &types.MsgCompleteTransfer{
		Creator: tf.buyer, TransferId: id,
	})
	require.NoError(t, err)
}

// Dismissing restarts the window rather than resuming it.
//
// The obvious implementation puts the transfer back exactly as it was, which
// means an objection raised on the last day is dismissed and the sale completes
// the same minute — with nobody able to stop it, because the window they would
// have used has already run. The moment just after an objection is overruled is
// precisely when somebody else is most likely to have something to add.
func TestDismissingAnObjectionRestartsTheChallengeWindow(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)
	require.NoError(t, tf.attest(t, id, tf.attestorA))
	require.NoError(t, tf.attest(t, id, tf.attestorB))

	// Objected on the last day of the window.
	tf.advance(6)
	_, mischief := tf.env.Addr(t)
	_, err := tf.srv.ObjectTransfer(tf.env.Ctx, &types.MsgObjectTransfer{
		Objector: mischief, TransferId: id, Reason: "no",
	})
	require.NoError(t, err)

	_, err = tf.srv.ResolveObjection(tf.env.Ctx, &types.MsgResolveObjection{
		Authority: tf.office, TransferId: id, Uphold: false, Reason: "no evidence offered",
	})
	require.NoError(t, err)

	_, err = tf.srv.CompleteTransfer(tf.env.Ctx, &types.MsgCompleteTransfer{
		Creator: tf.buyer, TransferId: id,
	})
	require.ErrorIs(t, err, types.ErrWindowOpen,
		"a dismissal must not hand the seller a sale nobody is left able to contest")

	tf.advance(8)
	_, err = tf.srv.CompleteTransfer(tf.env.Ctx, &types.MsgCompleteTransfer{
		Creator: tf.buyer, TransferId: id,
	})
	require.NoError(t, err)
}

// Upholding stops the sale and holds the animal — but as a freeze, which is a
// state an office owns and can lift, rather than a disputed state nothing can
// leave. A hold nobody can end is a confiscation and would eventually be used
// as one.
func TestUpholdingAnObjectionStopsTheSaleAndHoldsTheAnimal(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)
	require.NoError(t, tf.attest(t, id, tf.attestorA))
	require.NoError(t, tf.attest(t, id, tf.attestorB))

	_, owner := tf.env.Addr(t)
	_, err := tf.srv.ObjectTransfer(tf.env.Ctx, &types.MsgObjectTransfer{
		Objector: owner, TransferId: id, Reason: "stolen from Niacoulrab in July, complaint 2026/431",
	})
	require.NoError(t, err)

	_, err = tf.srv.ResolveObjection(tf.env.Ctx, &types.MsgResolveObjection{
		Authority: tf.office, TransferId: id, Uphold: true,
		Reason: "complaint 2026/431 on file; matter referred to the tribunal",
	})
	require.NoError(t, err)

	transfer := tf.state(t, id)
	require.Equal(t, types.TRANSFER_STATE_CANCELLED, transfer.State)
	require.True(t, transfer.ObjectionUpheld)
	require.Equal(t, types.STATUS_FROZEN, tf.status(t))

	// The seller still holds it — the chain stopped a sale, it did not decide
	// who owns the animal, and it must not pretend to have done.
	animal, err := tf.k.Animal.Get(tf.env.Ctx, tf.animalID)
	require.NoError(t, err)
	require.Equal(t, tf.seller, animal.Holder)

	// And the hold has an end.
	_, err = tf.srv.ReleaseAnimal(tf.env.Ctx, &types.MsgReleaseAnimal{
		Authority: tf.office, AnimalId: tf.animalID,
		Reason: "tribunal found for the holder, 2026/431 closed",
	})
	require.NoError(t, err)
	require.Equal(t, types.STATUS_ACTIVE, tf.status(t))
}

// A resolution has to be answerable, so it is an office and not a bystander —
// and it is the office that holds the record, because letting any office lift
// any other's hold would make the weakest office in the country the effective
// authority over every animal in it.
func TestOnlyTheRecordingOfficeMayResolveAnObjection(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)

	_, mischief := tf.env.Addr(t)
	_, err := tf.srv.ObjectTransfer(tf.env.Ctx, &types.MsgObjectTransfer{
		Objector: mischief, TransferId: id, Reason: "no",
	})
	require.NoError(t, err)

	_, stranger := tf.env.Addr(t)
	_, err = tf.srv.ResolveObjection(tf.env.Ctx, &types.MsgResolveObjection{
		Authority: stranger, TransferId: id, Uphold: false, Reason: "because I say so",
	})
	require.ErrorIs(t, err, types.ErrNotAuthority)

	_, err = tf.srv.ResolveObjection(tf.env.Ctx, &types.MsgResolveObjection{
		Authority: tf.other, TransferId: id, Uphold: false, Reason: "not my jurisdiction",
	})
	require.ErrorIs(t, err, types.ErrWrongJurisdiction)
}

// A decision with no reason recorded is unreviewable, which is the whole point
// of recording it rather than merely applying it.
func TestAResolutionMustCarryAReason(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)

	_, mischief := tf.env.Addr(t)
	_, err := tf.srv.ObjectTransfer(tf.env.Ctx, &types.MsgObjectTransfer{
		Objector: mischief, TransferId: id, Reason: "no",
	})
	require.NoError(t, err)

	_, err = tf.srv.ResolveObjection(tf.env.Ctx, &types.MsgResolveObjection{
		Authority: tf.office, TransferId: id, Uphold: false, Reason: "   ",
	})
	require.ErrorIs(t, err, types.ErrNoReason)
}

// The quieter liveness hole. A keeper proposes, the animal freezes against a
// second sale, and the buyer simply never answers. Without a withdrawal that
// animal is unsellable for ever and its keeper has done nothing wrong.
func TestASellerMayWithdrawAProposalNobodyTookUp(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	require.Equal(t, types.STATUS_TRANSFER_PENDING, tf.status(t))

	_, err := tf.srv.CancelTransfer(tf.env.Ctx, &types.MsgCancelTransfer{
		Seller: tf.seller, TransferId: id, Reason: "buyer never answered",
	})
	require.NoError(t, err)

	require.Equal(t, types.TRANSFER_STATE_CANCELLED, tf.state(t, id).State)
	require.Equal(t, types.STATUS_ACTIVE, tf.status(t))

	// And the animal is genuinely sellable again, which is the only thing the
	// keeper actually cares about.
	_, second := tf.env.Addr(t)
	_, err = tf.srv.ProposeTransfer(tf.env.Ctx, &types.MsgProposeTransfer{
		Creator: tf.seller, AnimalId: tf.animalID, To: second, Price: "150000",
	})
	require.NoError(t, err)
}

// Once an agent has travelled out and sworn the animal is the animal, the
// buyer has relied on the sale and so has the agent. A seller who could still
// walk away then holds exactly the veto that permissionless completion denies.
func TestASellerCannotWithdrawOnceAnAgentHasAttested(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)
	tf.accept(t, id)
	require.NoError(t, tf.attest(t, id, tf.attestorA))
	require.NoError(t, tf.attest(t, id, tf.attestorB))

	_, err := tf.srv.CancelTransfer(tf.env.Ctx, &types.MsgCancelTransfer{
		Seller: tf.seller, TransferId: id, Reason: "found a better price",
	})
	require.ErrorIs(t, err, types.ErrTransferAttested)
}

func TestOnlyTheSellerMayWithdraw(t *testing.T) {
	tf := setupTransfer(t)
	id := tf.propose(t)

	_, stranger := tf.env.Addr(t)
	_, err := tf.srv.CancelTransfer(tf.env.Ctx, &types.MsgCancelTransfer{
		Seller: stranger, TransferId: id, Reason: "mine now",
	})
	require.ErrorIs(t, err, types.ErrNotSeller)
}

// Releasing an animal that is mid-transfer would unfreeze it out from under a
// sale that is still open, leaving the register saying the animal is free while
// a live transfer can still complete against it — the double sale this module
// exists to prevent, walking back in through the door marked exit.
func TestReleaseCannotUnfreezeAnAnimalMidTransfer(t *testing.T) {
	tf := setupTransfer(t)
	tf.propose(t)

	_, err := tf.srv.ReleaseAnimal(tf.env.Ctx, &types.MsgReleaseAnimal{
		Authority: tf.office, AnimalId: tf.animalID, Reason: "let it go",
	})
	require.ErrorIs(t, err, types.ErrNotHeld)
	require.Equal(t, types.STATUS_TRANSFER_PENDING, tf.status(t))
}

// A freeze is the office's own stop — a fraud inquiry, a court order, a
// sanitary hold — and it blocks a sale like any other.
func TestAFrozenAnimalCannotBeSold(t *testing.T) {
	tf := setupTransfer(t)

	_, err := tf.srv.FreezeAnimal(tf.env.Ctx, &types.MsgFreezeAnimal{
		Authority: tf.office, AnimalId: tf.animalID,
		Reason: "PPR movement restriction, Rufisque département",
	})
	require.NoError(t, err)
	require.Equal(t, types.STATUS_FROZEN, tf.status(t))

	_, err = tf.srv.ProposeTransfer(tf.env.Ctx, &types.MsgProposeTransfer{
		Creator: tf.seller, AnimalId: tf.animalID, To: tf.buyer, Price: "150000",
	})
	require.ErrorIs(t, err, types.ErrAnimalNotActive)

	_, err = tf.srv.ReleaseAnimal(tf.env.Ctx, &types.MsgReleaseAnimal{
		Authority: tf.office, AnimalId: tf.animalID, Reason: "restriction lifted",
	})
	require.NoError(t, err)

	_, err = tf.srv.ProposeTransfer(tf.env.Ctx, &types.MsgProposeTransfer{
		Creator: tf.seller, AnimalId: tf.animalID, To: tf.buyer, Price: "150000",
	})
	require.NoError(t, err)
}

// Provenance is what a buyer standing in a market actually wants, and a refused
// sale belongs in it: an animal somebody once tried to sell and was stopped
// from selling is worth knowing about.
func TestProvenanceKeepsRefusedSales(t *testing.T) {
	tf := setupTransfer(t)

	first := tf.propose(t)
	_, mischief := tf.env.Addr(t)
	_, err := tf.srv.ObjectTransfer(tf.env.Ctx, &types.MsgObjectTransfer{
		Objector: mischief, TransferId: first, Reason: "wait",
	})
	require.NoError(t, err)
	_, err = tf.srv.ResolveObjection(tf.env.Ctx, &types.MsgResolveObjection{
		Authority: tf.office, TransferId: first, Uphold: false, Reason: "withdrawn by objector",
	})
	require.NoError(t, err)
	_, err = tf.srv.CancelTransfer(tf.env.Ctx, &types.MsgCancelTransfer{
		Seller: tf.seller, TransferId: first, Reason: "buyer lost interest",
	})
	require.NoError(t, err)

	second := tf.propose(t)

	q := keeper.NewQueryServerImpl(tf.k)
	res, err := q.TransfersByAnimal(tf.env.Ctx, &types.QueryTransfersByAnimalRequest{
		AnimalId: tf.animalID,
	})
	require.NoError(t, err)
	require.Len(t, res.Transfers, 2)
	require.Equal(t, first, res.Transfers[0].Id)
	require.Equal(t, mischief, res.Transfers[0].ObjectedBy)
	require.Equal(t, second, res.Transfers[1].Id)
}
