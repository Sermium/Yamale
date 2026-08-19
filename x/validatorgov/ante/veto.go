package ante

import (
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/signing"

	"yamale/blockchain/x/validatorgov/keeper"
)

// OperatorVetoDecorator cancels a pending operator recovery when the operator
// it names signs anything at all.
//
// A recovery is a claim that somebody's key is gone. One signature by that key
// is the complete disproof of it, and it is the only disproof an attacker
// cannot produce. Making it work costs the real operator nothing they would not
// have done anyway — any transaction, of any kind, sent for any reason.
//
// This is an ante decorator rather than a message-service hook because a hook
// only ever sees the messages that reach the router, and the rule has to see
// signatures. An operator who does not know this rule exists — which is the
// ordinary case, since they lost nothing and are not watching — would never
// send the particular message a hook would require, while sending ordinary
// transactions all week. The ante chain is the only place the chain looks at
// who signed, independently of what they signed.
//
// Placement matters, and there are two ways to get it wrong:
//
//   - Before signature verification, an unsigned transaction claiming to come
//     from the operator would cancel a legitimate recovery. Anybody could keep
//     a validator unrecoverable for free. This decorator must run after
//     SigVerificationDecorator, and it is wired that way in app/ante.go.
//   - During simulation, signatures are not verified at all, so a simulated
//     transaction is not evidence of anything. Simulation is skipped.
type OperatorVetoDecorator struct {
	keeper keeper.Keeper
}

func NewOperatorVetoDecorator(k keeper.Keeper) OperatorVetoDecorator {
	return OperatorVetoDecorator{keeper: k}
}

func (d OperatorVetoDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	// A simulated transaction carries no verified signature, and CheckTx runs
	// against a state that is thrown away at the next commit — a veto recorded
	// there would show up in the mempool's view of the world and nowhere else.
	// Only a transaction being executed in a block counts as having been signed.
	if simulate || ctx.IsCheckTx() || ctx.IsReCheckTx() || ctx.BlockHeight() == 0 {
		return next(ctx, tx, simulate)
	}

	sigTx, ok := tx.(signing.SigVerifiableTx)
	if !ok {
		return ctx, errorsmod.Wrap(sdkerrors.ErrTxDecode, "invalid transaction type")
	}

	signers, err := sigTx.GetSigners()
	if err != nil {
		return ctx, err
	}

	for _, signer := range signers {
		addr, err := d.keeper.AddressCodec().BytesToString(signer)
		if err != nil {
			return ctx, err
		}
		if err := d.keeper.VetoBySignature(ctx, addr); err != nil {
			return ctx, err
		}
	}

	return next(ctx, tx, simulate)
}
