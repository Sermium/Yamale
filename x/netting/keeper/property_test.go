package keeper_test

import (
	"fmt"
	"math/rand"
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/netting/types"
)

// These are properties rather than examples. A worked example proves that one
// arrangement of six banks nets correctly; a property proves that no
// arrangement this module can reach breaks the rule, including the arrangements
// nobody thought to write down.
//
// The seeds are fixed so a failure is reproducible. A netting bug that only
// appeared on one CI run and never again would be worse than no test.
var propertySeeds = []int64{1, 7, 42, 1729, 20260819}

// scenario is a randomised interbank day: a set of participants, prefunded
// reserves, and a stream of obligations between them.
type scenario struct {
	rng          *rand.Rand
	participants []string
}

// run submits a random stream of obligations, checking the zero-sum property
// after every single one rather than only at the end. A netting system whose
// positions balance at close but not in between is one whose net debit cap has
// been reading a wrong number all day.
func (s *scenario) run(t *testing.T, f *fixture, cycleID uint64, count int) {
	t.Helper()
	for i := range count {
		from := s.participants[s.rng.Intn(len(s.participants))]
		to := s.participants[s.rng.Intn(len(s.participants))]
		if from == to {
			continue
		}
		amount := int64(s.rng.Intn(5_000) + 1)
		// A rejected obligation is a legitimate outcome — it means the cap did
		// its job — and the invariants must hold either way.
		_ = f.trySubmit(from, to, eur, amount, fmt.Sprintf("obligation-%d", i))
		requirePositionsSumToZero(t, f, cycleID, eur)
		requireLockedWithinReserve(t, f, s.participants, eur)
	}
}

// PROPERTY: net positions in a currency always sum to zero.
//
// It is the statement that netting neither created nor destroyed value. Every
// obligation subtracts from one position exactly what it adds to another, so
// the only way this can fail is if one half of that pair is written and the
// other is not.
func requirePositionsSumToZero(t *testing.T, f *fixture, cycleID uint64, denom string) {
	t.Helper()
	total := math.ZeroInt()
	rng := collections.NewSuperPrefixedTripleRange[uint64, string, string](cycleID, denom)
	require.NoError(t, f.keeper.Position.Walk(f.ctx, rng,
		func(_ collections.Triple[uint64, string, string], amount math.Int) (bool, error) {
			total = total.Add(amount)
			return false, nil
		}))
	require.True(t, total.IsZero(), "positions in %s of cycle %d sum to %s", denom, cycleID, total)
}

// PROPERTY: no participant is ever committed beyond what it prefunded.
//
// This is what makes settlement unable to fail, so it has to hold after every
// message rather than only when a window closes.
func requireLockedWithinReserve(t *testing.T, f *fixture, participants []string, denom string) {
	t.Helper()
	for _, participant := range participants {
		locked := f.locked(t, participant, denom)
		reserve := f.reserve(t, participant, denom)
		require.False(t, locked.IsNegative(), "%s has negative locked collateral", participant)
		require.True(t, locked.LTE(reserve),
			"%s has %s committed against a reserve of %s", participant, locked, reserve)
	}
}

// PROPERTY: value is conserved across the whole netting layer.
//
// Every participant's own balance plus its reserve, summed over everybody, must
// be the same before and after a window's worth of obligations and its
// settlement. Netting moves claims; it must never make one.
func TestPropertyNettingConservesValue(t *testing.T) {
	for _, seed := range propertySeeds {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			f := initFixture(t)
			f.setParams(t, 10, policy(eur, 1_000_000))

			rng := rand.New(rand.NewSource(seed)) //nolint:gosec // reproducibility is the point
			s := &scenario{rng: rng}
			for i := range 6 {
				bank := f.newParticipant(t, coins(eur, 1_000_000))
				f.postReserve(t, bank, coins(eur, int64(rng.Intn(20_000)+1)))
				s.participants = append(s.participants, bank)
				_ = i
			}

			supplyBefore := f.env.Supply(eur)
			totalBefore := f.totalHeldBy(t, s.participants, eur)

			cycleID := f.currentCycle(t)
			s.run(t, f, cycleID, 60)
			f.endBlockAt(t, 10)

			require.Equal(t, supplyBefore.String(), f.env.Supply(eur).String(),
				"the netting layer must not change the supply of anything")
			require.Equal(t, totalBefore.String(), f.totalHeldBy(t, s.participants, eur).String(),
				"balances plus reserves must add up to what they did before")
			f.requireCustodyBalances(t, eur)
			requireLockedWithinReserve(t, f, s.participants, eur)
		})
	}
}

// PROPERTY: a settled window leaves every reserve non-negative and every
// commitment released.
//
// The second half matters as much as the first. Collateral that stayed
// committed after the position it backed was discharged would silently shrink
// what a participant can do in every window after this one.
func TestPropertySettlementReleasesEveryCommitment(t *testing.T) {
	for _, seed := range propertySeeds {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			f := initFixture(t)
			f.setParams(t, 10, policy(eur, 1_000_000))

			rng := rand.New(rand.NewSource(seed)) //nolint:gosec // reproducibility is the point
			s := &scenario{rng: rng}
			for range 5 {
				bank := f.newParticipant(t, coins(eur, 1_000_000))
				f.postReserve(t, bank, coins(eur, int64(rng.Intn(30_000)+1)))
				s.participants = append(s.participants, bank)
			}

			cycleID := f.currentCycle(t)
			s.run(t, f, cycleID, 50)
			f.endBlockAt(t, 10)

			require.Equal(t, types.CYCLE_STATUS_SETTLED, f.cycle(t, cycleID).Status,
				"a window built only through the message handlers must always settle")
			for _, participant := range s.participants {
				require.False(t, f.reserve(t, participant, eur).IsNegative())
				require.True(t, f.locked(t, participant, eur).IsZero(),
					"%s still has collateral committed after its window settled", participant)
			}
		})
	}
}

// PROPERTY: a slice settles entirely or not at all.
//
// Forced by corrupting one position so the invariant fails, at a random point
// in a random window. Every participant's reserve must be exactly what it was
// before the close, and every obligation must still be owed at its original
// amount — that is what "defined state" means for the participants who were not
// the cause.
func TestPropertyAHeldSliceMovesNothingAndLeavesEveryoneDefined(t *testing.T) {
	for _, seed := range propertySeeds {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			f := initFixture(t)
			f.setParams(t, 10, policy(eur, 1_000_000))

			rng := rand.New(rand.NewSource(seed)) //nolint:gosec // reproducibility is the point
			s := &scenario{rng: rng}
			for range 5 {
				bank := f.newParticipant(t, coins(eur, 1_000_000))
				f.postReserve(t, bank, coins(eur, int64(rng.Intn(30_000)+1)))
				s.participants = append(s.participants, bank)
			}

			cycleID := f.currentCycle(t)
			s.run(t, f, cycleID, 40)

			// Break the slice behind the handlers' backs, the way an imported
			// genesis or a bad migration would.
			victim := s.participants[rng.Intn(len(s.participants))]
			position := f.position(t, cycleID, eur, victim)
			require.NoError(t, f.keeper.Position.Set(f.ctx,
				collections.Join3(cycleID, eur, victim), position.AddRaw(1)))

			before := map[string]string{}
			lockedBefore := map[string]string{}
			obligationsBefore, err := f.keeper.ExportGenesis(f.ctx)
			require.NoError(t, err)
			for _, participant := range s.participants {
				before[participant] = f.reserve(t, participant, eur).String()
				lockedBefore[participant] = f.locked(t, participant, eur).String()
			}

			f.endBlockAt(t, 10)

			require.Equal(t, types.DENOM_STATUS_HELD, f.outcome(t, cycleID, eur).Status)
			for _, participant := range s.participants {
				require.Equal(t, before[participant], f.reserve(t, participant, eur).String(),
					"%s's reserve moved despite the slice being held", participant)
				require.Equal(t, lockedBefore[participant], f.locked(t, participant, eur).String(),
					"%s's commitment changed despite the slice being held", participant)
			}

			after, err := f.keeper.ExportGenesis(f.ctx)
			require.NoError(t, err)
			require.Equal(t, obligationsBefore.Obligations, after.Obligations,
				"every obligation in a held slice must still be owed, unchanged")
		})
	}
}

// PROPERTY: the same stream of obligations produces byte-identical state on two
// independent chains.
//
// This is the consensus property, and what it does and does not have teeth
// against is worth recording, because it was measured rather than assumed.
//
// It catches anything that makes two runs of the same traffic disagree: a
// currency's positions scattered so the slice no longer sums to zero, entries
// lost to a map key that never matches itself, a sequence read instead of
// peeked. It does *not* catch a pure reordering of the entries within a
// currency, because settling them is commutative — each reserve is read and
// written once. So this test is the backstop and the store's key order is the
// guarantee; neither substitutes for the other.
func TestPropertySettlementIsDeterministic(t *testing.T) {
	for _, seed := range propertySeeds {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			exports := make([]*types.GenesisState, 0, 2)
			for range 2 {
				f := initFixture(t)
				f.setParams(t, 10, policy(eur, 1_000_000), policy(ngn, 1_000_000))

				rng := rand.New(rand.NewSource(seed)) //nolint:gosec // reproducibility is the point
				participants := make([]string, 0, 5)
				for i := range 5 {
					// Addresses are derived from the seed rather than generated
					// randomly per fixture: two chains settling *different*
					// participants would prove nothing about ordering, because
					// the store order would differ for a legitimate reason.
					raw := make([]byte, 20)
					raw[0] = byte(seed)
					raw[1] = byte(i)
					bank, err := f.env.AddressCodec.BytesToString(raw)
					require.NoError(t, err)
					f.participants.approved[bank] = true
					participants = append(participants, bank)
				}

				for _, bank := range participants {
					require.NoError(t, f.keeper.Reserve.Set(f.ctx,
						collections.Join(bank, eur), math.NewInt(int64(rng.Intn(30_000)+10_000))))
					require.NoError(t, f.keeper.Reserve.Set(f.ctx,
						collections.Join(bank, ngn), math.NewInt(int64(rng.Intn(30_000)+10_000))))
				}

				cycleID := f.currentCycle(t)
				for i := range 60 {
					from := participants[rng.Intn(len(participants))]
					to := participants[rng.Intn(len(participants))]
					denom := eur
					if rng.Intn(2) == 0 {
						denom = ngn
					}
					if from == to {
						continue
					}
					_ = f.trySubmit(from, to, denom, int64(rng.Intn(3_000)+1), fmt.Sprintf("o-%d", i))
				}
				requirePositionsSumToZero(t, f, cycleID, eur)
				requirePositionsSumToZero(t, f, cycleID, ngn)

				f.endBlockAt(t, 10)

				exported, err := f.keeper.ExportGenesis(f.ctx)
				require.NoError(t, err)
				// Without this the comparison below passes on two empty
				// exports, which is the shape every vacuous test takes.
				require.NotEmpty(t, exported.Obligations, "the run produced no obligations to compare")
				require.NotEmpty(t, exported.Reserves, "the run produced no reserves to compare")
				exports = append(exports, exported)
			}

			require.Equal(t, exports[0].String(), exports[1].String(),
				"two chains given the same obligations must reach the same state")
		})
	}
}

// totalHeldBy sums what a set of participants holds, counting both their own
// balances and the reserve the module custodies for them.
func (f *fixture) totalHeldBy(t *testing.T, participants []string, denom string) math.Int {
	t.Helper()
	total := math.ZeroInt()
	for _, participant := range participants {
		addr, err := f.env.AddressCodec.StringToBytes(participant)
		require.NoError(t, err)
		total = total.Add(f.env.Balance(addr, denom))
		total = total.Add(f.reserve(t, participant, denom))
	}
	return total
}
