package simulation

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	"yamale/blockchain/x/stablecoin/keeper"
	"yamale/blockchain/x/stablecoin/types"
)

// Currency describes a mock fiat peg available to simulated issuers.
type Currency struct {
	Denom   string
	Display string
	Name    string
	Symbol  string
}

// SimulatableCurrencies are the mock fiat pegs a simulated issuer may apply
// for. Keeping the set small and fixed means denoms recur across runs, so the
// AMM has real pairs to build pools from once issuers start minting.
var SimulatableCurrencies = []Currency{
	{Denom: "uusd", Display: "usd", Name: "US Dollar", Symbol: "USD"},
	{Denom: "uchf", Display: "chf", Name: "Swiss Franc", Symbol: "CHF"},
	{Denom: "ueur", Display: "eur", Name: "Euro", Symbol: "EUR"},
	{Denom: "ugbp", Display: "gbp", Name: "Pound Sterling", Symbol: "GBP"},
	{Denom: "ujpy", Display: "jpy", Name: "Japanese Yen", Symbol: "JPY"},
}

// SimulateMsgRegisterCurrency applies to issue one of the mock currencies that
// nobody has claimed yet.
func SimulateMsgRegisterCurrency(
	ak types.AuthKeeper,
	bk types.BankKeeper,
	k keeper.Keeper,
	txGen client.TxConfig,
) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accs []simtypes.Account, chainID string,
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accs)

		msg := &types.MsgRegisterCurrency{Creator: simAccount.Address.String()}

		currency, found, err := unclaimedCurrency(ctx, k, r)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "unable to read currency state"), nil, err
		}
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), "every simulatable currency is already claimed"), nil, nil
		}

		msg.Denom = currency.Denom
		msg.DisplayDenom = currency.Display
		msg.Exponent = 6
		msg.Name = currency.Name
		msg.Symbol = currency.Symbol
		msg.Description = fmt.Sprintf("Simulated %s stablecoin", strings.ToLower(currency.Name))

		txCtx := simulation.OperationInput{
			R:             r,
			App:           app,
			TxGen:         txGen,
			Cdc:           nil,
			Msg:           msg,
			Context:       ctx,
			SimAccount:    simAccount,
			AccountKeeper: ak,
			Bankkeeper:    bk,
			ModuleName:    types.ModuleName,
		}

		return simulation.GenAndDeliverTxWithRandFees(txCtx)
	}
}

// unclaimedCurrency returns a currency with neither a pending application nor
// an approved issuer, starting from a random offset so the choice is not
// biased toward the first entry.
func unclaimedCurrency(ctx sdk.Context, k keeper.Keeper, r *rand.Rand) (Currency, bool, error) {
	var currency Currency

	offset := r.Intn(len(SimulatableCurrencies))
	for i := range SimulatableCurrencies {
		candidate := SimulatableCurrencies[(offset+i)%len(SimulatableCurrencies)]

		pending, err := k.IssuerApplication.Has(ctx, candidate.Denom)
		if err != nil {
			return currency, false, err
		}
		approved, err := k.ApprovedIssuer.Has(ctx, candidate.Denom)
		if err != nil {
			return currency, false, err
		}
		if !pending && !approved {
			return candidate, true, nil
		}
	}
	return currency, false, nil
}
