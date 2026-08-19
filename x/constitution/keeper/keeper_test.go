package keeper_test

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/constitution/keeper"
	module "yamale/blockchain/x/constitution/module"
	"yamale/blockchain/x/constitution/types"
)

// testRecoveryDestination is a valid address that is nobody's account, so a
// test that accidentally depended on it holding funds would fail rather than
// pass quietly. Fixed rather than generated: it is constitutional, and two
// fixtures with different destinations would be two different chains.
var testRecoveryDestination = sdk.AccAddress([]byte("foundation-test-addr")).String()

// stubStaking is an in-memory validator set. Ratification is weighed by
// consensus power, and arranging four validators of one seat each is three
// lines here and a bonding ceremony against a real x/staking.
type stubStaking struct {
	byOperator map[string]*stakingtypes.Validator
}

func newStubStaking() *stubStaking {
	return &stubStaking{byOperator: map[string]*stakingtypes.Validator{}}
}

func (s *stubStaking) add(operator sdk.AccAddress, seats int64) string {
	valAddr := sdk.ValAddress(operator)
	validator, err := stakingtypes.NewValidator(valAddr.String(), ed25519.GenPrivKey().PubKey(), stakingtypes.Description{})
	if err != nil {
		panic(err)
	}
	validator.Status = stakingtypes.Bonded
	validator.Tokens = sdk.DefaultPowerReduction.MulRaw(seats)
	validator.DelegatorShares = math.LegacyNewDecFromInt(validator.Tokens)
	s.byOperator[valAddr.String()] = &validator
	return valAddr.String()
}

func (s *stubStaking) unbond(operator sdk.AccAddress) {
	s.byOperator[sdk.ValAddress(operator).String()].Status = stakingtypes.Unbonding
}

func (s *stubStaking) GetLastTotalPower(context.Context) (math.Int, error) {
	total := int64(0)
	for _, validator := range s.byOperator {
		if validator.Status == stakingtypes.Bonded {
			total += validator.PotentialConsensusPower(sdk.DefaultPowerReduction)
		}
	}
	return math.NewInt(total), nil
}

func (s *stubStaking) Validator(_ context.Context, addr sdk.ValAddress) (stakingtypes.ValidatorI, error) {
	validator, ok := s.byOperator[addr.String()]
	if !ok {
		return nil, stakingtypes.ErrNoValidatorFound
	}
	return *validator, nil
}

type fixture struct {
	env     *integration.Env
	keeper  keeper.Keeper
	ms      types.MsgServer
	qs      types.QueryServer
	staking *stubStaking
}

func initFixture(t *testing.T) *fixture {
	t.Helper()

	env := integration.New(t, types.ModuleName, module.AppModule{})
	staking := newStubStaking()

	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, staking)

	genesis := types.DefaultGenesis()
	genesis.Invariants.EnforcementRecoveryDestination = testRecoveryDestination
	require.NoError(t, k.InitGenesis(env.Ctx, *genesis))

	env.Ctx = env.Ctx.WithBlockHeight(1)

	return &fixture{
		env:     env,
		keeper:  k,
		ms:      keeper.NewMsgServerImpl(k),
		qs:      keeper.NewQueryServerImpl(k),
		staking: staking,
	}
}

// addValidator gives the set one more member holding seats seats, and returns
// the account it signs with.
func (f *fixture) addValidator(t *testing.T, seats int64) string {
	t.Helper()
	account, accountStr := f.env.Addr(t)
	f.staking.add(account, seats)
	return accountStr
}

func (f *fixture) at(height int64) sdk.Context {
	return f.env.Ctx.WithBlockHeight(height)
}
