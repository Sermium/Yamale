package ante

import (
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"yamale/blockchain/x/validatorgov/keeper"
)

// ValidatorGateDecorator rejects MsgCreateValidator unless the validator
// operator address has been approved through governance via x/validatorgov
// (see MsgApplyValidator / MsgApproveValidator). Standard staking, slashing,
// and bonding behavior is unaffected once a candidate is approved.
type ValidatorGateDecorator struct {
	keeper keeper.Keeper
}

func NewValidatorGateDecorator(k keeper.Keeper) ValidatorGateDecorator {
	return ValidatorGateDecorator{keeper: k}
}

func (d ValidatorGateDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	// Genesis validators are onboarded through the trusted gentx collection
	// ceremony, not through governance, so the gate does not apply at height 0.
	if ctx.BlockHeight() == 0 {
		return next(ctx, tx, simulate)
	}

	if err := d.check(ctx, tx.GetMsgs(), 0); err != nil {
		return ctx, err
	}

	return next(ctx, tx, simulate)
}

// maxNesting bounds how deep the gate will unwrap.
//
// The limit is a denial-of-service guard, not a security one: a transaction can
// nest MsgExec arbitrarily, and following it without a bound would let an
// attacker make the ante handler do unbounded work for one fee. Anything deeper
// than this is refused outright rather than passed through, so the bound can
// never become a way around the check.
const maxNesting = 6

// check walks a transaction's messages, descending into the ones that carry
// others.
//
// Descending is the whole point. x/authz's MsgExec dispatches the messages
// inside it through the message router *after* the ante chain has run, so a
// gate that inspected only the top level never saw a MsgCreateValidator wrapped
// in one — and anyone could grant themselves that authorisation, since a grant
// to yourself is an ordinary unprivileged transaction. The permissioned
// validator set was joinable without a vote.
func (d ValidatorGateDecorator) check(ctx sdk.Context, msgs []sdk.Msg, depth int) error {
	if depth > maxNesting {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
			"transaction nests messages more than %d deep", maxNesting)
	}

	for _, msg := range msgs {
		// Anything carrying other messages is followed, not skipped.
		if exec, ok := msg.(*authztypes.MsgExec); ok {
			inner, err := exec.GetMessages()
			if err != nil {
				// An MsgExec whose contents cannot be decoded cannot be shown to
				// be safe, so it is refused rather than waved through.
				return errorsmod.Wrap(err, "cannot inspect the messages inside MsgExec")
			}
			if err := d.check(ctx, inner, depth+1); err != nil {
				return err
			}
			continue
		}

		createValMsg, ok := msg.(*stakingtypes.MsgCreateValidator)
		if !ok {
			continue
		}

		valAddr, err := sdk.ValAddressFromBech32(createValMsg.ValidatorAddress)
		if err != nil {
			return errorsmod.Wrap(err, "invalid validator operator address")
		}
		candidate := sdk.AccAddress(valAddr).String()

		approved, err := d.keeper.ApprovedValidator.Has(ctx, candidate)
		if err != nil {
			return err
		}
		if !approved {
			return errorsmod.Wrapf(sdkerrors.ErrUnauthorized,
				"validator candidate %s is not approved by governance; submit a MsgApplyValidator and await a governance vote", candidate)
		}
	}

	return nil
}
