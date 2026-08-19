package keeper_test

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/oracle/keeper"
	module "yamale/blockchain/x/oracle/module"
	"yamale/blockchain/x/oracle/types"
)

// The staking and NFT keepers are stubbed rather than wired for real.
//
// Everything this module asks staking is a fact about the validator set — is
// this address bonded, how much power does it carry — and a stub states those
// facts directly. Standing up real staking would mean bank, distribution and a
// genesis of delegations, all to produce answers the test would then have to
// engineer anyway; the paths under test here are the oracle's, and this keeps
// a failure pointing at them.

type stubValidator struct {
	stakingtypes.ValidatorI
	operator string
	bonded   bool
	power    int64
}

func (v stubValidator) GetOperator() string { return v.operator }
func (v stubValidator) IsBonded() bool      { return v.bonded }
func (v stubValidator) GetConsensusPower(_ math.Int) int64 {
	return v.power
}

type stubStaking struct {
	validators []stubValidator
}

func (s *stubStaking) GetLastTotalPower(_ context.Context) (math.Int, error) {
	total := int64(0)
	for _, v := range s.validators {
		if v.bonded {
			total += v.power
		}
	}
	return math.NewInt(total), nil
}

func (s *stubStaking) Validator(_ context.Context, addr sdk.ValAddress) (stakingtypes.ValidatorI, error) {
	for _, v := range s.validators {
		if v.operator == addr.String() {
			return v, nil
		}
	}
	return nil, stakingtypes.ErrNoValidatorFound
}

func (s *stubStaking) IterateBondedValidatorsByPower(_ context.Context, fn func(int64, stakingtypes.ValidatorI) bool) error {
	index := int64(0)
	for _, v := range s.validators {
		if !v.bonded {
			continue
		}
		if fn(index, v) {
			return nil
		}
		index++
	}
	return nil
}

// unbond removes a validator's stake, as jailing or unbonding would.
func (s *stubStaking) unbond(operator string) {
	for i := range s.validators {
		if s.validators[i].operator == operator {
			s.validators[i].bonded = false
		}
	}
}

type stubNFT struct {
	classes map[string]bool
	tokens  map[string]bool
}

func newStubNFT() *stubNFT {
	return &stubNFT{classes: map[string]bool{}, tokens: map[string]bool{}}
}

func (n *stubNFT) HasClass(_ context.Context, classID string) bool { return n.classes[classID] }
func (n *stubNFT) HasNFT(_ context.Context, classID, nftID string) bool {
	return n.tokens[classID+"/"+nftID]
}
func (n *stubNFT) GetOwner(_ context.Context, _, _ string) sdk.AccAddress { return nil }

func (n *stubNFT) mint(classID, nftID string) {
	n.classes[classID] = true
	n.tokens[classID+"/"+nftID] = true
}

// testNow is 2023-11-14T22:13:20Z, an arbitrary but realistic block time.
const testNow int64 = 1_700_000_000

type fixture struct {
	ctx     context.Context
	keeper  keeper.Keeper
	env     *integration.Env
	ms      types.MsgServer
	qs      types.QueryServer
	staking *stubStaking
	nft     *stubNFT
}

func initFixture(t *testing.T) *fixture {
	t.Helper()

	env := integration.New(t, types.ModuleName, module.AppModule{})
	staking := &stubStaking{}
	nft := newStubNFT()

	k := keeper.NewKeeper(
		env.StoreService,
		env.Codec,
		env.AddressCodec,
		env.Authority,
		staking,
		nft,
	)

	if err := k.Params.Set(env.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("failed to set params: %v", err)
	}

	// A definite clock: every staleness decision in the module is a function of
	// it, so a test that did not pin it would be testing the wall clock. It is
	// set well past the epoch because the appraisal window is a hundred days
	// wide, and a test near zero cannot express a date that far in the past.
	env.Ctx = env.Ctx.WithBlockTime(time.Unix(testNow, 0)).WithBlockHeight(1)

	return &fixture{
		ctx:     env.Ctx,
		keeper:  k,
		env:     env,
		ms:      keeper.NewMsgServerImpl(k),
		qs:      keeper.NewQueryServerImpl(k),
		staking: staking,
		nft:     nft,
	}
}

// at moves the chain's clock and height.
func (f *fixture) at(unix int64, height int64) {
	f.env.Ctx = f.env.Ctx.WithBlockTime(time.Unix(unix, 0)).WithBlockHeight(height)
	f.ctx = f.env.Ctx
}

// addValidator registers a bonded validator and returns its operator address
// and the account address that votes for it by default.
func (f *fixture) addValidator(t *testing.T, power int64) (operator string, feeder string) {
	t.Helper()

	addr, accStr := f.env.Addr(t)
	operator = sdk.ValAddress(addr).String()
	f.staking.validators = append(f.staking.validators, stubValidator{
		operator: operator,
		bonded:   true,
		power:    power,
	})
	return operator, accStr
}

// vote submits one denom's rate for a validator.
func (f *fixture) vote(t *testing.T, feeder, operator, denom, rate string) {
	t.Helper()
	_, err := f.ms.SubmitExchangeRates(f.ctx, &types.MsgSubmitExchangeRates{
		Feeder:    feeder,
		Validator: operator,
		Rates:     []types.RateVote{{Denom: denom, Rate: rate}},
	})
	require.NoError(t, err)
}

// tally runs the EndBlocker at a height that closes a voting round.
func (f *fixture) tally(t *testing.T) {
	t.Helper()

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)

	height := f.env.Ctx.BlockHeight()
	if uint64(height)%params.VotePeriod != 0 {
		height += int64(params.VotePeriod - uint64(height)%params.VotePeriod)
	}
	f.at(f.env.Ctx.BlockTime().Unix(), height)

	require.NoError(t, f.keeper.EndBlocker(f.ctx))
}

// rate reads the agreed rate for a denom.
func (f *fixture) rate(t *testing.T, denom string) types.ExchangeRate {
	t.Helper()
	r, err := f.keeper.ExchangeRate.Get(f.ctx, denom)
	require.NoError(t, err)
	return r
}
