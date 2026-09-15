package keeper

import (
	"bytes"
	"context"
	"strings"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

/* ========================================================================== */
/*  What the operator has published                                           */
/* ========================================================================== */

// ValidateCovenant checks everything about a filing that can be checked without
// reading state.
//
// Separate from the handler so each rule is testable on its own, because every
// rule here exists to stop one specific way a register can mislead somebody.
func ValidateCovenant(
	kind types.CovenantKind, title string, digest []byte, uri string,
	periodStart, periodEnd, actsBy int64,
) error {
	if kind == types.COVENANT_KIND_UNSPECIFIED {
		return types.ErrInvalidCovenant.Wrap("say what kind of filing this is")
	}
	if _, known := types.CovenantKind_name[int32(kind)]; !known {
		return types.ErrInvalidCovenant.Wrapf("unknown filing kind %d", int32(kind))
	}
	if title == "" {
		return types.ErrInvalidCovenant.Wrap("a filing needs a title a person can read")
	}
	if len(title) > types.MaxCovenantTitle {
		return types.ErrInvalidCovenant.Wrapf(
			"a title may not exceed %d bytes", types.MaxCovenantTitle)
	}
	if len(uri) > types.MaxCovenantURI {
		return types.ErrInvalidCovenant.Wrapf(
			"a location may not exceed %d bytes", types.MaxCovenantURI)
	}
	if len(digest) != types.SHA256Len {
		return types.ErrInvalidCovenant.Wrapf(
			"a SHA-256 digest is %d bytes; this one is %d", types.SHA256Len, len(digest))
	}
	// Thirty-two zero bytes is what an unset field looks like once something has
	// padded it to the right length: the correct length, matching nothing.
	if bytes.Equal(digest, make([]byte, types.SHA256Len)) {
		return types.ErrInvalidCovenant.Wrap(
			"that digest is thirty-two zero bytes, which is not a hash")
	}

	// A period is a span or it is nothing. One end without the other describes
	// no interval, and a reader checking whether a year is missing would be
	// comparing against half a fact.
	if (periodStart == 0) != (periodEnd == 0) {
		return types.ErrInvalidCovenant.Wrap(
			"a period needs both ends, or neither")
	}
	if periodStart != 0 && periodEnd < periodStart {
		return types.ErrInvalidCovenant.Wrap("the period ends before it starts")
	}
	if periodStart < 0 || periodEnd < 0 || actsBy < 0 {
		return types.ErrInvalidCovenant.Wrap("a time cannot be negative")
	}
	return nil
}

// PublishCovenant files an undertaking against a tokenisation.
//
// Append-only. Nothing here edits or removes an earlier filing: an operator who
// could withdraw a disclosure after publishing it has a register that says
// whatever is convenient today, which is worth less than no register because it
// looks authoritative.
//
// A correction goes forward, naming what it corrects, and the earlier version
// stays readable — a dispute is usually about what was said at a date rather
// than about what is said now.
func (k Keeper) PublishCovenant(
	ctx context.Context, assetID uint64, publisher string, msg *types.MsgPublishCovenant,
) (uint64, error) {
	title := strings.TrimSpace(msg.Title)
	uri := strings.TrimSpace(msg.Uri)
	if err := ValidateCovenant(
		msg.Kind, title, msg.Sha256, uri, msg.PeriodStart, msg.PeriodEnd, msg.ActsBy,
	); err != nil {
		return 0, err
	}

	// Correcting: the earlier filing must belong to THIS vehicle and must not
	// already have been corrected. Two corrections of one filing leave a reader
	// with two current versions and no rule for choosing between them.
	var previous types.Covenant
	if msg.Supersedes != 0 {
		var err error
		previous, err = k.Covenants.Get(ctx, msg.Supersedes)
		if err != nil {
			return 0, types.ErrCovenantNotFound.Wrapf("%d", msg.Supersedes)
		}
		if previous.AssetId != assetID {
			return 0, types.ErrInvalidCovenant.Wrapf(
				"filing %d belongs to tokenisation %d, not %d",
				previous.Id, previous.AssetId, assetID)
		}
		if previous.SupersededBy != 0 {
			return 0, types.ErrCovenantSuperseded.Wrapf(
				"filing %d was already corrected by %d", previous.Id, previous.SupersededBy)
		}
		if bytes.Equal(previous.Sha256, msg.Sha256) {
			return 0, types.ErrInvalidCovenant.Wrap(
				"the correction is byte-identical to the filing it corrects")
		}
	}

	id, err := k.NextCovenantID.Next(ctx)
	if err != nil {
		return 0, err
	}
	// Ids start at 1. Zero is indistinguishable from an unset proto field, and
	// `supersedes: 0` has to mean "corrects nothing" rather than "corrects
	// filing zero".
	id++

	cov := types.Covenant{
		Id:          id,
		AssetId:     assetID,
		Kind:        msg.Kind,
		Title:       title,
		Sha256:      msg.Sha256,
		Uri:         uri,
		PeriodStart: msg.PeriodStart,
		PeriodEnd:   msg.PeriodEnd,
		ActsBy:      msg.ActsBy,
		PublishedBy: publisher,
		PublishedAt: sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
		Supersedes:  msg.Supersedes,
	}
	if err := k.Covenants.Set(ctx, id, cov); err != nil {
		return 0, err
	}
	if err := k.CovenantsByAsset.Set(ctx, collections.Join(assetID, id)); err != nil {
		return 0, err
	}

	// The back-pointer last, so a failure above leaves the earlier filing
	// untouched rather than pointing at one that was never stored.
	if msg.Supersedes != 0 {
		previous.SupersededBy = id
		if err := k.Covenants.Set(ctx, previous.Id, previous); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// AssetCovenants returns one tokenisation's filings, oldest first.
//
// Oldest first because ids are issued in order, so that IS the order the
// vehicle's life happened in, and a register read backwards tells the story
// backwards.
func (k Keeper) AssetCovenants(
	ctx context.Context, assetID uint64, currentOnly bool,
) ([]types.Covenant, error) {
	rng := collections.NewPrefixedPairRange[uint64, uint64](assetID)
	it, err := k.CovenantsByAsset.Iterate(ctx, rng)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	out := []types.Covenant{}
	for ; it.Valid(); it.Next() {
		key, err := it.Key()
		if err != nil {
			return nil, err
		}
		cov, err := k.Covenants.Get(ctx, key.K2())
		if err != nil {
			return nil, err
		}
		if currentOnly && cov.SupersededBy != 0 {
			continue
		}
		out = append(out, cov)
	}
	return out, nil
}
