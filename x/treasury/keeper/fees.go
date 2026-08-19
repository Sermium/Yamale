package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/treasury/types"
)

// SweepIntoOperatingTreasury moves everything held by sourceModule into the
// treasury named by fee_operating_treasury_id, crediting its ledger.
//
// This is how a deployment with no native token pays for itself. Network fees
// are collected in the currencies the deployment's issuers actually issue, and
// they are operating income rather than a reward for producing blocks —
// validators on such a network are paid by service contract, not by inflation
// or by a share of the fee. Left in the fee collector the fees would be
// allocated to validators by x/distribution's BeginBlocker, which is precisely
// the compensation model the profile exists to remove; hence the sweep, and
// hence the caller runs it before that BeginBlocker.
//
// The whole balance is taken rather than one transaction's fee because the fee
// collector has more than one inflow: a transaction whose messages failed still
// had its fee deducted, and a per-transaction hook cannot see it. Anything left
// behind is distributed to validators, so "everything that is here" is the only
// rule that leaves nothing behind.
//
// The funds land in the treasury module account and are credited to the
// treasury's ledger in the same call. Splitting those two would break the
// module's central invariant — that a treasury's ledger equals what the module
// account actually holds on its behalf — in the direction that hides money.
//
// Returns the swept coins, or nil when routing is off or there was nothing to
// sweep.
func (k Keeper) SweepIntoOperatingTreasury(ctx context.Context, sourceModule string) (sdk.Coins, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if !params.RouteFeesToOperatingTreasury {
		return nil, nil
	}

	treasuryID := params.FeeOperatingTreasuryId
	exists, err := k.Treasury.Has(ctx, treasuryID)
	if err != nil {
		return nil, err
	}
	if !exists {
		// Refused rather than improvised. Sending the coins to the module
		// account without a ledger entry to name them would strand them
		// permanently: no treasury message pays out what no treasury holds, and
		// a module account has no key to sign a bank transfer with.
		return nil, types.ErrTreasuryNotFound.Wrapf(
			"fee_operating_treasury_id is %d, which no treasury has", treasuryID)
	}

	coins := k.bankKeeper.GetAllBalances(ctx, authtypes.NewModuleAddress(sourceModule))
	if coins.IsZero() {
		return nil, nil
	}

	if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, sourceModule, types.ModuleName, coins); err != nil {
		return nil, err
	}

	for _, coin := range coins {
		if err := k.creditBalance(ctx, treasuryID, coin.Denom, coin.Amount); err != nil {
			return nil, err
		}
	}

	return coins, nil
}
