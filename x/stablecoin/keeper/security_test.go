package keeper_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/stablecoin/keeper"
	"yamale/blockchain/x/stablecoin/types"
)

// Findings from the pre-genesis review of this module.

// Registering a currency is permissionless, and the denom becomes a store key.
// It was never validated, so an application could be filed for a denom the coin
// type cannot represent — and if governance ever approved one, minting reached
// sdk.NewCoin and panicked. The panic is recovered into a failed transaction
// rather than halting the chain, but the currency would be permanently
// unusable and the reason would be invisible.
func TestRegisteredDenomsAreValidated(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuerStr := f.env.Addr(t)

	for _, denom := range []string{"", "1nvalid", "a", strings.Repeat("u", 200)} {
		_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
			Creator: issuerStr, Denom: denom, DisplayDenom: "X", Exponent: 6,
			Name: "X", Symbol: "X", Description: "x",
		})
		require.Error(t, err, "denom %q must be refused at registration", denom)
	}

	_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: issuerStr, Denom: "ueur", DisplayDenom: "EUR", Exponent: 6,
		Name: "Euro", Symbol: "EUR", Description: "Euro",
	})
	require.NoError(t, err)
}

// The rest of the application is attacker-chosen text kept forever, in a store
// anybody can add to for the price of one transaction.
func TestCurrencyTextIsBounded(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuerStr := f.env.Addr(t)
	huge := strings.Repeat("a", 100_000)

	register := func(display, name, symbol, description string) error {
		_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
			Creator: issuerStr, Denom: "ueur", DisplayDenom: display, Exponent: 6,
			Name: name, Symbol: symbol, Description: description,
		})
		return err
	}

	require.Error(t, register(huge, "Euro", "EUR", "Euro"))
	require.Error(t, register("EUR", huge, "EUR", "Euro"))
	require.Error(t, register("EUR", "Euro", huge, "Euro"))
	require.Error(t, register("EUR", "Euro", "EUR", huge))

	require.NoError(t, register("EUR", "Euro", "EUR", "Euro issued by Alpine Bank"))
}

// An exponent is used to scale every displayed amount. An absurd one makes the
// currency unrenderable, and it is fixed at registration with no way to change
// it afterwards.
func TestExponentIsBounded(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	_, issuerStr := f.env.Addr(t)

	_, err := ms.RegisterCurrency(f.ctx, &types.MsgRegisterCurrency{
		Creator: issuerStr, Denom: "ueur", DisplayDenom: "EUR", Exponent: 100,
		Name: "Euro", Symbol: "EUR", Description: "Euro",
	})
	require.Error(t, err, "an exponent of 100 cannot describe a real currency")
}
