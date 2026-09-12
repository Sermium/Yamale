package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/syndicate/keeper"
	"yamale/blockchain/x/syndicate/types"
)

// The syndication layer.
//
// Two things are being proved here, and they are different in kind.
//
// The first is that money committed to a deal that does not happen comes back.
// That is the whole of the risk an investor takes before there is anything to
// own, and a platform that can take money and cannot return it is not a
// syndication layer, it is a hazard.
//
// The second is that a distribution reaches whoever holds the token AT THAT
// MOMENT, including somebody who bought it in a private sale the platform was
// never told about. That is the claim the product is sold on, and the tests
// below buy and sell between distributions specifically to try to break it.

const (
	usd  = "uusdc"
	sn   = "SN"
	day  = 24 * 60 * 60
	week = 7 * day
)

type fixture struct {
	env      *integration.Env
	k        keeper.Keeper
	srv      types.MsgServer
	q        types.QueryServer
	operator string
	partner  string
	drawdown string
	// Three onboarded investors, and one who never onboarded.
	alice, bob, carol string
	stranger          string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := integration.NewWith(t, []string{types.ModuleName})
	// A real clock. Every window in this module is a period a person
	// experiences, and a zero time.Time is 1754 — the arithmetic works and
	// every failure message reads as nonsense.
	env.Ctx = env.Ctx.WithBlockTime(time.Unix(1_788_000_000, 0)).WithBlockHeight(1)

	// nil group keeper: these tests exercise the raise and the market, not
	// admission. The operator is written straight into state, which is what
	// governance would have produced.
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority,
		env.BankKeeper, nil)
	require.NoError(t, k.Params.Set(env.Ctx, types.DefaultParams()))

	// The send restriction is registered here for the same reason app.go
	// registers it: without it, a bank transfer of a deal token settles
	// nothing and passes no onboarding check. A fixture that omitted it would
	// pass every test in this file while the property under test — that the
	// gate and the settlement survive a transfer the module never saw — was
	// enforced by nothing at all.
	env.BankKeeper.AppendSendRestriction(k.SendRestrictionFn)

	f := &fixture{env: env, k: k, srv: keeper.NewMsgServerImpl(k), q: keeper.NewQueryServerImpl(k)}

	_, f.operator = env.Addr(t)
	require.NoError(t, k.Operator.Set(env.Ctx, f.operator, types.Operator{
		Address: f.operator, Name: "Yamale Syndication", Jurisdiction: sn, Active: true,
	}))

	_, f.partner = env.Addr(t)
	_, f.drawdown = env.Addr(t)

	f.alice = f.investor(t, "alice")
	f.bob = f.investor(t, "bob")
	f.carol = f.investor(t, "carol")

	// Funded but never onboarded. Money is not the constraint; onboarding is.
	stranger, addr := env.NewFundedAddr(t, sdk.NewCoins(sdk.NewInt64Coin(usd, 1_000_000)))
	_ = stranger
	f.stranger = addr

	return f
}

// investor onboards somebody and funds them.
func (f *fixture) investor(t *testing.T, name string) string {
	t.Helper()
	_, addr := f.env.NewFundedAddr(t, sdk.NewCoins(sdk.NewInt64Coin(usd, 1_000_000)))
	_, err := f.srv.OnboardInvestor(f.env.Ctx, &types.MsgOnboardInvestor{
		Operator: f.operator, Investor: addr,
		Reference: "KYC/" + name, Jurisdiction: sn, Active: true,
	})
	require.NoError(t, err)
	return addr
}

func (f *fixture) advance(seconds int64) {
	f.env.Ctx = f.env.Ctx.WithBlockTime(f.env.Ctx.BlockTime().Add(time.Duration(seconds) * time.Second))
}

// openDeal opens one with sane terms. 100,000 target, 50,000 minimum,
// 1,000 minimum cheque, 2% sourcing, 1% syndication, 20% of distributions.
func (f *fixture) openDeal(t *testing.T, ref string) uint64 {
	t.Helper()
	res, err := f.srv.OpenDeal(f.env.Ctx, &types.MsgOpenDeal{
		Operator:        f.operator,
		Reference:       ref,
		Company:         "Sahel Logistics",
		Partner:         "Partech Africa",
		DrawdownAccount: f.drawdown,
		SettlementDenom: usd,
		Target:          math.NewInt(100_000),
		Minimum:         math.NewInt(50_000),
		MinimumTicket:   math.NewInt(1_000),
		FeeSchedule: types.FeeSchedule{
			SourcingBps: 200, SyndicationBps: 100, ManagementBps: 2000,
		},
		ClosesAt: f.env.Ctx.BlockTime().Unix() + week,
	})
	require.NoError(t, err)
	return res.Id
}

func (f *fixture) commit(t *testing.T, deal uint64, who string, amount int64) error {
	t.Helper()
	_, err := f.srv.Commit(f.env.Ctx, &types.MsgCommit{
		Investor: who, DealId: deal, Amount: sdk.NewInt64Coin(usd, amount),
	})
	return err
}

func (f *fixture) balance(t *testing.T, who, denom string) math.Int {
	t.Helper()
	addr, err := f.env.AddressCodec.StringToBytes(who)
	require.NoError(t, err)
	return f.env.Balance(addr, denom)
}

func (f *fixture) deal(t *testing.T, id uint64) types.Deal {
	t.Helper()
	d, err := f.k.Deal.Get(f.env.Ctx, id)
	require.NoError(t, err)
	return d
}

/* ========================================================================== */
/*  The raise                                                                 */
/* ========================================================================== */

// The whole path, which has to work before any refusal means anything.
func TestADealClosesAndIssuesProportionalOwnership(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-001")

	require.NoError(t, f.commit(t, id, f.alice, 60_000))
	require.NoError(t, f.commit(t, id, f.bob, 40_000))

	// The money is escrowed from the moment of commitment, not at close.
	require.Equal(t, math.NewInt(940_000), f.balance(t, f.alice, usd))

	f.advance(week + 1)
	res, err := f.srv.CloseDeal(f.env.Ctx, &types.MsgCloseDeal{
		Operator: f.operator, DealId: id,
	})
	require.NoError(t, err)

	// 3% of 100,000 to the operator, the rest to the company.
	require.Equal(t, math.NewInt(100_000), res.Raised)
	require.Equal(t, math.NewInt(3_000), res.Fees)
	require.Equal(t, math.NewInt(97_000), res.Invested)
	require.Equal(t, math.NewInt(3_000), f.balance(t, f.operator, usd))
	require.Equal(t, math.NewInt(97_000), f.balance(t, f.drawdown, usd))

	// Ownership is against the GROSS cheque, not the net. Charging the fee in
	// cash AND in ownership would take it twice, and the arithmetic would still
	// balance — which is what makes it the easy mistake.
	deal := f.deal(t, id)
	require.Equal(t, math.NewInt(100_000), deal.Supply)
	require.Equal(t, math.NewInt(60_000), f.balance(t, f.alice, deal.Denom))
	require.Equal(t, math.NewInt(40_000), f.balance(t, f.bob, deal.Denom))
}

// The failure that matters most: a deal that does not happen gives the money
// back. In full, fees included, because no service was delivered.
func TestAnAbortedDealRefundsInFull(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-002")

	require.NoError(t, f.commit(t, id, f.alice, 20_000))
	require.NoError(t, f.commit(t, id, f.bob, 10_000))
	require.Equal(t, math.NewInt(980_000), f.balance(t, f.alice, usd))

	f.advance(week + 1)
	_, err := f.srv.AbortDeal(f.env.Ctx, &types.MsgAbortDeal{
		Operator: f.operator, DealId: id, Reason: "the round was filled elsewhere",
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(1_000_000), f.balance(t, f.alice, usd))
	require.Equal(t, math.NewInt(1_000_000), f.balance(t, f.bob, usd))
	require.Zero(t, f.balance(t, f.operator, usd).Int64(),
		"the platform keeps nothing on a deal that never happened")
	require.Equal(t, types.DEAL_STATE_ABORTED, f.deal(t, id).State)
}

// And nobody needs the operator's cooperation to get their money back.
//
// This is what makes the absence of an automatic sweeper safe. An operator can
// decide not to proceed; it cannot decide to sit on escrowed money after a deal
// has demonstrably failed.
func TestAnybodyMayAbortADealThatMissedItsMinimum(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-003")
	require.NoError(t, f.commit(t, id, f.alice, 20_000))

	// Before the window runs, only the operator may pull it.
	_, err := f.srv.AbortDeal(f.env.Ctx, &types.MsgAbortDeal{
		Operator: f.stranger, DealId: id, Reason: "mine now",
	})
	require.ErrorIs(t, err, types.ErrNotOperator)

	f.advance(week + 1)
	_, err = f.srv.AbortDeal(f.env.Ctx, &types.MsgAbortDeal{
		Operator: f.stranger, DealId: id, Reason: "it missed its minimum",
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_000_000), f.balance(t, f.alice, usd))
}

// A deal below its minimum cannot be closed, only aborted. Closing it would
// fund a company with less than the round needed and leave the investors in a
// position nobody agreed to.
func TestADealBelowItsMinimumCannotBeClosed(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-004")
	require.NoError(t, f.commit(t, id, f.alice, 20_000))

	f.advance(week + 1)
	_, err := f.srv.CloseDeal(f.env.Ctx, &types.MsgCloseDeal{Operator: f.operator, DealId: id})
	require.ErrorIs(t, err, types.ErrMinimumNotMet)
}

// Closing early on a part-filled book is the operator choosing which
// commitments count.
func TestADealCannotBeClosedEarlyUnlessItIsFull(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-005")
	require.NoError(t, f.commit(t, id, f.alice, 60_000))

	_, err := f.srv.CloseDeal(f.env.Ctx, &types.MsgCloseDeal{Operator: f.operator, DealId: id})
	require.ErrorIs(t, err, types.ErrWindowOpen)

	// Full is different: there is nothing left to wait for, and holding a
	// filled allocation open only costs the investors.
	require.NoError(t, f.commit(t, id, f.bob, 40_000))
	_, err = f.srv.CloseDeal(f.env.Ctx, &types.MsgCloseDeal{Operator: f.operator, DealId: id})
	require.NoError(t, err)
}

// The right to withdraw is what makes committing early rational. A syndicate
// whose early commitments are hostages does not fill.
func TestAnInvestorMayWithdrawWhileTheDealIsOpen(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-006")
	require.NoError(t, f.commit(t, id, f.alice, 30_000))
	require.Equal(t, math.NewInt(30_000), f.deal(t, id).Raised)

	_, err := f.srv.WithdrawCommitment(f.env.Ctx, &types.MsgWithdrawCommitment{
		Investor: f.alice, DealId: id,
	})
	require.NoError(t, err)

	require.Equal(t, math.NewInt(1_000_000), f.balance(t, f.alice, usd))
	require.Zero(t, f.deal(t, id).Raised.Int64(),
		"the book must fall back, or the deal reports capital it does not hold")
}

func TestCommitmentsAreRefusedAfterTheWindow(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-007")
	f.advance(week + 1)
	require.ErrorIs(t, f.commit(t, id, f.alice, 60_000), types.ErrDealClosed)
}

func TestADealCannotBeOversubscribed(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-008")
	require.NoError(t, f.commit(t, id, f.alice, 90_000))
	require.ErrorIs(t, f.commit(t, id, f.bob, 20_000), types.ErrOverTarget)
	// And the room that IS left can still be taken.
	require.NoError(t, f.commit(t, id, f.bob, 10_000))
}

// The ticket minimum applies to the TOTAL. Applying it per increment would let
// somebody assemble a sub-minimum position out of legal-sized pieces; applying
// it only to the first would make every later increment unbounded downward.
func TestTheMinimumChequeAppliesToTheTotalNotTheIncrement(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-009")

	require.ErrorIs(t, f.commit(t, id, f.alice, 500), types.ErrBelowTicket)
	require.NoError(t, f.commit(t, id, f.alice, 1_000))
	// A top-up below the minimum is fine, because the position is already over it.
	require.NoError(t, f.commit(t, id, f.alice, 100))

	c, err := f.k.Commitment.Get(f.env.Ctx, collectionsJoin(id, f.alice))
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1_100), c.Amount)
}

/* ========================================================================== */
/*  The onboarding gate                                                       */
/* ========================================================================== */

func TestSomebodyWhoNeverOnboardedCannotCommit(t *testing.T) {
	f := setup(t)
	id := f.openDeal(t, "SYN-2026-010")
	require.ErrorIs(t, f.commit(t, id, f.stranger, 10_000), types.ErrNotOnboarded)
}

// The gate has to survive a plain bank send, because a deal token IS a plain
// bank denomination. A gate that only guarded this module's own handlers would
// be defeated by MsgSend — and the platform would be telling a regulator its
// tokens are held only by onboarded investors while the chain said otherwise.
func TestTheGateSurvivesAnOrdinaryBankSend(t *testing.T) {
	f := setup(t)
	id, deal := f.fundedDeal(t, "SYN-2026-011")

	alice, err := f.env.AddressCodec.StringToBytes(f.alice)
	require.NoError(t, err)
	stranger, err := f.env.AddressCodec.StringToBytes(f.stranger)
	require.NoError(t, err)
	bob, err := f.env.AddressCodec.StringToBytes(f.bob)
	require.NoError(t, err)

	// Not through any syndicate message. Straight at the bank.
	err = f.env.BankKeeper.SendCoins(f.env.Ctx, alice, stranger,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, math.NewInt(1_000))))
	require.ErrorIs(t, err, types.ErrNotOnboarded)

	// And an onboarded recipient goes through.
	require.NoError(t, f.env.BankKeeper.SendCoins(f.env.Ctx, alice, bob,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, math.NewInt(1_000)))))
	_ = id
}

// A struck-off investor keeps what they hold and keeps being paid on it. Their
// money is in a company; the chain cannot get it out, and freezing the position
// would strand it without helping anybody. What they lose is the ability to
// take on more.
func TestAStruckOffInvestorMaySellButNotBuy(t *testing.T) {
	f := setup(t)
	_, deal := f.fundedDeal(t, "SYN-2026-012")

	inv, err := f.k.Investor.Get(f.env.Ctx, f.alice)
	require.NoError(t, err)
	inv.Active = false
	require.NoError(t, f.k.Investor.Set(f.env.Ctx, f.alice, inv))

	alice, err := f.env.AddressCodec.StringToBytes(f.alice)
	require.NoError(t, err)
	bob, err := f.env.AddressCodec.StringToBytes(f.bob)
	require.NoError(t, err)

	// Out: allowed.
	require.NoError(t, f.env.BankKeeper.SendCoins(f.env.Ctx, alice, bob,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, math.NewInt(1_000)))))
	// In: refused.
	err = f.env.BankKeeper.SendCoins(f.env.Ctx, bob, alice,
		sdk.NewCoins(sdk.NewCoin(deal.Denom, math.NewInt(1_000))))
	require.ErrorIs(t, err, types.ErrInvestorInactive)
}
