package keeper

import (
	"context"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/tokenisation/types"
)

// ReportSale states what the asset sold for. It does not open redemption.
//
// The figure is recorded and public, and sits for the collection's challenge
// window. Verification without a window would be decorative: redemption is
// irreversible, so a price nobody can contest before it pays out only had to
// survive one block.
func (m msgServer) ReportSale(ctx context.Context, msg *types.MsgReportSale) (*types.MsgReportSaleResponse, error) {
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	if asset.Owner != msg.Reporter {
		return nil, types.ErrNotOwner
	}
	if asset.Status != types.STATUS_ACTIVE {
		return nil, types.ErrWrongStatus
	}
	if !msg.Price.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}

	c, err := m.Collections.Get(ctx, asset.CollectionId)
	if err != nil {
		return nil, types.ErrCollectionNotFound
	}

	now := sdk.UnwrapSDKContext(ctx).BlockTime()
	report := types.SaleReport{
		AssetId:     msg.AssetId,
		Price:       msg.Price,
		Reporter:    msg.Reporter,
		ReportedAt:  now,
		ClaimableAt: now.Add(time.Duration(c.ChallengeWindowSeconds) * time.Second),
	}
	if err := m.Sales.Set(ctx, msg.AssetId, report); err != nil {
		return nil, err
	}

	asset.Status = types.STATUS_REPORTED
	if err := m.Assets.Set(ctx, msg.AssetId, asset); err != nil {
		return nil, err
	}
	return &types.MsgReportSaleResponse{}, nil
}

// AttestSale signs a reported figure. Attestors must agree on the same number:
// an attestor who signs a different price is not confirming, they are proposing.
func (m msgServer) AttestSale(ctx context.Context, msg *types.MsgAttestSale) (*types.MsgAttestSaleResponse, error) {
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	c, err := m.Collections.Get(ctx, asset.CollectionId)
	if err != nil {
		return nil, types.ErrCollectionNotFound
	}
	if c.Verification != types.VERIFY_ATTESTORS {
		return nil, types.ErrWrongStatus
	}

	// Who is signing, which went unchecked until an app built over this module
	// asked what the threshold actually protects. It protected nothing: any
	// address could attest, so a sponsor met any threshold with N fresh keys
	// for the cost of the gas, and the one guard standing between a shareholder
	// and a sale reported below what was received was decorative.
	if !appointedAttestor(c.Attestors, msg.Attestor) {
		return nil, types.ErrNotAttestor.Wrapf(
			"%s is not appointed to attest for collection %s", msg.Attestor, asset.CollectionId)
	}

	report, err := m.Sales.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrNoSaleReported
	}
	if !report.Price.IsEqual(msg.Price) {
		return nil, types.ErrNotVerified
	}
	for _, a := range report.Attestors {
		if a == msg.Attestor {
			return nil, types.ErrAlreadyAttested
		}
	}

	report.Attestors = append(report.Attestors, msg.Attestor)
	return &types.MsgAttestSaleResponse{}, m.Sales.Set(ctx, msg.AssetId, report)
}

// DisputeSale suspends redemption and refers the figure to governance.
//
// The bond scales with the vehicle rather than being flat: a flat bond is
// trivial for a large fraud to post and prohibitive for a small holder to
// raise. A failed dispute forfeits it to the vault, never to the issuer —
// paying the issuer would reward provoking weak challenges.
func (m msgServer) DisputeSale(ctx context.Context, msg *types.MsgDisputeSale) (*types.MsgDisputeSaleResponse, error) {
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	if asset.Status != types.STATUS_REPORTED {
		return nil, types.ErrWrongStatus
	}
	report, err := m.Sales.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrNoSaleReported
	}
	if report.Disputed {
		return nil, types.ErrAlreadyDisputed
	}
	// Past the window, not inside it. The guard is right and its error was
	// exactly backwards, which would send whoever debugs a refused dispute
	// looking for a window that has already closed.
	if sdk.UnwrapSDKContext(ctx).BlockTime().After(report.ClaimableAt) {
		return nil, types.ErrWindowClosed
	}

	c, err := m.Collections.Get(ctx, asset.CollectionId)
	if err != nil {
		return nil, types.ErrCollectionNotFound
	}

	challenger, err := m.addressCodec.StringToBytes(msg.Challenger)
	if err != nil {
		return nil, err
	}
	bond := report.Price.Amount.MulRaw(int64(c.DisputeBondBps)).QuoRaw(10_000)
	if bond.IsPositive() {
		coins := sdk.NewCoins(sdk.NewCoin(report.Price.Denom, bond))
		if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, challenger, types.ModuleName, coins); err != nil {
			return nil, err
		}
	}

	report.Disputed = true
	if err := m.Sales.Set(ctx, msg.AssetId, report); err != nil {
		return nil, err
	}
	asset.Status = types.STATUS_DISPUTED
	return &types.MsgDisputeSaleResponse{}, m.Assets.Set(ctx, msg.AssetId, asset)
}

// ResolveDispute is governance deciding the contested figure. An empty
// corrected price upholds what was reported.
func (m msgServer) ResolveDispute(ctx context.Context, msg *types.MsgResolveDispute) (*types.MsgResolveDisputeResponse, error) {
	if err := m.assertGov(msg.Authority); err != nil {
		return nil, err
	}
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	if asset.Status != types.STATUS_DISPUTED {
		return nil, types.ErrWrongStatus
	}
	report, err := m.Sales.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrNoSaleReported
	}

	if msg.CorrectedPrice != nil && msg.CorrectedPrice.Amount.IsPositive() {
		report.Price = *msg.CorrectedPrice
	}
	report.Disputed = false
	if err := m.Sales.Set(ctx, msg.AssetId, report); err != nil {
		return nil, err
	}

	// Back to REPORTED rather than straight to REALISED: a corrected figure is
	// a new figure, and it deserves the same window the original got.
	asset.Status = types.STATUS_REPORTED
	return &types.MsgResolveDisputeResponse{}, m.Assets.Set(ctx, msg.AssetId, asset)
}

// FinaliseSale is the permissionless crank that opens redemption once a
// reported price has cleared verification and its window.
//
// A crank rather than an EndBlocker sweep: iterating every reported sale each
// block is a denial-of-service surface that grows with usage, and the outcome
// is fully determined by the report and the clock, so it does not matter when
// somebody pays for it.
func (k Keeper) FinaliseSale(ctx context.Context, assetID uint64) error {
	asset, err := k.Assets.Get(ctx, assetID)
	if err != nil {
		return types.ErrAssetNotFound
	}
	if asset.Status != types.STATUS_REPORTED {
		return types.ErrWrongStatus
	}
	report, err := k.Sales.Get(ctx, assetID)
	if err != nil {
		return types.ErrNoSaleReported
	}
	if report.Disputed {
		return types.ErrAlreadyDisputed
	}
	if sdk.UnwrapSDKContext(ctx).BlockTime().Before(report.ClaimableAt) {
		return types.ErrStillInWindow
	}

	c, err := k.Collections.Get(ctx, asset.CollectionId)
	if err != nil {
		return types.ErrCollectionNotFound
	}
	if c.Verification == types.VERIFY_ATTESTORS &&
		uint32(len(report.Attestors)) < c.AttestationThreshold {
		return types.ErrNotVerified
	}

	// The proceeds are the last distribution, and the largest. They run through
	// exactly the same index as every coupon before them — the sale is not a
	// special case in the accounting.
	if err := k.AccrueIncome(ctx, assetID, report.Price.Amount); err != nil {
		return err
	}

	asset.Status = types.STATUS_REALISED
	return k.Assets.Set(ctx, assetID, asset)
}

// appointedAttestor reports whether an account sits in a collection's register.
//
// A linear scan, and deliberately so: the register is small by construction —
// it is a list of institutions somebody voted for, not a user table — and a
// scan over it keeps the check next to the data it checks rather than behind an
// index that has to be kept in step.
func appointedAttestor(register []string, who string) bool {
	for _, a := range register {
		if a == who {
			return true
		}
	}
	return false
}
