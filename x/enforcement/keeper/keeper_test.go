package keeper_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	aliasmodule "yamale/blockchain/x/alias/module"
	aliastestutil "yamale/blockchain/x/alias/testutil"
	aliastypes "yamale/blockchain/x/alias/types"
	constitutiontestutil "yamale/blockchain/x/constitution/testutil"
	constitutiontypes "yamale/blockchain/x/constitution/types"
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

// TokensFromShares is one-to-one here. On a real chain it is not — a slashed
// validator returns fewer tokens per share — and the assessment that sizes a
// seizure's delay goes through this conversion for exactly that reason. The
// stub keeps the ratio at one so a test that stakes 800,000 can assert on
// 800,000 rather than on an arithmetic detail of the staking module.
func (v stubValidator) TokensFromShares(shares math.LegacyDec) math.LegacyDec { return shares }

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

// country is where every account in these tests is recorded, and the perimeter
// every validator here is granted.
//
// It has to be stated, because a target the chain cannot place is refused before
// any grant is consulted — which is the point of the perimeter and is also why
// these fixtures place every address they hand out. A test that forgot would fail
// with "no recorded jurisdiction", which is the correct refusal and not the one
// it meant to exercise. The cross-perimeter refusals are in perimeter_test.go,
// where a second country is introduced deliberately.
const country = "KE"

type fixture struct {
	ctx     context.Context
	keeper  keeper.Keeper
	env     *integration.Env
	ms      types.MsgServer
	qs      types.QueryServer
	staking *stubStaking

	// perimeter is a real x/alias keeper. The rule being enforced is that
	// module's — who may stop whose money — and a stub would be this test writing
	// down the answer it wanted.
	perimeter *aliastestutil.Perimeter

	// destination is the recovery destination — the foundation account.
	destination    sdk.AccAddress
	destinationStr string
}

func initFixture(t *testing.T) *fixture {
	t.Helper()

	env := integration.NewWith(t,
		[]string{types.ModuleName, constitutiontypes.ModuleName, aliastypes.ModuleName},
		module.AppModule{}, aliasmodule.AppModule{})
	staking := newStubStaking()
	perimeter := aliastestutil.Init(t, env)

	// Fixed rather than random. The recovery destination is constitutional now,
	// so two fixtures with different destinations are two different chains — and
	// a genesis exported from one would be refused by the other, which is
	// exactly what the round-trip tests do.
	destinationStr := testRecoveryDestination
	destination, err := env.AddressCodec.StringToBytes(destinationStr)
	require.NoError(t, err)

	// The constitution is real. What is being tested is that this module cannot
	// move the four parameters held there, and a stubbed settlement would test
	// the assertion rather than the arrangement.
	constitution := constitutiontestutil.Init(t, env, staking, constitutiontestutil.Invariants(destinationStr))

	k := keeper.NewKeeper(
		env.StoreService,
		env.Codec,
		env.AddressCodec,
		env.Authority,
		env.AuthKeeper,
		env.BankKeeper,
		staking,
		constitution,
		perimeter.Keeper,
	)

	// Permissive here on purpose: these tests are about the perimeter, the
	// delays, the quorum and the seizure, and a concentration check refusing in
	// the middle of them would be noise. The refusal itself is exercised where
	// it is the subject — see the caps tests and unwired_test.go.
	k.SetConcentrationKeeper(withinCaps{})

	params := types.DefaultParams()
	params.RecoveryDestination = destinationStr

	// The delay schedule and the rolling cap have no defaults — both are
	// denominated, and no denomination compiled into the binary is anybody's
	// currency — so the fixture states them, exactly as a real genesis has to.
	//
	// The numbers are scaled down from a deployment's so that a test can run a
	// case to execution in a few blocks rather than a few days. The shape is
	// the same: a floor everything waits, and tiers that make a large seizure
	// wait longer than a small one.
	params.SeizureDelayBlocks = 10
	params.SeizureDelayTiers = []types.SeizureDelayTier{
		{Threshold: sdk.NewCoin("uyml", math.NewInt(1_000_000)), DelayBlocks: 100},
		{Threshold: sdk.NewCoin("uyml", math.NewInt(10_000_000)), DelayBlocks: 1_000},
	}
	params.SeizureWindowBlocks = 5_000
	params.SeizureWindowCap = sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(100_000_000)))
	params.MaxSeizuresPerWindow = 5

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
		perimeter:      perimeter,
		destination:    destination,
		destinationStr: destinationStr,
	}
}

// addr returns a fresh account already recorded in the fixture's country.
//
// Every account these tests hand out goes through here rather than through
// env.Addr, because an account the chain cannot place cannot be acted on at all
// — correctly, and for a reason unrelated to whatever the test is checking.
func (f *fixture) addr(t *testing.T) (sdk.AccAddress, string) {
	t.Helper()
	addr, s := f.env.Addr(t)
	f.place(t, s)
	return addr, s
}

// fundedAddr returns a fresh placed account already holding coins.
func (f *fixture) fundedAddr(t *testing.T, amount sdk.Coins) (sdk.AccAddress, string) {
	t.Helper()
	addr, s := f.addr(t)
	f.env.Fund(t, addr, amount)
	return addr, s
}

// place records an existing account in the fixture's country.
//
// Needed on its own for the genesis tests, which carry addresses from one chain
// into a second fixture whose perimeter registry starts empty — the same
// situation as an account that existed before the perimeter did.
func (f *fixture) place(t *testing.T, addr string) {
	t.Helper()
	f.perimeter.Place(t, addr, country)
}

// grantEnforcement grants the enforcement role over the fixture's country.
func (f *fixture) grantEnforcement(t *testing.T, addr string) {
	t.Helper()
	f.perimeter.Grant(t, addr, aliastypes.ROLE_ENFORCEMENT_AUTHORITY, country)
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
	// Bonded *and* granted. Being trusted to secure the chain is a different
	// question from being permitted to stop a particular country's accounts, and
	// this module used to answer only the first. Every fixture validator is
	// granted the fixture's country here so the tests below exercise the rules
	// they are about; the refusal for a validator granted somewhere else is in
	// perimeter_test.go.
	f.grantEnforcement(t, account)
	return account
}

func coins(amount int64) sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin("uyml", math.NewInt(amount)))
}

// endBlock runs the module's end blocker at the current height.
func (f *fixture) endBlock(t *testing.T) {
	t.Helper()
	require.NoError(t, f.keeper.EndBlocker(f.ctx))
}

// runTo moves the chain to a height and runs the end blocker there.
//
// It does not run every intervening block, which is deliberate and is also what
// a real chain does not do: the queues are keyed by the height work falls due
// at and every walk is "at or before this height", so a module that only
// resolved things when the exact block was executed would strand any case whose
// height was skipped. Jumping here is what proves that.
func (f *fixture) runTo(t *testing.T, height int64) {
	t.Helper()
	f.atHeight(height)
	f.endBlock(t)
}

// setParams writes a modified parameter set, through the same validation a
// governance proposal would go through.
func (f *fixture) setParams(t *testing.T, mutate func(*types.Params)) types.Params {
	t.Helper()
	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	mutate(&params)
	require.NoError(t, params.Validate(), "the test asked for parameters the chain would refuse")
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	return params
}

// appointOmbudsman names an ombudsman and returns the address it signs with.
func (f *fixture) appointOmbudsman(t *testing.T) string {
	t.Helper()
	_, ombudsman := f.addr(t)
	f.setParams(t, func(p *types.Params) { p.Ombudsman = ombudsman })
	return ombudsman
}

// instrument is a valid legal instrument, dated before the fixture's block
// time. Every seizure needs one, so almost every test in this package builds
// one, and building it wrong is not what any of them are trying to test.
func instrument() types.LegalInstrument {
	return types.LegalInstrument{
		IssuingAuthority: "High Court of Kenya at Nairobi",
		Reference:        "HCCC/2026/0412",
		Kind:             types.LEGAL_INSTRUMENT_KIND_COURT_ORDER,
		Hash:             strings.Repeat("a1b2", 16),
		IssuedAt:         1_699_000_000,
	}
}
