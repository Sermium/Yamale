package simulation

import (
	"math/rand"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/stablecoin/keeper"
	"yamale/blockchain/x/stablecoin/types"
)

// maxSimulatedMint bounds a single simulated issuance so total supply stays in
// a range the AMM's integer math can work with comfortably.
const maxSimulatedMint = 1_000_000_000

// SimulateMsgMintCoin issues new supply of a governance-approved currency to a
// random recipient. Only the denom's approved issuer can sign it, so the
// operation looks up an approved issuer that is also a simulation account.
func SimulateMsgMintCoin(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		recipient, _ := simtypes.RandomAcc(r, accs)

		msg := &types.MsgMintCoin{}

		denom, issuer, found, err := randomApprovedIssuer(ctx, k, r, accs)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read approved issuers"), nil, err
		}
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "no approved issuer is a simulation account"), nil, nil
		}

		amount, err := simtypes.RandPositiveInt(r, math.NewInt(maxSimulatedMint))
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to generate a mint amount"), nil, nil
		}

		msg.Issuer = issuer.Address.String()
		msg.Denom = denom
		msg.Amount = amount.String()
		msg.Recipient = recipient.Address.String()

		txCtx := simulation.OperationInput{
			R:             r,
			App:           app,
			TxGen:         txGen,
			Cdc:           nil,
			Msg:           msg,
			Context:       ctx,
			SimAccount:    issuer,
			AccountKeeper: ak,
			Bankkeeper:    bk,
			ModuleName:    types.ModuleName,
		}

		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}

// randomApprovedIssuer picks an approved currency whose issuer is one of the
// simulation accounts, so the resulting message can actually be signed.
func randomApprovedIssuer(
	ctx sdk.Context,
	k keeper.Keeper,
	r *rand.Rand,
	accs []simtypes.Account,
) (denom string, issuer simtypes.Account, found bool, err error) {
	iter, err := k.ApprovedIssuer.Iterate(ctx, new(collections.Range[string]))
	if err != nil {
		return "", issuer, false, err
	}
	defer iter.Close()

	approved, err := iter.Values()
	if err != nil {
		return "", issuer, false, err
	}
	if len(approved) == 0 {
		return "", issuer, false, nil
	}

	offset := r.Intn(len(approved))
	for i := range approved {
		candidate := approved[(offset+i)%len(approved)]

		addr, addrErr := sdk.AccAddressFromBech32(candidate.Issuer)
		if addrErr != nil {
			continue
		}
		if acc, ok := simtypes.FindAccount(accs, addr); ok {
			return candidate.Denom, acc, true, nil
		}
	}
	return "", issuer, false, nil
}
