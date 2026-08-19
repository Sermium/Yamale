package ante

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/stablecoin/types"
)

// StablecoinKeeper is the part of x/stablecoin this decorator needs: the
// register of denominations governance has approved an issuer for.
type StablecoinKeeper interface {
	IsApprovedDenom(ctx context.Context, denom string) (bool, error)
}

// ApprovedFeeDenomDecorator refuses a transaction whose fee is offered in a
// denomination no approved issuer issues.
//
// It exists for the deployment that has no native token. There, the fee is paid
// in the currency the deployment's own issuers put on the chain, and every
// other denomination is something a user minted somewhere else or invented
// outright. Without this check a sender can pay the network in a worthless
// denom of their own naming and still consume a validator's block space, and
// the operating account fills with balances nobody will ever redeem — a spam
// price of zero, expressed as a treasury full of tokens.
//
// It runs before fee deduction so the rejection names the denomination.
// Deducting first would fail on the balance instead, telling a bank that its
// funded account has insufficient funds.
type ApprovedFeeDenomDecorator struct {
	keeper StablecoinKeeper
}

func NewApprovedFeeDenomDecorator(k StablecoinKeeper) ApprovedFeeDenomDecorator {
	return ApprovedFeeDenomDecorator{keeper: k}
}

func (d ApprovedFeeDenomDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		return next(ctx, tx, simulate)
	}

	// A zero fee is not a fee in an unapproved denomination. Whether it is
	// enough is the minimum-gas-price check's decision, not this one's, and
	// genesis transactions carry no fee at all.
	for _, coin := range feeTx.GetFee() {
		approved, err := d.keeper.IsApprovedDenom(ctx, coin.Denom)
		if err != nil {
			return ctx, err
		}
		if !approved {
			return ctx, errorsmod.Wrapf(types.ErrFeeDenomNotIssued,
				"%s: this chain has no native token, so fees are payable only in a currency with an approved issuer",
				coin.Denom)
		}
	}

	return next(ctx, tx, simulate)
}
