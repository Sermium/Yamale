package keeper_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/netting/keeper"
	module "yamale/blockchain/x/netting/module"
	"yamale/blockchain/x/netting/types"
)

// Bank and auth are real here, and that is deliberate. This module's entire
// safety argument is that the money backing a netted position is already inside
// its module account, so a test that stubbed the bank would be asserting that
// the module can write a number rather than that the number is covered. Every
// assertion below is on real balances and real total supply.
//
// x/paymsg is stubbed, because the only thing this module asks it is whether an
// institution is currently admitted — a fact, not a behaviour. Standing up the
// real participant registry would mean engineering that fact through a
// governance approval in every test that is not about approval.

const (
	eur = "ueur"
	ngn = "ungn"
)

type stubParticipants struct {
	approved map[string]bool
	err      error
}

func newStubParticipants() *stubParticipants {
	return &stubParticipants{approved: map[string]bool{}}
}

func (s *stubParticipants) ApprovedParticipantExists(_ context.Context, participant string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.approved[participant], nil
}

type fixture struct {
	env          *integration.Env
	ctx          sdk.Context
	keeper       keeper.Keeper
	ms           types.MsgServer
	q            types.QueryServer
	participants *stubParticipants
}

func initFixture(t *testing.T) *fixture {
	t.Helper()

	env := integration.New(t, types.ModuleName, module.AppModule{})
	participants := newStubParticipants()

	k := keeper.NewKeeper(
		env.StoreService,
		env.Codec,
		env.AddressCodec,
		env.Authority,
		env.AuthKeeper,
		env.BankKeeper,
		participants,
	)

	f := &fixture{
		env:          env,
		ctx:          env.Ctx,
		keeper:       k,
		ms:           keeper.NewMsgServerImpl(k),
		q:            keeper.NewQueryServerImpl(k),
		participants: participants,
	}
	require.NoError(t, k.InitGenesis(f.ctx, *types.DefaultGenesis()))
	return f
}

// setParams installs a netting policy. cycleBlocks of zero switches netting off
// entirely, which is what the default is.
func (f *fixture) setParams(t *testing.T, cycleBlocks uint64, policies ...types.DenomPolicy) {
	t.Helper()
	params := types.Params{CycleBlocks: cycleBlocks, DenomPolicies: policies}
	require.NoError(t, params.Validate())
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
}

func policy(denom string, threshold int64) types.DenomPolicy {
	return types.DenomPolicy{Denom: denom, GrossThreshold: math.NewInt(threshold)}
}

// newParticipant returns an approved institution holding the given coins.
func (f *fixture) newParticipant(t *testing.T, coins sdk.Coins) string {
	t.Helper()
	_, addr := f.env.NewFundedAddr(t, coins)
	f.participants.approved[addr] = true
	return addr
}

func (f *fixture) postReserve(t *testing.T, participant string, coins sdk.Coins) {
	t.Helper()
	_, err := f.ms.PostReserve(f.ctx, &types.MsgPostReserve{Participant: participant, Amount: coins})
	require.NoError(t, err)
}

// batchHash is a stand-in for SHA-256 over a salted retail batch. Derived from
// a label so two obligations in one test do not accidentally share one, which
// would hide an index collision.
func batchHash(label string) []byte {
	sum := sha256.Sum256([]byte(label))
	return sum[:]
}

func (f *fixture) submit(t *testing.T, from, to, denom string, amount int64) *types.MsgSubmitObligationResponse {
	t.Helper()
	res, err := f.ms.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
		FromParticipant: from,
		ToParticipant:   to,
		Denom:           denom,
		Amount:          math.NewInt(amount),
		BatchHash:       batchHash(fmt.Sprintf("%s->%s:%s:%d", from, to, denom, amount)),
	})
	require.NoError(t, err)
	return res
}

func (f *fixture) trySubmit(from, to, denom string, amount int64, label string) error {
	_, err := f.ms.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
		FromParticipant: from,
		ToParticipant:   to,
		Denom:           denom,
		Amount:          math.NewInt(amount),
		BatchHash:       batchHash(label),
	})
	return err
}

// endBlockAt runs the end blocker as if this were block `height`, which is what
// decides whether a window closes.
func (f *fixture) endBlockAt(t *testing.T, height int64) {
	t.Helper()
	f.ctx = f.ctx.WithBlockHeight(height)
	require.NoError(t, f.keeper.EndBlocker(f.ctx))
}

func (f *fixture) reserve(t *testing.T, participant, denom string) math.Int {
	t.Helper()
	amount, err := f.keeper.GetReserve(f.ctx, participant, denom)
	require.NoError(t, err)
	return amount
}

func (f *fixture) locked(t *testing.T, participant, denom string) math.Int {
	t.Helper()
	amount, err := f.keeper.GetLocked(f.ctx, participant, denom)
	require.NoError(t, err)
	return amount
}

func (f *fixture) position(t *testing.T, cycleID uint64, denom, participant string) math.Int {
	t.Helper()
	amount, err := f.keeper.GetPosition(f.ctx, cycleID, denom, participant)
	require.NoError(t, err)
	return amount
}

func (f *fixture) currentCycle(t *testing.T) uint64 {
	t.Helper()
	id, err := f.keeper.CurrentCycle.Get(f.ctx)
	require.NoError(t, err)
	return id
}

func (f *fixture) cycle(t *testing.T, id uint64) types.Cycle {
	t.Helper()
	c, err := f.keeper.Cycle.Get(f.ctx, id)
	require.NoError(t, err)
	return c
}

func (f *fixture) outcome(t *testing.T, cycleID uint64, denom string) types.DenomOutcome {
	t.Helper()
	for _, o := range f.cycle(t, cycleID).Outcomes {
		if o.Denom == denom {
			return o
		}
	}
	t.Fatalf("cycle %d has no outcome for %s", cycleID, denom)
	return types.DenomOutcome{}
}

func coins(denom string, amount int64) sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin(denom, math.NewInt(amount)))
}

// sdkCoins builds a two-currency amount, for the tests that prove currencies
// settle independently of each other.
func sdkCoins(denomA string, amountA int64, denomB string, amountB int64) sdk.Coins {
	return sdk.NewCoins(
		sdk.NewCoin(denomA, math.NewInt(amountA)),
		sdk.NewCoin(denomB, math.NewInt(amountB)),
	)
}

// moduleHoldings is what the netting account actually holds. Every test that
// touches reserves checks it, because the invariant a supervisor can verify
// from outside is that the reserves this module records add up to exactly the
// coins in its account — no more, which would mean it is claiming custody of
// money that is not there, and no less, which would mean value is stranded.
func (f *fixture) moduleHoldings(denom string) math.Int {
	return f.env.ModuleBalance(types.ModuleName, denom)
}

// totalReserves sums every reserve the module records, in one currency.
func (f *fixture) totalReserves(t *testing.T, denom string) math.Int {
	t.Helper()
	genesis, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)
	total := math.ZeroInt()
	for _, r := range genesis.Reserves {
		if r.Denom == denom {
			total = total.Add(r.Amount)
		}
	}
	return total
}

func (f *fixture) requireCustodyBalances(t *testing.T, denoms ...string) {
	t.Helper()
	for _, denom := range denoms {
		require.Equal(t, f.moduleHoldings(denom).String(), f.totalReserves(t, denom).String(),
			"recorded reserves in %s must equal what the module account holds", denom)
	}
}
