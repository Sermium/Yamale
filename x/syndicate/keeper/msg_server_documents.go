package keeper

import (
	"bytes"
	"context"
	"strings"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/syndicate/types"
)

/* ========================================================================== */
/*  The papers                                                                */
/* ========================================================================== */

// AttachDocument records the digest of a paper the deal depends on.
//
// # Why this is a message and not a field on the deal
//
// Documents arrive across the whole life of a syndication and not at the
// moment it opens: a term sheet before the raise, a subscription agreement at
// the close, a shareholders' agreement when the round completes, accounts every
// year after that, and a completion statement at the exit. A repeated field set
// at OpenDeal would hold the first of those and could never hold the rest,
// which would mean the register was complete exactly while the deal was
// unimportant and incomplete from the day money moved.
//
// # Accepted in every state, including REALISED
//
// A register that stopped accepting papers at the close would be missing
// precisely the documents a later dispute is about — the completion statement,
// the final accounts, the exit resolution. Nothing here changes the money, so
// there is no state in which accepting one is unsafe.
//
// # What it is worth, stated exactly
//
// It proves a document with these contents existed at this block, attested by
// an operator group whose members can be asked about it. It proves that a file
// produced years later either is or is not that document. It proves nothing
// about whether the document says what its title claims, whether anybody signed
// it, or whether it is still available anywhere.
func (m msgServer) AttachDocument(
	ctx context.Context, msg *types.MsgAttachDocument,
) (*types.MsgAttachDocumentResponse, error) {
	op, err := m.activeOperator(ctx, msg.Operator)
	if err != nil {
		return nil, err
	}

	deal, err := m.Deal.Get(ctx, msg.DealId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoDeal.Wrapf("%d", msg.DealId)
		}
		return nil, err
	}

	title := strings.TrimSpace(msg.Title)
	uri := strings.TrimSpace(msg.Uri)
	language := strings.ToLower(strings.TrimSpace(msg.Language))
	if err := types.ValidateDocument(msg.Kind, title, msg.Sha256, uri, language); err != nil {
		return nil, err
	}

	// Superseding: the old document must belong to THIS deal and must not
	// already have been superseded.
	//
	// The second check is the one that matters. Two documents both claiming to
	// replace the same agreement leave a reader with two current versions and
	// no rule for choosing between them — which is worse than having no
	// register, because the register looks authoritative.
	var previous types.Document
	if msg.Supersedes != 0 {
		previous, err = m.Document.Get(ctx, msg.Supersedes)
		if err != nil {
			if isNotFound(err) {
				return nil, types.ErrNoDocument.Wrapf("%d", msg.Supersedes)
			}
			return nil, err
		}
		if previous.DealId != deal.Id {
			return nil, types.ErrInvalidDocument.Wrapf(
				"document %d belongs to deal %d, not deal %d",
				previous.Id, previous.DealId, deal.Id)
		}
		if previous.SupersededBy != 0 {
			return nil, types.ErrAlreadySuperseded.Wrapf(
				"document %d was already replaced by document %d",
				previous.Id, previous.SupersededBy)
		}
		// Replacing a document with a byte-identical one is not an amendment,
		// it is an accident — and recording it would put a supersession in the
		// history that nobody can explain later.
		if bytes.Equal(previous.Sha256, msg.Sha256) {
			return nil, types.ErrInvalidDocument.Wrap(
				"the replacement is byte-identical to the document it replaces")
		}
	}

	id, err := nextID(ctx, m.NextDocumentID)
	if err != nil {
		return nil, err
	}

	doc := types.Document{
		Id:         id,
		DealId:     deal.Id,
		Kind:       msg.Kind,
		Title:      title,
		Sha256:     msg.Sha256,
		Uri:        uri,
		Language:   language,
		AttestedBy: op.Address,
		AttachedAt: blockTime(ctx),
		Supersedes: msg.Supersedes,
	}
	if err := m.Document.Set(ctx, id, doc); err != nil {
		return nil, err
	}
	if err := m.DocumentsByDeal.Set(ctx, collections.Join(deal.Id, id)); err != nil {
		return nil, err
	}

	// The back-pointer, written last so a failure above leaves the old document
	// untouched rather than pointing at one that was never stored.
	if msg.Supersedes != 0 {
		previous.SupersededBy = id
		if err := m.Document.Set(ctx, previous.Id, previous); err != nil {
			return nil, err
		}
	}

	return &types.MsgAttachDocumentResponse{Id: id}, nil
}

// DealDocuments returns one deal's papers, oldest first.
//
// Oldest first because ids are issued in order, so that IS the order the deal
// happened in, and a document register read backwards tells the story backwards.
func (k Keeper) DealDocuments(
	ctx context.Context, dealID uint64, currentOnly bool,
) ([]types.Document, error) {
	rng := collections.NewPrefixedPairRange[uint64, uint64](dealID)
	it, err := k.DocumentsByDeal.Iterate(ctx, rng)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	out := []types.Document{}
	for ; it.Valid(); it.Next() {
		key, err := it.Key()
		if err != nil {
			return nil, err
		}
		doc, err := k.Document.Get(ctx, key.K2())
		if err != nil {
			return nil, err
		}
		if currentOnly && doc.SupersededBy != 0 {
			continue
		}
		out = append(out, doc)
	}
	return out, nil
}
