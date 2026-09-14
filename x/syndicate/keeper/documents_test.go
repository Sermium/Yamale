package keeper_test

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/syndicate/types"
)

// The deal papers.
//
// x/syndicate could state a deal's economics to the last basis point and could
// say nothing whatever about the documents an investor is bound by. What these
// tests hold in place is the narrow, honest thing a chain can do about that: it
// can prove a document with given contents existed at a block, attested by a
// named operator, and it can refuse to let the record of that become
// self-contradictory.
//
// What it cannot do is on the page too, and the tests that matter most here are
// the refusals: a digest that is the wrong length, a digest of nothing, and an
// amendment chain that forks.

// digest is a real SHA-256, because a fixture that used 32 arbitrary bytes
// would pass every test here while proving nothing about what the field holds.
func digest(of string) []byte {
	sum := sha256.Sum256([]byte(of))
	return sum[:]
}

func (f *fixture) attach(t *testing.T, deal uint64, msg *types.MsgAttachDocument) (uint64, error) {
	t.Helper()
	msg.Operator = f.operator
	msg.DealId = deal
	res, err := f.srv.AttachDocument(f.env.Ctx, msg)
	if err != nil {
		return 0, err
	}
	return res.Id, nil
}

func termSheet(body string) *types.MsgAttachDocument {
	return &types.MsgAttachDocument{
		Kind:     types.DOCUMENT_KIND_TERM_SHEET,
		Title:    "Term sheet",
		Sha256:   digest(body),
		Uri:      "ipfs://bafy" + body,
		Language: "en",
	}
}

/* ========================================================================== */
/*  What is recorded                                                          */
/* ========================================================================== */

func TestADocumentRecordsItsDigestAndWhoAttestedToIt(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	docID, err := f.attach(t, id, termSheet("the terms"))
	require.NoError(t, err)
	require.NotZero(t, docID, "document 0 is never issued")

	doc, err := f.k.Document.Get(f.env.Ctx, docID)
	require.NoError(t, err)
	require.Equal(t, id, doc.DealId)
	require.Equal(t, digest("the terms"), doc.Sha256)
	require.Equal(t, types.DOCUMENT_KIND_TERM_SHEET, doc.Kind)

	// The attestation is the point: a hash nobody is accountable for proves a
	// file has not changed and nothing about who said it was the agreement.
	require.Equal(t, f.operator, doc.AttestedBy)
	require.Equal(t, f.env.Ctx.BlockTime().Unix(), doc.AttachedAt)
	require.Zero(t, doc.Supersedes)
	require.Zero(t, doc.SupersededBy)
}

func TestOnlyAnAdmittedOperatorMayAttachADocument(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	msg := termSheet("the terms")
	msg.Operator = f.alice
	msg.DealId = id
	_, err := f.srv.AttachDocument(f.env.Ctx, msg)
	require.ErrorIs(t, err, types.ErrNotOperator,
		"an investor attesting to the deal's own paperwork is the platform's word coming from "+
			"somebody with no accountability for it")
}

func TestDocumentsMayBeAttachedAfterTheDealIsRealised(t *testing.T) {
	// The completion statement, the final accounts and the exit resolution all
	// arrive after the money has moved. A register that closed at the close
	// would be missing exactly the papers a later dispute is about.
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")
	require.NoError(t, f.commit(t, id, f.alice, 60_000))
	f.advance(week + 1)
	_, err := f.srv.CloseDeal(f.env.Ctx, &types.MsgCloseDeal{Operator: f.operator, DealId: id})
	require.NoError(t, err)

	_, err = f.attach(t, id, &types.MsgAttachDocument{
		Kind:   types.DOCUMENT_KIND_RESOLUTION,
		Title:  "Completion statement",
		Sha256: digest("completed"),
	})
	require.NoError(t, err)
}

func TestADocumentOnADealTheChainDoesNotHoldIsRefused(t *testing.T) {
	f := setup(t)
	_, err := f.attach(t, 9041, termSheet("the terms"))
	require.ErrorIs(t, err, types.ErrNoDeal)
}

/* ========================================================================== */
/*  The digest                                                                */
/* ========================================================================== */

func TestATruncatedDigestIsRefused(t *testing.T) {
	// Not a weaker proof — a forgeable one. Sixteen bytes of SHA-256 is within
	// reach of a birthday attack, and the record would look identical to one
	// that was not.
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	msg := termSheet("the terms")
	msg.Sha256 = msg.Sha256[:16]
	_, err := f.attach(t, id, msg)
	require.ErrorIs(t, err, types.ErrInvalidDocument)
}

func TestThirtyTwoZeroBytesIsNotADigest(t *testing.T) {
	// What an unset field looks like once something has padded it to the right
	// length. It is the correct length and it matches nothing, which is the
	// worst combination available.
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	msg := termSheet("the terms")
	msg.Sha256 = make([]byte, 32)
	_, err := f.attach(t, id, msg)
	require.ErrorIs(t, err, types.ErrInvalidDocument)
}

func TestADocumentNeedsAKindAndATitle(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	noKind := termSheet("a")
	noKind.Kind = types.DOCUMENT_KIND_UNSPECIFIED
	_, err := f.attach(t, id, noKind)
	require.ErrorIs(t, err, types.ErrInvalidDocument,
		"an unspecified kind puts the agreement that binds somebody in the same bucket as the deck")

	noTitle := termSheet("b")
	noTitle.Title = "   "
	_, err = f.attach(t, id, noTitle)
	require.ErrorIs(t, err, types.ErrInvalidDocument)
}

func TestALanguageMustBeATwoLetterCode(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	msg := termSheet("a")
	msg.Language = "English"
	_, err := f.attach(t, id, msg)
	require.ErrorIs(t, err, types.ErrInvalidDocument)

	// Absent is fine. Plenty of documents have no single language, and
	// demanding one would make operators type a lie.
	none := termSheet("b")
	none.Language = ""
	_, err = f.attach(t, id, none)
	require.NoError(t, err)

	// And it is normalised, so "EN" and "en" are not two languages.
	upper := termSheet("c")
	upper.Language = "FR"
	docID, err := f.attach(t, id, upper)
	require.NoError(t, err)
	doc, err := f.k.Document.Get(f.env.Ctx, docID)
	require.NoError(t, err)
	require.Equal(t, "fr", doc.Language)
}

/* ========================================================================== */
/*  Amendment                                                                 */
/* ========================================================================== */

func TestAnAmendmentSupersedesRatherThanReplaces(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	v1, err := f.attach(t, id, termSheet("version one"))
	require.NoError(t, err)

	v2msg := termSheet("version two")
	v2msg.Title = "Term sheet (amended)"
	v2msg.Supersedes = v1
	v2, err := f.attach(t, id, v2msg)
	require.NoError(t, err)

	// Both directions are readable, and BOTH DOCUMENTS ARE STILL THERE. A
	// dispute is almost always about what the terms were at a date rather than
	// about what they are now, so the old version is the one somebody needs.
	old, err := f.k.Document.Get(f.env.Ctx, v1)
	require.NoError(t, err)
	require.Equal(t, v2, old.SupersededBy)
	require.Equal(t, digest("version one"), old.Sha256,
		"the superseded version keeps its own digest; it is the record of what was agreed then")

	current, err := f.k.Document.Get(f.env.Ctx, v2)
	require.NoError(t, err)
	require.Equal(t, v1, current.Supersedes)
	require.Zero(t, current.SupersededBy)
}

func TestAnAmendmentChainCannotFork(t *testing.T) {
	// Two documents replacing the same one leaves a reader with two current
	// versions of one agreement and no rule for choosing between them — worse
	// than no register at all, because the register looks authoritative.
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	v1, err := f.attach(t, id, termSheet("version one"))
	require.NoError(t, err)

	second := termSheet("version two")
	second.Supersedes = v1
	_, err = f.attach(t, id, second)
	require.NoError(t, err)

	third := termSheet("version three")
	third.Supersedes = v1
	_, err = f.attach(t, id, third)
	require.ErrorIs(t, err, types.ErrAlreadySuperseded)
}

func TestADocumentCannotSupersedeOneFromAnotherDeal(t *testing.T) {
	f := setup(t)
	one := f.openDeal(t, "SYN-2026-001")
	two := f.openDeal(t, "SYN-2026-002")

	v1, err := f.attach(t, one, termSheet("version one"))
	require.NoError(t, err)

	msg := termSheet("version two")
	msg.Supersedes = v1
	_, err = f.attach(t, two, msg)
	require.ErrorIs(t, err, types.ErrInvalidDocument)
}

func TestReplacingADocumentWithAnIdenticalOneIsRefused(t *testing.T) {
	// Not an amendment — an accident, and recording it would put a supersession
	// in the history that nobody can explain afterwards.
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	v1, err := f.attach(t, id, termSheet("version one"))
	require.NoError(t, err)

	same := termSheet("version one")
	same.Supersedes = v1
	_, err = f.attach(t, id, same)
	require.ErrorIs(t, err, types.ErrInvalidDocument)
}

func TestSupersedingADocumentThatDoesNotExistIsRefused(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	msg := termSheet("version two")
	msg.Supersedes = 9041
	_, err := f.attach(t, id, msg)
	require.ErrorIs(t, err, types.ErrNoDocument)
}

/* ========================================================================== */
/*  Reading them back                                                         */
/* ========================================================================== */

func TestDocumentsComeBackOldestFirstAndScopedToTheirDeal(t *testing.T) {
	f := setup(t)
	one := f.openDeal(t, "SYN-2026-001")
	two := f.openDeal(t, "SYN-2026-002")

	first, err := f.attach(t, one, termSheet("first"))
	require.NoError(t, err)
	second, err := f.attach(t, one, &types.MsgAttachDocument{
		Kind:  types.DOCUMENT_KIND_SUBSCRIPTION_AGREEMENT,
		Title: "Subscription agreement", Sha256: digest("second"),
	})
	require.NoError(t, err)
	_, err = f.attach(t, two, termSheet("somebody else's deal"))
	require.NoError(t, err)

	res, err := f.q.Documents(f.env.Ctx, &types.QueryDocumentsRequest{DealId: one})
	require.NoError(t, err)
	require.Len(t, res.Documents, 2)
	require.Equal(t, first, res.Documents[0].Id)
	require.Equal(t, second, res.Documents[1].Id,
		"ids are issued in order, so oldest first IS the order the deal happened in")
}

func TestCurrentOnlyHidesSupersededVersionsAndIsNotTheDefault(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	v1, err := f.attach(t, id, termSheet("version one"))
	require.NoError(t, err)
	v2msg := termSheet("version two")
	v2msg.Supersedes = v1
	v2, err := f.attach(t, id, v2msg)
	require.NoError(t, err)

	all, err := f.q.Documents(f.env.Ctx, &types.QueryDocumentsRequest{DealId: id})
	require.NoError(t, err)
	require.Len(t, all.Documents, 2, "the default shows history, because history is the question")

	current, err := f.q.Documents(f.env.Ctx,
		&types.QueryDocumentsRequest{DealId: id, CurrentOnly: true})
	require.NoError(t, err)
	require.Len(t, current.Documents, 1)
	require.Equal(t, v2, current.Documents[0].Id)
}

func TestADealWithNoPapersAnswersWithNoneRatherThanAnError(t *testing.T) {
	// An absence, not a failure. The distinction is the difference between an
	// interface that says "this deal has no documents attached" and one that
	// says the chain is broken.
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	res, err := f.q.Documents(f.env.Ctx, &types.QueryDocumentsRequest{DealId: id})
	require.NoError(t, err)
	require.Empty(t, res.Documents)
}

/* ========================================================================== */
/*  Genesis                                                                   */
/* ========================================================================== */

func TestDocumentsSurviveAnExportImportRound(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	v1, err := f.attach(t, id, termSheet("version one"))
	require.NoError(t, err)
	v2msg := termSheet("version two")
	v2msg.Supersedes = v1
	_, err = f.attach(t, id, v2msg)
	require.NoError(t, err)

	gs, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.Len(t, gs.Documents, 2)
	require.NoError(t, gs.Validate())

	// The counter is exported explicitly. Derived from the highest id present
	// it would reissue an id the moment a document was the last thing added and
	// something removed it — and two documents under one id is the register
	// contradicting itself.
	require.Equal(t, uint64(3), gs.NextDocumentId,
		"ids start at 1 because zero is never issued, so two documents leave the counter at 3")

	fresh := setup(t)
	require.NoError(t, fresh.k.InitGenesis(fresh.env.Ctx, *gs))
	back, err := fresh.k.ExportGenesis(fresh.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, gs.Documents, back.Documents)
	require.Equal(t, gs.NextDocumentId, back.NextDocumentId)
}

func TestAGenesisWhereTwoDocumentsReplaceTheSameOneIsRefused(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")
	v1, err := f.attach(t, id, termSheet("version one"))
	require.NoError(t, err)
	v2msg := termSheet("version two")
	v2msg.Supersedes = v1
	_, err = f.attach(t, id, v2msg)
	require.NoError(t, err)

	gs, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)

	// A third, hand-written into the file, replacing the same predecessor. The
	// message handler refuses this; a genesis file is the way round it.
	forked := gs.Documents[1]
	forked.Id = gs.NextDocumentId
	forked.Sha256 = digest("version three")
	gs.NextDocumentId++
	gs.Documents = append(gs.Documents, forked)

	require.ErrorContains(t, gs.Validate(), "two current versions")
}

func TestAGenesisWhoseBackPointerDisagreesIsRefused(t *testing.T) {
	// The back-pointer is the forward pointer read the other way, not
	// independent information. A file where the two disagree shows a reader a
	// superseded document as current, or the reverse.
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")
	v1, err := f.attach(t, id, termSheet("version one"))
	require.NoError(t, err)
	v2msg := termSheet("version two")
	v2msg.Supersedes = v1
	_, err = f.attach(t, id, v2msg)
	require.NoError(t, err)

	gs, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	gs.Documents[0].SupersededBy = 0
	require.ErrorContains(t, gs.Validate(), "does not say so")
}

func TestAGenesisDocumentOnAnAbsentDealIsRefused(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")
	_, err := f.attach(t, id, termSheet("version one"))
	require.NoError(t, err)

	gs, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	gs.Documents[0].DealId = 9041
	require.ErrorContains(t, gs.Validate(), "which is not here")
}
