package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/oracle/types"
)

const (
	classID = "invoice"
	nftID   = "inv-001"
)

// approvedAppraiser walks the lifecycle a valuer actually goes through:
// applying openly, then being admitted by governance.
func (f *fixture) approvedAppraiser(t *testing.T, classIDs ...string) string {
	t.Helper()

	_, addr := f.env.Addr(t)
	_, err := f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator:     addr,
		Name:        "Test Valuers LLP",
		Credentials: "RICS 12345",
		ClassIds:    classIDs,
	})
	require.NoError(t, err)

	_, err = f.ms.ApproveAppraiser(f.ctx, &types.MsgApproveAppraiser{
		Authority: f.env.AuthorityString(t), Appraiser: addr, Approve: true,
	})
	require.NoError(t, err)

	return addr
}

func (f *fixture) submitAppraisal(t *testing.T, appraiser, value string, valuedAt int64) error {
	t.Helper()
	_, err := f.ms.SubmitAppraisal(f.ctx, &types.MsgSubmitAppraisal{
		Appraiser:  appraiser,
		ClassId:    classID,
		NftId:      nftID,
		Value:      value,
		ValueDenom: "uusd",
		ValuedAt:   valuedAt,
		Method:     "RICS Red Book",
		ReportUri:  "ipfs://report",
		ReportHash: "abc123",
	})
	return err
}

func TestOnlyApprovedAppraisersMayValue(t *testing.T) {
	f := initFixture(t)
	f.nft.mint(classID, nftID)
	now := f.env.Ctx.BlockTime().Unix()

	// An address nobody admitted.
	_, stranger := f.env.Addr(t)
	require.ErrorIs(t, f.submitAppraisal(t, stranger, "1000", now), types.ErrNotAnAppraiser)

	// An applicant who has not yet been approved. Applying grants nothing —
	// that is the whole point of the application being open to anyone.
	_, applicant := f.env.Addr(t)
	_, err := f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: applicant, Name: "Hopeful", Credentials: "none",
	})
	require.NoError(t, err)
	require.ErrorIs(t, f.submitAppraisal(t, applicant, "1000", now), types.ErrNotAnAppraiser)

	// Approved, in scope.
	appraiser := f.approvedAppraiser(t, classID)
	require.NoError(t, f.submitAppraisal(t, appraiser, "1000", now))
}

func TestAppraiserScopeIsEnforced(t *testing.T) {
	f := initFixture(t)
	f.nft.mint(classID, nftID)
	now := f.env.Ctx.BlockTime().Unix()

	// Admitted to value property, asked to value an invoice.
	appraiser := f.approvedAppraiser(t, "realestate")
	require.ErrorIs(t, f.submitAppraisal(t, appraiser, "1000", now), types.ErrOutOfScope)

	// An empty scope is the widest grant the module makes, and it does admit
	// every class.
	unrestricted := f.approvedAppraiser(t)
	require.NoError(t, f.submitAppraisal(t, unrestricted, "1000", now))
}

// Governance may admit somebody to less than they asked for without making them
// reapply, but approving cannot widen a scope by accident.
func TestGovernanceMayNarrowTheScope(t *testing.T) {
	f := initFixture(t)
	_, addr := f.env.Addr(t)

	_, err := f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: addr, Name: "Broad Valuers", Credentials: "RICS",
		ClassIds: []string{"invoice", "realestate", "art"},
	})
	require.NoError(t, err)

	_, err = f.ms.ApproveAppraiser(f.ctx, &types.MsgApproveAppraiser{
		Authority: f.env.AuthorityString(t), Appraiser: addr, Approve: true,
		ClassIds: []string{"invoice"},
	})
	require.NoError(t, err)

	stored, err := f.keeper.Appraiser.Get(f.ctx, addr)
	require.NoError(t, err)
	require.Equal(t, []string{"invoice"}, stored.ClassIds)
}

func TestAppraisalRequiresTheAssetToExist(t *testing.T) {
	f := initFixture(t)
	appraiser := f.approvedAppraiser(t)
	now := f.env.Ctx.BlockTime().Unix()

	// Nothing minted: a valuation of a token that does not exist would be
	// credit extended against nothing.
	require.ErrorIs(t, f.submitAppraisal(t, appraiser, "1000", now), types.ErrAssetNotFound)

	f.nft.mint(classID, nftID)
	require.NoError(t, f.submitAppraisal(t, appraiser, "1000", now))
}

func TestAppraisalRejectsUnusableValuations(t *testing.T) {
	f := initFixture(t)
	f.nft.mint(classID, nftID)
	appraiser := f.approvedAppraiser(t)
	now := f.env.Ctx.BlockTime().Unix()

	require.ErrorIs(t, f.submitAppraisal(t, appraiser, "-1", now), types.ErrInvalidValuation)
	require.ErrorIs(t, f.submitAppraisal(t, appraiser, "not-a-number", now), types.ErrInvalidValuation)

	// A future date would read as fresher than anything real, and would keep
	// reading that way as the staleness window never opens.
	require.ErrorIs(t, f.submitAppraisal(t, appraiser, "1000", now+3600), types.ErrValuationInFuture)

	// Zero is allowed: an asset that has become worthless is a real finding,
	// and refusing it would leave the last positive number standing.
	require.NoError(t, f.submitAppraisal(t, appraiser, "0", now))
}

// Revaluing supersedes rather than deletes: the record of what an asset was
// said to be worth, and by whom, is what an auditor needs after a dispute.
func TestRevaluationKeepsTheHistory(t *testing.T) {
	f := initFixture(t)
	f.nft.mint(classID, nftID)
	appraiser := f.approvedAppraiser(t)
	first := f.env.Ctx.BlockTime().Unix()

	require.NoError(t, f.submitAppraisal(t, appraiser, "1000", first))

	f.at(first+86_400, 2)
	require.NoError(t, f.submitAppraisal(t, appraiser, "800", first+86_400))

	current, err := f.qs.Appraisal(f.ctx, &types.QueryAppraisalRequest{ClassId: classID, NftId: nftID})
	require.NoError(t, err)
	require.Equal(t, "800", current.Appraisal.Value)
	require.False(t, current.Appraisal.Superseded)
	require.True(t, current.AppraiserStillApproved)

	history, err := f.qs.AppraisalHistory(f.ctx, &types.QueryAppraisalHistoryRequest{ClassId: classID, NftId: nftID})
	require.NoError(t, err)
	require.Len(t, history.Appraisals, 1)
	require.Equal(t, "1000", history.Appraisals[0].Value)
	require.True(t, history.Appraisals[0].Superseded)

	// Backdating past the standing valuation would silently replace a current
	// view of the world with an older one.
	require.Error(t, f.submitAppraisal(t, appraiser, "900", first-1))
}

// Revocation stops new valuations. The existing ones stay exactly as they are —
// they were validly signed when they were made — but a consumer can see that
// the signer's authority has since been withdrawn.
func TestRevocationStopsNewValuationsButKeepsOldOnes(t *testing.T) {
	f := initFixture(t)
	f.nft.mint(classID, nftID)
	appraiser := f.approvedAppraiser(t)
	now := f.env.Ctx.BlockTime().Unix()

	require.NoError(t, f.submitAppraisal(t, appraiser, "1000", now))

	_, strangerStr := f.env.Addr(t)
	_, err := f.ms.RevokeAppraiser(f.ctx, &types.MsgRevokeAppraiser{
		Authority: strangerStr, Appraiser: appraiser, Reason: "no reason",
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	_, err = f.ms.RevokeAppraiser(f.ctx, &types.MsgRevokeAppraiser{
		Authority: f.env.AuthorityString(t), Appraiser: appraiser, Reason: "licence lapsed",
	})
	require.NoError(t, err)

	require.ErrorIs(t, f.submitAppraisal(t, appraiser, "1200", now), types.ErrNotAnAppraiser)

	resp, err := f.qs.Appraisal(f.ctx, &types.QueryAppraisalRequest{ClassId: classID, NftId: nftID})
	require.NoError(t, err)
	require.Equal(t, "1000", resp.Appraisal.Value, "the valuation must survive its author being revoked")
	require.False(t, resp.AppraiserStillApproved, "but the consumer must be able to see the authority is gone")
}

// An approved valuer re-applying would otherwise silently reset their scope and
// status, which reads as a downgrade nobody voted for.
func TestReapplyingIsRefusedWhilePendingOrApproved(t *testing.T) {
	f := initFixture(t)
	appraiser := f.approvedAppraiser(t, classID)

	_, err := f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: appraiser, Name: "Same Firm", Credentials: "RICS", ClassIds: []string{"everything"},
	})
	require.ErrorIs(t, err, types.ErrApplicationExists)

	stored, err := f.keeper.Appraiser.Get(f.ctx, appraiser)
	require.NoError(t, err)
	require.Equal(t, types.AppraiserStatus_APPRAISER_STATUS_APPROVED, stored.Status)
	require.Equal(t, []string{classID}, stored.ClassIds)

	// A rejected applicant may try again.
	_, rejected := f.env.Addr(t)
	_, err = f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: rejected, Name: "Second Chance", Credentials: "pending licence",
	})
	require.NoError(t, err)
	_, err = f.ms.ApproveAppraiser(f.ctx, &types.MsgApproveAppraiser{
		Authority: f.env.AuthorityString(t), Appraiser: rejected, Approve: false,
	})
	require.NoError(t, err)
	_, err = f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: rejected, Name: "Second Chance", Credentials: "licence granted",
	})
	require.NoError(t, err)
}

func TestApplyingIsBounded(t *testing.T) {
	f := initFixture(t)
	_, addr := f.env.Addr(t)

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)

	tooMany := make([]string, params.MaxClassIdsPerAppraiser+1)
	for i := range tooMany {
		tooMany[i] = string(rune('a' + i%26))
	}

	_, err = f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: addr, Name: "Greedy", Credentials: "RICS", ClassIds: tooMany,
	})
	require.Error(t, err)

	// Anonymous applications are refused: a valuation with no attributable
	// author is exactly what this module exists to prevent.
	_, err = f.ms.ApplyAppraiser(f.ctx, &types.MsgApplyAppraiser{
		Creator: addr, Name: "", Credentials: "RICS",
	})
	require.ErrorIs(t, err, types.ErrNotAnAppraiser)
}
