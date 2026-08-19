package keeper_test

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/enforcement/keeper"
	module "yamale/blockchain/x/enforcement/module"
	"yamale/blockchain/x/enforcement/types"
)

// Bank is real here, and that is the point. The freeze is enforced as a bank
// send restriction, so a test that stubbed the bank would be testing that the
// module can write a record rather than that the record stops any money. The
// restriction is registered on the real keeper below, exactly as app.go does
// it, and the assertions are on balances.
//
// Staking is stubbed: everything this module asks it is a fact about the
// validator set — is this address bonded, what power does it carry, what has
// this account delegated — and standing up real staking would mean engineering
// those facts through a genesis of delegations instead of stating them.

type stubValidator struct {
	stakingtypes.ValidatorI
	operator string
	account  string
	bonded   bool
	power    int64
}

func (v stubValidator) GetOperator() string              { return v.operator }
func (v stubValidator) IsBonded() bool                   { return v.bonded }
func (v stubValidator) GetConsensusPower(math.Int) int64 { return v.power }

type stubStaking struct {
	validators  []stubValidator
	delegations map[string][]stakingtypes.Delegation
	unbonding   map[string][]stakingtypes.UnbondingDelegation
	// undelegated records what was force-unbonded, so a test can assert that a
	// seizure actually reached staked funds rather than quietly skipping them.
	undelegated map[string]math.LegacyDec
	// undelegateFails makes Undelegate refuse, as the chain does when a
	// delegator already has the maximum number of unbonding entries.
	undelegateFails bool
}

func newStubStaking() *stubStaking {
	return &stubStaking{
		delegations: map[string][]stakingtypes.Delegation{},
		unbonding:   map[string][]stakingtypes.UnbondingDelegation{},
		undelegated: map[string]math.LegacyDec{},
	}
}

func (s *stubStaking) GetLastTotalPower(context.Context) (math.Int, error) {
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

func (s *stubStaking) BondDenom(context.Context) (string, error) { return "uyml", nil }

func (s *stubStaking) GetDelegatorDelegations(_ context.Context, delegator sdk.AccAddress, maxRetrieve uint16) ([]stakingtypes.Delegation, error) {
	all := s.delegations[delegator.String()]
	if len(all) > int(maxRetrieve) {
		return all[:maxRetrieve], nil
	}
	return all, nil
}

func (s *stubStaking) Undelegate(_ context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress, shares math.LegacyDec) (time.Time, math.Int, error) {
	if s.undelegateFails {
		return time.Time{}, math.ZeroInt(), stakingtypes.ErrMaxUnbondingDelegationEntries
	}

	key := delAddr.String()
	current, ok := s.undelegated[key]
	if !ok {
		current = math.LegacyZeroDec()
	}
	s.undelegated[key] = current.Add(shares)

	// The stake leaves the delegation set and becomes an unbonding entry, as it
	// does on chain: it is not spendable, and it is not gone.
	remaining := make([]stakingtypes.Delegation, 0, len(s.delegations[key]))
	for _, d := range s.delegations[key] {
		if d.ValidatorAddress != valAddr.String() {
			remaining = append(remaining, d)
		}
	}
	s.delegations[key] = remaining
	s.unbonding[key] = append(s.unbonding[key], stakingtypes.UnbondingDelegation{
		DelegatorAddress: key,
		ValidatorAddress: valAddr.String(),
	})

	return time.Time{}, shares.TruncateInt(), nil
}

func (s *stubStaking) GetUnbondingDelegations(_ context.Context, delegator sdk.AccAddress, maxRetrieve uint16) ([]stakingtypes.UnbondingDelegation, error) {
	all := s.unbonding[delegator.String()]
	if len(all) > int(maxRetrieve) {
		return all[:maxRetrieve], nil
	}
	return all, nil
}

// delegate records that an account has staked with a validator.
func (s *stubStaking) delegate(delegator, validator string, shares int64) {
	s.delegations[delegator] = append(s.delegations[delegator], stakingtypes.Delegation{
		DelegatorAddress: delegator,
		ValidatorAddress: validator,
		Shares:           math.LegacyNewDec(shares),
	})
}

// matureUnbonding is the unbonding period ending: the entry disappears and the
// coins are back in the account.
func (s *stubStaking) matureUnbonding(delegator string) {
	delete(s.unbonding, delegator)
}

type fixture struct {
	ctx     context.Context
	keeper  keeper.Keeper
	env     *integration.Env
	ms      types.MsgServer
	qs      types.QueryServer
	staking *stubStaking

	// destination is the recovery destination — the foundation account.
	destination    sdk.AccAddress
	destinationStr string
}

func initFixture(t *testing.T) *fixture {
	t.Helper()

	env := integration.New(t, types.ModuleName, module.AppModule{})
	staking := newStubStaking()

	k := keeper.NewKeeper(
		env.StoreService,
		env.Codec,
		env.AddressCodec,
		env.Authority,
		env.AuthKeeper,
		env.BankKeeper,
		staking,
	)

	destination, destinationStr := env.Addr(t)

	params := types.DefaultParams()
	params.RecoveryDestination = destinationStr

	// Through InitGenesis rather than by setting params directly: the case
	// sequence is seeded there, and a fixture that skipped it would number the
	// first case zero — which is exactly the state the module is written to
	// make impossible.
	genesis := types.DefaultGenesis()
	genesis.Params = params
	if err := k.InitGenesis(env.Ctx, *genesis); err != nil {
		t.Fatalf("failed to init genesis: %v", err)
	}

	// Registered exactly as app.go registers it. Without this the tests would
	// prove that a Freeze record exists, not that it stops a transfer.
	env.BankKeeper.AppendSendRestriction(k.SendRestriction)

	env.Ctx = env.Ctx.WithBlockTime(time.Unix(1_700_000_000, 0)).WithBlockHeight(1)

	return &fixture{
		ctx:            env.Ctx,
		keeper:         k,
		env:            env,
		ms:             keeper.NewMsgServerImpl(k),
		qs:             keeper.NewQueryServerImpl(k),
		staking:        staking,
		destination:    destination,
		destinationStr: destinationStr,
	}
}

// atHeight moves the chain forward.
func (f *fixture) atHeight(height int64) {
	f.env.Ctx = f.env.Ctx.WithBlockHeight(height)
	f.ctx = f.env.Ctx
}

// addValidator registers a bonded validator and returns the account address it
// signs with — which is what the messages take, and what a validator actually
// holds a key for.
func (f *fixture) addValidator(t *testing.T, power int64) string {
	t.Helper()

	addr, account := f.env.Addr(t)
	f.staking.validators = append(f.staking.validators, stubValidator{
		operator: sdk.ValAddress(addr).String(),
		account:  account,
		bonded:   true,
		power:    power,
	})
	return account
}

func coins(amount int64) sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(amount)))
}
