package keeper

import (
	"context"
	"time"

	"cosmossdk.io/math"
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

	// Recorded, because until it was the bond went into the shared module
	// account and no message anywhere gave it back or gave it to anyone. A
	// bond nobody can lose is not a deterrent and a bond nobody can recover is
	// a fee for objecting; ResolveDispute now decides which it was.
	report.Challenger = msg.Challenger
	report.Bond = sdk.NewCoin(report.Price.Denom, bond)

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

	upheld := msg.CorrectedPrice != nil && msg.CorrectedPrice.Amount.IsPositive()
	if upheld {
		report.Price = *msg.CorrectedPrice
		// The figure moved, so the challenge was right and the bond goes home.
		if err := m.refundBond(ctx, report); err != nil {
			return nil, err
		}
	} else if err := m.forfeitBond(ctx, msg.AssetId, report); err != nil {
		// The figure stood, so the challenge cost the holders a window. The
		// bond is theirs in full: the sponsor did nothing to earn a share of a
		// penalty paid for delaying their own holders.
		return nil, err
	}
	report.Challenger = ""
	report.Bond = sdk.NewCoin(report.Price.Denom, math.ZeroInt())

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
	//
	// But a reported price is a claim about a sale that happened elsewhere, and
	// this used to credit the index from the claim alone. No coins moved; the
	// index rose anyway; the first holder to redeem was paid out of the module
	// account, which holds every other vehicle's money. Finalisation now waits
	// on the holders' share having actually arrived.
	vault, err := k.Vaults.Get(ctx, assetID)
	if err != nil {
		return err
	}
	owed := holderCut(asset, report.Price.Amount)
	if report.ProceedsPaid.Denom != vault.Denom || report.ProceedsPaid.Amount.LT(owed) {
		return types.ErrProceedsUnpaid.Wrapf(
			"vehicle %d owes its holders %s%s and has been paid %s", assetID, owed, vault.Denom, report.ProceedsPaid)
	}
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

// refundBond returns a challenge bond to whoever posted it.
//
// The bond never entered the vault's ledger, so it is paid straight back out
// of the module account it was sent to. Nothing else has a claim on it.
func (m msgServer) refundBond(ctx context.Context, report types.SaleReport) error {
	if report.Challenger == "" || !report.Bond.Amount.IsPositive() {
		return nil
	}
	challenger, err := m.addressCodec.StringToBytes(report.Challenger)
	if err != nil {
		return err
	}
	return m.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, challenger, sdk.NewCoins(report.Bond))
}

// forfeitBond hands a lost bond to the vehicle's holders.
//
// It goes to them whole rather than through holder_share_bps: this is not
// income from the asset, it is a penalty paid for delaying the people who were
// waiting on it, and the sponsor was not the one kept waiting.
func (m msgServer) forfeitBond(ctx context.Context, assetID uint64, report types.SaleReport) error {
	if !report.Bond.Amount.IsPositive() {
		return nil
	}
	vault, err := m.Vaults.Get(ctx, assetID)
	if err != nil {
		return err
	}
	if report.Bond.Denom != vault.Denom {
		return types.ErrWrongDenom
	}
	if err := m.hold(ctx, assetID, report.Bond); err != nil {
		return err
	}
	return m.creditHolders(ctx, assetID, report.Bond.Amount, report.Bond.Amount)
}

// PaySaleProceeds pays the holders' share of a reported price into the vault.
//
// Permissionless on purpose. The obligation is the sponsor's, but a sponsor
// who has gone quiet after reporting a sale would otherwise strand every
// holder behind a finalisation that can never happen, and money arriving is
// never the problem. What is refused is money arriving that nobody is owed:
// an overpayment would sit in the ledger with no claim against it.
func (m msgServer) PaySaleProceeds(ctx context.Context, msg *types.MsgPaySaleProceeds) (*types.MsgPaySaleProceedsResponse, error) {
	if !msg.Amount.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	asset, err := m.Assets.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrAssetNotFound
	}
	if asset.Status != types.STATUS_REPORTED && asset.Status != types.STATUS_DISPUTED {
		return nil, types.ErrWrongStatus
	}
	report, err := m.Sales.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, types.ErrNoSaleReported
	}
	vault, err := m.Vaults.Get(ctx, msg.AssetId)
	if err != nil {
		return nil, err
	}
	if msg.Amount.Denom != vault.Denom {
		return nil, types.ErrWrongDenom
	}

	owed := holderCut(asset, report.Price.Amount)
	paid := report.ProceedsPaid.Amount
	if report.ProceedsPaid.Denom != vault.Denom {
		// Nothing has been paid yet, and the zero value carries no denom.
		paid = math.ZeroInt()
	}
	outstanding := owed.Sub(paid)
	if !outstanding.IsPositive() {
		return nil, types.ErrOverpayment.Wrapf("vehicle %d has already been paid %s%s", msg.AssetId, paid, vault.Denom)
	}
	if msg.Amount.Amount.GT(outstanding) {
		return nil, types.ErrOverpayment.Wrapf(
			"vehicle %d still owes %s%s and was offered %s", msg.AssetId, outstanding, vault.Denom, msg.Amount)
	}

	payer, err := m.addressCodec.StringToBytes(msg.Payer)
	if err != nil {
		return nil, err
	}
	if err := m.bankKeeper.SendCoinsFromAccountToModule(ctx, payer, types.ModuleName, sdk.NewCoins(msg.Amount)); err != nil {
		return nil, err
	}
	if err := m.hold(ctx, msg.AssetId, msg.Amount); err != nil {
		return nil, err
	}

	report.ProceedsPaid = sdk.NewCoin(vault.Denom, paid.Add(msg.Amount.Amount))
	if err := m.Sales.Set(ctx, msg.AssetId, report); err != nil {
		return nil, err
	}
	return &types.MsgPaySaleProceedsResponse{
		Paid:        report.ProceedsPaid,
		Outstanding: sdk.NewCoin(vault.Denom, outstanding.Sub(msg.Amount.Amount)),
	}, nil
}
