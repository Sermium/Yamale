// Package simulation drives the oracle module's randomized operations.
//
// What is worth simulating here is not one vote succeeding — the keeper tests
// cover that — but the interaction between voting and the validator set moving
// underneath it. Validators bond, unbond, get jailed and come back while rounds
// are open, and the tally has to keep agreeing a rate every node computes
// identically throughout. That is only exercised under the whole application.
package simulation

import (
	"math/rand"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"yamale/blockchain/x/oracle/keeper"
	"yamale/blockchain/x/oracle/types"
)

// SimulateMsgSubmitExchangeRates reports prices for a validator the simulation
// can actually sign for.
func SimulateMsgSubmitExchangeRates(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgSubmitExchangeRates{}

		params, err := k.Params.Get(ctx)
		if err != nil {
			return noOp(msg, "unable to read params"), nil, err
		}

		validator, feeder, found, err := randomSignableValidator(ctx, k, r, accs)
		if err != nil {
			return noOp(msg, "unable to read the validator set"), nil, err
		}
		if !found {
			return noOp(msg, "no bonded validator has a feeder the simulation holds"), nil, nil
		}

		// Rates drift around a base rather than being drawn independently, so
		// the median has something to be a median of. Fully random prices would
		// make every round's result meaningless and would never exercise the
		// case the aggregation actually cares about: near-agreement with
		// outliers.
		rates := make([]types.RateVote, 0, len(params.AcceptedDenoms))
		for _, denom := range params.AcceptedDenoms {
			rate := math.LegacyNewDecWithPrec(int64(90+r.Intn(21)), 2)
			rates = append(rates, types.RateVote{Denom: denom, Rate: rate.String()})
		}

		msg.Feeder = feeder.Address.String()
		msg.Validator = validator
		msg.Rates = rates

		return deliver(r, app, ctx, txGen, ak, bk, feeder, msg, nil)
	}
}

// SimulateMsgDelegateFeeder hands voting to a random hot key.
//
// This is the operation that makes the previous one interesting: once a feeder
// has been delegated, a validator voting with its own key must start being
// rejected, and the simulation will notice if it is not.
func SimulateMsgDelegateFeeder(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgDelegateFeeder{}

		operators, err := bondedOperators(ctx, k)
		if err != nil {
			return noOp(msg, "unable to read the validator set"), nil, err
		}
		if len(operators) == 0 {
			return noOp(msg, "no bonded validators"), nil, nil
		}

		offset := r.Intn(len(operators))
		for i := range operators {
			operator := operators[(offset+i)%len(operators)]
			valAddr, err := sdk.ValAddressFromBech32(operator)
			if err != nil {
				continue
			}
			account, ok := simtypes.FindAccount(accs, sdk.AccAddress(valAddr))
			if !ok {
				continue
			}

			feeder, _ := simtypes.RandomAcc(r, accs)
			msg.Operator = account.Address.String()
			msg.Validator = operator
			msg.Feeder = feeder.Address.String()

			return deliver(r, app, ctx, txGen, ak, bk, account, msg, nil)
		}

		return noOp(msg, "no validator's operator key is held by the simulation"), nil, nil
	}
}

// SimulateMsgApplyAppraiser asks to be admitted as a valuer.
//
// Approval is a governance decision, so this operation only ever produces
// pending applications — which is the point: it exercises the path where an
// unapproved party tries to act, and the module has to keep refusing.
func SimulateMsgApplyAppraiser(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgApplyAppraiser{
			Creator:     simAccount.Address.String(),
			Name:        simtypes.RandStringOfLength(r, 10),
			Credentials: simtypes.RandStringOfLength(r, 20),
		}

		// Re-applying while pending or approved is rejected by design, and the
		// simulator treats a rejected delivery as fatal rather than as a no-op.
		if existing, err := k.Appraiser.Get(ctx, msg.Creator); err == nil {
			if existing.Status != types.AppraiserStatus_APPRAISER_STATUS_REJECTED {
				return noOp(msg, "account already has an appraiser record"), nil, nil
			}
		}

		return deliver(r, app, ctx, txGen, ak, bk, simAccount, msg, nil)
	}
}

func deliver(
	r *rand.Rand,
	app *baseapp.BaseApp,
	ctx sdk.Context,
	txGen client.TxConfig,
	ak types.AuthKeeper,
	bk types.BankKeeper,
	signer simtypes.Account,
	msg sdk.Msg,
	spent sdk.Coins,
) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
	return simulation.GenAndDeliverTxWithRandFees(simulation.OperationInput{
		R:               r,
		App:             app,
		TxGen:           txGen,
		Cdc:             nil,
		Msg:             msg,
		CoinsSpentInMsg: spent,
		Context:         ctx,
		SimAccount:      signer,
		AccountKeeper:   ak,
		Bankkeeper:      bk,
		ModuleName:      types.ModuleName,
	})
}

func noOp(msg sdk.Msg, reason string) simtypes.OperationMsg {
	return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), reason)
}

// bondedOperators lists the operator addresses that may currently vote.
func bondedOperators(ctx sdk.Context, k keeper.Keeper) ([]string, error) {
	var operators []string
	err := k.StakingKeeper().IterateBondedValidatorsByPower(ctx, func(_ int64, validator stakingtypes.ValidatorI) bool {
		operators = append(operators, validator.GetOperator())
		return false
	})
	return operators, err
}

// randomSignableValidator finds a bonded validator whose current feeder key is
// one the simulation holds, so the vote it generates can actually be signed.
func randomSignableValidator(
	ctx sdk.Context,
	k keeper.Keeper,
	r *rand.Rand,
	accs []simtypes.Account,
) (string, simtypes.Account, bool, error) {
	operators, err := bondedOperators(ctx, k)
	if err != nil {
		return "", simtypes.Account{}, false, err
	}
	if len(operators) == 0 {
		return "", simtypes.Account{}, false, nil
	}

	offset := r.Intn(len(operators))
	for i := range operators {
		operator := operators[(offset+i)%len(operators)]

		feeder, err := k.FeederOf(ctx, operator)
		if err != nil {
			continue
		}
		addr, err := sdk.AccAddressFromBech32(feeder)
		if err != nil {
			continue
		}
		if account, ok := simtypes.FindAccount(accs, addr); ok {
			return operator, account, true, nil
		}
	}

	return "", simtypes.Account{}, false, nil
}
