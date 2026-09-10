package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/syndicate/types"
)

// Closing, aborting, and paying money back out.
//
// These three are where the platform touches other people's money, so each one
// is written to be readable by somebody who is deciding whether to trust it.

// CloseDeal funds the company and issues the tokens.
//
// The order matters and is not arbitrary:
//
//  1. the fees come out of the raise,
//  2. the net goes to the drawdown account,
//  3. the tokens are minted and allocated against the GROSS commitment.
//
// Against the gross, because that is what the investor paid and what their
// proportion of the syndicate is. Allocating against the net would mean the
// fee was charged twice — once in cash and again in ownership — and the
// arithmetic would still balance, which is what makes it the easy mistake.
func (m msgServer) CloseDeal(
	ctx context.Context, msg *types.MsgCloseDeal,
) (*types.MsgCloseDealResponse, error) {
	deal, err := m.operatorDeal(ctx, msg.Operator, msg.DealId)
	if err != nil {
		return nil, err
	}
	if deal.State != types.DEAL_STATE_OPEN {
		return nil, types.ErrDealNotOpen.Wrapf("deal %d is %s", deal.Id, deal.State)
	}

	// A deal can be closed early once it is FULL, because there is nothing left
	// to wait for and holding a filled allocation open only costs the
	// investors. Otherwise the window has to run: closing early on a
	// part-filled book is the operator choosing which commitments count.
	if blockTime(ctx) < deal.ClosesAt && deal.Raised.LT(deal.Target) {
		return nil, types.ErrWindowOpen.Wrapf(
			"deal %d closes at %d and has raised %s of %s",
			deal.Id, deal.ClosesAt, deal.Raised, deal.Target)
	}
	if deal.Raised.LT(deal.Minimum) {
		return nil, types.ErrMinimumNotMet.Wrapf(
			"deal %d raised %s against a minimum of %s", deal.Id, deal.Raised, deal.Minimum)
	}

	operator, err := m.addressCodec.StringToBytes(deal.Operator)
	if err != nil {
		return nil, err
	}
	drawdown, err := m.addressCodec.StringToBytes(deal.DrawdownAccount)
	if err != nil {
		return nil, err
	}

	entryBps := int64(deal.Fees.SourcingBps) + int64(deal.Fees.SyndicationBps)
	fees := deal.Raised.Mul(math.NewInt(entryBps)).Quo(math.NewInt(types.MaxBps))
	invested := deal.Raised.Sub(fees)

	if fees.IsPositive() {
		if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, operator,
			sdk.NewCoins(sdk.NewCoin(deal.SettlementDenom, fees))); err != nil {
			return nil, err
		}
	}
	if invested.IsPositive() {
		if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, drawdown,
			sdk.NewCoins(sdk.NewCoin(deal.SettlementDenom, invested))); err != nil {
			return nil, err
		}
	}

	// One token per smallest unit of settlement raised, so a holder's
	// proportion is their balance over the supply and there is no rounding
	// decision hidden anywhere. The alternative — a fixed supply of, say, a
	// million tokens divided among subscribers — buries a division per investor
	// and leaves dust that has to go somewhere.
	deal.Denom = fmt.Sprintf("%s%d", types.DenomPrefix, deal.Id)
	deal.Supply = deal.Raised
	deal.State = types.DEAL_STATE_CLOSED
	deal.ClosedAt = blockTime(ctx)

	if err := m.bankKeeper.MintCoins(ctx, types.ModuleName,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, deal.Supply))); err != nil {
		return nil, err
	}
	if err := m.ByDenom.Set(ctx, deal.Denom, deal.Id); err != nil {
		return nil, err
	}
	// Written before the allocation, because allocating moves tokens and the
	// send restriction reads the deal. A deal recorded after its own tokens
	// have moved is a deal whose first transfers ran ungated.
	if err := m.Deal.Set(ctx, deal.Id, deal); err != nil {
		return nil, err
	}

	rng := collections.NewPrefixedPairRange[uint64, string](deal.Id)
	var allocation []types.Commitment
	if err := m.Commitment.Walk(ctx, rng,
		func(_ collections.Pair[uint64, string], c types.Commitment) (bool, error) {
			allocation = append(allocation, c)
			return false, nil
		}); err != nil {
		return nil, err
	}

	for _, c := range allocation {
		investor, err := m.addressCodec.StringToBytes(c.Investor)
		if err != nil {
			return nil, err
		}

		// The subscriber's position is written HERE, at the deal's index, and
		// not left to the send restriction to create lazily.
		//
		// The lazy path is what Settle does for a holder it has never seen, and
		// it has to assume they have just arrived — so it starts them at the
		// CURRENT index, which is right for a secondary buyer and catastrophic
		// for an original subscriber. An investor who subscribes, never
		// transfers, and claims after three distributions would be settled at
		// the index those distributions had already moved to, and would be paid
		// nothing at all while the chain reported their full holding.
		//
		// It would also have been invisible: relying on the restriction to
		// create the position works right up until somebody claims without
		// having transferred first, which is the ordinary behaviour of the
		// ordinary investor.
		if err := m.Position.Set(ctx, collections.Join(deal.Id, c.Investor), types.Position{
			DealId:  deal.Id,
			Holder:  c.Investor,
			Index:   deal.DistributionIndex,
			Accrued: math.ZeroInt(),
		}); err != nil {
			return nil, err
		}

		if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, investor,
			sdk.NewCoins(sdk.NewCoin(deal.Denom, c.Amount))); err != nil {
			return nil, err
		}
		if err := m.Commitment.Remove(ctx, collections.Join(deal.Id, c.Investor)); err != nil {
			return nil, err
		}
	}

	return &types.MsgCloseDealResponse{
		Raised:   deal.Raised,
		Fees:     fees,
		Invested: invested,
		Denom:    deal.Denom,
	}, nil
}

// AbortDeal refunds everybody in full.
//
// In full INCLUDING the fees, because no service was delivered. A platform that
// keeps a sourcing fee on a deal that never happened is charging for the
// attempt, and an investor who discovers that once does not commit again — so
// the fee is not merely unfair, it is a bad trade for the platform.
//
// Anybody may call this once the window has run and the minimum was not met.
// Only the operator may pull a deal early. The asymmetry is the point: an
// operator can decide not to proceed, but cannot decide to sit on escrowed
// money after a deal has demonstrably failed, and an investor who wants their
// cheque back should not need the operator's cooperation to get it.
func (m msgServer) AbortDeal(
	ctx context.Context, msg *types.MsgAbortDeal,
) (*types.MsgAbortDealResponse, error) {
	deal, err := m.Deal.Get(ctx, msg.DealId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoDeal.Wrapf("deal %d", msg.DealId)
		}
		return nil, err
	}
	if deal.State != types.DEAL_STATE_OPEN {
		return nil, types.ErrDealNotOpen.Wrapf("deal %d is %s", deal.Id, deal.State)
	}

	failed := blockTime(ctx) >= deal.ClosesAt && deal.Raised.LT(deal.Minimum)
	if !failed {
		if _, err := m.operatorDeal(ctx, msg.Operator, deal.Id); err != nil {
			return nil, err
		}
	}

	rng := collections.NewPrefixedPairRange[uint64, string](deal.Id)
	var refunds []types.Commitment
	if err := m.Commitment.Walk(ctx, rng,
		func(_ collections.Pair[uint64, string], c types.Commitment) (bool, error) {
			refunds = append(refunds, c)
			return false, nil
		}); err != nil {
		return nil, err
	}

	for _, c := range refunds {
		investor, err := m.addressCodec.StringToBytes(c.Investor)
		if err != nil {
			return nil, err
		}
		if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, investor,
			sdk.NewCoins(sdk.NewCoin(deal.SettlementDenom, c.Amount))); err != nil {
			return nil, err
		}
		if err := m.Commitment.Remove(ctx, collections.Join(deal.Id, c.Investor)); err != nil {
			return nil, err
		}
	}

	deal.State = types.DEAL_STATE_ABORTED
	deal.Raised = math.ZeroInt()
	deal.ClosedAt = blockTime(ctx)
	return &types.MsgAbortDealResponse{}, m.Deal.Set(ctx, deal.Id, deal)
}

// Distribute pays a dividend or exit proceeds into a deal, pro-rata.
//
// Permissionless as to WHO pays. The money arrives from wherever it arrives —
// the company, the partner's escrow agent, a nominee — and refusing a payment
// because the sender was not on a list would strand it in an account nobody
// controls. What the module controls is where it goes, which is pro-rata to
// whoever holds the token at this moment.
func (m msgServer) Distribute(
	ctx context.Context, msg *types.MsgDistribute,
) (*types.MsgDistributeResponse, error) {
	deal, err := m.Deal.Get(ctx, msg.DealId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoDeal.Wrapf("deal %d", msg.DealId)
		}
		return nil, err
	}
	if deal.State != types.DEAL_STATE_CLOSED {
		return nil, types.ErrDealNotFunded.Wrapf("deal %d is %s", deal.Id, deal.State)
	}
	if msg.Amount.Denom != deal.SettlementDenom {
		return nil, types.ErrWrongDenom.Wrapf(
			"this deal settles in %s", deal.SettlementDenom)
	}
	if !msg.Amount.Amount.IsPositive() {
		return nil, types.ErrInvalidAmount
	}
	if !deal.Supply.IsPositive() {
		return nil, types.ErrDealNotFunded.Wrapf("deal %d has no tokens in issue", deal.Id)
	}

	payer, err := m.addressCodec.StringToBytes(msg.Payer)
	if err != nil {
		return nil, err
	}
	if err := m.bankKeeper.SendCoinsFromAccountToModule(
		ctx, payer, types.ModuleName, sdk.NewCoins(msg.Amount)); err != nil {
		return nil, err
	}

	// The management fee comes off the top of each distribution rather than
	// accruing annually against committed capital. An annual fee on an illiquid
	// position has to be collected from somebody — by calling more capital,
	// which a syndicate cannot do, or by selling down the position, which it
	// cannot either. Charged here, the platform is paid when and only when the
	// investors are.
	fee := msg.Amount.Amount.
		Mul(math.NewInt(int64(deal.Fees.ManagementBps))).
		Quo(math.NewInt(types.MaxBps))
	net := msg.Amount.Amount.Sub(fee)

	if fee.IsPositive() {
		operator, err := m.addressCodec.StringToBytes(deal.Operator)
		if err != nil {
			return nil, err
		}
		if err := m.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, operator,
			sdk.NewCoins(sdk.NewCoin(deal.SettlementDenom, fee))); err != nil {
			return nil, err
		}
	}

	// One addition to one number, and every holder's entitlement moves. See
	// Keeper.Settle for why this is an index rather than a walk over holders.
	//
	// The remainder of the division stays in the pot. It is paid out with the
	// next distribution rather than swept, because sweeping it needs somebody
	// to sweep it to, and on a realised deal the only people with a claim on it
	// are the holders themselves.
	deal.DistributionIndex = deal.DistributionIndex.Add(
		net.Mul(math.NewInt(types.IndexScale)).Quo(deal.Supply))
	deal.Distributed = deal.Distributed.Add(net)

	if msg.Final {
		// Trading stops. Every token is now a claim on a known fixed sum, which
		// is what it actually is — and a secondary market still quoting a price
		// for a position that no longer changes is a trap for whoever buys last.
		deal.State = types.DEAL_STATE_REALISED
	}

	return &types.MsgDistributeResponse{}, m.Deal.Set(ctx, deal.Id, deal)
}

// Claim withdraws what a holder has accrued, without giving up the position.
func (m msgServer) Claim(
	ctx context.Context, msg *types.MsgClaim,
) (*types.MsgClaimResponse, error) {
	deal, err := m.Deal.Get(ctx, msg.DealId)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNoDeal.Wrapf("deal %d", msg.DealId)
		}
		return nil, err
	}

	holder, err := m.addressCodec.StringToBytes(msg.Holder)
	if err != nil {
		return nil, err
	}

	balance := m.bankKeeper.GetBalance(
		sdk.UnwrapSDKContext(ctx), holder, deal.Denom).Amount
	if err := m.Settle(ctx, deal.Id, holder, balance); err != nil {
		return nil, err
	}

	key := collections.Join(deal.Id, msg.Holder)
	pos, err := m.Position.Get(ctx, key)
	if err != nil {
		if isNotFound(err) {
			return nil, types.ErrNothingToClaim.Wrapf("deal %d", deal.Id)
		}
		return nil, err
	}
	if !pos.Accrued.IsPositive() {
		return nil, types.ErrNothingToClaim.Wrapf("deal %d", deal.Id)
	}

	paid := sdk.NewCoin(deal.SettlementDenom, pos.Accrued)
	if err := m.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, holder, sdk.NewCoins(paid)); err != nil {
		return nil, err
	}

	pos.Accrued = math.ZeroInt()
	if err := m.Position.Set(ctx, key, pos); err != nil {
		return nil, err
	}
	return &types.MsgClaimResponse{Paid: paid}, nil
}

// operatorDeal resolves a signer to a deal it is entitled to act on.
func (m msgServer) operatorDeal(
	ctx context.Context, signer string, id uint64,
) (types.Deal, error) {
	deal, err := m.Deal.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return types.Deal{}, types.ErrNoDeal.Wrapf("deal %d", id)
		}
		return types.Deal{}, err
	}
	if _, err := m.activeOperator(ctx, signer); err != nil {
		return types.Deal{}, err
	}
	// The operator that OPENED it, not any admitted operator. A second platform
	// able to close another's deal could move somebody else's escrowed money to
	// a drawdown account of its choosing.
	if deal.Operator != signer {
		return types.Deal{}, types.ErrNotOperator.Wrapf(
			"deal %d was opened by %s", id, deal.Operator)
	}
	return deal, nil
}
