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

	"yamale/blockchain/x/paymsg/keeper"
	"yamale/blockchain/x/paymsg/types"
)

// SimulateMsgSendPayment settles a random credit transfer between two accounts
// through a pair of governance-approved participants. It can only run once at
// least two participants have been approved, which happens through the gov
// proposal simulator (see the module's ProposalMsgs).
func SimulateMsgSendPayment(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		creditorAccount, _ := simtypes.RandomAcc(r, accs)
		msg := &types.MsgSendPayment{}

		participants, err := approvedParticipants(ctx, k)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read approved participants"), nil, err
		}
		if len(participants) < 2 {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "fewer than two approved participants"), nil, nil
		}

		// Pick two distinct participants for the two sides of the instruction.
		i := r.Intn(len(participants))
		j := r.Intn(len(participants) - 1)
		if j >= i {
			j++
		}

		// The debtor has to be somebody the instructing participant actually
		// acts for, so the debtor is chosen from that participant's customers
		// rather than from the accounts at large. Picking at random produced
		// instructions the chain now rejects, which the simulator treats as a
		// fatal undeliverable transaction rather than a no-op — it caught this
		// the moment the rule was added.
		debtorAccount, found := randomCustomerOf(ctx, k, r, accs, participants[i])
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg),
				"the instructing participant has no customer the simulation can sign for"), nil, nil
		}
		msg.Debtor = debtorAccount.Address.String()

		spendable := bk.SpendableCoins(ctx, debtorAccount.Address)
		amount := spendable.AmountOf(sdk.DefaultBondDenom)
		if !amount.IsPositive() {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "debtor holds no transferable balance"), nil, nil
		}

		// Send at most half the balance so the account can still pay fees.
		amount, err = simtypes.RandPositiveInt(r, amount.Quo(math.NewInt(2)))
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to generate a payment amount"), nil, nil
		}
		sent := sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, amount))

		msg.EndToEndId = simtypes.RandStringOfLength(r, 24)
		msg.InstructingParticipant = participants[i]
		msg.InstructedParticipant = participants[j]
		msg.Creditor = creditorAccount.Address.String()
		msg.Denom = sdk.DefaultBondDenom
		msg.Amount = amount.String()
		msg.PurposeCode = randomPurposeCode(r)
		msg.RemittanceInformation = simtypes.RandStringOfLength(r, 20)

		txCtx := simulation.OperationInput{
			R:               r,
			App:             app,
			TxGen:           txGen,
			Cdc:             nil,
			Msg:             msg,
			CoinsSpentInMsg: sent,
			Context:         ctx,
			SimAccount:      debtorAccount,
			AccountKeeper:   ak,
			Bankkeeper:      bk,
			ModuleName:      types.ModuleName,
		}

		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}

// randomCustomerOf picks an account the participant acts for and whose key the
// simulation holds, so the resulting instruction can actually be signed.
//
// A participant is its own instructing agent when it pays out of its own
// balance, so it counts as one of its own candidates.
func randomCustomerOf(
	ctx sdk.Context,
	k keeper.Keeper,
	r *rand.Rand,
	accs []simtypes.Account,
	participant string,
) (simtypes.Account, bool) {
	candidates := make([]string, 0, 4)
	candidates = append(candidates, participant)

	if err := k.Customer.Walk(ctx, nil, func(_ string, customer types.Customer) (bool, error) {
		if customer.Participant == participant {
			candidates = append(candidates, customer.Customer)
		}
		return false, nil
	}); err != nil {
		return simtypes.Account{}, false
	}

	offset := r.Intn(len(candidates))
	for n := range candidates {
		addr, err := sdk.AccAddressFromBech32(candidates[(offset+n)%len(candidates)])
		if err != nil {
			continue
		}
		if account, ok := simtypes.FindAccount(accs, addr); ok {
			return account, true
		}
	}
	return simtypes.Account{}, false
}

// approvedParticipants returns every governance-approved participant address.
func approvedParticipants(ctx sdk.Context, k keeper.Keeper) ([]string, error) {
	iter, err := k.ApprovedParticipant.Iterate(ctx, new(collections.Range[string]))
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	return iter.Keys()
}

// randomPurposeCode returns an ISO 20022 external purpose code.
func randomPurposeCode(r *rand.Rand) string {
	codes := []string{"SALA", "SUPP", "TRAD", "INTC", "TAXS", "PENS"}
	return codes[r.Intn(len(codes))]
}
