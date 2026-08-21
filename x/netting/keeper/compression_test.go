package keeper_test

import (
	"fmt"
	"math/rand"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

// The number the whole design is bought with, measured rather than asserted.
//
// Compression is what a netting layer is for: it is the share of the gross
// value submitted that never has to be funded, and therefore the share of a
// participant's liquidity that stops being tied up in settlement. Anything the
// specification claims about tiering rests on this figure being real on
// something resembling a working day rather than on a hand-picked ring of three
// banks.
//
// The floor asserted here is deliberately well below what the run achieves.
// A test that pinned the exact number would fail on any harmless change to the
// generator and would say nothing about whether netting works.
func TestCompressionOnARealisticInterbankDay(t *testing.T) {
	const (
		participants = 8
		obligations  = 400
		// A working day's low-value traffic between eight institutions, with
		// the largest items kept out of the window by the threshold.
		threshold = 250_000
		seed      = 20260819
	)

	f := initFixture(t)
	f.setParams(t, 100, policy(eur, threshold))

	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // a measurement, and it has to be repeatable

	banks := make([]string, 0, participants)
	for range participants {
		bank := f.newParticipant(t, coins(eur, 5_000_000_000))
		// Prefunded with roughly a tenth of the gross flow each. That the run
		// completes at all with reserves this thin is itself part of the
		// result: the liquidity a netted system needs is the *net* exposure,
		// not the turnover.
		f.postReserve(t, bank, coins(eur, 4_000_000))
		banks = append(banks, bank)
	}

	cycleID := f.currentCycle(t)
	submittedGross := math.ZeroInt()
	submittedCount := 0
	rejected := 0

	for i := range obligations {
		from := banks[rng.Intn(len(banks))]
		to := banks[rng.Intn(len(banks))]
		if from == to {
			continue
		}
		// Log-ish spread: a lot of small items, a few large ones, which is what
		// retail-driven interbank traffic actually looks like.
		amount := int64(rng.Intn(20_000) + 1)
		if rng.Intn(10) == 0 {
			amount = int64(rng.Intn(200_000) + 20_000)
		}
		if err := f.trySubmit(from, to, eur, amount, fmt.Sprintf("day-%d", i)); err != nil {
			rejected++
			continue
		}
		submittedGross = submittedGross.Add(math.NewInt(amount))
		submittedCount++
	}

	require.Zero(t, rejected,
		"the run must not depend on obligations the cap turned away; raise the reserves if this fires")
	require.Positive(t, submittedCount)

	f.endBlockAt(t, 100)

	outcome := f.outcome(t, cycleID, eur)
	require.Equal(t, types.DENOM_STATUS_SETTLED, outcome.Status)
	require.Equal(t, submittedGross.String(), outcome.GrossAmount.String(),
		"the chain's gross figure must be the sum of what was actually submitted")

	bps := types.CompressionBps(outcome.GrossAmount, outcome.NetAmount)
	t.Logf("multilateral netting over %d obligations between %d participants: "+
		"gross %s, net funded %s, compression %d.%02d%%",
		submittedCount, participants, outcome.GrossAmount, outcome.NetAmount, bps/100, bps%100)

	// Also reported: what bilateral netting alone would have achieved on the
	// same traffic. The gap between the two is the argument for multilateral
	// netting being worth the extra difficulty, and it is measured rather than
	// asserted from first principles.
	bilateral := f.bilateralNetTotal(t, cycleID, eur)
	bilateralBps := types.CompressionBps(outcome.GrossAmount, bilateral)
	t.Logf("the same traffic netted only bilaterally would still have to fund %s "+
		"(compression %d.%02d%%), so multilateral netting removes a further %s",
		bilateral, bilateralBps/100, bilateralBps%100, bilateral.Sub(outcome.NetAmount))

	require.Greater(t, bps, uint64(7_500),
		"multilateral netting over a day of interbank traffic should remove well over three quarters of it")
	require.Greater(t, bps, bilateralBps,
		"multilateral netting must beat bilateral netting on the same traffic, or the extra difficulty buys nothing")
	f.requireCustodyBalances(t, eur)
}

// bilateralNetTotal is what would have to be funded if each pair of
// participants netted only against each other — the comparison the design
// decision was made against.
//
// Computed in the test rather than on-chain: the chain does not do bilateral
// netting, and adding state to measure a road not taken would be state to
// maintain forever.
func (f *fixture) bilateralNetTotal(t *testing.T, cycleID uint64, denom string) math.Int {
	t.Helper()

	genesis, err := f.keeper.ExportGenesis(f.ctx)
	require.NoError(t, err)

	pairs := map[string]math.Int{}
	for _, obligation := range genesis.Obligations {
		if obligation.CycleId != cycleID || obligation.Denom != denom {
			continue
		}
		if obligation.Mode != types.SETTLEMENT_MODE_NET {
			continue
		}
		// Ordered so that A→B and B→A land on the same key, with the sign
		// carrying the direction.
		a, b, sign := obligation.FromParticipant, obligation.ToParticipant, int64(-1)
		if a > b {
			a, b, sign = b, a, 1
		}
		key := a + "\x00" + b
		running, ok := pairs[key]
		if !ok {
			running = math.ZeroInt()
		}
		pairs[key] = running.Add(obligation.Amount.MulRaw(sign))
	}

	total := math.ZeroInt()
	for _, net := range pairs {
		total = total.Add(net.Abs())
	}
	return total
}
