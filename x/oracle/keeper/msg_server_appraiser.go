package keeper

import (
	"context"
	"errors"
	"strings"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/oracle/types"
)

// ApplyAppraiser records a pending application to become a valuer.
//
// Anyone may apply and applying grants nothing. The application exists so that
// what governance approved — the name, the credentials it relied on, the scope
// asked for — is on the chain next to the decision, rather than living in a
// forum post that can be edited after the vote.
func (k msgServer) ApplyAppraiser(ctx context.Context, msg *types.MsgApplyAppraiser) (*types.MsgApplyAppraiserResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	if _, err := k.addressCodec.StringToBytes(msg.Creator); err != nil {
		return nil, errorsmod.Wrap(err, "invalid appraiser address")
	}
	if strings.TrimSpace(msg.Name) == "" {
		return nil, errorsmod.Wrap(types.ErrNotAnAppraiser, "name must be set, so a valuation can be attributed to somebody")
	}
	if strings.TrimSpace(msg.Credentials) == "" {
		return nil, errorsmod.Wrap(types.ErrNotAnAppraiser, "credentials must be set, so governance has something to check")
	}
	// Applying is permissionless, so these strings are attacker-chosen state the
	// chain keeps forever.
	if err := types.ValidateAppraiserText(msg.Name, msg.Credentials); err != nil {
		return nil, err
	}

	classIDs, err := k.validateClassIDs(msg.ClassIds, params.MaxClassIdsPerAppraiser)
	if err != nil {
		return nil, err
	}

	// An approved valuer re-applying would otherwise silently reset their scope
	// and status back to pending, which reads as a downgrade nobody voted for.
	// Widening a scope is a governance decision, taken through ApproveAppraiser.
	existing, err := k.Appraiser.Get(ctx, msg.Creator)
	switch {
	case err == nil:
		if existing.Status != types.AppraiserStatus_APPRAISER_STATUS_REJECTED {
			return nil, errorsmod.Wrapf(types.ErrApplicationExists, "%s is already %s", msg.Creator, statusWord(existing.Status))
		}
	case !errors.Is(err, collections.ErrNotFound):
		return nil, err
	}

	return &types.MsgApplyAppraiserResponse{}, k.Appraiser.Set(ctx, msg.Creator, types.Appraiser{
		Address:     msg.Creator,
		Name:        msg.Name,
		Credentials: msg.Credentials,
		ClassIds:    classIDs,
		Status:      types.AppraiserStatus_APPRAISER_STATUS_PENDING,
	})
}

// ApproveAppraiser records governance's decision on an application.
func (k msgServer) ApproveAppraiser(ctx context.Context, msg *types.MsgApproveAppraiser) (*types.MsgApproveAppraiserResponse, error) {
	if err := k.assertAuthority(msg.Authority); err != nil {
		return nil, err
	}

	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	appraiser, err := k.Appraiser.Get(ctx, msg.Appraiser)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrapf(types.ErrApplicationMissing, "%s", msg.Appraiser)
		}
		return nil, err
	}
	if appraiser.Status != types.AppraiserStatus_APPRAISER_STATUS_PENDING {
		return nil, errorsmod.Wrapf(types.ErrNotPending, "%s is %s", msg.Appraiser, statusWord(appraiser.Status))
	}

	if !msg.Approve {
		appraiser.Status = types.AppraiserStatus_APPRAISER_STATUS_REJECTED
		return &types.MsgApproveAppraiserResponse{}, k.Appraiser.Set(ctx, msg.Appraiser, appraiser)
	}

	// Governance may narrow the scope that was applied for. Leaving class_ids
	// empty keeps what the applicant asked for, so the common case — approve as
	// requested — does not require restating the list and cannot widen it by
	// accident.
	if len(msg.ClassIds) > 0 {
		classIDs, err := k.validateClassIDs(msg.ClassIds, params.MaxClassIdsPerAppraiser)
		if err != nil {
			return nil, err
		}
		appraiser.ClassIds = classIDs
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	appraiser.Status = types.AppraiserStatus_APPRAISER_STATUS_APPROVED
	appraiser.ApprovedAtHeight = uint64(sdkCtx.BlockHeight())

	return &types.MsgApproveAppraiserResponse{}, k.Appraiser.Set(ctx, msg.Appraiser, appraiser)
}

// RevokeAppraiser withdraws a valuer's authority to sign new valuations.
//
// Their existing appraisals are left exactly as they are. They were validly
// signed when they were made, and deleting them would rewrite the record rather
// than correct it — an auditor asking why an asset was valued at that number
// needs the number to still be there. Consumers can see that the signer's
// authority has since been withdrawn: the appraisal query reports it.
func (k msgServer) RevokeAppraiser(ctx context.Context, msg *types.MsgRevokeAppraiser) (*types.MsgRevokeAppraiserResponse, error) {
	if err := k.assertAuthority(msg.Authority); err != nil {
		return nil, err
	}

	appraiser, err := k.Appraiser.Get(ctx, msg.Appraiser)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrapf(types.ErrApplicationMissing, "%s", msg.Appraiser)
		}
		return nil, err
	}
	if appraiser.Status != types.AppraiserStatus_APPRAISER_STATUS_APPROVED {
		return nil, errorsmod.Wrapf(types.ErrNotAnAppraiser, "%s is %s", msg.Appraiser, statusWord(appraiser.Status))
	}

	appraiser.Status = types.AppraiserStatus_APPRAISER_STATUS_REJECTED
	return &types.MsgRevokeAppraiserResponse{}, k.Appraiser.Set(ctx, msg.Appraiser, appraiser)
}

// SubmitAppraisal records a signed valuation of one tokenised asset.
func (k msgServer) SubmitAppraisal(ctx context.Context, msg *types.MsgSubmitAppraisal) (*types.MsgSubmitAppraisalResponse, error) {
	// Bounded first, before any state is read: an oversized message should cost
	// a length comparison to reject, not a store lookup.
	//
	// An approved valuer is trusted to be honest about numbers, not trusted with
	// unbounded state — and a stolen valuer key should not be able to fill every
	// validator's disk.
	if err := types.ValidateAppraisalText(msg.ClassId, msg.NftId, msg.Method, msg.ReportUri, msg.ReportHash); err != nil {
		return nil, err
	}

	appraiser, err := k.Appraiser.Get(ctx, msg.Appraiser)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, errorsmod.Wrapf(types.ErrNotAnAppraiser, "%s", msg.Appraiser)
		}
		return nil, err
	}
	if appraiser.Status != types.AppraiserStatus_APPRAISER_STATUS_APPROVED {
		return nil, errorsmod.Wrapf(types.ErrNotAnAppraiser, "%s is %s", msg.Appraiser, statusWord(appraiser.Status))
	}
	if !scopeCovers(appraiser.ClassIds, msg.ClassId) {
		return nil, errorsmod.Wrapf(types.ErrOutOfScope, "%s may not value class %s", msg.Appraiser, msg.ClassId)
	}

	// The asset must exist. Without this an appraiser could publish valuations
	// for tokens that were never minted, and a lending module reading them would
	// extend credit against nothing.
	if !k.nftKeeper.HasNFT(ctx, msg.ClassId, msg.NftId) {
		return nil, errorsmod.Wrapf(types.ErrAssetNotFound, "%s/%s", msg.ClassId, msg.NftId)
	}

	value, ok := math.NewIntFromString(msg.Value)
	if !ok {
		return nil, errorsmod.Wrapf(types.ErrInvalidValuation, "%q is not an integer", msg.Value)
	}
	// Zero is allowed: an asset that has become worthless is a real finding, and
	// forcing the valuer to omit it instead would leave the last positive number
	// standing as the current one.
	if value.IsNegative() {
		return nil, errorsmod.Wrapf(types.ErrInvalidValuation, "value must not be negative, got %s", msg.Value)
	}
	if err := sdk.ValidateDenom(msg.ValueDenom); err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidValuation, "invalid value denom: %s", err)
	}
	if strings.TrimSpace(msg.Method) == "" {
		return nil, errorsmod.Wrap(types.ErrInvalidValuation, "method must be set, so the number can be understood")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().Unix()
	if msg.ValuedAt <= 0 {
		return nil, errorsmod.Wrap(types.ErrInvalidValuation, "valued_at must be set")
	}
	// A valuation dated in the future would read as fresher than anything real,
	// and would keep reading that way as the staleness window it is supposed to
	// trip never opens.
	if msg.ValuedAt > now {
		return nil, errorsmod.Wrapf(types.ErrValuationInFuture, "valued_at %d is after the current block time %d", msg.ValuedAt, now)
	}

	key := collections.Join(msg.ClassId, msg.NftId)

	// A newer valuation supersedes the current one rather than deleting it. The
	// previous number moves to history with a sequence, so the record of what
	// the asset was said to be worth over time survives.
	if previous, err := k.Appraisal.Get(ctx, key); err == nil {
		// Backdating past the standing valuation would let a later report
		// silently replace a more current one with an older view of the world.
		if msg.ValuedAt < previous.ValuedAt {
			return nil, errorsmod.Wrapf(types.ErrInvalidValuation,
				"valued_at %d predates the current valuation of %d", msg.ValuedAt, previous.ValuedAt)
		}

		seq, err := k.nextAppraisalSeq(ctx, key)
		if err != nil {
			return nil, err
		}
		previous.Superseded = true
		if err := k.AppraisalHistory.Set(ctx, collections.Join3(msg.ClassId, msg.NftId, seq), previous); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, collections.ErrNotFound) {
		return nil, err
	}

	return &types.MsgSubmitAppraisalResponse{}, k.Appraisal.Set(ctx, key, types.Appraisal{
		ClassId:         msg.ClassId,
		NftId:           msg.NftId,
		Value:           value.String(),
		ValueDenom:      msg.ValueDenom,
		Appraiser:       msg.Appraiser,
		ValuedAt:        msg.ValuedAt,
		SubmittedAt:     now,
		SubmittedHeight: uint64(sdkCtx.BlockHeight()),
		Method:          msg.Method,
		ReportUri:       msg.ReportUri,
		ReportHash:      msg.ReportHash,
	})
}

// nextAppraisalSeq returns the next history slot for an asset.
//
// The counter is stored rather than derived by counting history entries,
// because counting would be O(n) in the number of revaluations and an asset
// revalued quarterly for a decade accumulates enough of them to matter.
func (k Keeper) nextAppraisalSeq(ctx context.Context, key collections.Pair[string, string]) (uint64, error) {
	seq, err := k.AppraisalSeq.Get(ctx, key)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return 0, err
	}
	seq++
	return seq, k.AppraisalSeq.Set(ctx, key, seq)
}

// validateClassIDs checks a requested scope and rejects duplicates.
func (k Keeper) validateClassIDs(classIDs []string, max uint64) ([]string, error) {
	if uint64(len(classIDs)) > max {
		return nil, errorsmod.Wrapf(types.ErrLimitReached,
			"an appraiser may cover at most %d classes, got %d", max, len(classIDs))
	}

	seen := make(map[string]bool, len(classIDs))
	for _, id := range classIDs {
		if strings.TrimSpace(id) == "" {
			return nil, errorsmod.Wrap(types.ErrOutOfScope, "class id must not be empty")
		}
		// The count is capped above; each entry needs its own bound, or fifty
		// class ids of a megabyte each would clear the count check.
		if len(id) > types.MaxIdentifierLength {
			return nil, errorsmod.Wrapf(types.ErrLimitReached,
				"class id must be at most %d bytes, got %d", types.MaxIdentifierLength, len(id))
		}
		if seen[id] {
			return nil, errorsmod.Wrapf(types.ErrOutOfScope, "class id %s appears twice", id)
		}
		seen[id] = true
	}

	return classIDs, nil
}

// scopeCovers reports whether an appraiser's scope admits a class.
//
// An empty scope means every class. That is the widest grant the module can
// make, and governance should make it rarely — it is preserved as an option
// because a chain whose asset classes are still being created would otherwise
// have to amend its valuer's scope for every new one.
func scopeCovers(scope []string, classID string) bool {
	if len(scope) == 0 {
		return true
	}
	for _, id := range scope {
		if id == classID {
			return true
		}
	}
	return false
}

func statusWord(status types.AppraiserStatus) string {
	switch status {
	case types.AppraiserStatus_APPRAISER_STATUS_PENDING:
		return "pending"
	case types.AppraiserStatus_APPRAISER_STATUS_APPROVED:
		return "approved"
	case types.AppraiserStatus_APPRAISER_STATUS_REJECTED:
		return "rejected or revoked"
	default:
		return "unspecified"
	}
}
