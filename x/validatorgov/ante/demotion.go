package ante

import (
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"

	"yamale/blockchain/x/validatorgov/keeper"
)

// DemotionGateDecorator refuses MsgUnjail from a validator the concentration
// check is holding down.
//
// Without it the ceilings are advisory. A demotion is carried out by jailing,
// and x/staking's Jail sets no jailed-until time, so the ordinary unjail path
// is open the moment the block ends — an operator over the cap could send
// MsgUnjail every block and be back in the set until the next epoch, forever.
// The gate closes that: a demoted validator comes back when the breach clears
// and the epoch check restores it, and by no other route.
//
// It is a decorator and not a check inside x/slashing because the message
// belongs to x/slashing and this rule does not. It descends into MsgExec for
// the same reason the validator gate does: authz dispatches what it carries
// after the ante chain has run, and a grant to yourself is an ordinary
// unprivileged transaction, so a gate that inspected only the top level would
// be one MsgExec away from being no gate at all.
type DemotionGateDecorator struct {
	keeper keeper.Keeper
}

func NewDemotionGateDecorator(k keeper.Keeper) DemotionGateDecorator {
	return DemotionGateDecorator{keeper: k}
}

func (d DemotionGateDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	// Genesis transactions are collected before any validator exists, so there
	// is nothing to have demoted and nothing to check.
	if ctx.BlockHeight() == 0 {
		return next(ctx, tx, simulate)
	}

	if err := d.check(ctx, tx.GetMsgs(), 0); err != nil {
		return ctx, err
	}

	return next(ctx, tx, simulate)
}

func (d DemotionGateDecorator) check(ctx sdk.Context, msgs []sdk.Msg, depth int) error {
	if depth > maxNesting {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
			"transaction nests messages more than %d deep", maxNesting)
	}

	for _, msg := range msgs {
		if exec, ok := msg.(*authztypes.MsgExec); ok {
			inner, err := exec.GetMessages()
			if err != nil {
				return errorsmod.Wrap(err, "cannot inspect the messages inside MsgExec")
			}
			if err := d.check(ctx, inner, depth+1); err != nil {
				return err
			}
			continue
		}

		unjailMsg, ok := msg.(*slashingtypes.MsgUnjail)
		if !ok {
			continue
		}

		valAddr, err := sdk.ValAddressFromBech32(unjailMsg.ValidatorAddr)
		if err != nil {
			return errorsmod.Wrap(err, "invalid validator operator address")
		}
		operator := sdk.AccAddress(valAddr).String()

		demoted, err := d.keeper.Demotion.Has(ctx, operator)
		if err != nil {
			return err
		}
		if demoted {
			return errorsmod.Wrapf(sdkerrors.ErrUnauthorized,
				"validator %s is held down by a concentration ceiling; it is restored automatically at the epoch the breach clears, and cannot unjail itself before then", operator)
		}
	}

	return nil
}
