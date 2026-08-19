package ante_test

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"

	"yamale/blockchain/x/stablecoin/ante"
	"yamale/blockchain/x/stablecoin/types"
)

// approvedDenoms answers from a fixed set, so the test is about the decorator's
// decision rather than about collections.
type approvedDenoms struct {
	denoms map[string]bool
	err    error
}

func (a approvedDenoms) IsApprovedDenom(_ context.Context, denom string) (bool, error) {
	if a.err != nil {
		return false, a.err
	}
	return a.denoms[denom], nil
}

// feeTx is the minimum sdk.FeeTx the decorator reads.
type feeTx struct {
	fee sdk.Coins
}

func (t feeTx) GetMsgs() []sdk.Msg                    { return nil }
func (t feeTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }
func (t feeTx) GetGas() uint64                        { return 200_000 }
func (t feeTx) GetFee() sdk.Coins                     { return t.fee }
func (t feeTx) FeePayer() []byte                      { return nil }
func (t feeTx) FeeGranter() []byte                    { return nil }

func run(t *testing.T, k ante.StablecoinKeeper, fee sdk.Coins) error {
	t.Helper()
	d := ante.NewApprovedFeeDenomDecorator(k)
	_, err := d.AnteHandle(sdk.Context{}, feeTx{fee: fee}, false, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		return ctx, nil
	})
	return err
}

func TestAFeeInAnIssuedCurrencyIsAccepted(t *testing.T) {
	k := approvedDenoms{denoms: map[string]bool{"uzar": true}}
	require.NoError(t, run(t, k, sdk.NewCoins(sdk.NewCoin("uzar", math.NewInt(500)))))
}

// The failure this prevents: a sender paying the network in a denomination they
// minted themselves consumes block space at a real cost of nothing.
func TestAFeeInAnUnissuedDenomIsRefused(t *testing.T) {
	k := approvedDenoms{denoms: map[string]bool{"uzar": true}}
	err := run(t, k, sdk.NewCoins(sdk.NewCoin("uscam", math.NewInt(500))))
	require.ErrorIs(t, err, types.ErrFeeDenomNotIssued)
	require.Contains(t, err.Error(), "uscam", "the sender has to be told which denomination")
}

// One good denomination does not carry a bad one alongside it.
func TestEveryFeeCoinMustBeIssued(t *testing.T) {
	k := approvedDenoms{denoms: map[string]bool{"uzar": true}}
	err := run(t, k, sdk.NewCoins(
		sdk.NewCoin("uzar", math.NewInt(500)),
		sdk.NewCoin("uscam", math.NewInt(500)),
	))
	require.ErrorIs(t, err, types.ErrFeeDenomNotIssued)
}

// A zero fee is the minimum-gas-price check's business, not this one's, and a
// genesis transaction carries none at all.
func TestNoFeeIsNotAnUnapprovedFee(t *testing.T) {
	k := approvedDenoms{denoms: map[string]bool{}}
	require.NoError(t, run(t, k, sdk.NewCoins()))
}
