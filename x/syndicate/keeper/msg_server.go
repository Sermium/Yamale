package keeper

import (
	"bytes"
	"context"
	"strings"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/group"

	aliastypes "yamale/blockchain/x/alias/types"
	"yamale/blockchain/x/syndicate/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns the write side of the syndication layer.
func NewMsgServerImpl(keeper Keeper) types.MsgServer { return &msgServer{Keeper: keeper} }

var _ types.MsgServer = msgServer{}

/* ========================================================================== */
/*  Who may act                                                               */
/* ========================================================================== */

// UpdateParams sets the platform-wide ceilings. Governance only.
func (m msgServer) UpdateParams(
	ctx context.Context, msg *types.MsgUpdateParams,
) (*types.MsgUpdateParamsResponse, error) {
	if err := m.assertGovernance(msg.Authority); err != nil {
		return nil, err
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, m.Params.Set(ctx, msg.Params)
}

// RegisterOperator admits a platform operator, or strikes one off.
func (m msgServer) RegisterOperator(
	ctx context.Context, msg *types.MsgRegisterOperator,
) (*types.MsgRegisterOperatorResponse, error) {
	if err := m.assertGovernance(msg.Authority); err != nil {
		return nil, err
	}
	if _, err := m.addressCodec.StringToBytes(msg.Operator); err != nil {
		return nil, types.ErrNotOperator.Wrapf("%q is not an address", msg.Operator)
	}

	// A group account, so that opening a deal, deciding it has closed and
	// moving escrowed money to a drawdown account each need several signatures
	// inside the operator. A platform able to do those on one key is one key
	// away from taking money and not investing it — and these investors are
	// spread across jurisdictions with no practical recourse.
	if err := m.assertGroupAccount(ctx, msg.Operator); err != nil {
		return nil, err
	}

	jurisdiction := aliastypes.NormaliseCountry(msg.Jurisdiction)
	if !aliastypes.AssignedCountry(jurisdiction) {
		return nil, types.ErrInvalidJurisdiction.Wrapf("%q", msg.Jurisdiction)
	}

	existing, err := m.Operator.Get(ctx, msg.Operator)
	admittedAt := blockTime(ctx)
	if err == nil {
		// Re-registering keeps the original admission date. It is the date the
		// platform was first trusted, and every deal it has opened was opened
		// under it; overwriting it on a routine re-registration would quietly
		// rewrite that history.
		admittedAt = existing.AdmittedAt
	} else if !isNotFound(err) {
		return nil, err
	}

	return &types.MsgRegisterOperatorResponse{}, m.Operator.Set(ctx, msg.Operator, types.Operator{
		Address:      msg.Operator,
		Name:         msg.Name,
		Jurisdiction: jurisdiction,
		Active:       msg.Active,
		AdmittedAt:   admittedAt,
	})
}

// OnboardInvestor records completed KYC/AML, or withdraws it.
//
// The chain does not perform the checks and this does not claim it did. It
// records that a named, accountable operator says they were performed, against
// a file reference somebody can later be asked to produce. That is the only
// honest thing a chain can offer, and it is worth more than nothing: the
// assertion is dated, attributable, and cannot be backdated.
func (m msgServer) OnboardInvestor(
	ctx context.Context, msg *types.MsgOnboardInvestor,
) (*types.MsgOnboardInvestorResponse, error) {
	op, err := m.activeOperator(ctx, msg.Operator)
	if err != nil {
		return nil, err
	}
	if _, err := m.addressCodec.StringToBytes(msg.Investor); err != nil {
		return nil, types.ErrInvalidInvestor.Wrapf("%q", msg.Investor)
	}

	jurisdiction := aliastypes.NormaliseCountry(msg.Jurisdiction)
	if !aliastypes.AssignedCountry(jurisdiction) {
		return nil, types.ErrInvalidJurisdiction.Wrapf("%q", msg.Jurisdiction)
	}

	onboardedAt := blockTime(ctx)
	if existing, err := m.Investor.Get(ctx, msg.Investor); err == nil {
		onboardedAt = existing.OnboardedAt
	} else if !isNotFound(err) {
		return nil, err
	}

	return &types.MsgOnboardInvestorResponse{}, m.Investor.Set(ctx, msg.Investor, types.Investor{
		Address:      msg.Investor,
		OnboardedBy:  op.Address,
		Reference:    strings.TrimSpace(msg.Reference),
		Jurisdiction: jurisdiction,
		Active:       msg.Active,
		OnboardedAt:  onboardedAt,
	})
}

/* ========================================================================== */
/*  The raise                                                                 */
/* ========================================================================== */

// OpenDeal puts an allocation in front of the investor base.
func (m msgServer) OpenDeal(
	ctx context.Context, msg *types.MsgOpenDeal,
) (*types.MsgOpenDealResponse, error) {
	op, err := m.activeOperator(ctx, msg.Operator)
	if err != nil {
		return nil, err
	}

	reference := strings.TrimSpace(msg.Reference)
	if reference == "" {
		return nil, types.ErrNoReference
	}
	if taken, err := m.ByDealRef.Has(ctx, reference); err != nil {
		return nil, err
	} else if taken {
		return nil, types.ErrRefTaken.Wrapf("%q", reference)
	}

	if _, err := m.addressCodec.StringToBytes(msg.DrawdownAccount); err != nil {
		return nil, types.ErrInvalidTerms.Wrapf("drawdown account %q", msg.DrawdownAccount)
	}
	if err := sdk.ValidateDenom(msg.SettlementDenom); err != nil {
		return nil, types.ErrInvalidTerms.Wrapf("settlement denom %q", msg.SettlementDenom)
	}

	params, err := m.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateTerms(msg, params, blockTime(ctx)); err != nil {
		return nil, err
	}

	id, err := nextID(ctx, m.NextDealID)
	if err != nil {
		return nil, err
	}

	deal := types.Deal{
		Id:                id,
		Reference:         reference,
		Company:           msg.Company,
		Partner:           msg.Partner,
		DrawdownAccount:   msg.DrawdownAccount,
		Operator:          op.Address,
		SettlementDenom:   msg.SettlementDenom,
		Target:            msg.Target,
		Minimum:           msg.Minimum,
		MinimumTicket:     msg.MinimumTicket,
		Fees:              msg.Fees,
		ClosesAt:          msg.ClosesAt,
		State:             types.DEAL_STATE_OPEN,
		Raised:            math.ZeroInt(),
		Supply:            math.ZeroInt(),
		DistributionIndex: math.ZeroInt(),
		Distributed:       math.ZeroInt(),
		OpenedAt:          blockTime(ctx),
	}
	if err := m.Deal.Set(ctx, id, deal); err != nil {
		return nil, err
	}
	if err := m.ByDealRef.Set(ctx, reference, id); err != nil {
		return nil, err
	}
	return &types.MsgOpenDealResponse{Id: id}, nil
}

// validateTerms refuses a deal nobody could subscribe to, or that nobody should.
func validateTerms(msg *types.MsgOpenDeal, params types.Params, now int64) error {
	if !msg.Target.IsPositive() {
		return types.ErrInvalidTerms.Wrap("the target must be positive")
	}
	if !msg.Minimum.IsPositive() {
		return types.ErrInvalidTerms.Wrap("the minimum must be positive")
	}
	if msg.Minimum.GT(msg.Target) {
		return types.ErrInvalidTerms.Wrapf(
			"the minimum (%s) is above the target (%s), so the deal can never close",
			msg.Minimum, msg.Target)
	}
	if !msg.MinimumTicket.IsPositive() {
		return types.ErrInvalidTerms.Wrap("the minimum cheque must be positive")
	}
	// A minimum cheque above the whole target admits exactly one investor, and
	// a syndicate of one is not a syndicate.
	if msg.MinimumTicket.GT(msg.Target) {
		return types.ErrInvalidTerms.Wrapf(
			"the minimum cheque (%s) is larger than the entire allocation (%s)",
			msg.MinimumTicket, msg.Target)
	}

	entry := uint64(msg.Fees.SourcingBps) + uint64(msg.Fees.SyndicationBps)
	if entry > uint64(params.MaxEntryFeeBps) {
		return types.ErrFeeTooHigh.Wrapf(
			"sourcing and syndication come to %d basis points, and the ceiling is %d",
			entry, params.MaxEntryFeeBps)
	}
	if msg.Fees.ManagementBps > params.MaxManagementFeeBps {
		return types.ErrFeeTooHigh.Wrapf(
			"the management fee is %d basis points, and the ceiling is %d",
			msg.Fees.ManagementBps, params.MaxManagementFeeBps)
	}

	open := msg.ClosesAt - now
	if open < params.MinOpenPeriod {
		return types.ErrWindowTooShort.Wrapf(
			"this deal would be open for %d seconds and the minimum is %d",
			open, params.MinOpenPeriod)
	}
	if open > params.MaxOpenPeriod {
		return types.ErrWindowTooLong.Wrapf(
			"this deal would hold committed money for %d seconds and the maximum is %d",
			open, params.MaxOpenPeriod)
	}
	return nil
}

// Commit escrows an investor's cheque.
func (m msgServer) Commit(
	ctx context.Context, msg *types.MsgCommit,
) (*types.MsgCommitResponse, error) {
	deal, err := m.openDeal(ctx, msg.DealId)
	if err != nil {
		return nil, err
	}
	if err := m.eligible(ctx, msg.Investor); err != nil {
		return nil, err
	}
	if msg.Amount.Denom != deal.SettlementDenom {
		return nil, types.ErrWrongDenom.Wrapf(
			"this deal settles in %s and you offered %s",
			deal.SettlementDenom, msg.Amount.Denom)
	}
	if !msg.Amount.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	if blockTime(ctx) >= deal.ClosesAt {
		return nil, types.ErrDealClosed.Wrapf("deal %d stopped accepting commitments", deal.Id)
	}

	investor, err := m.addressCodec.StringToBytes(msg.Investor)
	if err != nil {
		return nil, types.ErrInvalidInvestor.Wrapf("%q", msg.Investor)
	}

	// Topping up an existing commitment is one commitment, and the ticket
	// minimum applies to the TOTAL rather than to the increment. Applying it to
	// each increment instead would let somebody assemble a sub-minimum position
	// out of legal-sized pieces, and applying it only to the first would make
	// the second increment unbounded downward.
	key := collections.Join(deal.Id, msg.Investor)
	existing := math.ZeroInt()
	committedAt := blockTime(ctx)
	if c, err := m.Commitment.Get(ctx, key); err == nil {
		existing = c.Amount
		committedAt = c.CommittedAt
	} else if !isNotFound(err) {
		return nil, err
	}

	total := existing.Add(msg.Amount.Amount)
	if total.LT(deal.MinimumTicket) {
		return nil, types.ErrBelowTicket.Wrapf(
			"this deal's minimum cheque is %s %s and you would hold %s",
			deal.MinimumTicket, deal.SettlementDenom, total)
	}

	raised := deal.Raised.Add(msg.Amount.Amount)
	if raised.GT(deal.Target) {
		return nil, types.ErrOverTarget.Wrapf(
			"the allocation is %s and %s is already committed; there is room for %s",
			deal.Target, deal.Raised, deal.Target.Sub(deal.Raised))
	}

	// The money moves NOW. A commitment that is only a promise lets a deal
	// close on paper and fail to fund, which is the failure that ends the
	// platform's relationship with the partner firm — and that relationship is
	// the only asset the platform actually has.
	if err := m.bankKeeper.SendCoinsFromAccountToModule(
		ctx, investor, types.ModuleName, sdk.NewCoins(msg.Amount)); err != nil {
		return nil, err
	}

	if err := m.Commitment.Set(ctx, key, types.Commitment{
		DealId:      deal.Id,
		Investor:    msg.Investor,
		Amount:      total,
		CommittedAt: committedAt,
	}); err != nil {
		return nil, err
	}

	deal.Raised = raised
	return &types.MsgCommitResponse{}, m.Deal.Set(ctx, deal.Id, deal)
}

// WithdrawCommitment takes an escrowed cheque back, in full.
//
// Unconditional while the deal is OPEN, and that is not generosity. Committed
// money is out of the investor's hands and earning nothing against a deal that
// may never happen; the right to withdraw is what makes committing early
// rational, and a syndicate whose early commitments are hostages does not fill.
func (m msgServer) WithdrawCommitment(
	ctx context.Context, msg *types.MsgWithdrawCommitment,
) (*types.MsgWithdrawCommitmentResponse, error) {
	deal, err := m.openDeal(ctx, msg.DealId)
	if err != nil {
		return nil, err
	}

	key := collections.Join(deal.Id, msg.Investor)
	commitment, err := m.Commitment.Get(ctx, key)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoCommitment.Wrapf("deal %d", deal.Id)
		}
		return nil, err
	}

	investor, err := m.addressCodec.StringToBytes(msg.Investor)
	if err != nil {
		return nil, err
	}
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, investor,
		sdk.NewCoins(sdk.NewCoin(deal.SettlementDenom, commitment.Amount))); err != nil {
		return nil, err
	}
	if err := m.Commitment.Remove(ctx, key); err != nil {
		return nil, err
	}

	deal.Raised = deal.Raised.Sub(commitment.Amount)
	return &types.MsgWithdrawCommitmentResponse{}, m.Deal.Set(ctx, deal.Id, deal)
}

/* ========================================================================== */
/*  Helpers                                                                   */
/* ========================================================================== */

func (m msgServer) openDeal(ctx context.Context, id uint64) (types.Deal, error) {
	deal, err := m.Deal.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return types.Deal{}, types.ErrNoDeal.Wrapf("deal %d", id)
		}
		return types.Deal{}, err
	}
	if deal.State != types.DEAL_STATE_OPEN {
		return types.Deal{}, types.ErrDealNotOpen.Wrapf("deal %d is %s", id, deal.State)
	}
	return deal, nil
}

// assertGovernance refuses anybody but the gov module account.
func (m msgServer) assertGovernance(signer string) error {
	want, err := m.addressCodec.BytesToString(m.GetAuthority())
	if err != nil {
		return err
	}
	got, err := m.addressCodec.StringToBytes(signer)
	if err != nil {
		return types.ErrNotAuthority.Wrapf("%q is not an address", signer)
	}
	if !bytes.Equal(got, m.GetAuthority()) {
		return types.ErrNotAuthority.Wrapf(
			"this message is governance-only: expected %s, got %s", want, signer)
	}
	return nil
}

// assertGroupAccount refuses an operator that is a plain key.
func (m msgServer) assertGroupAccount(ctx context.Context, addr string) error {
	if m.groupKeeper == nil {
		return types.ErrOperatorNotGroup.Wrapf(
			"this chain has no group module, so %s cannot be verified as one", addr)
	}
	if _, err := m.groupKeeper.GroupPolicyInfo(ctx, &group.QueryGroupPolicyInfoRequest{
		Address: addr,
	}); err != nil {
		return types.ErrOperatorNotGroup.Wrapf("%s", addr)
	}
	return nil
}

// blockTime is used throughout rather than a height, because every period this
// module expresses — a raise window, a holding period — is a period a person
// experiences, and a person experiences days.
func blockTime(ctx context.Context) int64 {
	return sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
}
